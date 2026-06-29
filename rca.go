// Package rca is the OPEN deterministic signature engine for gpufleet and the
// reference implementation of the project's two core invariants:
//
//  1. Determinism-first (RULES §B): a fault class is decided purely by matching
//     normalized evidence — NO model, NO randomness, NO closed logic, no
//     time.Now() in the verdict body, sorted iteration only.
//  2. ABSTAIN-by-default (RULES §B): a signature may FIRE only with >= 2
//     INDEPENDENT corroborating signals, where independence is judged on the
//     signal's SignalSource (different sources), never on a producer-declared
//     field. With fewer independent sources it ABSTAINs — a one-vote veto.
//
// LLM narration is NOT here — that lives server-side in the closed control
// plane (RULES §B). This open library only adjudicates (or abstains) and emits
// a gpufleet.v1 Verdict with the cited signals. cost_impact is left unset and
// narration empty here.
//
// The engine consumes the REAL gpufleet.v1 contract types (the vendored proto
// gen module), not a hand-rolled mirror: input is an EvidencePack, output is a
// Verdict / FaultClass / CitedSignal / GateSignature.
package rca

import (
	"sort"

	gpufleetv1 "github.com/rocker-zhang/gpufleet-proto/gen/go/gpufleet/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FaultConfidenceFloor is the minimum confidence on any FIRED (non-ABSTAIN)
// verdict. It prevents a confident-FAULT-with-confidence-0 (RULES / module
// CLAUDE.md §3: "FAULT confidence clamp >= 0.95").
const FaultConfidenceFloor = 0.95

// abstainConfidence is the confidence attached to an ABSTAIN: for ABSTAIN,
// confidence is the confidence in abstaining. The gate abstains with full
// confidence — the >=2-independent-signal gate is a hard, deterministic rule.
const abstainConfidence = 1.0

// Evidence is one normalized, citable observation drawn from the EvidencePack
// timeline. It is the unit the gate matches against. It carries ONLY observed
// facts (a stable signal id, its provenance source, and a non-adjudicating
// label) — never thresholds, prompts, or heuristics (RULES §E).
//
// Source is the load-bearing independence axis (module CLAUDE.md §3 / TASK-0018):
// two pieces of Evidence corroborate only if their Source differs.
type Evidence struct {
	// SignalID must resolve back to a real EvidencePack TimelineEntry.signal_id
	// (evidence_grounding == 1.0): the gate never cites a signal not in input.
	SignalID string
	// Source is the provenance/independence class (kernel dmesg-xid vs DCGM vs
	// NCCL ...). Independence is derived from THIS, never a declared field.
	Source gpufleetv1.SignalSource
	// Device is the device_uuid the signal is attributed to ("" when the producer
	// did not attribute one). Independence is still judged on Source (unchanged);
	// Device only co-locates corroborating legs via SameDevice.
	Device string
	// Ts is the observation timestamp (denormalized onto the cited signal).
	Ts *timestamppb.Timestamp
	// Label is a short, non-adjudicating human note carried into the citation.
	Label string
}

// Signature is a deterministic fault matcher. Implementations MUST require at
// least two INDEPENDENT corroborating signals (distinct SignalSource) before
// firing; otherwise they MUST abstain. A signature never decides a class from a
// single source and never consults a model.
type Signature interface {
	// FaultClass is the gpufleet.v1 fault class this signature adjudicates.
	FaultClass() gpufleetv1.FaultClass
	// GateSignature is the versioned, shared signature id (audit metadata only;
	// NOT an input to the class decision).
	GateSignature() gpufleetv1.GateSignature
	// Match inspects the window's evidence and returns the corroborating
	// citations if the signature's >=2-independent-source pattern is satisfied,
	// or (nil, false) to abstain. The returned slice MUST reference only real
	// input evidence.
	Match(window []Evidence) (cited []Evidence, fired bool)
}

// Engine runs a registry of signatures over a window. The first signature to
// fire wins (deterministic registration order); if none fire, the engine
// ABSTAINs. Construct via the registry (NewDefaultEngine / NewEngine).
type Engine struct {
	sigs []Signature
}

// NewEngine builds an engine from an explicit, ordered list of signatures.
func NewEngine(sigs ...Signature) *Engine { return &Engine{sigs: sigs} }

// Signatures returns the registered signatures in evaluation order.
func (e *Engine) Signatures() []Signature { return e.sigs }

// Evaluate runs the gate over an EvidencePack and returns a gpufleet.v1 Verdict.
//
// It extracts the citable timeline signals, then offers them to each signature
// in order. The first signature whose >=2-independent-source pattern matches
// FIRES; the verdict cites exactly the corroborating evidence (and only real
// input signals). If no signature fires — including the case where a pattern's
// signals are all the SAME source (forged / non-independent) — the verdict is
// ABSTAIN.
func (e *Engine) Evaluate(pack *gpufleetv1.EvidencePack) *gpufleetv1.Verdict {
	window := EvidenceFromPack(pack)
	contractVersion := ""
	if pack != nil {
		contractVersion = pack.GetContractVersion()
	}

	// Grounding set: the signal IDs actually present in the window. A signature
	// may only cite evidence that exists here — the engine drops any other leg
	// before firing, so a buggy/future signature can never ground a verdict on a
	// fabricated signal (evidence_grounding is enforced HERE in the engine, not
	// only asserted in tests).
	present := make(map[string]bool, len(window))
	for _, w := range window {
		present[w.SignalID] = true
	}

	for _, s := range e.sigs {
		cited, fired := s.Match(window)
		if !fired {
			continue
		}
		// Defense in depth, in two load-bearing steps that hold even if a future
		// signature is buggy:
		//   1. GROUND: drop any cited leg not present in the window (no fabricated
		//      or hallucinated evidence may be cited).
		//   2. INDEPENDENCE: re-enforce the >=2-INDEPENDENT-SOURCE gate on the
		//      grounded survivors. Same-source duplicates can never satisfy it.
		citedSignals, distinctSources := independentCitations(groundCited(cited, present))
		if distinctSources < 2 {
			continue // one-vote veto: not enough independent, grounded corroboration.
		}
		return &gpufleetv1.Verdict{
			ContractVersion: contractVersion,
			FaultClass:      s.FaultClass(),
			Confidence:      FaultConfidenceFloor,
			CitedSignals:    citedSignals,
			Signature:       s.GateSignature(),
			PlaybookId:      s.GateSignature().String(),
			// narration empty, cost_impact unset, produced_at unset: open gate.
		}
	}

	return &gpufleetv1.Verdict{
		ContractVersion: contractVersion,
		FaultClass:      gpufleetv1.FaultClass_FAULT_CLASS_ABSTAIN,
		Confidence:      abstainConfidence,
		Signature:       gpufleetv1.GateSignature_GATE_SIGNATURE_UNSPECIFIED,
		// ABSTAIN cites nothing it adjudicated on and fires no class.
	}
}

// EvidenceFromPack flattens an EvidencePack's canonical timeline into the
// citable Evidence the gate matches against. The timeline is the canonical
// ordered sequence (signal.proto), each entry carrying a stable signal_id and
// its provenance Source. Entries without a signal_id are skipped (uncitable):
// the gate can only ground on real, referenceable evidence.
func EvidenceFromPack(pack *gpufleetv1.EvidencePack) []Evidence {
	if pack == nil {
		return nil
	}
	out := make([]Evidence, 0, len(pack.GetTimeline()))
	for _, te := range pack.GetTimeline() {
		id := te.GetSignalId()
		if id == "" {
			continue
		}
		// Skip entries whose provenance is UNSPECIFIED: an unattributable source
		// cannot count toward INDEPENDENT corroboration, so admitting it would let
		// {UNSPECIFIED, DCGM} masquerade as two independent sources. Same spirit as
		// skipping an empty signal_id — uncitable provenance.
		if te.GetSource() == gpufleetv1.SignalSource_SIGNAL_SOURCE_UNSPECIFIED {
			continue
		}
		out = append(out, Evidence{
			SignalID: id,
			Source:   te.GetSource(),
			Device:   te.GetDeviceUuid(),
			Ts:       te.GetTs(),
			Label:    te.GetLabel(),
		})
	}
	return out
}

// HasIDPrefix reports whether a signal id equals prefix or has it as a
// dot-bounded prefix (e.g. "device.lost.dcgm" matches prefix "device.lost" but
// "device.lostx" does not). This is the single definition of the load-bearing
// signal-id prefix match the playbooks use to group related ids; keeping it here
// means a bug in the dot-boundary check is fixed in exactly one place.
func HasIDPrefix(id, prefix string) bool {
	if id == prefix {
		return true
	}
	return len(id) > len(prefix) && id[:len(prefix)] == prefix && id[len(prefix)] == '.'
}

// SameDevice reports whether two evidences are consistent with the same device.
// It returns false ONLY when both carry a non-empty device id and the ids
// differ; if either device id is absent, device attribution is unavailable and
// the legs are treated as co-located (historical behavior), so telemetry that
// predates device tagging still corroborates.
func SameDevice(a, b Evidence) bool {
	if a.Device == "" || b.Device == "" {
		return true
	}
	return a.Device == b.Device
}

// FirstSameDevicePair returns the first (a in as, b in bs) pair that satisfies
// SameDevice, scanning as in order then bs in order (deterministic). The two
// candidate slices come from the two distinct legs, so any returned pair is
// already from two independent sources. Returns (nil,nil,false) if no pair is
// co-located (every cross pair has two differing non-empty device ids).
func FirstSameDevicePair(as, bs []*Evidence) (a, b *Evidence, ok bool) {
	for _, a := range as {
		for _, b := range bs {
			if SameDevice(*a, *b) {
				return a, b, true
			}
		}
	}
	return nil, nil, false
}

// groundCited drops any cited Evidence whose SignalID is not present in the
// window — the engine never grounds a verdict on evidence that does not exist
// (no fabricated/hallucinated citations). Returns a fresh slice.
func groundCited(ev []Evidence, present map[string]bool) []Evidence {
	out := make([]Evidence, 0, len(ev))
	for _, e := range ev {
		if present[e.SignalID] {
			out = append(out, e)
		}
	}
	return out
}

// independentCitations converts corroborating evidence into sorted
// gpufleet.v1 CitedSignals and reports how many DISTINCT SignalSources are
// present. Independence is derived purely from Source (TASK-0018): the count is
// of distinct sources, NOT of signals, so two readings of the same source count
// once. Output ordering is deterministic (by signal_id) for stable verdicts.
func independentCitations(ev []Evidence) (cited []*gpufleetv1.CitedSignal, distinctSources int) {
	sorted := make([]Evidence, len(ev))
	copy(sorted, ev)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SignalID < sorted[j].SignalID })

	sources := map[gpufleetv1.SignalSource]bool{}
	cited = make([]*gpufleetv1.CitedSignal, 0, len(sorted))
	for _, e := range sorted {
		sources[e.Source] = true
		cited = append(cited, &gpufleetv1.CitedSignal{
			SignalId: e.SignalID,
			Source:   e.Source,
			Ts:       e.Ts,
			Note:     e.Label,
		})
	}
	return cited, len(sources)
}
