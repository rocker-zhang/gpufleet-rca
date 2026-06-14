package xid79_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/xid79"
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

func TestXID79_Match(t *testing.T) {
	const (
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		ncc  = gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL
	)

	cases := []struct {
		name      string
		window    []rca.Evidence
		wantFired bool
		wantIDs   []string // unordered set; checked as multiset
	}{
		{
			name:      "xid + dcgm device-lost -> FIRE",
			window:    []rca.Evidence{ev("dmesg.xid79", xidS), ev("device.lost.dcgm", dcgm)},
			wantFired: true,
			wantIDs:   []string{"dmesg.xid79", "device.lost.dcgm"},
		},
		{
			name:      "xid alone -> no fire",
			window:    []rca.Evidence{ev("dmesg.xid79", xidS)},
			wantFired: false,
		},
		{
			name:      "device-lost alone -> no fire",
			window:    []rca.Evidence{ev("device.lost.dcgm", dcgm)},
			wantFired: false,
		},
		{
			name:      "same-source forged (both DMESG_XID) -> no fire",
			window:    []rca.Evidence{ev("dmesg.xid79", xidS), ev("device.lost.forged", xidS)},
			wantFired: false,
		},
		{
			name:      "corroborator from NCCL (not a device-lost source) -> no fire",
			window:    []rca.Evidence{ev("dmesg.xid79", xidS), ev("device.lost.nccl", ncc)},
			wantFired: false,
		},
		{
			name:      "device-lost id but DMESG_XID source -> not an independent corroborator",
			window:    []rca.Evidence{ev("dmesg.xid79", xidS), ev("device.lost.dmesg", xidS)},
			wantFired: false,
		},
	}

	var s xid79.Sig
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

func TestXID79_ContractIDs(t *testing.T) {
	var s xid79.Sig
	if s.FaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS {
		t.Errorf("fault class = %v", s.FaultClass())
	}
	if s.GateSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_XID79_FALLEN_OFF_BUS {
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
