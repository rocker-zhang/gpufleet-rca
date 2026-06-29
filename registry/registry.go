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
	"github.com/rocker-zhang/gpufleet-rca/playbooks/oomkill"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/powerthrottle"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/thermalthrottle"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/xid79"
	"github.com/rocker-zhang/gpufleet-rca/playbooks/xidfatal"
)

// Default returns the ordered set of registered public signatures.
//
// Slot 1: XID79 (GPU fell off the bus) — registered.
// Slot 2: NCCL collective timeout — registered.
// Slot 3: ECC uncorrectable (double-bit memory ECC) — registered.
// Slot 4: LINK_DEGRADED (NVLink/PCIe link degraded) — registered.
// Slot 5: THERMAL_THROTTLE (hardware thermal clock cap) — registered.
// Slot 6: POWER_THROTTLE (hardware power-brake clock cap) — registered.
// Slot 7: OOM_KILL (kernel OOM killer reaped a job process) — registered.
// Slot 8: XID_FATAL (fatal Xid + DCGM health failure) — registered.
//
// The public fault classes are mutually exclusive by their leg patterns
// (distinct id-prefix + source pairs), so registration order does not change
// verdicts; it is kept STABLE and additive. New PUBLIC signatures — each of
// which MUST also require >=2 independent sources and ship the full
// ABSTAIN/FIRE/forged/negative test set — append after the last slot (do NOT
// reorder existing slots).
func Default() []rca.Signature {
	return []rca.Signature{
		xid79.New(),            // slot 1: GATE_SIGNATURE_XID79_FALLEN_OFF_BUS
		nccltimeout.New(),      // slot 2: GATE_SIGNATURE_NCCL_TIMEOUT
		eccuncorrectable.New(), // slot 3: GATE_SIGNATURE_ECC_UNCORRECTABLE
		linkdegraded.New(),     // slot 4: GATE_SIGNATURE_LINK_DEGRADED
		thermalthrottle.New(),  // slot 5: GATE_SIGNATURE_THERMAL_THROTTLE
		powerthrottle.New(),    // slot 6: GATE_SIGNATURE_POWER_THROTTLE
		oomkill.New(),          // slot 7: GATE_SIGNATURE_OOM_KILL
		xidfatal.New(),         // slot 8: GATE_SIGNATURE_XID_FATAL
		// slot 9: next public signature — append here.
	}
}

// NewDefaultEngine builds the gate Engine from the registered public
// signatures, in registration order.
func NewDefaultEngine() *rca.Engine {
	return rca.NewEngine(Default()...)
}
