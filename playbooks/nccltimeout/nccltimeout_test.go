package nccltimeout_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/nccltimeout"
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

func TestNCCLTimeout_Match(t *testing.T) {
	const (
		ncc   = gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL
		sched = gpufleetv1.SignalSource_SIGNAL_SOURCE_SCHEDULER
		dcgm  = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		prom  = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS
		proc  = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC
		xidS  = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
	)

	cases := []struct {
		name      string
		window    []rca.Evidence
		wantFired bool
		wantIDs   []string // unordered set; checked as multiset
	}{
		{
			name:      "nccl + scheduler collective-stall -> FIRE",
			window:    []rca.Evidence{ev("nccl.timeout", ncc), ev("collective.stall.scheduler", sched)},
			wantFired: true,
			wantIDs:   []string{"nccl.timeout", "collective.stall.scheduler"},
		},
		{
			name:      "nccl + dcgm collective-stall -> FIRE",
			window:    []rca.Evidence{ev("nccl.timeout.rank3", ncc), ev("collective.stall.dcgm", dcgm)},
			wantFired: true,
			wantIDs:   []string{"nccl.timeout.rank3", "collective.stall.dcgm"},
		},
		{
			name:      "nccl + prometheus collective-stall -> FIRE",
			window:    []rca.Evidence{ev("nccl.timeout", ncc), ev("collective.stall.prom", prom)},
			wantFired: true,
			wantIDs:   []string{"nccl.timeout", "collective.stall.prom"},
		},
		{
			name:      "nccl + proc collective-stall -> FIRE",
			window:    []rca.Evidence{ev("nccl.timeout", ncc), ev("collective.stall.proc", proc)},
			wantFired: true,
			wantIDs:   []string{"nccl.timeout", "collective.stall.proc"},
		},
		{
			name:      "nccl alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("nccl.timeout", ncc)},
			wantFired: false,
		},
		{
			name:      "collective-stall alone -> no fire (single leg)",
			window:    []rca.Evidence{ev("collective.stall.scheduler", sched)},
			wantFired: false,
		},
		{
			name:      "same-source forged (NCCL + NCCL) -> no fire",
			window:    []rca.Evidence{ev("nccl.timeout", ncc), ev("collective.stall.rank7", ncc)},
			wantFired: false,
		},
		{
			name:      "corroborator from NCCL source despite stall id -> not independent",
			window:    []rca.Evidence{ev("nccl.timeout", ncc), ev("collective.stall.forged", ncc)},
			wantFired: false,
		},
		{
			name:      "stall id but DMESG_XID source -> not a collective-stall witness",
			window:    []rca.Evidence{ev("nccl.timeout", ncc), ev("collective.stall.x", xidS)},
			wantFired: false,
		},
		{
			name:      "nccl.timeout id but non-NCCL source -> not the NCCL leg",
			window:    []rca.Evidence{ev("nccl.timeout", sched), ev("collective.stall.dcgm", dcgm)},
			wantFired: false,
		},
		{
			name:      "negative: unrelated window (xid79 device-lost) -> no fire",
			window:    []rca.Evidence{ev("dmesg.xid79", xidS), ev("device.lost.dcgm", dcgm)},
			wantFired: false,
		},
		{
			name:      "negative: empty window -> no fire",
			window:    nil,
			wantFired: false,
		},
	}

	var s nccltimeout.Sig
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

// TestNCCLTimeout_CitationGrounding proves the FIRED citations reference EXACTLY
// the two real input legs and nothing else (evidence_grounding == 1.0): no
// fabricated signal, and unrelated evidence in the window is not cited.
func TestNCCLTimeout_CitationGrounding(t *testing.T) {
	const (
		ncc   = gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL
		sched = gpufleetv1.SignalSource_SIGNAL_SOURCE_SCHEDULER
		dcgm  = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
	)

	ncclLeg := ev("nccl.timeout.rank0", ncc)
	stallLeg := ev("collective.stall.scheduler", sched)
	// A second valid corroborator placed AFTER the intended one: deterministic
	// first-match-wins means it must NOT be cited.
	later := ev("collective.stall.dcgm", dcgm)
	window := []rca.Evidence{ncclLeg, stallLeg, later}

	var s nccltimeout.Sig
	cited, fired := s.Match(window)
	if !fired {
		t.Fatal("expected FIRE on nccl + independent stall")
	}
	if len(cited) != 2 {
		t.Fatalf("expected exactly 2 cited legs, got %d: %v", len(cited), ids(cited))
	}
	// Every cited signal must be one of the real input evidences (identity by
	// SignalID + Source), grounding the citation in the actual window.
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
	if !sameSet(ids(cited), []string{"nccl.timeout.rank0", "collective.stall.scheduler"}) {
		t.Errorf("cited = %v, want exactly the two real legs", ids(cited))
	}
}

func TestNCCLTimeout_ContractIDs(t *testing.T) {
	var s nccltimeout.Sig
	if s.FaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_NCCL_TIMEOUT {
		t.Errorf("fault class = %v", s.FaultClass())
	}
	if s.GateSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_NCCL_TIMEOUT {
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
