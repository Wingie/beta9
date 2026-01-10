<div align="center">
<p align="center">
<img alt="Logo" src="static/beam-logo-white.png#gh-dark-mode-only" width="30%">
<img alt="Logo" src="static/beam-logo-dark.png#gh-light-mode-only" width="30%">
</p>

## Run AI Workloads at Scale

<p align="center">
  </a>
    <a href="https://colab.research.google.com/drive/1jSDyYY7FY3Y3jJlCzkmHlH8vTyF-TEmB?usp=sharing">
    <img alt="Colab" src="https://colab.research.google.com/assets/colab-badge.svg">
  </a>
  <a href="https://github.com/beam-cloud/beta9/stargazers">
    <img alt="⭐ Star the Repo" src="https://img.shields.io/github/stars/beam-cloud/beta9">
  </a>
  <a href="https://docs.beam.cloud">
    <img alt="Documentation" src="https://img.shields.io/badge/docs-quickstart-purple">
  </a>
  <a href="https://join.slack.com/t/beam-cloud/shared_invite/zt-39hbkt8ty-CTVv4NsgLoYArjWaVkwcFw">
    <img alt="Join Slack" src="https://img.shields.io/badge/Beam-Join%20Slack-orange?logo=slack">
  </a>
    <a href="https://twitter.com/beam_cloud">
    <img alt="Twitter" src="https://img.shields.io/twitter/follow/beam_cloud.svg?style=social&logo=twitter">
  </a>
    <a href="https://github.com/beam-cloud/beta9?tab=AGPL-3.0-1-ov-file">
    <img alt="AGPL" src="https://img.shields.io/badge/License-AGPL-green">
  </a>
</p>

</div>

