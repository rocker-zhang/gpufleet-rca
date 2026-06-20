// Package eccuncorrectable is the public reference playbook for an
// UNCORRECTABLE (double-bit) GPU memory ECC error.
//
// PUBLIC SEMANTICS ONLY (RULES §F): an uncorrectable / double-bit ECC error
// (DBE) is a publicly documented GPU memory fault — the ECC machinery detected
// a multi-bit error it cannot correct, and NVIDIA surfaces it both as a DCGM
// uncorrectable ECC counter and as a kernel/dmesg NVRM Xid line on one of the
// public ECC XIDs (48/63/64/94/95). This playbook embeds ONLY that public
// failure-class knowledge — no externally-sourced or secret error-code meanings,
// no thresholds beyond the public corroboration pattern.
//
// It is the third demonstration of the >=2-INDEPENDENT-signal gate: a DCGM ECC
// counter delta is, on its own, one source reporting on itself, so it can never
// FIRE alone. The fault FIRES only when that DCGM counter is corroborated by an
// INDEPENDENT kernel/dmesg ECC Xid line from a DIFFERENT source (DMESG_XID).
// Either leg alone — or two readings of the SAME source (e.g. a second DCGM
// counter) — ABSTAINs. Independence is judged on SignalSource, never a
// producer-declared field (TASK-0018).
//
// Signal-id prefix conventions (the open agent's normalized timeline will emit
// matching ids in a later agent-integration card; documented here so that card
// matches):
//
//   - "ecc.dbe"       — the DCGM uncorrectable/double-bit ECC counter delta.
//     Source MUST be SIGNAL_SOURCE_DCGM. A window may carry several distinct ids
//     of this kind (e.g. one per memory location: "ecc.dbe.fb",
//     "ecc.dbe.l2"); they are prefix-matched.
//   - "dmesg.xid.ecc" — an INDEPENDENT kernel/dmesg ECC Xid line (public ECC
//     XIDs 48/63/64/94/95) drawn from SIGNAL_SOURCE_DMESG_XID, e.g.
//     "dmesg.xid.ecc.94". DCGM is deliberately excluded from this leg — it is
//     the same source as the counter leg and would not be independent.
package eccuncorrectable

import (
	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	rca "github.com/rocker-zhang/gpufleet-rca"
)

// Signal-id prefixes used by the open agent's normalized timeline. They are
// matched as PREFIXES so a window may carry several distinct ids of a kind
// (e.g. one DCGM ECC counter per memory location) while remaining publicly
// defined, non-proprietary conventions.
const (
	// eccDBEPrefix marks the uncorrectable/double-bit ECC counter delta. The
	// playbook additionally requires the source to be a metrics source (DCGM or
	// PROMETHEUS, see isECCCounterSource), so the id alone never forges a fire.
	eccDBEPrefix = "ecc.dbe"
	// eccXidPrefix marks an INDEPENDENT kernel/dmesg ECC Xid line (public ECC
	// XIDs 48/63/64/94/95) drawn from a non-DCGM source (DMESG_XID).
	eccXidPrefix = "dmesg.xid.ecc"
)

// Sig is the ECC-uncorrectable signature. It carries no state.
type Sig struct{}

// New returns the ECC-uncorrectable signature for registration.
func New() Sig { return Sig{} }

// FaultClass is the public adjudicated outcome for an uncorrectable ECC error.
func (Sig) FaultClass() gpufleetv1.FaultClass {
	return gpufleetv1.FaultClass_FAULT_CLASS_ECC_UNCORRECTABLE
}

// GateSignature is the shared, versioned signature id (audit metadata only).
func (Sig) GateSignature() gpufleetv1.GateSignature {
	return gpufleetv1.GateSignature_GATE_SIGNATURE_ECC_UNCORRECTABLE
}

// Match fires when the window contains (a) a DCGM uncorrectable/double-bit ECC
// counter whose source is SIGNAL_SOURCE_DCGM AND (b) a kernel/dmesg ECC Xid
// corroboration from a DIFFERENT source (DMESG_XID). The returned citations are
// exactly the matched evidence (real inputs only). It returns (nil, false) —
// ABSTAIN — otherwise, including when both legs share the same source (forged
// independence, e.g. DCGM+DCGM).
func (Sig) Match(window []rca.Evidence) (cited []rca.Evidence, fired bool) {
	var dbe *rca.Evidence
	var corroborator *rca.Evidence

	for i := range window {
		e := &window[i]
		switch {
		case rca.HasIDPrefix(e.SignalID, eccDBEPrefix) && isECCCounterSource(e.Source):
			if dbe == nil {
				dbe = e
			}
		case rca.HasIDPrefix(e.SignalID, eccXidPrefix) && isECCXidSource(e.Source):
			if corroborator == nil {
				corroborator = e
			}
		}
	}

	if dbe == nil || corroborator == nil {
		return nil, false
	}
	// Independence is on SOURCE: the corroborator must not share the DCGM leg's
	// source. (By construction the corroborator is DMESG_XID, but this makes the
	// load-bearing rule explicit and local to the signature.)
	if corroborator.Source == dbe.Source {
		return nil, false
	}
	return []rca.Evidence{*dbe, *corroborator}, true
}

// isECCCounterSource is the public set of metrics sources that can witness the
// uncorrectable (double-bit) ECC COUNTER delta: the local DCGM-exporter scrape
// (DCGM) or the existing Prometheus read of the SAME counter via an instant
// increase() query (PROMETHEUS) on a Prometheus-primary node. Both are genuine,
// independent metrics observations of the public DCGM_FI_DEV_ECC_DBE_VOL_TOTAL
// counter; either one, corroborated by the INDEPENDENT kernel dmesg ECC Xid,
// fires the fault. The corroborator leg (isECCXidSource) is deliberately disjoint
// from this set, so the two legs are always distinct sources (real independence).
func isECCCounterSource(s gpufleetv1.SignalSource) bool {
	switch s {
	case gpufleetv1.SignalSource_SIGNAL_SOURCE_DCGM,
		gpufleetv1.SignalSource_SIGNAL_SOURCE_PROMETHEUS:
		return true
	default:
		return false
	}
}

// isECCXidSource is the public set of sources that can witness an uncorrectable
// ECC error INDEPENDENTLY of the DCGM counter: the kernel/dmesg NVRM Xid log
// (DMESG_XID). SIGNAL_SOURCE_DCGM is deliberately excluded — a second DCGM
// counter is the same source as the counter leg and would not be independent.
func isECCXidSource(s gpufleetv1.SignalSource) bool {
	switch s {
	case gpufleetv1.SignalSource_SIGNAL_SOURCE_DMESG_XID:
		return true
	default:
		return false
	}
}
