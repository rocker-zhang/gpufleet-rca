// Package thermalthrottle is the public reference playbook for a GPU whose
// clocks are being held down by a HARDWARE THERMAL limit — the device is hot
// enough that firmware is capping core/memory clocks to stay in its thermal
// envelope, so sustained compute throughput is silently lost.
//
// PUBLIC SEMANTICS ONLY (RULES §F): thermal throttling is a publicly documented
// GPU behaviour. DCGM exposes a thermal-violation / "clocks throttled by thermal
// limit" reason, and the kernel independently logs a hardware thermal slowdown
// for the device. This playbook embeds only that public failure-class knowledge,
// not any proprietary threshold or secret reason-code meaning.
//
// It is the fifth demonstration of the >=2-INDEPENDENT-signal gate: a DCGM
// thermal-violation counter is one source reporting on itself, so it can never
// FIRE alone. The fault FIRES only when that DCGM signal is corroborated by an
// INDEPENDENT kernel/dmesg thermal-slowdown log witnessed by a DIFFERENT source
// (SIGNAL_SOURCE_DMESG_XID). Either leg alone — or a signal whose source does
// not match its leg — ABSTAINs. Independence is judged on SignalSource, never a
// producer-declared field (TASK-0018).
//
// Signal-id prefix conventions (the open agent's normalized timeline emits
// matching ids; documented here so the agent-integration card matches):
//
//   - "thermal.violation"       — the DCGM thermal-violation / clocks-throttled-
//     by-thermal reason. Source MUST be SIGNAL_SOURCE_DCGM.
//   - "dmesg.thermal.slowdown"  — an INDEPENDENT kernel/dmesg hardware thermal
//     slowdown line for the device. Source MUST be SIGNAL_SOURCE_DMESG_XID
//     (a different source than the DCGM counter).
package thermalthrottle

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// Signal-id prefixes used by the open agent's normalized timeline. They are
// matched as PREFIXES so a window may carry several distinct ids of a kind while
// remaining publicly defined, non-proprietary conventions.
const (
	// thermalViolationPrefix marks the DCGM thermal-violation / clocks-throttled-
	// by-thermal reason. The playbook additionally requires the source to be
	// SIGNAL_SOURCE_DCGM, so the id alone never forges a fire.
	thermalViolationPrefix = "thermal.violation"
	// thermalSlowdownPrefix marks an INDEPENDENT kernel/dmesg hardware thermal
	// slowdown line, drawn from SIGNAL_SOURCE_DMESG_XID.
	thermalSlowdownPrefix = "dmesg.thermal.slowdown"
)

// Sig is the thermal-throttle signature. It carries no state.
type Sig struct{}

// New returns the thermal-throttle signature for registration.
func New() Sig { return Sig{} }

// FaultClass is the public adjudicated outcome for a thermally throttled GPU.
func (Sig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_THERMAL_THROTTLE
}

// GateSignature is the shared, versioned signature id (audit metadata only).
func (Sig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_THERMAL_THROTTLE
}

// Match fires when the window contains (a) a DCGM thermal-violation whose source
// is SIGNAL_SOURCE_DCGM AND (b) a kernel/dmesg thermal-slowdown corroboration
// from SIGNAL_SOURCE_DMESG_XID that is co-located on the same device. It
// collects ALL matching candidates for each leg and calls rca.FirstSameDevicePair
// to find the first co-located pair, so a mixed-device window with an early
// cross-device pair does not suppress a valid same-device pair later in the
// window. It returns (nil, false) — ABSTAIN — otherwise.
func (Sig) Match(window []rca.Evidence) (cited []rca.Evidence, fired bool) {
	var violations []*rca.Evidence
	var slowdowns []*rca.Evidence

	for i := range window {
		e := &window[i]
		switch {
		case rca.HasIDPrefix(e.SignalID, thermalViolationPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM:
			violations = append(violations, e)
		case rca.HasIDPrefix(e.SignalID, thermalSlowdownPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID:
			slowdowns = append(slowdowns, e)
		}
	}

	a, b, ok := rca.FirstSameDevicePair(violations, slowdowns)
	if !ok {
		return nil, false
	}
	return []rca.Evidence{*a, *b}, true
}
