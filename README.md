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
adjudicates or abstains, and emits a `Verdict` with the cited signals.

## The XID79 reference signature

`XID79` (NVRM Xid 79, "GPU fell off the bus") fires only when **both** a dmesg
Xid 79 event **and** an ECC double-bit-error delta appear in the same window —
two independent sources. Either alone abstains. The table-driven test proves:

- 1 signal → ABSTAIN
- 2 signals → FIRE (cites both signals)
- unrelated signals → ABSTAIN (false-positive = 0)

## Use

```sh
echo '{"device_uuid":"GPU-1","signals":[{"name":"dmesg.xid79"},{"name":"ecc.dbe.delta"}]}' \
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
