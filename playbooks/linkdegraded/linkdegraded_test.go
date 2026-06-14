package linkdegraded_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/linkdegraded"
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

func TestLinkDegraded_Match(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		prom = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS
		proc = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC
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
			name:      "dcgm link.error + prometheus link.degraded -> FIRE",
			window:    []rca.Evidence{ev("link.error", dcgm), ev("link.degraded.pcie", prom)},
			wantFired: true,
			wantIDs:   []string{"link.error", "link.degraded.pcie"},
		},
		{
			name:      "dcgm link.error + proc link.degraded -> FIRE",
			window:    []rca.Evidence{ev("link.error.nvlink2", dcgm), ev("link.degraded.width", proc)},
			wantFired: true,
			wantIDs:   []string{"link.error.nvlink2", "link.degraded.width"},
		},
		{
			name:      "dcgm link.error alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("link.error", dcgm)},
			wantFired: false,
		},
		{
			name:      "link.degraded alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("link.degraded.pcie", prom)},
			wantFired: false,
		},
		{
			name:      "same-source forged (DCGM + DCGM) -> no fire",
			window:    []rca.Evidence{ev("link.error", dcgm), ev("link.degraded.forged", dcgm)},
			wantFired: false,
		},
		{
			name:      "link.degraded id but DCGM source -> not independent (leg2 excludes DCGM)",
			window:    []rca.Evidence{ev("link.error", dcgm), ev("link.degraded.dcgm", dcgm)},
			wantFired: false,
		},
		{
			name:      "link.error id but non-DCGM source -> not the DCGM leg",
			window:    []rca.Evidence{ev("link.error", prom), ev("link.degraded.pcie", prom)},
			wantFired: false,
		},
		{
			name:      "link.degraded id but DMESG_XID source -> not a degraded witness",
			window:    []rca.Evidence{ev("link.error", dcgm), ev("link.degraded.x", xidS)},
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
	}

	var s linkdegraded.Sig
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

// TestLinkDegraded_CitationGrounding proves the FIRED citations reference EXACTLY
// the two real input legs and nothing else (evidence_grounding == 1.0): no
// fabricated signal, and unrelated/extra evidence in the window is not cited.
// It also pins deterministic first-match-wins on each leg.
func TestLinkDegraded_CitationGrounding(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		prom = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS
		proc = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC
	)

	errLeg := ev("link.error.nvlink2", dcgm)
	degLeg := ev("link.degraded.pcie", prom)
	// Second valid corroborators placed AFTER the intended ones: deterministic
	// first-match-wins means they must NOT be cited.
	laterErr := ev("link.error.pcie", dcgm)
	laterDeg := ev("link.degraded.width", proc)
	window := []rca.Evidence{errLeg, degLeg, laterErr, laterDeg}

	var s linkdegraded.Sig
	cited, fired := s.Match(window)
	if !fired {
		t.Fatal("expected FIRE on dcgm link.error + independent link.degraded")
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
	if !sameSet(ids(cited), []string{"link.error.nvlink2", "link.degraded.pcie"}) {
		t.Errorf("cited = %v, want exactly the two real legs", ids(cited))
	}
}

func TestLinkDegraded_ContractIDs(t *testing.T) {
	var s linkdegraded.Sig
	if s.FaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_LINK_DEGRADED {
		t.Errorf("fault class = %v", s.FaultClass())
	}
	if s.GateSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_LINK_DEGRADED {
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
