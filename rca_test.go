package rca_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/xid79"
)

// entry builds a citable TimelineEntry (a normalized signal in the window).
func entry(id string, src gpufleetv1.SignalSource) *gpufleetv1.TimelineEntry {
	return &gpufleetv1.TimelineEntry{SignalId: id, Source: src, Label: id}
}

func pack(entries ...*gpufleetv1.TimelineEntry) *gpufleetv1.EvidencePack {
	return &gpufleetv1.EvidencePack{ContractVersion: "v1", Timeline: entries}
}

// citedIDs extracts the cited signal ids from a verdict (sorted by the engine).
func citedIDs(v *gpufleetv1.Verdict) []string {
	out := make([]string, 0, len(v.GetCitedSignals()))
	for _, c := range v.GetCitedSignals() {
		out = append(out, c.GetSignalId())
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGate_XID79(t *testing.T) {
	const (
		dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM
		xidS = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
		proc = gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC
	)

	cases := []struct {
		name      string
		pack      *gpufleetv1.EvidencePack
		wantClass gpufleetv1.FaultClass
		wantSig   gpufleetv1.GateSignature
		wantCited []string // exact, sorted by signal_id; nil => expect ABSTAIN
	}{
		{
			name: "two independent corroborating signals -> FIRE XID79",
			pack: pack(
				entry("dmesg.xid79", xidS),
				entry("device.lost.dcgm", dcgm),
			),
			wantClass: gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS,
			wantSig:   gpufleetv1.GateSignature_GATE_SIGNATURE_XID79_FALLEN_OFF_BUS,
			// sorted: "device.lost.dcgm" < "dmesg.xid79"
			wantCited: []string{"device.lost.dcgm", "dmesg.xid79"},
		},
		{
			name:      "one signal (xid only) -> ABSTAIN",
			pack:      pack(entry("dmesg.xid79", xidS)),
			wantClass: gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN,
		},
		{
			name:      "one signal (device-lost only) -> ABSTAIN",
			pack:      pack(entry("device.lost.dcgm", dcgm)),
			wantClass: gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN,
		},
		{
			// LOAD-BEARING: two readings of the SAME source must NOT corroborate.
			// Both legs carry a DMESG_XID source; independence is judged on
			// source, so this is one vote -> ABSTAIN, even though the ids differ
			// and one even uses the device.lost id (forged independence).
			name: "two same-source signals (forged) -> ABSTAIN",
			pack: pack(
				entry("dmesg.xid79", xidS),
				entry("device.lost.forged", xidS),
			),
			wantClass: gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN,
		},
		{
			name: "unrelated signals -> ABSTAIN (FP=0)",
			pack: pack(
				entry("thermal.throttle", dcgm),
				entry("nccl.timeout", gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL),
			),
			wantClass: gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN,
		},
		{
			name:      "empty window -> ABSTAIN",
			pack:      pack(),
			wantClass: gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN,
		},
		{
			name: "xid + device-lost via PROC (nvidia-smi) -> FIRE XID79",
			pack: pack(
				entry("dmesg.xid79", xidS),
				entry("device.lost.smi", proc),
			),
			wantClass: gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS,
			wantSig:   gpufleetv1.GateSignature_GATE_SIGNATURE_XID79_FALLEN_OFF_BUS,
			wantCited: []string{"device.lost.smi", "dmesg.xid79"},
		},
	}

	eng := rca.NewEngine(xid79.New())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := eng.Evaluate(tc.pack)

			if v.GetFaultClass() != tc.wantClass {
				t.Fatalf("fault_class = %v, want %v", v.GetFaultClass(), tc.wantClass)
			}

			fired := tc.wantClass != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN
			if fired {
				if v.GetSignature() != tc.wantSig {
					t.Errorf("signature = %v, want %v", v.GetSignature(), tc.wantSig)
				}
				if v.GetConfidence() < rca.FaultConfidenceFloor {
					t.Errorf("FIRE confidence = %v, want >= %v (clamp)", v.GetConfidence(), rca.FaultConfidenceFloor)
				}
				if got := citedIDs(v); !equalStrings(got, tc.wantCited) {
					t.Errorf("cited = %v, want exactly %v", got, tc.wantCited)
				}
				// evidence_grounding == 1.0: every cited id must be a real input.
				assertGrounded(t, tc.pack, v)
				// >=2 DISTINCT sources among the citations.
				assertIndependent(t, v)
			} else {
				if len(v.GetCitedSignals()) != 0 {
					t.Errorf("ABSTAIN must cite nothing, got %v", citedIDs(v))
				}
				if v.GetSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_UNSPECIFIED {
					t.Errorf("ABSTAIN signature = %v, want UNSPECIFIED", v.GetSignature())
				}
			}
		})
	}
}

// assertGrounded enforces evidence_grounding == 1.0: every cited signal_id must
// resolve to a real TimelineEntry.signal_id in the input pack.
func assertGrounded(t *testing.T, p *gpufleetv1.EvidencePack, v *gpufleetv1.Verdict) {
	t.Helper()
	real := map[string]bool{}
	for _, te := range p.GetTimeline() {
		real[te.GetSignalId()] = true
	}
	for _, c := range v.GetCitedSignals() {
		if !real[c.GetSignalId()] {
			t.Errorf("cited %q is NOT in the input window (evidence_grounding violated)", c.GetSignalId())
		}
	}
}

