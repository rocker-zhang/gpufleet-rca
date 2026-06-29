package thermalthrottle_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/thermalthrottle"
)

func ev(id string, src gpufleetv1.SignalSource) rca.Evidence {
	return rca.Evidence{SignalID: id, Source: src, Label: id}
}

func ids(cited []rca.Evidence) []string {
	out := make([]string, 0, len(cited))
	for _, c := range cited {
		out = append(out, c.SignalID)
	}
	return out
}

func TestThermalThrottle_Match(t *testing.T) {
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
			name:      "dcgm thermal.violation + dmesg thermal.slowdown -> FIRE",
			window:    []rca.Evidence{ev("thermal.violation", dcgm), ev("dmesg.thermal.slowdown", xidS)},
			wantFired: true,
			wantIDs:   []string{"thermal.violation", "dmesg.thermal.slowdown"},
		},
		{
			name:      "prefix-suffixed ids -> FIRE",
			window:    []rca.Evidence{ev("thermal.violation.gpu3", dcgm), ev("dmesg.thermal.slowdown.gpu3", xidS)},
			wantFired: true,
			wantIDs:   []string{"thermal.violation.gpu3", "dmesg.thermal.slowdown.gpu3"},
		},
		{
			name:      "dcgm thermal.violation alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("thermal.violation", dcgm)},
			wantFired: false,
		},
		{
			name:      "dmesg thermal.slowdown alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("dmesg.thermal.slowdown", xidS)},
			wantFired: false,
		},
		{
			name:      "thermal.slowdown id but DCGM source -> not the independent kernel leg",
			window:    []rca.Evidence{ev("thermal.violation", dcgm), ev("dmesg.thermal.slowdown", dcgm)},
			wantFired: false,
		},
		{
			name:      "thermal.violation id but DMESG_XID source -> not the DCGM leg",
			window:    []rca.Evidence{ev("thermal.violation", xidS), ev("dmesg.thermal.slowdown", xidS)},
			wantFired: false,
		},
		{
			name:      "swapped sources (violation@xid + slowdown@dcgm) -> no fire",
			window:    []rca.Evidence{ev("thermal.violation", xidS), ev("dmesg.thermal.slowdown", dcgm)},
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
	}

	var s thermalthrottle.Sig
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

// TestThermalThrottle_CitationGrounding proves the FIRED citations reference
// EXACTLY the two real input legs and nothing else (evidence_grounding == 1.0):
// no fabricated signal, and unrelated/extra evidence in the window is not cited.
// It also pins deterministic first-match-wins on each leg.
func TestThermalThrottle_CitationGrounding(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
	)

	violationLeg := ev("thermal.violation.gpu0", dcgm)
	slowdownLeg := ev("dmesg.thermal.slowdown.gpu0", xidS)
	// Second valid corroborators placed AFTER the intended ones: deterministic
	// first-match-wins means they must NOT be cited.
	laterViolation := ev("thermal.violation.gpu1", dcgm)
	laterSlowdown := ev("dmesg.thermal.slowdown.gpu1", xidS)
	window := []rca.Evidence{violationLeg, slowdownLeg, laterViolation, laterSlowdown}

	var s thermalthrottle.Sig
	cited, fired := s.Match(window)
	if !fired {
		t.Fatal("expected FIRE on dcgm thermal.violation + independent thermal.slowdown")
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
	if !sameSet(ids(cited), []string{"thermal.violation.gpu0", "dmesg.thermal.slowdown.gpu0"}) {
		t.Errorf("cited = %v, want exactly the two real legs", ids(cited))
	}
}

func TestThermalThrottle_ContractIDs(t *testing.T) {
	var s thermalthrottle.Sig
	if s.FaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_THERMAL_THROTTLE {
		t.Errorf("fault class = %v", s.FaultClass())
	}
	if s.GateSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_THERMAL_THROTTLE {
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