**[Beam](https://beam.cloud?utm_source=github_readme)** is a fast, open-source runtime for serverless AI workloads. It gives you a Pythonic interface to deploy and scale AI applications with zero infrastructure overhead.

![Watch the demo](static/readme.gif)

## ✨ Features

- **Fast Image Builds**: Launch containers in under a second using a custom container runtime
- **Parallelization and Concurrency**: Fan out workloads to 100s of containers
- **First-Class Developer Experience**: Hot-reloading, webhooks, and scheduled jobs
- **Scale-to-Zero**: Workloads are serverless by default
- **Volume Storage**: Mount distributed storage volumes
- **GPU Support**: Run on our cloud (4090s, H100s, and more) or bring your own GPUs

## 📦 Installation

```shell
pip install beam-client
```

## ⚡️ Quickstart

1. Create an account [here](https://beam.cloud?utm_source=github_readme)
2. Follow our [Getting Started Guide](https://platform.beam.cloud/onboarding?utm_source=github_readme)

## Creating a sandbox

Spin up isolated containers to run LLM-generated code:

```python
from beam import Image, Sandbox


sandbox = Sandbox(image=Image()).create()
response = sandbox.process.run_code("print('I am running remotely')")

print(response.result)
```

## Deploy a serverless inference endpoint

Create an autoscaling endpoint for your custom model:

```python
from beam import Image, endpoint
from beam import QueueDepthAutoscaler

@endpoint(
    image=Image(python_version="python3.11"),
    gpu="A10G",
    cpu=2,
    memory="16Gi",
    autoscaler=QueueDepthAutoscaler(max_containers=5, tasks_per_container=30)
)
def handler():
    return {"label": "cat", "confidence": 0.97}
```

## Run background tasks

Schedule resilient background tasks (or replace your Celery queue) by adding a simple decorator:

```python
from beam import Image, TaskPolicy, schema, task_queue


class Input(schema.Schema):
    image_url = schema.String()


@task_queue(
    name="image-processor",
    image=Image(python_version="python3.11"),
    cpu=1,
    memory=1024,
    inputs=Input,
    task_policy=TaskPolicy(max_retries=3),
)
def my_background_task(input: Input, *, context):
    image_url = input.image_url
    print(f"Processing image: {image_url}")
    return {"image_url": image_url}


if __name__ == "__main__":
    # Invoke a background task from your app (without deploying it)
    my_background_task.put(image_url="https://example.com/image.jpg")

    # You can also deploy this behind a versioned endpoint with:
    # beam deploy app.py:my_background_task --name image-processor
```

> ## Self-Hosting vs Cloud
>
> Beta9 is the open-source engine powering [Beam](https://beam.cloud), our fully-managed cloud platform. You can self-host Beta9 for free or choose managed cloud hosting through Beam.

---

## Fork: External Worker Support

This fork of [beam-cloud/beta9](https://github.com/beam-cloud/beta9) adds support for **external GPU workers** - connecting machines outside your k3s cluster to participate in job execution.

### The Journey

**Problem**: Beta9's original architecture required:
1. A closed-source agent binary (~70MB) not included in the repo
2. Tailscale VPN for network connectivity

**Solution**: We built an open-source Go agent (`b9agent`) that:
- Registers external machines with the gateway via HTTP
- Maintains keepalive heartbeats to stay in the worker pool
- Uses Tailscale mesh VPN for secure machine-to-machine connectivity
- Provides a real-time TUI dashboard showing worker status and jobs

### Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                              TAILSCALE MESH VPN                              │
│  (Encrypted overlay network - all machines share private 100.x.x.x IPs)      │
└──────────────────────────────────────────────────────────────────────────────┘
         │                           │                           │
         ▼                           ▼                           ▼
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│   GATEWAY       │         │   WORKER 1      │         │   WORKER 2      │
│   (k3s master)  │         │   (external)    │         │   (external)    │
│                 │         │                 │         │                 │
│  ┌───────────┐  │         │  ┌───────────┐  │         │  ┌───────────┐  │
│  │ beta9-gw  │◄─┼─────────┼──│ b9agent   │  │         │  │ b9agent   │  │
│  │ :1994     │  │   HTTP  │  │ (Go TUI)  │  │         │  │ (Go TUI)  │  │
│  └───────────┘  │         │  └───────────┘  │         │  └───────────┘  │
│        │        │         │        │        │         │        │        │
│        ▼        │         │        ▼        │         │        ▼        │
│  ┌───────────┐  │         │  ┌───────────┐  │         │  ┌───────────┐  │
│  │ k3s API   │◄─┼─────────┼──│ kubelet   │  │         │  │ kubelet   │  │
│  │ :6443     │  │  JOIN   │  │ (k3s)     │  │         │  │ (k3s)     │  │
│  └───────────┘  │         │  └───────────┘  │         │  └───────────┘  │
│        │        │         │        │        │         │        │        │
│        ▼        │         │        ▼        │         │        ▼        │
│  ┌───────────┐  │         │  ┌───────────┐  │         │  ┌───────────┐  │
│  │ scheduler │──┼─────────┼──│ PODS      │  │         │  │ PODS      │  │
│  └───────────┘  │         │  │ (jobs)    │  │         │  │ (jobs)    │  │
│                 │         │  └───────────┘  │         │  └───────────┘  │
└─────────────────┘         └─────────────────┘         └─────────────────┘
     Oracle Cloud              Mac (M-series)           Lambda Labs (GPU)
```

### Component Roles

| Component | Role | Location |
|-----------|------|----------|
| **Gateway** | Central API server, receives jobs, manages machine registry | k3s master (cloud) |
| **Agent** (`b9agent`) | Registers machine, sends heartbeats, monitors jobs | Each worker machine |
| **Worker** | k3s node that executes pods (containers) scheduled by gateway | Each worker machine |
| **Tailscale** | Mesh VPN providing encrypted 100.x.x.x network | All machines |

### Multi-Machine Connection Guide

**Prerequisites:**
1. All machines must be on the same Tailscale network
2. Gateway k3s cluster running with `beta9-gateway` service exposed
3. k3s join token from gateway (for worker k3s to join cluster)

**Connect a new machine:**

```bash
# 1. Install Tailscale and join your network
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --authkey=<YOUR_TAILSCALE_KEY>

# 2. Note your Tailscale IP (100.x.x.x)
tailscale ip -4

# 3. Create machine token on gateway
beta9 machine create --pool external

# 4. Initialize agent config on worker
b9agent init \
  --gateway <GATEWAY_TAILSCALE_IP>:1994 \
  --token <MACHINE_TOKEN> \
  --pool external

# 5. Join k3s cluster (as worker node)
curl -sfL https://get.k3s.io | K3S_URL=https://<GATEWAY_TAILSCALE_IP>:6443 \
  K3S_TOKEN=<K3S_JOIN_TOKEN> sh -

# 6. Start the agent (TUI dashboard)
b9agent
```

**Verify connection:**
```bash
# On gateway - check machine is registered and ready
beta9 machine list

# On worker - TUI shows status, jobs appear when scheduled
```

### Gateway API Modifications

#### 1. Added Machine Keepalive Endpoint

**File**: `pkg/api/v1/machine.go`

Added `POST /api/v1/machine/keepalive` - required because upstream gateway had the internal function but no HTTP endpoint.

#### 2. Fixed Tailscale Dependency in Registration

**File**: `pkg/api/v1/machine.go`

Made `GetRemoteConfig()` errors non-fatal for external workers.

### Machine State Lifecycle

```
┌─────────┐    ┌────────────┐    ┌─────────────────┐
│ pending │───>│ registered │───>│     ready       │
└─────────┘    └────────────┘    └─────────────────┘
     │               │                    │
beta9 machine    POST              POST /keepalive
create          /register          (every 60 sec)
```

**TTL**: Machine state expires after 5 minutes without keepalive.

### Agent TUI Dashboard

The Go agent provides a real-time dashboard:

```
╔══ Beta9 Agent: 159ecc90 ══════════════════════════════════════════════════════╗
║ Status: READY │ Gateway: 100.72.101.23 │ Pool: external │ Uptime: 2h 34m      ║
║ CPU: 18.2% │ Memory: 56.1% │ GPUs: 0 │ Last Heartbeat: 3s ago                 ║
╠═══════════════════════════════════════════════════════════════════════════════╣
║ WORKER PODS                                                                   ║
╟───────────────────────────────────────────────────────────────────────────────╢
║ worker-abc123   RUNNING     hello_beta9:hello     12s                         ║
║ worker-def456   COMPLETED   hello_beta9:hello    847ms  (2 min ago)           ║
║ worker-ghi789   FAILED      test_job:process     1.2s   (5 min ago)           ║
╚═══════════════════════════════════════════════════════════════════════════════╝
Press Ctrl+C to quit
```

**Agent Config** (`~/.b9agent/config.yaml`):
```yaml
gateway:
  host: "100.72.101.23"
  port: 1994
machine:
  id: "159ecc90"
  token: "<machine-token>"
  hostname: "100.100.74.117"
pool: "external"
k3s:
  token: "<k3s-bearer-token>"
```

---

## TODOs

### MPS (Metal Performance Shaders) Inference Engine
- [ ] Add MPS device detection for Apple Silicon (M1/M2/M3) GPUs
- [ ] Expose MPS capability in machine registration metrics
- [ ] Enable PyTorch MPS backend for ML inference on Mac workers
- [ ] Connect MPS workers to Tailscale mesh for job scheduling

### Custom Device Support
- [ ] Abstract device detection beyond NVIDIA GPUs
- [ ] Support AMD ROCm devices
- [ ] Support Intel Arc/oneAPI devices
- [ ] Support cloud-specific accelerators (TPU, Trainium, Inferentia)
- [ ] Device capability reporting in keepalive metrics

### Agent Improvements
- [ ] Interactive TUI with log viewing (press Enter on job)
- [ ] `b9agent config show/set` subcommands
- [ ] Auto-reconnect on gateway disconnect
- [ ] Prometheus metrics endpoint

---

## 👋 Contributing

We welcome contributions big or small. These are the most helpful things for us:

- Submit a [feature request](https://github.com/beam-cloud/beta9/issues/new?assignees=&labels=&projects=&template=feature-request.md&title=) or [bug report](https://github.com/beam-cloud/beta9/issues/new?assignees=&labels=&projects=&template=bug-report.md&title=)
- Open a PR with a new feature or improvement

## ❤️ Thanks to Our Contributors

<a href="https://github.com/beam-cloud/beta9/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=beam-cloud/beta9" />
</a>
