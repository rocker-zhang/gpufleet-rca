// Package xidfatal is the public reference playbook for a FATAL Xid event — the
// driver logged a fatal Xid for the device (a hardware/driver fault that leaves
// the GPU unusable until reset), corroborated by an independent DCGM device-
// health failure. This is distinct from XID79 fallen-off-bus (its own playbook):
// here the device is still on the bus but the driver has declared a fatal fault.
//
// PUBLIC SEMANTICS ONLY (RULES §F): Xid messages and DCGM device-health checks
// are publicly documented. The driver logs a fatal Xid line, and DCGM
// independently reports the device as health-failed. This playbook requires only
// that public pattern — a fatal-Xid log corroborated by an independent health
// failure — and encodes no secret per-Xid-number meaning or proprietary
// threshold.
//
// It is the eighth demonstration of the >=2-INDEPENDENT-signal gate: a driver
// Xid line is one source reporting on itself, so it can never FIRE alone. The
// fault FIRES only when that kernel/dmesg Xid signal is corroborated by an
// INDEPENDENT DCGM device-health failure witnessed by a DIFFERENT source
// (SIGNAL_SOURCE_DCGM). Either leg alone — or a signal whose source does not
// match its leg — ABSTAINs. Independence is judged on SignalSource, never a
// producer-declared field (TASK-0018).
//
// Signal-id prefix conventions (the open agent's normalized timeline emits
// matching ids; documented here so the agent-integration card matches):
//
//   - "dmesg.xid.fatal"      — the driver fatal-Xid log line for the device.
//     Source MUST be SIGNAL_SOURCE_DMESG_XID. (Distinct from xid79's
//     "dmesg.xid79" prefix, so the two playbooks never cross-match.)
//   - "device.health.failed" — an INDEPENDENT DCGM device-health failure for the
//     same device. Source MUST be SIGNAL_SOURCE_DCGM (a different source than the
//     driver Xid line).
package xidfatal

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// Signal-id prefixes used by the open agent's normalized timeline. They are
// matched as PREFIXES so a window may carry several distinct ids of a kind (e.g.
// one line per Xid number) while remaining publicly defined, non-proprietary
// conventions.
const (
	// xidFatalPrefix marks the driver fatal-Xid log line. The playbook
	// additionally requires the source to be SIGNAL_SOURCE_DMESG_XID, so the id
	// alone never forges a fire. It is deliberately distinct from xid79's
	// "dmesg.xid79" prefix.
	xidFatalPrefix = "dmesg.xid.fatal"
	// deviceHealthFailedPrefix marks an INDEPENDENT DCGM device-health failure,
	// drawn from SIGNAL_SOURCE_DCGM.
	deviceHealthFailedPrefix = "device.health.failed"
)

// Sig is the fatal-Xid signature. It carries no state.
type Sig struct{}

// New returns the fatal-Xid signature for registration.
func New() Sig { return Sig{} }

// FaultClass is the public adjudicated outcome for a fatal Xid event.
func (Sig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_XID_FATAL
}

// GateSignature is the shared, versioned signature id (audit metadata only).
func (Sig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_XID_FATAL
}

// Match fires when the window contains (a) a driver fatal-Xid log whose source
// is SIGNAL_SOURCE_DMESG_XID AND (b) an INDEPENDENT DCGM device-health failure
// from SIGNAL_SOURCE_DCGM. The returned citations are exactly the two matched
// evidences (real inputs only). It returns (nil, false) — ABSTAIN — otherwise:
// a missing leg, or a signal whose source does not match its leg.
func (Sig) Match(window []rca.Evidence) (cited []rca.Evidence, fired bool) {
	var xid *rca.Evidence
	var health *rca.Evidence

	for i := range window {
		e := &window[i]
		switch {
		case rca.HasIDPrefix(e.SignalID, xidFatalPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID:
			if xid == nil {
				xid = e
			}
		case rca.HasIDPrefix(e.SignalID, deviceHealthFailedPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM:
			if health == nil {
				health = e
			}
		}
	}

	if xid == nil || health == nil {
		return nil, false
	}
	// Independence holds structurally: the two legs are pinned to distinct
	// sources (DMESG_XID and DCGM), so a fire always cites two independent
	// sources. The engine re-checks the >=2-independent-source rule centrally.
	return []rca.Evidence{*xid, *health}, true
}
