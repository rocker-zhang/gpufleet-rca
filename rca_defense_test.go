package rca_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// rogueSig is a deliberately-broken signature that ALWAYS fires and returns
// whatever legs it is told to — used to prove the ENGINE's defense-in-depth
// (grounding + independence re-enforcement) holds even when a signature lies.
type rogueSig struct{ cited []rca.Evidence }

func (rogueSig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS
}
func (rogueSig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_UNSPECIFIED
}
func (r rogueSig) Match([]rca.Evidence) ([]rca.Evidence, bool) { return r.cited, true }

const dmesg = gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID
const dcgm = gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM

// A signature that fires while citing a FABRICATED leg (not in the window) must
// be grounded: the fabricated leg is dropped, leaving <2 independent sources →
// the engine ABSTAINS instead of firing on hallucinated evidence.
func TestEngine_GroundsFabricatedCitation(t *testing.T) {
	p := pack(entry("dmesg.xid79.GPU-0", dmesg)) // only ONE real signal in the window
	eng := rca.NewEngine(rogueSig{cited: []rca.Evidence{
		{SignalID: "dmesg.xid79.GPU-0", Source: dmesg}, // grounded (present)
		{SignalID: "fabricated.leg.GPU-0", Source: dcgm}, // NOT in window → dropped
	}})
	v := eng.Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("engine fired on a fabricated citation: %s", v.GetFaultClass())
	}
}

// A signature that fires citing two SAME-source legs (both present) must still
// ABSTAIN — the engine re-enforces ≥2 INDEPENDENT sources on the grounded legs.
func TestEngine_ReEnforcesIndependenceOnSameSource(t *testing.T) {
	p := pack(entry("dmesg.xid79.GPU-0", dmesg), entry("dmesg.xid79.GPU-1", dmesg))
	eng := rca.NewEngine(rogueSig{cited: []rca.Evidence{
		{SignalID: "dmesg.xid79.GPU-0", Source: dmesg},
		{SignalID: "dmesg.xid79.GPU-1", Source: dmesg}, // same source → 1 distinct
	}})
	v := eng.Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("engine fired on two same-source legs: %s", v.GetFaultClass())
	}
}

// Two genuinely independent grounded legs DO fire — confirms the defenses don't
// over-block a legitimate corroboration.
func TestEngine_FiresOnTwoIndependentGroundedLegs(t *testing.T) {
	p := pack(entry("dmesg.xid79.GPU-0", dmesg), entry("device.lost.GPU-0", dcgm))
	eng := rca.NewEngine(rogueSig{cited: []rca.Evidence{
		{SignalID: "dmesg.xid79.GPU-0", Source: dmesg},
		{SignalID: "device.lost.GPU-0", Source: dcgm},
	}})
	v := eng.Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS {
		t.Fatalf("engine should fire on two independent grounded legs, got %s", v.GetFaultClass())
	}
}

// An UNSPECIFIED-source leg is uncitable provenance: paired with one real source
// it must NOT count as a second independent source → ABSTAIN.
func TestEngine_UnspecifiedSourceNotIndependent(t *testing.T) {
	p := pack(
		entry("dmesg.xid79.GPU-0", dmesg),
		entry("mystery.GPU-0", gpufleetv1.SignalSource_SIGNAL_SOURCE_UNSPECIFIED),
	)
	eng := rca.NewEngine(rogueSig{cited: []rca.Evidence{
		{SignalID: "dmesg.xid79.GPU-0", Source: dmesg},
		{SignalID: "mystery.GPU-0", Source: gpufleetv1.SignalSource_SIGNAL_SOURCE_UNSPECIFIED},
	}})
	v := eng.Evaluate(p)
	if v.GetFaultClass() != gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN {
		t.Fatalf("UNSPECIFIED source must not count as independent: %s", v.GetFaultClass())
	}
}
