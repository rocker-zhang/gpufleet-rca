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

// TestDefault_ECCUncorrectableRegistered proves the slot-3 ECC-uncorrectable
// signature is wired into the default engine: a DCGM double-bit ECC counter +
// an INDEPENDENT dmesg ECC Xid line routes to it and produces an
// ECC_UNCORRECTABLE verdict at the FAULT confidence floor, citing exactly the
// two real legs.
func TestDefault_ECCUncorrectableRegistered(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "ecc.dbe.fb", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
			{SignalId: "dmesg.xid.ecc.94", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ECC_UNCORRECTABLE {
		t.Fatalf("ECC pack: got %v, want ECC_UNCORRECTABLE", v.GetFaultClass())
	}
	if v.GetSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_ECC_UNCORRECTABLE {
		t.Fatalf("signature = %v, want GATE_SIGNATURE_ECC_UNCORRECTABLE", v.GetSignature())
	}
	if v.GetConfidence() != rca.FaultConfidenceFloor {
		t.Fatalf("confidence = %v, want FaultConfidenceFloor (%v)", v.GetConfidence(), rca.FaultConfidenceFloor)
	}
	if got := citedIDs(v); !sameSet(got, []string{"ecc.dbe.fb", "dmesg.xid.ecc.94"}) {
		t.Fatalf("cited = %v, want exactly the two real legs", got)
	}
}

// TestDefault_ECCUncorrectableForgedSameSourceAbstains proves the engine's
// defense-in-depth >=2-independent-source gate: two same-source DCGM legs never
// fire ECC_UNCORRECTABLE even though both ids match the playbook prefixes.
func TestDefault_ECCUncorrectableForgedSameSourceAbstains(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "ecc.dbe.fb", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
			{SignalId: "dmesg.xid.ecc.94", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("forged DCGM+DCGM ECC: got %v, want ABSTAIN", v.GetFaultClass())
	}
}

// TestDefault_LinkDegradedRegistered proves the slot-4 link-degraded signature
// is wired into the default engine: a DCGM link error counter + an INDEPENDENT
// non-DCGM link-degraded observation routes to it and produces a LINK_DEGRADED
// verdict at the FAULT confidence floor, citing exactly the two real legs.
func TestDefault_LinkDegradedRegistered(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "link.error.nvlink2", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
			{SignalId: "link.degraded.pcie", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_LINK_DEGRADED {
		t.Fatalf("link pack: got %v, want LINK_DEGRADED", v.GetFaultClass())
	}
	if v.GetSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_LINK_DEGRADED {
		t.Fatalf("signature = %v, want GATE_SIGNATURE_LINK_DEGRADED", v.GetSignature())
	}
	if v.GetConfidence() != rca.FaultConfidenceFloor {
		t.Fatalf("confidence = %v, want FaultConfidenceFloor (%v)", v.GetConfidence(), rca.FaultConfidenceFloor)
	}
	if got := citedIDs(v); !sameSet(got, []string{"link.error.nvlink2", "link.degraded.pcie"}) {
		t.Fatalf("cited = %v, want exactly the two real legs", got)
	}
}

// TestDefault_LinkDegradedForgedSameSourceAbstains proves the engine's
// defense-in-depth >=2-independent-source gate: two same-source DCGM legs never
// fire LINK_DEGRADED even though both ids match the playbook prefixes.
func TestDefault_LinkDegradedForgedSameSourceAbstains(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "link.error.nvlink2", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
			{SignalId: "link.degraded.pcie", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("forged DCGM+DCGM link: got %v, want ABSTAIN", v.GetFaultClass())
	}
}

// TestDefault_NCCLNoPrecedenceRegression proves slots 3 & 4 did not perturb
// slot 2: an NCCL-timeout pack still routes to NCCL_TIMEOUT.
func TestDefault_NCCLNoPrecedenceRegression(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			{SignalId: "nccl.timeout.rank0", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL},
			{SignalId: "collective.stall.scheduler", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_SCHEDULER},
		},
	}
	v := registry.NewDefaultEngine().Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_NCCL_TIMEOUT {
		t.Fatalf("NCCL pack: got %v, want NCCL_TIMEOUT", v.GetFaultClass())
	}
	if v.GetSignature() != gpufleetv1.GateSignature_GATE_SIGNATURE_NCCL_TIMEOUT {
		t.Fatalf("signature = %v, want NCCL_TIMEOUT", v.GetSignature())
	}
}

// TestDefault_MixedClassWindowRoutesDeterministically proves a window carrying
// the complete leg-sets of ALL FOUR classes routes to the first registered slot
// (XID79), deterministically and repeatably — the four leg-patterns are
// mutually exclusive, so first-match-wins is stable regardless of timeline order.
func TestDefault_MixedClassWindowRoutesDeterministically(t *testing.T) {
	p := &gpufleetv1.EvidencePack{
		ContractVersion: "v1",
		Timeline: []*gpufleetv1.TimelineEntry{
			// link-degraded legs first in the timeline...
			{SignalId: "link.error.nvlink2", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
			{SignalId: "link.degraded.pcie", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS},
			// ...ecc legs...
			{SignalId: "ecc.dbe.fb", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
			{SignalId: "dmesg.xid.ecc.94", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID},
			// ...nccl legs...
			{SignalId: "nccl.timeout.rank0", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL},
			{SignalId: "collective.stall.scheduler", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_SCHEDULER},
			// ...and xid79 legs LAST.
			{SignalId: "dmesg.xid79", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID},
			{SignalId: "device.lost.dcgm", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM},
		},
	}
	eng := registry.NewDefaultEngine()
	first := eng.Evaluate(p)
	if first.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS {
		t.Fatalf("mixed window: got %v, want first-slot XID79 (GPU_FALLEN_OFF_BUS)", first.GetFaultClass())
	}
	if got := citedIDs(first); !sameSet(got, []string{"dmesg.xid79", "device.lost.dcgm"}) {
		t.Fatalf("cited = %v, want exactly the XID79 legs", got)
	}
	// Determinism: re-evaluating the same window yields the identical verdict.
	second := eng.Evaluate(p)
	if second.GetFaultClass() != first.GetFaultClass() || second.GetSignature() != first.GetSignature() {
		t.Fatalf("non-deterministic routing: %v/%v then %v/%v",
			first.GetFaultClass(), first.GetSignature(), second.GetFaultClass(), second.GetSignature())
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
