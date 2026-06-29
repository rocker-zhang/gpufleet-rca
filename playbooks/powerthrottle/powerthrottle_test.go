package powerthrottle_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/powerthrottle"
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

func TestPowerThrottle_Match(t *testing.T) {
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
			name:      "dcgm power.violation + dmesg power.brake -> FIRE",
			window:    []rca.Evidence{ev("power.violation", dcgm), ev("dmesg.power.brake", xidS)},
			wantFired: true,
			wantIDs:   []string{"power.violation", "dmesg.power.brake"},
		},
		{
			name:      "prefix-suffixed ids -> FIRE",
			window:    []rca.Evidence{ev("power.violation.gpu3", dcgm), ev("dmesg.power.brake.gpu3", xidS)},
			wantFired: true,
			wantIDs:   []string{"power.violation.gpu3", "dmesg.power.brake.gpu3"},
		},
		{
			name:      "dcgm power.violation alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("power.violation", dcgm)},
			wantFired: false,
		},
		{
			name:      "dmesg power.brake alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("dmesg.power.brake", xidS)},
			wantFired: false,
		},
		{
			name:      "power.brake id but DCGM source -> not the independent kernel leg",
			window:    []rca.Evidence{ev("power.violation", dcgm), ev("dmesg.power.brake", dcgm)},
			wantFired: false,
		},
		{
			name:      "power.violation id but DMESG_XID source -> not the DCGM leg",
			window:    []rca.Evidence{ev("power.violation", xidS), ev("dmesg.power.brake", xidS)},
			wantFired: false,
		},
		{
			name:      "swapped sources (violation@xid + brake@dcgm) -> no fire",
			window:    []rca.Evidence{ev("power.violation", xidS), ev("dmesg.power.brake", dcgm)},
			wantFired: false,
		},
		{
			name:      "negative: unrelated window (link degraded) -> no fire",
			window:    []rca.Evidence{ev("link.error", dcgm), ev("link.degraded.pcie", prom)},
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
			window:    []rca.Evidence{evd("power.violation", dcgm, "GPU-7"), evd("dmesg.power.brake", xidS, "GPU-7")},
			wantFired: true,
			wantIDs:   []string{"power.violation", "dmesg.power.brake"},
		},
		{
			name:      "different device -> no fire",
			window:    []rca.Evidence{evd("power.violation", dcgm, "GPU-0"), evd("dmesg.power.brake", xidS, "GPU-9")},
			wantFired: false,
		},
		{
			name:      "one leg missing device -> FIRE (attribution unavailable)",
			window:    []rca.Evidence{evd("power.violation", dcgm, "GPU-0"), evd("dmesg.power.brake", xidS, "")},
			wantFired: true,
			wantIDs:   []string{"power.violation", "dmesg.power.brake"},
		},
		{
			// Mixed-device window: the first corroborator candidate is on GPU-9
			// (cross-device with legA on GPU-0); the second is on GPU-0 (same
			// device). Must FIRE citing the same-device pair, not ABSTAIN on the
			// first cross-device pair.
			name: "mixed-device window: early cross-device, later same-device -> FIRE (same-device pair)",
			window: []rca.Evidence{
				evd("power.violation", dcgm, "GPU-0"),
				evd("dmesg.power.brake.cross", xidS, "GPU-9"),
				evd("dmesg.power.brake.same", xidS, "GPU-0"),
			},
			wantFired: true,
			wantIDs:   []string{"power.violation", "dmesg.power.brake.same"},
		},
	}

	var s powerthrottle.Sig
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

// TestPowerThrottle_CitationGrounding proves the FIRED citations reference
// EXACTLY the two real input legs and nothing else (evidence_grounding == 1.0):
// no fabricated signal, and unrelated/extra evidence in the window is not cited.
// It also pins deterministic first-match-wins on each leg.
func TestPowerThrottle_CitationGrounding(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
	)

	violationLeg := ev("power.violation.gpu0", dcgm)
	brakeLeg := ev("dmesg.power.brake.gpu0", xidS)
	// Second valid corroborators placed AFTER the intended ones: deterministic
	// first-match-wins means they must NOT be cited.
	laterViolation := ev("power.violation.gpu1", dcgm)
	laterBrake := ev("dmesg.power.brake.gpu1", xidS)
	window := []rca.Evidence{violationLeg, brakeLeg, laterViolation, laterBrake}

	var s powerthrottle.Sig
	cited, fired := s.Match(window)
	if !fired {
		t.Fatal("expected FIRE on dcgm power.violation + independent power.brake")
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
	if !sameSet(ids(cited), []string{"power.violation.gpu0", "dmesg.power.brake.gpu0"}) {
		t.Errorf("cited = %v, want exactly the two real legs", ids(cited))
	}
}

func TestPowerThrottle_ContractIDs(t *testing.T) {
	var s powerthrottle.Sig
	if s.FaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_POWER_THROTTLE {
		t.Errorf("fault class = %v", s.FaultClass())
	}
	if s.GateSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_POWER_THROTTLE {
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
