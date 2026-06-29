package xidfatal_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/xidfatal"
)

func ev(id string, src gpufleetv1.SignalSource) rca.Evidence {
	return rca.Evidence{SignalID: id, Source: src, Label: id}
}

func evd(id string, src gpufleetv1.SignalSource, device string) rca.Evidence {
	return rca.Evidence{SignalID: id, Source: src, Label: id, Device: device}
}

func ids(cited []rca.Evidence) []string {
	out := make([]string, 0, len(cited))
	for _, c := range cited {
		out = append(out, c.SignalID)
	}
	return out
}

func TestXIDFatal_Match(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		prom = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
		ncc  = gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL
	)

	cases := []struct {
		name      string
		window    []rca.Evidence
		wantFired bool
		wantIDs   []string // unordered set; checked as multiset
	}{
		{
			name:      "dmesg xid.fatal + dcgm device.health.failed -> FIRE",
			window:    []rca.Evidence{ev("dmesg.xid.fatal.119", xidS), ev("device.health.failed", dcgm)},
			wantFired: true,
			wantIDs:   []string{"dmesg.xid.fatal.119", "device.health.failed"},
		},
		{
			name:      "bare prefixes -> FIRE",
			window:    []rca.Evidence{ev("dmesg.xid.fatal", xidS), ev("device.health.failed.gpu2", dcgm)},
			wantFired: true,
			wantIDs:   []string{"dmesg.xid.fatal", "device.health.failed.gpu2"},
		},
		{
			name:      "dmesg xid.fatal alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("dmesg.xid.fatal.119", xidS)},
			wantFired: false,
		},
		{
			name:      "dcgm device.health.failed alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("device.health.failed", dcgm)},
			wantFired: false,
		},
		{
			name:      "device.health.failed id but DMESG_XID source -> not the independent DCGM leg",
			window:    []rca.Evidence{ev("dmesg.xid.fatal", xidS), ev("device.health.failed", xidS)},
			wantFired: false,
		},
		{
			name:      "xid.fatal id but DCGM source -> not the kernel leg",
			window:    []rca.Evidence{ev("dmesg.xid.fatal", dcgm), ev("device.health.failed", dcgm)},
			wantFired: false,
		},
		{
			name:      "swapped sources (xid@dcgm + health@xid) -> no fire",
			window:    []rca.Evidence{ev("dmesg.xid.fatal", dcgm), ev("device.health.failed", xidS)},
			wantFired: false,
		},
		{
			name:      "negative: xid79 fallen-off-bus window -> no fire (distinct prefix)",
			window:    []rca.Evidence{ev("dmesg.xid79", xidS), ev("device.lost.dcgm", dcgm)},
			wantFired: false,
		},
		{
			name:      "negative: unrelated window (nccl timeout) -> no fire",
			window:    []rca.Evidence{ev("nccl.timeout", ncc), ev("collective.stall.dcgm", dcgm)},
			wantFired: false,
		},
		{
			name:      "negative: empty window -> no fire",
			window:    nil,
			wantFired: false,
		},
		// same-device guard cases
		{
			name:      "same device -> FIRE",
			window:    []rca.Evidence{evd("dmesg.xid.fatal.119", xidS, "GPU-7"), evd("device.health.failed", dcgm, "GPU-7")},
			wantFired: true,
			wantIDs:   []string{"dmesg.xid.fatal.119", "device.health.failed"},
		},
		{
			name:      "different device -> no fire",
			window:    []rca.Evidence{evd("dmesg.xid.fatal.119", xidS, "GPU-0"), evd("device.health.failed", dcgm, "GPU-9")},
			wantFired: false,
		},
		{
			name:      "one leg missing device -> FIRE (attribution unavailable)",
			window:    []rca.Evidence{evd("dmesg.xid.fatal.119", xidS, "GPU-0"), evd("device.health.failed", dcgm, "")},
			wantFired: true,
			wantIDs:   []string{"dmesg.xid.fatal.119", "device.health.failed"},
		},
		{
			// Mixed-device window: the first corroborator candidate is on GPU-9
			// (cross-device with legA on GPU-0); the second is on GPU-0 (same
			// device). Must FIRE citing the same-device pair, not ABSTAIN on the
			// first cross-device pair.
			name: "mixed-device window: early cross-device, later same-device -> FIRE (same-device pair)",
			window: []rca.Evidence{
				evd("dmesg.xid.fatal.119", xidS, "GPU-0"),
				evd("device.health.failed.cross", dcgm, "GPU-9"),
				evd("device.health.failed.same", dcgm, "GPU-0"),
			},
			wantFired: true,
			wantIDs:   []string{"dmesg.xid.fatal.119", "device.health.failed.same"},
		},
	}

	var s xidfatal.Sig
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cited, fired := s.Match(tc.window)
			if fired != tc.wantFired {
				t.Fatalf("fired = %v, want %v", fired, tc.wantFired)
			}
			if !fired {
				if cited != nil {
					t.Errorf("no-fire must cite nothing, got %v", ids(cited))
				}
				return
			}
			got := ids(cited)
			if !sameSet(got, tc.wantIDs) {
				t.Errorf("cited = %v, want set %v", got, tc.wantIDs)
			}
		})
	}
}

// TestXIDFatal_CitationGrounding proves the FIRED citations reference EXACTLY the
// two real input legs and nothing else (evidence_grounding == 1.0): no fabricated
// signal, and unrelated/extra evidence in the window is not cited. It also pins
// deterministic first-match-wins on each leg.
func TestXIDFatal_CitationGrounding(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
	)

	xidLeg := ev("dmesg.xid.fatal.119", xidS)
	healthLeg := ev("device.health.failed.gpu0", dcgm)
	// Second valid corroborators placed AFTER the intended ones: deterministic
	// first-match-wins means they must NOT be cited.
	laterXid := ev("dmesg.xid.fatal.79", xidS)
	laterHealth := ev("device.health.failed.gpu1", dcgm)
	window := []rca.Evidence{xidLeg, healthLeg, laterXid, laterHealth}

	var s xidfatal.Sig
	cited, fired := s.Match(window)
	if !fired {
		t.Fatal("expected FIRE on driver xid.fatal + independent dcgm device.health.failed")
	}
	if len(cited) != 2 {
		t.Fatalf("expected exactly 2 cited legs, got %d: %v", len(cited), ids(cited))
	}
	inputByID := map[string]rca.Evidence{}
	for _, e := range window {
		inputByID[e.SignalID] = e
	}
	for _, c := range cited {
		src, ok := inputByID[c.SignalID]
		if !ok {
			t.Errorf("cited signal %q is not a real input evidence", c.SignalID)
			continue
		}
		if c.Source != src.Source || c.Label != src.Label {
			t.Errorf("cited %q does not match input (src/label drifted)", c.SignalID)
		}
	}
	if !sameSet(ids(cited), []string{"dmesg.xid.fatal.119", "device.health.failed.gpu0"}) {
		t.Errorf("cited = %v, want exactly the two real legs", ids(cited))
	}
}

func TestXIDFatal_ContractIDs(t *testing.T) {
	var s xidfatal.Sig
	if s.FaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_XID_FATAL {
		t.Errorf("fault class = %v", s.FaultClass())
	}
	if s.GateSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_XID_FATAL {
		t.Errorf("gate signature = %v", s.GateSignature())
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, y := range b {
		m[y]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}
