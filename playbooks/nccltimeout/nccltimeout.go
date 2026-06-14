// Package nccltimeout is the public reference playbook for an NCCL collective
// timeout — the canonical distributed-training hang where the NCCL watchdog
// fires because a collective (all-reduce/all-gather/...) stops making progress.
//
// PUBLIC SEMANTICS ONLY (RULES §F): "NCCL watchdog timeout" is publicly
// documented NCCL behaviour — the comm's blocking-wait/watchdog aborts after a
// collective fails to complete within its timeout. This playbook embeds ONLY
// that public failure-class knowledge — no externally-sourced or secret
// error-code meanings, no thresholds beyond the public corroboration pattern.
//
// It is the second demonstration of the >=2-INDEPENDENT-signal gate: an NCCL
// watchdog line is, by itself, just the NCCL library reporting on itself, so it
// can never FIRE alone. The fault FIRES only when that NCCL signal is
// corroborated by an INDEPENDENT, NON-NCCL signal that the collective/job is
// genuinely stalled — observed by a source that does not re-read the NCCL log
// (scheduler "job alive but not progressing", DCGM/Prometheus "GPUs idle /
// comms stalled while running", or PROC "process alive, no progress"). Either
// leg alone — or two readings of the SAME source (e.g. a second NCCL line from
// a different rank) — ABSTAINs. Independence is judged on SignalSource, never a
// producer-declared field (TASK-0018).
//
// Signal-id prefix conventions (the open agent's normalized timeline will emit
// matching ids in a later agent-integration card; documented here so that card
// matches):
//
//   - "nccl.timeout"     — the NCCL watchdog/timeout line. Source MUST be
//     SIGNAL_SOURCE_NCCL. A window may carry several distinct ids of this kind
//     (e.g. one per rank: "nccl.timeout.rank3"); they are prefix-matched.
//   - "collective.stall" — an INDEPENDENT "collective/job stalled" observation
//     from a non-NCCL source (scheduler/DCGM/Prometheus/PROC), e.g.
//     "collective.stall.scheduler", "collective.stall.dcgm".
package nccltimeout

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// Signal-id prefixes used by the open agent's normalized timeline. They are
// matched as PREFIXES so a window may carry several distinct ids of a kind
// (e.g. one NCCL line per rank) while remaining publicly defined, non-proprietary
// conventions.
const (
	// ncclTimeoutPrefix marks the NCCL watchdog/timeout line. The playbook
	// additionally requires the source to be SIGNAL_SOURCE_NCCL, so the id
	// alone never forges a fire.
	ncclTimeoutPrefix = "nccl.timeout"
	// collectiveStallPrefix marks an INDEPENDENT "collective/job stalled"
	// corroboration drawn from a non-NCCL source (scheduler/DCGM/Prometheus/
	// PROC).
	collectiveStallPrefix = "collective.stall"
)

// Sig is the NCCL-timeout signature. It carries no state.
type Sig struct{}

// New returns the NCCL-timeout signature for registration.
func New() Sig { return Sig{} }

// FaultClass is the public adjudicated outcome for an NCCL collective timeout.
func (Sig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_NCCL_TIMEOUT
}

// GateSignature is the shared, versioned signature id (audit metadata only).
func (Sig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_NCCL_TIMEOUT
}

// Match fires when the window contains (a) an NCCL watchdog/timeout signal whose
// source is SIGNAL_SOURCE_NCCL AND (b) a "collective stalled" corroboration from
// a DIFFERENT, non-NCCL source. The returned citations are exactly the matched
// evidence (real inputs only). It returns (nil, false) — ABSTAIN — otherwise,
// including when both legs share the same source (forged independence, e.g.
// NCCL+NCCL).
func (Sig) Match(window []rca.Evidence) (cited []rca.Evidence, fired bool) {
	var nccl *rca.Evidence
	var corroborator *rca.Evidence

	for i := range window {
		e := &window[i]
		switch {
		case hasPrefix(e.SignalID, ncclTimeoutPrefix) && e.Source == gpufleetv1.SignalSource_SIGNAL_SOURCE_NCCL:
			if nccl == nil {
				nccl = e
			}
		case hasPrefix(e.SignalID, collectiveStallPrefix) && isCollectiveStallSource(e.Source):
			if corroborator == nil {
				corroborator = e
			}
		}
	}

	if nccl == nil || corroborator == nil {
		return nil, false
	}
	// Independence is on SOURCE: the corroborator must not share the NCCL leg's
	// source. (By construction the corroborator is non-NCCL, but this makes the
	// load-bearing rule explicit and local to the signature.)
	if corroborator.Source == nccl.Source {
		return nil, false
	}
	return []rca.Evidence{*nccl, *corroborator}, true
}

// isCollectiveStallSource is the public set of sources that can witness a stuck
// collective INDEPENDENTLY of the NCCL log: the scheduler (job alive but not
// progressing), DCGM / Prometheus (GPUs idle or comms stalled while the job is
// "running"), or PROC (process alive, no progress). SIGNAL_SOURCE_NCCL is
// deliberately excluded — a second NCCL line (even from a different rank) is the
// same source and would not be independent.
func isCollectiveStallSource(s gpufleetv1.SignalSource) bool {
	switch s {
	case gpufleetv1.SignalSource_SIGNAL_SOURCE_SCHEDULER,
		gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM,
		gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS,
		gpufleetv1.SignalSource_SIGNAL_SOURCE_PROC:
		return true
	default:
		return false
	}
}

// hasPrefix reports whether id starts with prefix (exact match or "prefix.*"),
// so distinct ids of the same kind ("collective.stall.scheduler",
// "collective.stall.dcgm") all match without pulling in unrelated ids.
func hasPrefix(id, prefix string) bool {
	if id == prefix {
		return true
	}
	return len(id) > len(prefix) && id[:len(prefix)] == prefix && id[len(prefix)] == '.'
}
