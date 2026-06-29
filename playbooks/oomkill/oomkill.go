// Package oomkill is the public reference playbook for a job process reaped by
// the Out-Of-Memory killer — the kernel killed a training process under memory
// pressure, so the ranks pinned to the affected device abort and the collective
// stalls.
//
// PUBLIC SEMANTICS ONLY (RULES §F): the Linux OOM killer and GPU framebuffer
// memory pressure are publicly documented. The kernel logs an "oom-killer:
// Killed process ..." line, and DCGM independently exposes high framebuffer /
// memory pressure on the device the dying ranks were using. This playbook embeds
// only that public failure-class knowledge, not any proprietary threshold or
// secret meaning.
//
// It is the seventh demonstration of the >=2-INDEPENDENT-signal gate: a kernel
// oom-killer line is one source reporting on itself, so it can never FIRE alone.
// The fault FIRES only when that kernel/dmesg signal is corroborated by an
// INDEPENDENT DCGM framebuffer/memory-pressure observation witnessed by a
// DIFFERENT source (SIGNAL_SOURCE_DCGM). Either leg alone — or a signal whose
// source does not match its leg — ABSTAINs. Independence is judged on
// SignalSource, never a producer-declared field (TASK-0018).
//
// Signal-id prefix conventions (the open agent's normalized timeline emits
// matching ids; documented here so the agent-integration card matches):
//
//   - "dmesg.oom.killed"  — the kernel oom-killer line reaping a job process.
//     Source MUST be SIGNAL_SOURCE_DMESG_XID.
//   - "mem.pressure.fb"   — an INDEPENDENT DCGM framebuffer / memory-pressure
//     observation on the affected device. Source MUST be SIGNAL_SOURCE_DCGM
//     (a different source than the kernel line).
package oomkill

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// Signal-id prefixes used by the open agent's normalized timeline. They are
// matched as PREFIXES so a window may carry several distinct ids of a kind (e.g.
// one oom line per killed pid) while remaining publicly defined, non-proprietary
// conventions.
const (
	// oomKilledPrefix marks the kernel oom-killer line reaping a job process. The
	// playbook additionally requires the source to be SIGNAL_SOURCE_DMESG_XID, so
	// the id alone never forges a fire.
	oomKilledPrefix = "dmesg.oom.killed"
	// memPressurePrefix marks an INDEPENDENT DCGM framebuffer / memory-pressure
	// observation, drawn from SIGNAL_SOURCE_DCGM.
	memPressurePrefix = "mem.pressure.fb"
)

// Sig is the OOM-kill signature. It carries no state.
type Sig struct{}

// New returns the OOM-kill signature for registration.
func New() Sig { return Sig{} }

// FaultClass is the public adjudicated outcome for an OOM-killed job process.
func (Sig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_OOM_KILL
}

// GateSignature is the shared, versioned signature id (audit metadata only).
func (Sig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_OOM_KILL
}

// Match fires when the window contains (a) a kernel oom-killer line whose source
// is SIGNAL_SOURCE_DMESG_XID AND (b) an INDEPENDENT DCGM framebuffer/memory-
// pressure observation from SIGNAL_SOURCE_DCGM. The returned citations are
// exactly the two matched evidences (real inputs only). It returns (nil, false)
// — ABSTAIN — otherwise: a missing leg, or a signal whose source does not match
// its leg.
func (Sig) Match(window []rca.Evidence) (cited []rca.Evidence, fired bool) {
	var killed *rca.Evidence
	var pressure *rca.Evidence

	for i := range window {
		e := &window[i]
		switch {
		case rca.HasIDPrefix(e.SignalID, oomKilledPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID:
			if killed == nil {
				killed = e
			}
		case rca.HasIDPrefix(e.SignalID, memPressurePrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM:
			if pressure == nil {
				pressure = e
			}
		}
	}

	if killed == nil || pressure == nil {
		return nil, false
	}
	// Independence holds structurally: the two legs are pinned to distinct
	// sources (DMESG_XID and DCGM), so a fire always cites two independent
	// sources. The engine re-checks the >=2-independent-source rule centrally.
	return []rca.Evidence{*killed, *pressure}, true
}
