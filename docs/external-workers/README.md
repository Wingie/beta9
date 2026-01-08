# External Workers Guide

Connect any Linux machine with GPUs to your Beta9 cluster for serverless GPU compute.

## Overview

External workers allow you to:

- **Bring your own GPUs** - Lambda Labs, Crusoe, local hardware
- **Hybrid cloud** - Mix cloud and on-prem resources
- **Cost optimization** - Use spot instances or reserved capacity

## Architecture

```
                         SSH Tunnel (Port 1994)
    ┌──────────────┐    ─────────────────────────>    ┌──────────────┐
    │              │                                   │              │
    │  External    │                                   │    Beta9     │
    │  GPU Worker  │                                   │   Gateway    │
    │              │    <─────────────────────────     │              │
    │  (Python     │      k3s API (Port 6443)         │   (k3s      │
    │   Agent)     │      via Reverse Tunnel           │   cluster)   │
    │              │                                   │              │
    └──────────────┘                                   └──────────────┘
         │                                                    │
         │ Runs k3s                                          │ Deploys
         │ locally                                           │ worker pods
         ▼                                                    ▼
    ┌──────────────┐                                   ┌──────────────┐
    │   Worker     │  <── Workloads dispatched ───     │  Scheduler   │
    │    Pods      │                                   │              │
    └──────────────┘                                   └──────────────┘
```

## Connection Options

### 1. SSH Tunnel Mode (Recommended)

For environments where you have SSH access to the gateway host:

- **No VPN required**
- **Works through NAT/firewalls**
- **Uses existing SSH infrastructure**
- **Simpler setup**

### 2. Tailscale VPN Mode

Original Beam approach using Tailscale:

- Requires Tailscale account and setup
- Direct mesh networking
- Uses proprietary agent binary

This guide focuses on **SSH Tunnel Mode**.

## Prerequisites

### On the Gateway Host

- Beta9 gateway running with NodePort access (31994)
- SSH server accessible
- `beta9` CLI installed

### On the External Worker

- Linux (Debian/Ubuntu 22.04+ recommended)
- Python 3.10+
- Root or sudo access (for k3s installation)
- SSH client
- NVIDIA drivers (for GPU workers)

## Quick Start

### Step 1: Create Worker Pool (if needed)

On your gateway/control machine:

```bash
# Check existing pools
beta9 pool list

# Create GPU pool if it doesn't exist
# Pools are defined in gateway config.yaml
```

### Step 2: Create Machine Token

```bash
# Generate a machine entry with token
beta9 machine create --pool gpu

# Output:
# Machine ID: 1165a9b6
# Token: swwMV_WA4C_FxN-_UY5UiRgdg2dTM1GUDLVUzO9c6fzyOs0BUJuXKLfxb-QcZkwAnlOGscu0U3WYmF1eQpcSBg==
#
# Note: Token expires in 1 hour if not registered
```

### Step 3: Set Up SSH Tunnel

On the external worker:

```bash
# Forward local port 1994 to gateway's NodePort
ssh -L 1994:localhost:31994 your-gateway-host

# Keep this terminal open, or use:
ssh -fNL 1994:localhost:31994 your-gateway-host
```

### Step 4: Install and Run Agent

```bash
# Clone the repo or copy cmd/agent/ directory
cd /path/to/beta9/cmd/agent

# Install dependencies
pip install -r requirements.txt

# Run agent with token from Step 2
python -m cmd.agent \
    --token "YOUR_TOKEN_HERE" \
    --machine-id "1165a9b6" \
    --pool-name gpu \
    --gateway-host localhost \
    --gateway-port 1994
```

### Step 5: Verify Registration

```bash
# On gateway machine
beta9 machine list

# Should show:
# ID        Pool  Status  CPU    Memory  GPUs
# 1165a9b6  gpu   ready   8000m  16Gi    1
```

## What Happens Next

Once registered and "ready":

1. **Gateway detects healthy pool** - Logs show "pool is healthy"
2. **Workloads can be scheduled** - GPU tasks dispatched to your machine
3. **k3s receives pods** - Gateway deploys worker pods via k3s API
4. **Agent maintains connection** - Keepalive every 60 seconds

## Detailed Guides

- [SSH Tunnel Setup](ssh-tunnel-setup.md) - Persistent tunnel configuration
- [Registration Flow](registration-flow.md) - State machine and API details
- [Keepalive Requirements](keepalive-requirements.md) - TTL behavior and metrics
- [Troubleshooting](troubleshooting.md) - Common issues and fixes

## Security Considerations

- **Token security** - Treat machine tokens like passwords
- **SSH key auth** - Use key-based SSH, not passwords
- **Firewall** - Only expose necessary ports
- **Network isolation** - Consider VLANs for GPU workers
