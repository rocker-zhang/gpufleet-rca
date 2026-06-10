// Package rca is the deterministic signature engine for gpufleet and the
// reference implementation of the project's two core invariants:
//
//  1. Determinism-first: a fault class is decided purely by matching signals,
//     with no model and no randomness.
//  2. ABSTAIN-by-default: a signature may only FIRE when it has >= 2 independent
//     corroborating signals. With fewer, it ABSTAINs. This is a one-vote veto:
//     a single signal is never enough.
//
// LLM narration is NOT here — that lives server-side in the closed control
// plane. This open library only adjudicates (or abstains) and emits a
// Verdict-like result with the cited signals.
package rca

import "sort"

// Signal is one observed, independent piece of evidence in a window.
// Name is the stable signal identifier (e.g. "dmesg.xid79", "ecc.dbe.delta").
type Signal struct {
	Name   string
	Detail string
}

// Window is the normalized evidence for a single device/job over a period.
// It is the input the engine matches against — never prompts or heuristics.
type Window struct {
	DeviceUUID string
	Signals    []Signal
}

// Has reports whether a signal with the given name is present.
func (w Window) Has(name string) bool {
	for _, s := range w.Signals {
		if s.Name == name {
			return true
		}
	}
	return false
}

// Outcome is the verdict disposition.
type Outcome int

const (
	// Abstain means the >=2-signal gate was not met; the engine declines to
	// adjudicate. This is the default and safe outcome.
	Abstain Outcome = iota
	// Fire means a signature matched with >=2 corroborating signals.
	Fire
)

func (o Outcome) String() string {
	if o == Fire {
		return "FIRE"
	}
	return "ABSTAIN"
}

// Verdict is the deterministic result for one window. CitedSignals lists the
// independent signals that justified a FIRE (empty on ABSTAIN). It carries no
// narration — narration is added server-side by the closed control plane.
type Verdict struct {
	DeviceUUID   string
	Outcome      Outcome
	FaultClass   string   // set only on FIRE
	CitedSignals []string // sorted; the >=2 signals that corroborated
}

// Signature is a deterministic fault matcher. Implementations MUST require at
// least two independent corroborating signals before firing.
type Signature interface {
	// Class is the stable fault-class identifier this signature decides.
	Class() string
	// Evaluate inspects the window and returns a Verdict. It must ABSTAIN
	// unless >= 2 independent signals corroborate.
	Evaluate(w Window) Verdict
}

// Engine runs a set of signatures over a window. The first signature to FIRE
// wins (signatures are evaluated in registration order); if none fire, the
// engine ABSTAINs.
type Engine struct {
	sigs []Signature
}

// NewEngine builds an engine from the given signatures.
func NewEngine(sigs ...Signature) *Engine { return &Engine{sigs: sigs} }

// Evaluate runs all signatures and returns the first FIRE, else an ABSTAIN.
func (e *Engine) Evaluate(w Window) Verdict {
	for _, s := range e.sigs {
		v := s.Evaluate(w)
		if v.Outcome == Fire {
			return v
		}
	}
	return Verdict{DeviceUUID: w.DeviceUUID, Outcome: Abstain}
}

// requireAtLeastTwo enforces the gate: it returns a sorted, de-duplicated list
// of the present signals and whether >= 2 of them were found.
func requireAtLeastTwo(w Window, names ...string) ([]string, bool) {
	seen := map[string]bool{}
	for _, n := range names {
		if w.Has(n) {
			seen[n] = true
		}
	}
	cited := make([]string, 0, len(seen))
	for n := range seen {
		cited = append(cited, n)
	}
	sort.Strings(cited)
	return cited, len(cited) >= 2
}
