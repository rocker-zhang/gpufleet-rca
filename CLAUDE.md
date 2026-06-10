# CLAUDE.md — gpufleet-rca (module session rules)

You are a Claude session **scoped to this repo only** (`gpufleet-rca`). This is
an OPEN module (Apache-2.0). Your edits are **confined to this repo**.

## What this module is

The deterministic signature engine + the >=2-signal gate + ABSTAIN reference
implementation + 2-3 basic public playbooks (e.g. XID79). LLM narration is NOT
here.

## Hard boundaries (do not cross)

- **Determinism-first.** No randomness, no model, no `time.Now()` affecting
  output, no map iteration without sorting. Same window → same verdict.
- **ABSTAIN-by-default / >=2-signal gate is load-bearing.** A signature may FIRE
  only with >= 2 independent corroborating signals; otherwise it ABSTAINs. Never
  add a single-signal fire path. The gate is a one-vote veto — do not weaken it.
- **No LLM here.** Narration is server-side in the closed control plane. Do NOT
  add any Claude/Anthropic API call to this repo.
- **Edits confined here.** Need a change in `semantics`, `agent`, `cli`, or the
  control plane? ABSTAIN and file a blocker. Do not reach across.
- **`proto/` is READ-ONLY.** Read vendored contracts; never edit them. A needed
  contract change = a *contract change proposal* blocker for the orchestrator.
- **No externally-sourced content.** Never encode proprietary error-code
  semantics into a signature or playbook.

## If you are blocked

File a short blocker and stop. No cross-repo workarounds.

## Definition of done

`go test -race ./...` and `go vet ./...` pass; any new signature has a
table-driven test proving (1 signal → ABSTAIN), (2 signals → FIRE), and a
negative case with FP=0.