// assertIndependent enforces the >=2-independent-source gate on a FIRED verdict.
func assertIndependent(t *testing.T, v *gpufleetv1.Verdict) {
	t.Helper()
	sources := map[gpufleetv1.SignalSource]bool{}
	for _, c := range v.GetCitedSignals() {
		sources[c.GetSource()] = true
	}
	if len(sources) < 2 {
		t.Errorf("FIRE cited only %d distinct source(s), want >= 2", len(sources))
	}
}

func TestGate_Determinism(t *testing.T) {
	eng := rca.NewEngine(xid79.New())
	// Note reversed input order: a FIRE must be byte-identical regardless of
	// timeline order (sorted-output guarantee).
	p := pack(
		entry("device.lost.dcgm", gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM),
		entry("dmesg.xid79", gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID),
	)
	first := eng.Evaluate(p)
	if !equalStrings(citedIDs(first), []string{"device.lost.dcgm", "dmesg.xid79"}) {
		t.Fatalf("unexpected first verdict cited = %v", citedIDs(first))
	}
	for i := 0; i < 200; i++ {
		got := eng.Evaluate(p)
		if got.GetFaultClass() != first.GetFaultClass() ||
			!equalStrings(citedIDs(got), citedIDs(first)) ||
			got.GetConfidence() != first.GetConfidence() {
			t.Fatalf("non-deterministic verdict on iter %d", i)
		}
	}
}

func TestGate_NilPackAbstains(t *testing.T) {
	v := rca.NewEngine(xid79.New()).Evaluate(nil)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("nil pack: got %v, want ABSTAIN", v.GetFaultClass())
	}
}

func TestSameDevice(t *testing.T) {
	ev := func(device string) rca.Evidence { return rca.Evidence{Device: device} }
	cases := []struct {
		name string
		a, b rca.Evidence
		want bool
	}{
		{"both empty -> true", ev(""), ev(""), true},
		{"a empty -> true (attribution unavailable)", ev(""), ev("GPU-0"), true},
		{"b empty -> true (attribution unavailable)", ev("GPU-0"), ev(""), true},
		{"same id -> true", ev("GPU-7"), ev("GPU-7"), true},
		{"different ids -> false", ev("GPU-0"), ev("GPU-9"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rca.SameDevice(tc.a, tc.b); got != tc.want {
				t.Errorf("SameDevice(%q, %q) = %v, want %v", tc.a.Device, tc.b.Device, got, tc.want)
			}
		})
	}
}

func TestFirstSameDevicePair(t *testing.T) {
	ep := func(device string) *rca.Evidence {
		e := rca.Evidence{Device: device}
		return &e
	}
	gpu0a := ep("GPU-0")
	gpu0b := ep("GPU-0") // second GPU-0, distinct pointer
	gpu9 := ep("GPU-9")
	empty := ep("")

	cases := []struct {
		name   string
		as, bs []*rca.Evidence
		wantA  *rca.Evidence
		wantB  *rca.Evidence
		wantOK bool
	}{
		{
			name:   "both nil -> no pair",
			wantOK: false,
		},
		{
			name:   "as nil -> no pair",
			bs:     []*rca.Evidence{gpu9},
			wantOK: false,
		},
		{
			name:   "bs nil -> no pair",
			as:     []*rca.Evidence{gpu0a},
			wantOK: false,
		},
		{
			name:   "only cross-device pair -> no pair",
			as:     []*rca.Evidence{gpu0a},
			bs:     []*rca.Evidence{gpu9},
			wantOK: false,
		},
		{
			name:   "same device -> first pair",
			as:     []*rca.Evidence{gpu0a},
			bs:     []*rca.Evidence{gpu0b},
			wantA:  gpu0a,
			wantB:  gpu0b,
			wantOK: true,
		},
		{
			name:   "one leg device-less -> first pair (attribution unavailable)",
			as:     []*rca.Evidence{empty},
			bs:     []*rca.Evidence{gpu0a},
			wantA:  empty,
			wantB:  gpu0a,
			wantOK: true,
		},
		{
			// Key mixed-device scenario: early cross-device pair, later same-device pair.
			// FirstSameDevicePair must skip the cross-device (gpu0a, gpu9) and
			// return the first same-device pair (gpu0a, gpu0b).
			name:   "mixed: early cross-device then same-device -> first same-device pair",
			as:     []*rca.Evidence{gpu0a},
			bs:     []*rca.Evidence{gpu9, gpu0b},
			wantA:  gpu0a,
			wantB:  gpu0b,
			wantOK: true,
		},
		{
			// Deterministic: all device-less => first pair (firstA, firstB).
			name:   "device-less window -> returns (firstA, firstB)",
			as:     []*rca.Evidence{empty, gpu0a},
			bs:     []*rca.Evidence{empty, gpu9},
			wantA:  empty,
			wantB:  empty,
			wantOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b, ok := rca.FirstSameDevicePair(tc.as, tc.bs)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if a != tc.wantA {
				t.Errorf("a.Device = %q, want %q", a.Device, tc.wantA.Device)
			}
			if b != tc.wantB {
				t.Errorf("b.Device = %q, want %q", b.Device, tc.wantB.Device)
			}
		})
	}
}
