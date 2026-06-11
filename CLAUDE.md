# rca — module brief (CLAUDE.md)

## 1. 身份
- **Class:** OPEN (Apache-2.0).
- **Language:** Go.
- **Kind:** library + thin bin (`cmd/rca`). NOT a service, NOT a daemon.
- **One-line purpose:** the deterministic signature engine — the ≥2-signal gate
  + ABSTAIN reference implementation + 2–3 basic **public** playbooks (XID79
  first). It adjudicates a normalized signal window into `{FIRE class | ABSTAIN}`
  with cited signals. No LLM narration.
- **Role:** **shared library (共享库)** — linked into the `agent` daemon at
  compile time and **reused server-side** by the closed `controlplane`. It is
  NOT a pipeline tier and NOT on a runtime hop (D-0008).

## 2. 在系统里的位置
- **Consumes (read-only contracts from `proto/`):**
  - `SignalSchema@0.1.0` — the normalized signal window it matches against.
  - `GateClassSchema` — the **shared** fault-class enum + signature IDs, versioned
    in `proto/` because both open `rca` and closed `controlplane` depend on it
    (TASK-0021). The signal `source` taxonomy used for independence lives here.
  - `Verdict@0.1.0` — the output shape (`fault_class|ABSTAIN`, confidence,
    `cited_signals[]`, narration[empty here], cost_impact[unset here]).
- **Produces:** a deterministic `Verdict` (FIRE class or ABSTAIN) + the cited
  independent signals. No narration, no cost math, no I/O side effects.
- **Edges:**
  - `agent` links `rca` to run the gate locally and produce a **local Verdict**
    for the evidence pack / local read-only API.
  - `controlplane` **reuses the same open gate** as the floor of its deep RCA,
    then re-derives independence server-side (see §3) and adds closed playbooks +
    LLM narration on top. The deep playbooks and corpus never ship back into
    `rca`.
  - `semantics` is a sibling shared lib (device→job, MFU/$cost); `rca` does not
    import it — cost attribution is not a fault class.
- See `../ARCHITECTURE.md` (esp. §2 boundary spec and the `GateClassSchema` row).

## 3. 继承的红线
Inherits all of `../RULES.md`. Module-specific hard lines:

- **Deterministic gate, ≥2 independent signals or ABSTAIN.** A signature FIRES
  only with ≥2 corroborating signals; otherwise ABSTAIN. Never add a single-signal
  fire path. The gate is a one-vote veto — do not weaken it.
- **Independence is judged by SOURCE, not by a declared field (TASK-0018,
  load-bearing trust).** Two signals count as independent only if they come from
  **different `SignalSource`s** (e.g. kernel NVRM log vs device ECC counters vs
  DCGM/PCIe vs NCCL). Never trust a producer-supplied `independence_class`: the
  open `agent` is an untrusted boundary and could tag two same-source signals
  (both `kernel_log`) as different classes to forge a 2-signal fire. The closed
  `controlplane` **re-derives** independence from `source` and never trusts the
  declared field; `rca`'s open gate must enforce the same rule (independence
  bound to `source`, or `independence_class ⊆ source` validated against a legal
  mapping). Two same-source signals → ABSTAIN regardless of class labels. This is
  the single most load-bearing trust property in the product.
- **No LLM here.** Narration is server-side in the closed control plane. Never add
  any Claude/Anthropic API call to this repo. The gate's class is never decided by
  a model.
- **FAULT confidence clamp ≥ 0.95** (prevents confident-FAULT-with-confidence-0).
- **Public playbooks only.** Use only public failure-class knowledge (public XID
  codes, public NCCL/PCIe semantics). No externally-sourced or secret error-code
  semantics in any signature or playbook (RULES §F).
- **`proto/` is READ-ONLY.** A needed contract change = a contract-change-proposal
  blocker for the orchestrator, never an edit here.
- **Edits confined to `rca/`.** Need `semantics`/`agent`/`cli`/`controlplane`/
  proto? ABSTAIN and file a blocker.

## 4. 当前任务 & 里程碑焦点
See `../ops/BOARD.md`. Cards touching this module:
- **TASK-0001** — XID79 public playbook + ≥2-signal gate skeleton + ABSTAIN
  (registry for 2–3 public playbooks; `cmd/rca` reads a window, prints Verdict).
- **TASK-0018** (P0, shared with controlplane) — independence judged by `source`,
  not declared field; forged-independence test must ABSTAIN; FAULT confidence
  clamp ≥ 0.95. This is the load-bearing trust fix the module reflects.
- **TASK-0021** (P0, with TASK-0017) — proto versions the shared
  `GateClassSchema` (class enum + signature IDs + source taxonomy) that this gate
  and the controlplane share.

**M1 focus:** deliver the open gate + XID79 as the deterministic floor reused on
both sides of the boundary, with independence enforced by source.

## 5. 构建与测试
```sh
source ../.envrc            # project-local toolchain; do FIRST every session
go build ./...              # cross amd64 + arm64 static, CGO_ENABLED=0, mock-NVML default
go test -race ./...         # green, with real ABSTAIN + FIRE assertions
go vet ./...
echo '{"device_uuid":"GPU-1","signals":[{"name":"dmesg.xid79"},{"name":"ecc.dbe.delta"}]}' \
  | go run ./cmd/rca        # reads a window, prints a Verdict
```
**CI (one line):** lint + arm64 `-race` test matrix + static cross builds
(amd64+arm64) + govulncheck + syft SBOM + gitleaks.

## 6. session 工作规则
- Edits confined to this repo (`rca/`); `proto/` is read-only.
- Need a contract change or another module → **ABSTAIN and file a short blocker**;
  no cross-repo workarounds.
- Every new signature ships a table-driven test proving: 1 signal → ABSTAIN,
  2 **different-source** signals → FIRE, same-source-forged → ABSTAIN, and a
  negative case with FP=0. `cited_signals` must reference real inputs
  (evidence_grounding == 1.0).
- Provenance: personal hardware/time only; no externally-sourced code, data, or
  secret error-code semantics.

## 7. 模块路线图
Mirror of `ROADMAP.md` (one line per milestone touching this module):
- **M0** — buildable lib + bin scaffold, green CI, hand-rolled types mirroring proto.
- **M1** — ≥2-signal gate + XID79 public playbook + ABSTAIN; independence by source (TASK-0018); reused by agent + controlplane.
- **M2** — 2–3 more public playbooks (NCCL timeout, PCIe/device-lost, thermal/throttle); registry hardened; consume vendored `GateClassSchema`.
- **M3** — gate conformance fixtures shared open/closed; fuzz forged-independence; confidence clamp coverage.
- **M4** — broaden public signature catalog toward 8–12 deterministic classes (public-knowledge subset); golden-window tests.
- **M5** — stabilize the open gate ABI; perf/determinism guarantees documented; bench-aligned signature audit.
- **M6** — v1 open gate freeze; semver stability; long-tail public playbook additions only.
