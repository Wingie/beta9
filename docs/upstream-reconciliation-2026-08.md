# Upstream reconciliation triage — August 2026

Status: **analysis only. Nothing has been merged, rebased, or built.**
This document is the input to a merge the repository owner will perform.

## 1. Measured state

All figures re-measured on 2026-08-18 from a fresh clone with both remotes
configured. `upstream` had never been configured in this fork before today.

```
origin    git@github.com:Wingie/beta9.git       (our fork)
upstream  https://github.com/beam-cloud/beta9   (never previously synced)
```

| Ref | Commit | Date | Note |
|---|---|---|---|
| merge base | `299106d5` | 2025-11-26 | `Bumped SDK Version (#1542)` |
| `origin/main` | `689961d2` | 2026-07-26 | fork main |
| parent submodule pin | `43800352` | 2026-08-10 | head of open fork PR #5 |
| `upstream/main` | `e13aa4eb` | 2026-08-17 | `disk fixes (#1852)` |

Divergence, from `git rev-list --left-right --count`:

- pin vs `origin/main` — **0 behind, 6 ahead** (PR #5 plus the merged #4 line)
- `origin/main` vs `upstream/main` — **278 behind, 139 ahead**
- pin vs `upstream/main` — **278 behind, 145 ahead**

The fork has been diverging for ~9 months. Upstream's 278 commits span
2026-01-11 to 2026-08-17 and the project is actively developed.

## 2. The headline finding

**Upstream independently built the external-agent system this fork exists to
provide.** This is the single fact that determines the merge strategy.

The fork's founding purpose — quoting the owner — was that when delivering
beta9 for a client, "a part was not written or pushed from their side so I
wrote it." That part was the external/remote worker agent. Upstream has since
written it, and written substantially more of it than we did.

| | our fork (`43800352`) | `upstream/main` |
|---|---|---|
| `pkg/agent/` | 16 files | **42 files** |
| agent entrypoint | `cmd/b9agent/` | `cmd/agent/` (with `install`, `join`, `vast` subcommands) |
| transport | our own Tailscale/tsnet wiring in `pkg/gateway` + `pkg/network` | `pkg/agent/transport.go`, tsnet-based, `types.BackendRouteTransportTSNet` |
| pool controller | `pkg/scheduler/pool_external.go` | `pkg/scheduler/pool_agent.go` (+ `pool_external_test.go`, `private_pool_fallback.go`) |
| GPU reporting | `InferenceStatus.GPUType` bolted onto our HTTP keepalive | `joinRequest.GPU []string` / `GPUIDs` / `GPUCount` in the native join protocol |
| service install | none | `pkg/agent/service/` — systemd + launchd |
| extras | — | `preflight.go`, `telemetry.go`, `capacity.go`, `route_proxy.go`, `lock.go`, `vast/` (17 files, Vast.ai host integration) |

Upstream's agent work is **43 commits**, beginning at `4850eff8`
`feat: add reserved worker pools (#1595)` and including `#1646`, `#1647`,
`#1648`, `#1650`, `#1663 fix keepalives`, `#1661 feat: improve gpu offerings`,
`#1676 clean up gpu assignment logic`, `#1683 remote node network improvements`,
`#1738 feat: vast sidecar`.

Consequence: **a merge is the wrong operation.** Merging would land two
independent agent implementations into one Go package (`package agent`) with
two entrypoints. Git confirms this — `pkg/agent/agent.go`, `state.go` and
`state_test.go` come back as **add/add** conflicts, meaning both sides created
the same paths from nothing.

(Curiously, the two implementations share **zero** top-level identifier names,
so a naive "keep both" resolution might actually compile. That would be a trap,
not a success — it would ship two rival agents in one binary.)

## 3. Three-bucket triage

Grouped by feature cluster rather than by commit. 145 commits; the great
majority are iteration on four clusters.

### SUPERSEDED — upstream now does this; drop ours

| Cluster | Our commits (representative) | Superseded by | Evidence |
|---|---|---|---|
| **External worker agent** (`pkg/agent/*`, `cmd/b9agent/*`, TUI, registration, job monitor, metrics, config-file persistence) | `3cee58bf`, `6358fd07`, `883e69b1`, `3404748e`, `9ab69374`, `064640ce`, `ba966396`, `5d4fb21c`, `559943ca` | `pkg/agent/` (42 files) + `cmd/agent/` | upstream `#1595`, `#1646`–`#1650`; add/add conflict on all three shared paths |
| **GPU type propagation via keepalive** (fork PR #5) | `907c90b3` | `joinRequest.GPU/GPUIDs/GPUCount`; `NormalizeGPUType()` | upstream `#1661`, `#1676`, `#1663`. Upstream carries GPU identity in the native join protocol and normalizes vendor strings; our version bolted a `gpu_type` string onto an HTTP keepalive we invented. |
| **Tailscale / tsnet service discovery for external workers** (`directRedisHost`, `ExternalS3Port`, `ExternalRegistryPort`, `ExternalImageRegistry`, `ExternalBaseImageRegistry`, `K3S_HOST_IP`, raw-hostname direct mode) | `011c37d2`, `ef83f57f`, `72be54ad`, `2265f919`, `deb71384`, `9068dbf3`, `b9512df7`, `a528a08d`, `bdde7a42` | `pkg/agent/transport.go` + `route_proxy.go`; `TailscaleConfig.AgentAuthKey` | Upstream solves worker reachability inside the agent transport. Our config-surface hacks exist only because we had no agent transport. |
| **External worker pool controller** | `c6670d44`, modifications to `pool_external.go` | `pkg/scheduler/pool_agent.go` | Git detects `pool_external.go → pool_provider.go` as a **rename** upstream; our edits auto-merge into a file upstream has repurposed. Semantically stale even though it does not conflict. |
| **Go/Tailscale version churn** | `63ce3f7f`, `25d220c3`, `17a3dee3`, `b6e08e57`, `5a535da3`, `ee8a2e86`, `3e2d00be`, `feafc62a`, `b49b3f4a` | upstream is on Go 1.25 with its own tailscale pin | Pure yak-shaving to make our tsnet build work. Delete; take upstream `go.mod`/`go.sum` wholesale. |
| **`fmt.Fprintf` → `fmt.Fprint` vet fix** | part of `45b9a14f` | upstream deleted the enclosing block in `pkg/worker/image.go` | Zero-judgement drop. |
| **Committed binaries** | `b9agent` (11.2 MB), `bin/agent-go` (9.1 MB) | — | Still tracked at the pin despite `0ea19eb8`/`c584f246` claiming removal. ~20 MB of Go binaries, not in LFS. Drop unconditionally. |

### STILL NEEDED — genuinely ours, must survive

| Cluster | Our commits | What it does | Why upstream would not have it |
|---|---|---|---|
| **Ollama / native-MPS inference stack** — `pkg/gateway/inference_router.go`, `inference_handlers.go`, `model_registry.go`, `pkg/types/inference.go`, `pkg/agent/inference.go`, `sdk/src/beta9/inference.py` | `c3a2d33d`, `c8b22310`, `18f8b776`, `4681081e`, `17f549db`, `f6caa36f`, `e463ced4`, `55adb935`, `cd7acd37` | Model registry + inference routing to Ollama-backed nodes; SDK client with batch embeddings | `git grep -il ollama upstream/main -- pkg cmd sdk` returns **nothing**. Beta9 is a container-scheduling runtime; a resident model server is our product layer, not theirs. |
| **`GPU_MPS` / Apple Silicon** — `pkg/types/gpu.go` | `e8a0f530` | Adds `GPU_MPS` GpuType for Apple-Silicon inference hosts | Upstream expanded GpuType to 29 NVIDIA/Gaudi entries but has no Metal/MPS concept (`git grep -il "GPU_MPS\|metal performance"` upstream → empty). **Must be re-applied on top of upstream's new `NormalizeGPUType()`, not merged as a line edit.** |
| **Agentosaurus infra** — `manifests/k3d/*`, `hack/k3d.yaml`, `manifests/kustomize/.../deployment.yaml`, `.github/workflows/build-agentosaurus.yml`, `docker/Dockerfile.*` registry pinning | `db2531eb`, `c04e57ed`, `9ca05efb`, `8553199b`, `9942c40b`, `3765508e`, `d14e540a`, `cdd6b6aa`, `4533f7af`, `ddd893cc`, `22d2304a`, `b1c7b5b5`, `14d47d1f`, `8681ca02` | Podman/k3d compatibility, ARM64 builds, `registry.agentosaurus.com` mirroring, NodePort exposure, CoreDNS custom config, gateway RBAC `jobs` permission | Deployment-environment specific. Never upstreamable. Keep, but expect to re-derive against upstream's current manifests rather than merge. |
| **ARM64 / JuiceFS / NVIDIA-toolkit build fixes** | `62e55b93`, `9cc612a5`, `c13b4058`, `94d0d03e`, `6db086c1`, `17353380`, `d141fe3f`, `8cfefe43`, `c0a22c2f`, `2fc92c1f` | Multi-arch build support for our ARM64 Oracle Cloud host | Upstream builds x86-first. **Needs re-verification** — upstream may have improved multi-arch since. See UNCLEAR. |
| **Sentry instrumentation** — `cmd/worker/main.go`, `cmd/gateway/main.go` | `7fd6b196` | Production error reporting | Ours. Conflicts trivially with upstream's new `configureGOMAXPROCS()`; keep both. |
| **P0 security fixes** (fork PR #4) | `95618d01` | auth fails-open, unauthenticated control API, SSRF, plaintext creds, gateway panic | **Mostly scoped to code we wrote** (our control API, our inference registry). The `apiv1.InferenceRegistry` interface fix in `pkg/api/v1/machine.go` guards *our* registry. Carry forward with the inference stack. |
| **SDK dependency bumps** — `rich<15`, `websockets<17` | `84ea11c1` | browser-use compatibility for FlowState | Downstream consumer constraint. Conflicts with upstream's `websockets<16`. Re-apply after merge. |

### UNCLEAR — needs the owner's decision

1. **Is the MPS/Ollama inference stack still a product requirement?**
   It is the only substantial thing in this fork that upstream does not
   provide. If FlowState's sovereign-cluster offering still routes inference
   through beta9, it must be ported. If inference has since moved elsewhere,
   *the entire fork can be retired* and replaced by vanilla upstream plus a
   thin manifests overlay. **This decision determines whether the remaining
   work is small or trivial.**

2. **Does upstream's agent cover the client deployment that motivated the
   fork?** Upstream's agent has `install`/`join`, systemd + launchd services,
   preflight checks and tsnet transport. If it does, our agent is pure
   liability. If a specific behaviour is missing, name it — it is likely a
   small patch on upstream rather than a reason to keep our implementation.

3. **Is `GPU_MPS` still needed, and should it go upstream?** Upstream's
   `NormalizeGPUType()` is the natural place for it. Adding Apple-Silicon
   support as an upstream PR would remove a permanent fork delta. Worth doing
   if MPS survives decision 1.

4. **ARM64**: does upstream now build multi-arch cleanly? If yes, ~10 of our
   commits evaporate. Requires an actual build, which has not been run (see §6).

5. **Fork PR #5 (`907c90b3`)**: recommend **close without merging**, superseded
   by upstream `#1661`/`#1676`. It is currently what the FlowState parent repo
   pins. Confirm before closing.

6. **`sdk/src/beta9/clients/*/__init__.py`** (14 files) are generated protobuf
   clients. Do not hand-merge — regenerate with `make protocol` after taking
   upstream's `.proto` files. Confirm this is acceptable (it discards our
   generated deltas, which is the intent).

## 4. Conflict assessment (measured, not estimated)

`git merge upstream/main` on a throwaway branch: **18 conflicted files**
(15 content, 3 add/add). Upstream additionally brings 474 new files and 265
modifications that merge cleanly.

| File | Hunks | Character |
|---|---|---|
| `go.sum` | 33 | regenerate, do not hand-merge |
| `go.mod` | 21 | regenerate |
| `sdk/uv.lock` | 12 | regenerate (`uv lock`) |
| `docker/Dockerfile.worker` | 6 | our ARM64/registry changes vs upstream restructuring |
| `docker/Dockerfile.gateway` | 4 | same |
| `pkg/gateway/gateway.go` | 3 | **all three are our inference registry** vs upstream signature changes (`isReady` added to health group, `gatewayTailscaleConfig()` extracted, `draining atomic.Bool`) — mechanical |
| `pkg/agent/agent.go` | 1 | **add/add — rival implementations** |
| `pkg/agent/state.go` | 1 | **add/add — entire file, 4→419** |
| `pkg/agent/state_test.go` | 1 | **add/add** |
| `pkg/types/gpu.go` | 1 | our `GPU_MPS` vs upstream's 29-type rewrite + `NormalizeGPUType` |
| `pkg/types/config.go` | 1 | our `DirectRedisHost` vs upstream's `AgentAuthKey` |
| `pkg/worker/image.go` | 1 | upstream deleted the block — take upstream |
| `cmd/worker/main.go` | 1 | our Sentry init vs upstream `configureGOMAXPROCS()` — keep both |
| `sdk/pyproject.toml` | 1 | `websockets<17` vs `<16` |
| `sdk/src/beta9/__init__.py` | 1 | our `inference` export vs upstream's new exports |
| `manifests/kustomize/.../deployment.yaml` | 1 | our hostNetwork/registry vs upstream |
| `.gitignore`, `README.md` | 1 each | cosmetic |

Also: upstream **renamed** `pkg/scheduler/pool_external.go` → `pool_provider.go`
and **deleted** `pkg/worker/criu_cedana.go` and
`manifests/kustomize/components/monitoring/elasticsearch.yaml`.

**Verdict: the conflicts are not the hard part.** Excluding the three
lockfiles (66 of ~91 hunks, all machine-regenerable) there are ~25 hunks of
real work, nearly all small. The hard part is the *architectural* decision in
`pkg/agent`, which no merge algorithm can resolve.

## 5. Recommended plan

**Do not `git merge upstream/main`. Rebuild the fork on top of upstream.**

Rationale: the merge would produce a tree containing two agent implementations,
two entrypoints and two pool controllers, and would preserve ~9 months of
superseded scaffolding that exists only to work around the absence of the agent
upstream has now written. The delta worth keeping is small and additive.

Order of operations:

1. **Snapshot.** Tag the current pin so it is never lost:
   `git tag fork-pre-upstream-sync-20260818 43800352 && git push origin --tags`.
   Do **not** move the parent's submodule pointer until step 7 passes.
   (Branch `upstream-main-snapshot-20260818` has been pushed to the fork so
   upstream history is available without configuring a remote.)

2. **New base.** `git checkout -b sync/upstream-20260818 upstream/main`.

3. **Delete, do not port.** Everything in the SUPERSEDED bucket is simply
   absent from the new branch — including `pkg/agent/*` (ours), `cmd/b9agent/`,
   `b9agent`, `bin/agent-go`, and the external-worker config surface in
   `pkg/types/config.go`.

4. **Re-apply the keep-list as fresh commits** against upstream's current
   shape, in this order (each is small and independently reviewable):
   a. `GPU_MPS` into upstream's `gpu.go` + `NormalizeGPUType()`
   b. inference stack: `pkg/types/inference.go`, `pkg/gateway/model_registry.go`,
      `inference_router.go`, `inference_handlers.go`, the `apiv1` registry
      interface, `sdk/src/beta9/inference.py` — carrying the PR #4 security
      fixes inline, not as a later patch
   c. Sentry init in `cmd/gateway` + `cmd/worker`
   d. agentosaurus manifests/k3d/CI overlay, re-derived against upstream's
      current manifests
   e. SDK dependency bumps (`rich<15`, `websockets<17`)

5. **Regenerate, never hand-merge:** `make protocol` (protobuf clients),
   `go mod tidy`, `uv lock` in `sdk/`.

6. **Verify** — actual commands from upstream's `Makefile` and
   `.github/workflows/ci.yml` (Go 1.25.0, Python 3.11):
   ```
   make verify-protocol      # ./bin/verify_proto.sh
   make test-pkg             # go test -v ./pkg/...
   make -C sdk tests
   cd sdk && uv run ruff format . && uv run ruff check .
   make gateway              # docker build -f docker/Dockerfile.gateway
   make worker               # docker build -f docker/Dockerfile.worker
   ```
   The Go build and `make test-pkg` are the gate. Nothing should be pinned
   until they pass on ARM64.

7. **Deploy behind the existing rollback.** Do not move the parent submodule
   pointer until a gateway built from this branch runs. Note the current
   gateway pods have been in `Init:Error` for 26 days — **the deployed state is
   already broken, so "it worked before" is not an available baseline.** Fix or
   characterise that failure first, otherwise a post-merge failure will be
   unattributable.

**Rollback:** the parent pins the submodule by SHA, so rollback is
`git -C backend/beta9 checkout 43800352` plus reverting the parent's pointer
commit — one revert, no data migration. The `fork-pre-upstream-sync-20260818`
tag guarantees the old tree survives even if branches are pruned. Never move
the submodule pointer backwards on a branch that forked before the bump; git
merges that silently.

**Effort:** roughly 60–120k tokens across ~15–25 LLM calls for step 4 (five
independent re-application commits, each needing the upstream shape read
first), plus build/test iterations that cannot be estimated until a Go
toolchain is available. Steps 1–3 and 5 are mechanical.

## 6. What was and was not done

**Run:** fresh clone; `git remote add upstream`; `git fetch upstream`;
`rev-list --left-right --count`; `merge-base`; `git log`/`diff`/`ls-tree`
across both trees; `git merge-tree --write-tree`; a real
`git merge --no-commit upstream/main` on a throwaway branch (conflicts counted
and read, then `git merge --abort`); top-level-identifier collision analysis of
both `pkg/agent` implementations; `git cat-file -s` on the committed binaries.

**Not run:** **no build, no test, no lint.** There is no Go toolchain on this
host (`which go` → not found). The commands in §6 above were read from
upstream's `Makefile` and `.github/workflows/ci.yml`; they have **not** been
executed. Nothing in this document should be read as implying the merge is
verified to compile.

**Not touched:** the parent repo's submodule pointer, any commit in
`/home/flowstate/FlowState`, `Wingie/beta9` `main`, and all containers and the
k3s/beta9 cluster.
