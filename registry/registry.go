// Package registry wires the public deterministic signatures into a gate
// Engine. It is the single place that knows the concrete playbook set, kept
// separate from the engine core so playbooks depend only on the leaf `rca`
// package (no import cycle) and so the catalog grows additively.
//
// Registration is ORDERED and deterministic: the engine evaluates signatures in
// this order and the first to FIRE wins. New PUBLIC signatures append here.
package registry

import (
	rca "github.com/rocker-zhang/gpufleet-rca"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/nccltimeout"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/xid79"
)

// Default returns the ordered set of registered public signatures.
//
// Slot 1: XID79 (GPU fell off the bus) — registered.
// Slot 2: NCCL collective timeout — registered.
// Slot 3: reserved for the next PUBLIC signature (e.g. PCIe/link-degraded),
// which MUST also require >=2 independent sources. It is intentionally left
// unregistered here, NOT stubbed with an empty no-op signature: an empty
// signature that never fires is dead weight, and one that fires on <2 sources
// would violate the gate. When the next public playbook lands (its own package
// under playbooks/, with the full ABSTAIN/FIRE/forged/negative test set),
// append it to this slice.
func Default() []rca.Signature {
	return []rca.Signature{
		xid79.New(),       // GATE_SIGNATURE_XID79_FALLEN_OFF_BUS
		nccltimeout.New(), // GATE_SIGNATURE_NCCL_TIMEOUT
		// slot 3: next public signature — append here (e.g. LINK_DEGRADED).
	}
}

// NewDefaultEngine builds the gate Engine from the registered public
// signatures, in registration order.
func NewDefaultEngine() *rca.Engine {
	return rca.NewEngine(Default()...)
}
