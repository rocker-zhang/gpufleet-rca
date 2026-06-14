package registry_test

import (
	"testing"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
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
