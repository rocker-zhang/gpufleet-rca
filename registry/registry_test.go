package registry_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/registry"
)

// TestDefault_HasXID79Registered proves XID79 is registered (not an empty stub)
// by driving a real 2-independent-source window through the default engine.
func TestDefault_HasXID79Registered(t *testing.T) {
	if len(registry.Default()) == 0 {
		t.Fatal("default registry is empty; XID79 must be registered")
	}

	eng := registry.NewDefaultEngine()
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "dmesg.xid79", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID},
			{SignalId: "device.lost.dcgm", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
		},
	}
	v := eng.Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS {
		t.Fatalf("registered XID79 did not fire on a valid window: %v", v.GetFaultClass())
	}
	if v.GetSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_XID79_FALLEN_OFF_BUS {
		t.Fatalf("signature = %v", v.GetSignature())
	}
}

// TestDefault_AbstainsOnSingleSignal proves ABSTAIN-by-default through the
// registered engine.
func TestDefault_AbstainsOnSingleSignal(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "dmesg.xid79", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("single signal: got %v, want ABSTAIN", v.GetFaultClass())
	}
}

// TestDefault_NCCLTimeoutRegistered proves the slot-2 NCCL-timeout signature is
// wired into the default engine: an NCCL-timeout pack (NCCL watchdog + an
// INDEPENDENT non-NCCL collective-stall) routes to it and produces a
// NCCL_TIMEOUT verdict at the FAULT confidence floor.
func TestDefault_NCCLTimeoutRegistered(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "nccl.timeout.rank0", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL},
			{SignalId: "collective.stall.scheduler", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_SCHEDULER},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_NCCL_TIMEOUT {
		t.Fatalf("NCCL-timeout pack: got %v, want NCCL_TIMEOUT", v.GetFaultClass())
	}
	if v.GetSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_NCCL_TIMEOUT {
		t.Fatalf("signature = %v, want GATE_SIGNATURE_NCCL_TIMEOUT", v.GetSignature())
	}
	if v.GetConfidence() != rca.FaultConfidenceFloor {
		t.Fatalf("confidence = %v, want FaultConfidenceFloor (%v)", v.GetConfidence(), rca.FaultConfidenceFloor)
	}
	// cited_signals reference exactly the two real input legs (grounding == 1.0).
	if got := citedIDs(v); !sameSet(got, []string{"nccl.timeout.rank0", "collective.stall.scheduler"}) {
		t.Fatalf("cited = %v, want exactly the two real legs", got)
	}
}

// TestDefault_NCCLTimeoutForgedSameSourceAbstains proves the engine's
// defense-in-depth >=2-independent-source gate: two same-source NCCL legs never
// fire NCCL_TIMEOUT even though both ids match the playbook prefixes.
func TestDefault_NCCLTimeoutForgedSameSourceAbstains(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "nccl.timeout.rank0", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL},
			{SignalId: "collective.stall.rank1", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("forged NCCL+NCCL: got %v, want ABSTAIN", v.GetFaultClass())
	}
}

// TestDefault_XID79NoPrecedenceRegression proves slot 2 did not perturb slot 1:
// an XID79 pack still routes to XID79, not NCCL_TIMEOUT.
func TestDefault_XID79NoPrecedenceRegression(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "dmesg.xid79", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID},
			{SignalId: "device.lost.dcgm", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS {
		t.Fatalf("XID79 pack: got %v, want GPU_FALLEN_OFF_BUS", v.GetFaultClass())
	}
	if v.GetSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_XID79_FALLEN_OFF_BUS {
		t.Fatalf("signature = %v, want XID79", v.GetSignature())
	}
}

func citedIDs(v *gpufleetv1.Verdict) []string {
	out := make([]string, 0, len(v.GetCitedSignals()))
	for _, c := range v.GetCitedSignals() {
		out = append(out, c.GetSignalId())
	}
	return out
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
