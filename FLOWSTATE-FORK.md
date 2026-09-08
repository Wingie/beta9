# What this fork is

Rebased onto `beam-cloud/beta9` `upstream/main` on 2026-09-08. Before that the
fork had been diverging for about nine months: 135 commits ahead of merge-base
`299106d5` (2025-11-26) and 312 behind.

Everything this fork now adds to upstream is in this document. If something is
not listed here, it is upstream's and should be changed upstream.

## Why the old fork was retired rather than merged

The fork existed to supply an external-worker agent that upstream did not have.
Upstream has since written one, and a larger one: `pkg/agent/` is 29 files with
`service/` (systemd + launchd), `preflight.go`, `route_proxy.go`, `ssh.go`,
`telemetry.go` and `thunder.go`, plus a `cmd/agent` entrypoint with `install`
and `join` subcommands and a Vast.ai integration.

Merging would have produced one Go package holding two rival agent
implementations with two entrypoints and two pool controllers — `git merge`
reports add/add conflicts on `pkg/agent/agent.go`, `state.go` and
`state_test.go`, meaning both sides created the same paths from nothing. So the
fork was rebuilt on top of upstream instead, and the superseded work was
dropped rather than ported.

## What this fork adds

| Area | Files | Why it is not upstreamable |
|---|---|---|
| Sentry | `cmd/gateway/main.go`, `cmd/worker/main.go` | Reads `SENTRY_DSN` from the environment and no-ops when unset. Our production error reporting. |
| Registry CI | `.github/workflows/build-agentosaurus.yml` | Builds gateway/worker/runner multi-arch and pushes to `registry.agentosaurus.com`. Deployment-specific. |
| k3s environment fixes | `manifests/k3d/coredns-custom.yaml`, `manifests/k3d/metrics-server.yaml` | Two self-contained cluster fixes: CoreDNS forwarding, because k3s pods on flannel cannot route to Tailscale MagicDNS at 100.100.100.100; and a `hostNetwork` metrics-server, because the pod network cannot reach the kubelet endpoint on 10250. Neither touches beta9. |

That is the whole delta.

## What was dropped, and where it went

All of it is reachable from the tag `fork-pre-upstream-sync-pin-95618d01`
(the parent repo's submodule pin before this rebase) and from
`fork-pre-upstream-sync-20260818`. Recover any path with:

    git checkout fork-pre-upstream-sync-pin-95618d01 -- <path>

- **Our external-worker agent** — `pkg/agent/*` (TUI, keepalive, registration,
  job monitor, state, metrics) and `cmd/b9agent/`. Superseded by upstream's.
- **The Ollama / native-MPS inference stack** — `pkg/gateway/inference_router.go`,
  `inference_handlers.go`, `model_registry.go`, `pkg/types/inference.go`,
  `pkg/agent/inference.go`, `sdk/src/beta9/inference.py`. Dropped by owner
  decision on 2026-09-08. Upstream has no Ollama or inference surface at all,
  so this was the fork's only substantial product delta; with it gone the fork
  is close to vestigial.
- **`GPU_MPS`** in `pkg/types/gpu.go`. It only ever typed Apple-Silicon
  inference hosts, so it went with the inference stack. Upstream has since
  rewritten `gpu.go` around `NormalizeGPUType()`.
- **The `:9999` control API** — `pkg/agent/control.go`, serving `/finetune/*`
  and `/coding/*`. The parent repo's `backend/Quantoxbay/aginto/harness/go/`
  extends it and three Django call sites speak to it, but nothing listens on
  that port today and `AGINTO_AGENT_URL` is not defined in settings. When a
  customer cluster is real, rebuild it as a sidecar next to upstream's stock
  agent rather than as a fork of the agent.
- **The external-worker config surface** in `pkg/types/config.go`
  (`DirectRedisHost`, `ExternalS3Port`, `ExternalRegistryPort`,
  `ExternalImageRegistry`, `ExternalBaseImageRegistry`, `K3S_HOST_IP`) and the
  Tailscale/tsnet wiring around it. These worked around the absence of an agent
  transport; upstream ships one.
- **Go and Tailscale version churn** — about nine commits of it. Upstream's
  `go.mod` is taken whole.
- **Two committed Go binaries**, `b9agent` (11.2 MB) and `bin/agent-go`
  (9.1 MB), still tracked at the old pin despite two commits claiming to remove
  them.
- **The k3d/kustomize manifest edits** for `registry.agentosaurus.com`,
  NodePorts and hostNetwork. Not re-derived: upstream restructured
  `manifests/kustomize/` and the beta9 k3s namespace was torn down the same
  day, so there was nothing to test a re-derivation against. Derive fresh
  against whatever upstream is current when a cluster is next stood up.

## The SDK is no longer consumed from this repo

The FlowState parent repo used to take the Python SDK as a uv path dependency
on `backend/beta9/sdk`, which made every `uv` invocation rebuild a bind-mounted
git submodule. It now installs `beta9` from PyPI. Upstream had independently
adopted the `rich<15` bump we forked for, leaving `websockets<17` vs `<16` as
the only difference, and the resolver settles that on its own.

Do not reintroduce the path dependency.
