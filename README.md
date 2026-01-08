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

## FlowState Fork - Modifications from Upstream

This is a fork of [beam-cloud/beta9](https://github.com/beam-cloud/beta9) with modifications to support external GPU workers via SSH tunnel (without requiring the closed-source agent binary or Tailscale VPN).

### Changes Made

#### 1. Added Machine Keepalive Endpoint

**File**: `pkg/api/v1/machine.go`

**Why**: The upstream gateway has a `SetMachineKeepAlive()` function but NO HTTP endpoint to call it. Without this, machines can register but never transition to "ready" status.

**Change**: Added `POST /api/v1/machine/keepalive` endpoint:

```go
g.POST("/keepalive", group.MachineKeepalive)
```

**Behavior**:
- Accepts machine_id, pool_name, agent_version, metrics
- Calls `providerRepo.SetMachineKeepAlive()` to:
  - Set machine status to "ready"
  - Refresh 5-minute TTL
  - Store metrics

#### 2. Fixed Tailscale Dependency in Registration

**File**: `pkg/api/v1/machine.go`

**Why**: `RegisterMachine()` called `GetRemoteConfig()` which requires Tailscale to resolve Redis hostnames. For external workers using SSH tunnel (no Tailscale), this caused registration to fail with "Unable to create remote config".

**Change**: Return null config instead of failing:

```go
remoteConfig, err := providers.GetRemoteConfig(g.config, g.tailscale)
if err != nil {
    // Return nil config - external workers via SSH tunnel don't need it
    remoteConfig = nil
}
```

### Why These Changes?

Beta9's original architecture requires:
1. **Closed-source agent binary** (~70MB) - distributed at release.beam.cloud, not in this repo
2. **Tailscale VPN** - for network connectivity between agent and gateway

Our use case:
- Connect external GPU workers (Lambda Labs, local machines) to self-hosted Beta9
- Use **SSH tunnels** instead of VPN for simpler network setup
- Write our own **Python agent** instead of using closed-source binary

### Machine Lifecycle with These Changes

```
┌─────────┐    ┌────────────┐    ┌─────────────────┐
│ pending │───>│ registered │───>│     ready       │
└─────────┘    └────────────┘    └─────────────────┘
     │               │                    │
beta9 machine    POST              POST /keepalive
create          /register          (every 60 sec)
```

**Important**: Machine state has 5-minute TTL. If keepalive not sent within 5 minutes, the machine is automatically removed from the pool (Redis key expires).

### API Reference

#### POST /api/v1/machine/register
```json
{
  "machine_id": "abc123",
  "hostname": "my-worker",
  "provider_name": "generic",
  "pool_name": "gpu",
  "cpu": "8000m",
  "memory": "16Gi",
  "gpu_count": "1",
  "private_ip": "192.168.1.100"
}
```

#### POST /api/v1/machine/keepalive
```json
{
  "machine_id": "abc123",
  "provider_name": "generic",
  "pool_name": "gpu",
  "agent_version": "0.1.0",
  "metrics": {
    "cpu_utilization_pct": 15.5,
    "memory_utilization_pct": 42.3
  }
}
```

### Testing

```bash
# SSH tunnel to gateway
ssh -L 1994:localhost:31994 your-server

# Create machine token
beta9 machine create --pool gpu

# Register
curl -X POST http://localhost:1994/api/v1/machine/register \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"machine_id":"abc","pool_name":"gpu",...}'

# Keepalive (must be within 5 minutes!)
curl -X POST http://localhost:1994/api/v1/machine/keepalive \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"machine_id":"abc","pool_name":"gpu","agent_version":"0.1.0"}'
```

---

## 👋 Contributing

We welcome contributions big or small. These are the most helpful things for us:

- Submit a [feature request](https://github.com/beam-cloud/beta9/issues/new?assignees=&labels=&projects=&template=feature-request.md&title=) or [bug report](https://github.com/beam-cloud/beta9/issues/new?assignees=&labels=&projects=&template=bug-report.md&title=)
- Open a PR with a new feature or improvement

## ❤️ Thanks to Our Contributors

<a href="https://github.com/beam-cloud/beta9/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=beam-cloud/beta9" />
</a>
