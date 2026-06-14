# gpufleet-rca

Apache-2.0 · OPEN module · `github.com/rocker-zhang/gpufleet-rca`

The deterministic **signature engine** and the project's reference
implementation of two non-negotiable invariants:

1. **Determinism-first** — fault classes are decided by matching signals. No
   model, no randomness. Same window → same verdict, always.
2. **ABSTAIN-by-default (>=2-signal gate)** — a signature may only **FIRE** when
   at least **two independent corroborating signals** are present. With fewer,
   it **ABSTAINs**. One signal is never enough (one-vote veto).

LLM narration is **not** here. Deep RCA (gate → closed playbook selection → LLM
narration) runs server-side in the closed control plane. This open library only
adjudicates or abstains, and emits a `gpufleet.v1` `Verdict` with the cited
signals.

## Contract types

The gate consumes and produces the **real** vendored `gpufleet.v1` proto gen
types (`github.com/rocker-zhang/gpufleet-proto/gen/go`, pinned `proto/v0.1.0`
with a local `replace ../proto/gen/go`, matching `agent`/`cli`/`semantics`) —
not a hand-rolled mirror. Input is an `EvidencePack` (the agent's normalized
signal window; the gate matches its `timeline` of `signal_id` + `SignalSource`
entries). Output is a `Verdict` (`FaultClass | ABSTAIN`, `confidence`,
`cited_signals[]`, `signature`) with `narration` empty and `cost_impact` unset.

**Independence is judged on `SignalSource`** (TASK-0018), never a
producer-declared field: two readings of the same source do not corroborate.

## The XID79 reference signature

`xid79` (NVRM Xid 79, "GPU fell off the bus") fires only when **both** an
`xid=79` dmesg signal (`SIGNAL_SOURCE_DMESG_XID`) **and** an independent
"device lost / unreachable" signal from a **different** source (DCGM /
PCIe-Prometheus / nvidia-smi via PROC) appear in the same window. Either alone —
or two readings of the same source — abstains. Public XID semantics only. The
table-driven tests prove:

- 1 signal → ABSTAIN
- 2 different-source signals → FIRE (cites exactly those two signals; FAULT
  confidence clamped ≥ 0.95)
- 2 same-source signals (forged independence) → ABSTAIN
- unrelated signals → ABSTAIN (false-positive = 0)

## Use

`cmd/rca` reads an `EvidencePack` as protojson from a file argument or stdin
(e.g. the agent's `/signals` output) and prints the `Verdict` as protojson:

```sh
echo '{"contractVersion":"v1","timeline":[
  {"source":"SIGNAL_SOURCE_DMESG_XID","signalId":"dmesg.xid79"},
  {"source":"SIGNAL_SOURCE_DCGM","signalId":"device.lost.dcgm"}]}' \
  | go run ./cmd/rca
```

## Boundaries

- Determinism + ABSTAIN are load-bearing — do not weaken the gate.
- No LLM/Anthropic call in this repo.
- `proto/` is a read-only dependency; this repo never edits it.

## Develop

```sh
go test -race ./...
go vet ./...
```

CI: lint, arm64 test matrix with `-race`, static cross builds (amd64+arm64,
`CGO_ENABLED=0`), govulncheck, syft SBOM, gitleaks.
