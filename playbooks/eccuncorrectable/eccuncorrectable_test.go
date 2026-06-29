package eccuncorrectable_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/eccuncorrectable"
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

func TestECCUncorrectable_Match(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
		prom = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS
		proc = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC
		ncc  = gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL
	)

	cases := []struct {
		name      string
		window    []rca.Evidence
		wantFired bool
		wantIDs   []string // unordered set; checked as multiset
	}{
		{
			name:      "dcgm dbe + dmesg ecc xid -> FIRE",
			window:    []rca.Evidence{ev("ecc.dbe", dcgm), ev("dmesg.xid.ecc.94", xidS)},
			wantFired: true,
			wantIDs:   []string{"ecc.dbe", "dmesg.xid.ecc.94"},
		},
		{
			name:      "dcgm dbe (fb) + dmesg ecc xid 48 -> FIRE",
			window:    []rca.Evidence{ev("ecc.dbe.fb", dcgm), ev("dmesg.xid.ecc.48", xidS)},
			wantFired: true,
			wantIDs:   []string{"ecc.dbe.fb", "dmesg.xid.ecc.48"},
		},
		{
			name:      "PROMETHEUS dbe (increase query) + dmesg ecc xid -> FIRE (Prom-primary node)",
			window:    []rca.Evidence{ev("ecc.dbe.GPU-x", prom), ev("dmesg.xid.ecc.94", xidS)},
			wantFired: true,
			wantIDs:   []string{"ecc.dbe.GPU-x", "dmesg.xid.ecc.94"},
		},
		{
			name:      "PROMETHEUS dbe alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("ecc.dbe.GPU-x", prom)},
			wantFired: false,
		},
		{
			name:      "dcgm dbe alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("ecc.dbe", dcgm)},
			wantFired: false,
		},
		{
			name:      "dmesg ecc xid alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("dmesg.xid.ecc.94", xidS)},
			wantFired: false,
		},
		{
			name:      "same-source forged (DCGM + DCGM) -> no fire",
			window:    []rca.Evidence{ev("ecc.dbe", dcgm), ev("dmesg.xid.ecc.forged", dcgm)},
			wantFired: false,
		},
		{
			name:      "ecc.dbe id but non-DCGM source -> not the DCGM leg",
			window:    []rca.Evidence{ev("ecc.dbe", xidS), ev("dmesg.xid.ecc.94", xidS)},
			wantFired: false,
		},
		{
			name:      "dmesg.xid.ecc id but non-DMESG_XID source -> not the corroborator leg",
			window:    []rca.Evidence{ev("ecc.dbe", dcgm), ev("dmesg.xid.ecc.94", prom)},
			wantFired: false,
		},
		{
			name:      "corroborator from PROC source despite ecc-xid id -> not independent witness",
			window:    []rca.Evidence{ev("ecc.dbe", dcgm), ev("dmesg.xid.ecc.x", proc)},
			wantFired: false,
		},
		{
			name:      "negative: unrelated window (xid79 device-lost) -> no fire",
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
			window:    []rca.Evidence{evd("ecc.dbe", dcgm, "GPU-7"), evd("dmesg.xid.ecc.94", xidS, "GPU-7")},
			wantFired: true,
			wantIDs:   []string{"ecc.dbe", "dmesg.xid.ecc.94"},
		},
		{
			name:      "different device -> no fire",
			window:    []rca.Evidence{evd("ecc.dbe", dcgm, "GPU-0"), evd("dmesg.xid.ecc.94", xidS, "GPU-9")},
			wantFired: false,
		},
		{
			name:      "one leg missing device -> FIRE (attribution unavailable)",
			window:    []rca.Evidence{evd("ecc.dbe", dcgm, "GPU-0"), evd("dmesg.xid.ecc.94", xidS, "")},
			wantFired: true,
			wantIDs:   []string{"ecc.dbe", "dmesg.xid.ecc.94"},
		},
		{
			// Mixed-device window: the first corroborator candidate is on GPU-9
			// (cross-device with legA on GPU-0); the second is on GPU-0 (same
			// device). Must FIRE citing the same-device pair, not ABSTAIN on the
			// first cross-device pair.
			name: "mixed-device window: early cross-device, later same-device -> FIRE (same-device pair)",
			window: []rca.Evidence{
				evd("ecc.dbe", dcgm, "GPU-0"),
				evd("dmesg.xid.ecc.cross", xidS, "GPU-9"),
				evd("dmesg.xid.ecc.same", xidS, "GPU-0"),
			},
			wantFired: true,
			wantIDs:   []string{"ecc.dbe", "dmesg.xid.ecc.same"},
		},
	}

	var s eccuncorrectable.Sig
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

// TestECCUncorrectable_CitationGrounding proves the FIRED citations reference
// EXACTLY the two real input legs and nothing else (evidence_grounding == 1.0):
// no fabricated signal, and unrelated/extra evidence in the window is not cited.
// It also pins deterministic first-match-wins on each leg.
func TestECCUncorrectable_CitationGrounding(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
	)

	dbeLeg := ev("ecc.dbe.fb", dcgm)
	xidLeg := ev("dmesg.xid.ecc.94", xidS)
	// Second valid corroborators placed AFTER the intended ones: deterministic
	// first-match-wins means they must NOT be cited.
	laterDBE := ev("ecc.dbe.l2", dcgm)
	laterXid := ev("dmesg.xid.ecc.63", xidS)
	window := []rca.Evidence{dbeLeg, xidLeg, laterDBE, laterXid}

	var s eccuncorrectable.Sig
	cited, fired := s.Match(window)
	if !fired {
		t.Fatal("expected FIRE on dcgm dbe + independent dmesg ecc xid")
	}
	if len(cited) != 2 {
		t.Fatalf("expected exactly 2 cited legs, got %d: %v", len(cited), ids(cited))
	}
	// Every cited signal must be one of the real input evidences (identity by
	// SignalID + Source + Label), grounding the citation in the actual window.
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
	// Exactly the two intended legs (deterministic first-match), not the noise.
	if !sameSet(ids(cited), []string{"ecc.dbe.fb", "dmesg.xid.ecc.94"}) {
		t.Errorf("cited = %v, want exactly the two real legs", ids(cited))
	}
}

func TestECCUncorrectable_ContractIDs(t *testing.T) {
	var s eccuncorrectable.Sig
	if s.FaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ECC_UNCORRECTABLE {
		t.Errorf("fault class = %v", s.FaultClass())
	}
	if s.GateSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_ECC_UNCORRECTABLE {
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
