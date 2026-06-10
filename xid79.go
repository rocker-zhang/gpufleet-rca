package rca

// XID79 is a public reference playbook/signature for NVRM Xid 79
// ("GPU has fallen off the bus"). It is the canonical demonstration of the
// >=2-signal gate: it FIRES only when BOTH a dmesg Xid 79 event AND an ECC
// double-bit-error delta are present in the same window. Either signal alone
// ABSTAINs — a single signal never adjudicates.
//
// These two signals are independent: one comes from the kernel NVRM log, the
// other from the device ECC counters. Corroboration across independent sources
// is what the gate requires.
type XID79 struct{}

const (
	signalDmesgXid79 = "dmesg.xid79"
	signalECCDBE     = "ecc.dbe.delta"
)

// Class returns the stable fault-class identifier.
func (XID79) Class() string { return "xid79_gpu_fell_off_bus" }

// Evaluate fires only with >= 2 corroborating signals, else abstains.
func (s XID79) Evaluate(w Window) Verdict {
	cited, ok := requireAtLeastTwo(w, signalDmesgXid79, signalECCDBE)
	if !ok {
		return Verdict{DeviceUUID: w.DeviceUUID, Outcome: Abstain}
	}
	return Verdict{
		DeviceUUID:   w.DeviceUUID,
		Outcome:      Fire,
		FaultClass:   s.Class(),
		CitedSignals: cited,
	}
}
