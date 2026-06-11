# rca — module roadmap (M1..M6)

`rca` is an OPEN shared library (Go lib + thin `cmd/rca` bin): the deterministic
≥2-signal gate + ABSTAIN reference + public playbooks. It is linked into the
`agent` daemon and **reused server-side** by the closed `controlplane` as the
floor of its deep RCA. No LLM narration ever lives here. This file mirrors the
module section of `../ARCHITECTURE.md` and `../ops/BOARD.md`; the orchestrator
owns sequencing.

Invariants that hold across every milestone:
- Deterministic: same window → same verdict (no model, no randomness, no
  `time.Now()` in output, sorted iteration).
- ABSTAIN-by-default: FIRE needs ≥2 **independent** corroborating signals.
- **Independence judged by `SignalSource`, never a declared field** (TASK-0018).
- FAULT confidence clamped ≥ 0.95. No Anthropic/LLM call in this repo.

---

## M0 — scaffold & contracts (done as scaffold)
**Deliver:** buildable `rca` lib + `cmd/rca` bin; green open CI (lint, arm64
`-race`, static cross builds, govulncheck, SBOM, gitleaks); hand-rolled types
mirroring `proto/` (tag + gen vendoring deferred to TASK-0017).
**Exit:** `go build`/`go test -race`/`go vet` green on amd64+arm64; bin reads a
window from stdin and prints a Verdict.

## M1 — ≥2-signal gate + XID79 + ABSTAIN (current focus)
**Deliver (TASK-0001, TASK-0018, with TASK-0021):**
- `Signature`/`Engine` gate API; first-FIRE-wins over a registry with 2–3 public
  playbook slots (not empty stubs).
- **XID79** public playbook: FIRES only with ≥2 corroborating signals from
  **different sources** (dmesg/NVRM Xid 79 + ECC DBE delta / DCGM device-lost);
  either alone ABSTAINs.
- Independence bound to `SignalSource` (or `independence_class ⊆ source`
  validated); producer-declared independence never trusted.
- FAULT `confidence` clamp ≥ 0.95.
**Exit:**
- Table-driven tests: 1 signal → ABSTAIN; 2 different-source → FIRE citing exactly
  the corroborating evidence; **two same-source signals tagged as different
  classes → ABSTAIN** (forged-independence test); unrelated signals → ABSTAIN
  (FP=0).
- `evidence_grounding == 1.0` (cited signals are real inputs).
- Same gate compiles into `agent` and is importable by `controlplane`.

## M2 — broaden public playbooks + vendored shared schema
**Deliver:** 2–3 more public signatures (NCCL collective timeout, PCIe/device-lost
corroboration, thermal/throttle), each ≥2-source. Consume the **vendored,
versioned** `GateClassSchema` (class enum + signature IDs + source taxonomy) from
`proto/` once TASK-0017/0021 land, replacing hand-rolled constants.
**Exit:** each new signature has the full ABSTAIN/FIRE/forged/negative test set;
class IDs match `proto` `GateClassSchema`; FP=0 across the suite.

## M3 — conformance fixtures + adversarial independence
**Deliver:** a shared golden-window fixture set the closed side can reuse to prove
open and closed gates agree on the same inputs; fuzz/property tests that hammer
forged-independence and source-collision cases; explicit confidence-clamp
coverage.
**Exit:** fixtures runnable in both `rca` CI and (reused) `controlplane` CI;
no fixture both FIREs in `rca` and would be rejected server-side (or vice versa);
fuzzer finds no single-source FIRE.

## M4 — toward the public subset of 8–12 deterministic classes
**Deliver:** extend the public signature catalog toward the public-knowledge
subset of the project's 8–12 deterministic fault classes (the closed corpus stays
server-side); golden-window regression per class.
**Exit:** catalog documented with per-class required sources; every class passes
the standard 4-case test matrix; precision-oriented audit shows FP=0 on negatives.

## M5 — ABI stabilization + determinism/perf guarantees
**Deliver:** stabilize the open gate API surface (Signature/Engine/Verdict);
document determinism + sorted-output guarantees; align signatures with the public
`bench` question set (no answer key consumed here).
**Exit:** buf-style breaking checks clean against `proto` shared schema; no
non-deterministic path in `go test -race -count=N`; bench question cases run
against the open gate without leaking any answer key.

## M6 — v1 freeze
**Deliver:** v1 open gate freeze; semver stability promise; only additive,
public-knowledge playbook additions thereafter.
**Exit:** tagged `v1.0.0`; CI enforces no breaking changes to the gate ABI; new
work is additive signatures + tests only.
