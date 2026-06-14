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
	"github.com/rocker-zhang/gpufleet-rca/playbooks/eccuncorrectable"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/linkdegraded"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/nccltimeout"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/xid79"
)

// Default returns the ordered set of registered public signatures.
//
// Slot 1: XID79 (GPU fell off the bus) — registered.
// Slot 2: NCCL collective timeout — registered.
// Slot 3: ECC uncorrectable (double-bit memory ECC) — registered.
// Slot 4: LINK_DEGRADED (NVLink/PCIe link degraded) — registered.
//
// The four public fault classes are mutually exclusive by their leg patterns
// (distinct id-prefix + source pairs), so registration order does not change
// verdicts; it is kept STABLE and additive. New PUBLIC signatures — each of
// which MUST also require >=2 independent sources and ship the full
// ABSTAIN/FIRE/forged/negative test set — append after slot 4 (do NOT reorder
// existing slots).
func Default() []rca.Signature {
	return []rca.Signature{
		xid79.New(),            // slot 1: GATE_SIGNATURE_XID79_FALLEN_OFF_BUS
		nccltimeout.New(),      // slot 2: GATE_SIGNATURE_NCCL_TIMEOUT
		eccuncorrectable.New(), // slot 3: GATE_SIGNATURE_ECC_UNCORRECTABLE
		linkdegraded.New(),     // slot 4: GATE_SIGNATURE_LINK_DEGRADED
		// slot 5: next public signature — append here.
	}
}

// NewDefaultEngine builds the gate Engine from the registered public
// signatures, in registration order.
func NewDefaultEngine() *rca.Engine {
	return rca.NewEngine(Default()...)
}
