// Package linkdegraded is the public reference playbook for a DEGRADED GPU
// interconnect link — an NVLink or PCIe link running below its expected
// width/speed or accumulating link errors.
//
// PUBLIC SEMANTICS ONLY (RULES §F): NVLink CRC/replay errors and PCIe link
// errors are publicly documented interconnect faults — DCGM exposes NVLink/PCIe
// link error counters, and a link running below its negotiated width/speed (or
// a PCIe link-down) is independently observable from PCIe/sysfs metrics
// (Prometheus) or an nvidia-smi link probe surfaced through PROC. This playbook
// embeds ONLY that public failure-class knowledge — no externally-sourced or
// secret error-code meanings, no thresholds beyond the public corroboration
// pattern.
//
// It is the fourth demonstration of the >=2-INDEPENDENT-signal gate: a DCGM
// link error counter is, on its own, one source reporting on itself, so it can
// never FIRE alone. The fault FIRES only when that DCGM counter is corroborated
// by an INDEPENDENT link width/speed downgrade (or PCIe link-down) witnessed by
// a DIFFERENT, non-DCGM source (PROMETHEUS or PROC). Either leg alone — or two
// readings of the SAME source (e.g. a second DCGM counter) — ABSTAINs.
// Independence is judged on SignalSource, never a producer-declared field
// (TASK-0018).
//
// Signal-id prefix conventions (the open agent's normalized timeline will emit
// matching ids in a later agent-integration card; documented here so that card
// matches):
//
//   - "link.error"    — the DCGM NVLink CRC/replay or PCIe link error counter
//     delta. Source MUST be SIGNAL_SOURCE_DCGM. A window may carry several
//     distinct ids of this kind (e.g. one per link: "link.error.nvlink2",
//     "link.error.pcie"); they are prefix-matched.
//   - "link.degraded" — an INDEPENDENT link width/speed downgrade or PCIe
//     link-down observation from a non-DCGM source (PROMETHEUS or PROC), e.g.
//     "link.degraded.width" (nvidia-smi probe via PROC),
//     "link.degraded.pcie" (PCIe metric via Prometheus). DCGM is deliberately
//     excluded from this leg — it is the same source as the error-counter leg
//     and would not be independent.
package linkdegraded

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// Signal-id prefixes used by the open agent's normalized timeline. They are
// matched as PREFIXES so a window may carry several distinct ids of a kind
// (e.g. one DCGM counter per link) while remaining publicly defined,
// non-proprietary conventions.
const (
	// linkErrorPrefix marks the DCGM NVLink CRC/replay or PCIe link error
	// counter delta. The playbook additionally requires the source to be
	// SIGNAL_SOURCE_DCGM, so the id alone never forges a fire.
	linkErrorPrefix = "link.error"
	// linkDegradedPrefix marks an INDEPENDENT link width/speed downgrade or PCIe
	// link-down corroboration drawn from a non-DCGM source (PROMETHEUS or PROC).
	linkDegradedPrefix = "link.degraded"
)

// Sig is the link-degraded signature. It carries no state.
type Sig struct{}

// New returns the link-degraded signature for registration.
func New() Sig { return Sig{} }

// FaultClass is the public adjudicated outcome for a degraded interconnect link.
func (Sig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_LINK_DEGRADED
}

// GateSignature is the shared, versioned signature id (audit metadata only).
func (Sig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_LINK_DEGRADED
}

// Match fires when the window contains (a) a DCGM link error counter whose
// source is SIGNAL_SOURCE_DCGM AND (b) a link width/speed downgrade (or PCIe
// link-down) corroboration from a DIFFERENT, non-DCGM source (PROMETHEUS or
// PROC) that is co-located on the same device. It collects ALL matching
// candidates for each leg and calls rca.FirstSameDevicePair to find the first
// co-located pair, so a mixed-device window with an early cross-device pair
// does not suppress a valid same-device pair later in the window.
// It returns (nil, false) — ABSTAIN — otherwise.
func (Sig) Match(window []rca.Evidence) (cited []rca.Evidence, fired bool) {
	var linkErrs []*rca.Evidence
	var corroborators []*rca.Evidence

	for i := range window {
		e := &window[i]
		switch {
		case rca.HasIDPrefix(e.SignalID, linkErrorPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM:
			linkErrs = append(linkErrs, e)
		case rca.HasIDPrefix(e.SignalID, linkDegradedPrefix) && isLinkDegradedSource(e.Source):
			corroborators = append(corroborators, e)
		}
	}

	a, b, ok := rca.FirstSameDevicePair(linkErrs, corroborators)
	if !ok {
		return nil, false
	}
	return []rca.Evidence{*a, *b}, true
}

// isLinkDegradedSource is the public set of sources that can witness a degraded
// link INDEPENDENTLY of the DCGM error counter: PCIe/sysfs metrics via
// Prometheus, or an nvidia-smi link probe surfaced through PROC.
// SIGNAL_SOURCE_DCGM is deliberately excluded — a second DCGM counter is the
// same source as the error-counter leg and would not be independent.
func isLinkDegradedSource(s gpufleetv1.SignalSource) bool {
	switch s {
	case gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS,
		gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC:
		return true
	default:
		return false
	}
}
