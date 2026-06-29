// Package xid79 is the public reference playbook for NVIDIA XID 79
// ("GPU has fallen off the bus" / device lost on the PCIe bus).
//
// PUBLIC SEMANTICS ONLY (RULES §F): XID 79 is a publicly documented NVIDIA
// error number meaning the GPU became unreachable on the bus. This playbook
// embeds ONLY that public failure-class knowledge — no externally-sourced or
// secret error-code meanings, no thresholds beyond the public corroboration
// pattern.
//
// It is the canonical demonstration of the >=2-INDEPENDENT-signal gate: it
// FIRES only when an xid=79 kernel/dmesg signal is corroborated by an
// INDEPENDENT "device lost / unreachable" signal from a DIFFERENT source
// (DCGM / PCIe-prometheus / nvidia-smi via PROC). Either signal alone — or two
// readings of the SAME source — ABSTAINs. Independence is judged on
// SignalSource, never a producer-declared field (TASK-0018).
package xid79

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// Signal-id prefixes used by the open agent's normalized timeline. They are
// matched as PREFIXES so a window may carry several distinct ids of a kind
// (e.g. two DCGM samples) while remaining publicly defined, non-proprietary
// conventions.
const (
	// xidPrefix marks the kernel/dmesg NVRM Xid line. The playbook additionally
	// requires the source to be DMESG_XID, so the id alone never forges a fire.
	xidPrefix = "dmesg.xid79"
	// deviceLostPrefix marks a "device lost / unreachable" corroboration drawn
	// from a non-dmesg source (DCGM health, PCIe link-down, nvidia-smi probe).
	deviceLostPrefix = "device.lost"
)

// Sig is the XID79 signature. It carries no state.
type Sig struct{}

// New returns the XID79 signature for registration.
func New() Sig { return Sig{} }

// FaultClass is the public adjudicated outcome for XID 79.
func (Sig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_GPU_FALLEN_OFF_BUS
}

// GateSignature is the shared, versioned signature id (audit metadata only).
func (Sig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_XID79_FALLEN_OFF_BUS
}

// Match fires when the window contains (a) an xid=79 dmesg signal whose source
// is DMESG_XID AND (b) a "device lost" corroboration from a DIFFERENT,
// non-dmesg source that is co-located on the same device. It collects ALL
// matching candidates for each leg and calls rca.FirstSameDevicePair to find
// the first co-located pair, so a mixed-device window with an early cross-device
// pair does not suppress a valid same-device pair later in the window.
// It returns (nil, false) — ABSTAIN — otherwise.
func (Sig) Match(window []rca.Evidence) (cited []rca.Evidence, fired bool) {
	var xids []*rca.Evidence
	var corroborators []*rca.Evidence

	for i := range window {
		e := &window[i]
		switch {
		case rca.HasIDPrefix(e.SignalID, xidPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID:
			xids = append(xids, e)
		case rca.HasIDPrefix(e.SignalID, deviceLostPrefix) && isDeviceLostSource(e.Source):
			corroborators = append(corroborators, e)
		}
	}

	a, b, ok := rca.FirstSameDevicePair(xids, corroborators)
	if !ok {
		return nil, false
	}
	return []rca.Evidence{*a, *b}, true
}

// isDeviceLostSource is the public set of sources that can witness a GPU lost
// on the bus independently of the kernel dmesg log: DCGM health, a PCIe
// link-down counter via Prometheus, or an nvidia-smi probe surfaced through
// PROC. DMESG_XID is deliberately excluded — it is the same source as the xid
// leg and would not be independent.
func isDeviceLostSource(s gpufleetv1.SignalSource) bool {
	switch s {
	case gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM,
		gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS,
		gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC:
		return true
	default:
		return false
	}
}
