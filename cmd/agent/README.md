# Beta9 Python Agent

Open-source agent for connecting external GPU workers to Beta9.

## Overview

This Python agent replaces the closed-source Beam agent binary (`release.beam.cloud/agent`). It provides:

- **HTTP-based registration** - No proprietary dependencies
- **SSH tunnel connectivity** - No Tailscale/VPN required
- **Full source visibility** - Understand exactly what runs on your machines
- **Cross-platform** - Works on Linux and macOS (for testing)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       Python Agent                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  main.py ─────────> CLI entry point, signal handling            │
│       │                                                          │
│       ├──> config.py ────> Configuration dataclass              │
│       │                                                          │
│       ├──> registration.py ──> POST /api/v1/machine/register    │
│       │                                                          │
│       ├──> keepalive.py ─────> Keepalive loop (60s interval)    │
│       │                                                          │
│       ├──> metrics.py ───────> System metrics (psutil)          │
│       │                                                          │
│       └──> utils.py ─────────> Logging, ID generation           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Installation

### Prerequisites

- Python 3.10+
- pip

### Install Dependencies

```bash
cd backend/beta9/cmd/agent
pip install -r requirements.txt
```

### Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| httpx | >= 0.27.0 | Async HTTP client |
| psutil | >= 5.9.0 | System metrics |
| click | >= 8.1.0 | CLI framework |

## Usage

### Basic Usage

```bash
python -m cmd.agent \
    --token "YOUR_MACHINE_TOKEN" \
    --machine-id "abc12345" \
    --pool-name gpu \
    --gateway-host localhost \
    --gateway-port 1994
```

### With SSH Tunnel

```bash
# Terminal 1: Start SSH tunnel
ssh -L 1994:localhost:31994 your-gateway-host

# Terminal 2: Run agent
python -m cmd.agent \
    --token "YOUR_TOKEN" \
    --pool-name gpu \
    --gateway-host localhost \
    --gateway-port 1994
```

### One-Time Test Run

```bash
python -m cmd.agent \
    --token "YOUR_TOKEN" \
    --pool-name gpu \
    --once \
    --debug
```

## CLI Arguments

| Argument | Env Variable | Default | Required | Description |
|----------|--------------|---------|----------|-------------|
| `--token` | `BETA9_TOKEN` | - | Yes | Machine registration token |
| `--machine-id` | `BETA9_MACHINE_ID` | Auto-generated | No | 8 character hex ID |
| `--pool-name` | `BETA9_POOL_NAME` | `external` | No | Worker pool name |
| `--provider-name` | `BETA9_PROVIDER_NAME` | `generic` | No | Provider identifier |
| `--gateway-host` | `BETA9_GATEWAY_HOST` | `localhost` | No | Gateway HTTP host |
| `--gateway-port` | `BETA9_GATEWAY_PORT` | `1994` | No | Gateway HTTP port |
| `--hostname` | `HOSTNAME` | System hostname | No | Machine hostname |
| `--keepalive-interval` | - | `60` | No | Seconds between keepalives |
| `--debug` | `BETA9_DEBUG` | `false` | No | Enable verbose logging |
| `--once` | - | `false` | No | Single registration, then exit |
| `--dry-run` | - | `false` | No | Print config without registering |

## Environment Variables

You can configure the agent via environment variables:

```bash
export BETA9_TOKEN="your-token-here"
export BETA9_POOL_NAME="gpu"
export BETA9_GATEWAY_HOST="localhost"
export BETA9_GATEWAY_PORT="1994"
export BETA9_DEBUG="true"

python -m cmd.agent
```

## Configuration File

The agent can also read from a config file (future enhancement):

```yaml
# agent.yaml
token: "your-token"
pool_name: gpu
gateway:
  host: localhost
  port: 1994
keepalive_interval: 60
```

## Lifecycle

### Startup

1. Parse CLI arguments and environment variables
2. Validate configuration
3. Generate machine ID if not provided
4. Collect initial system metrics (CPU, memory, GPU)

### Registration

5. Send `POST /api/v1/machine/register` to gateway
6. Parse response for config and machine state
7. If error, exit with error code

### Keepalive Loop

8. Every 60 seconds:
   - Collect current system metrics
   - Send `POST /api/v1/machine/keepalive`
   - Log response status
9. On SIGINT/SIGTERM: graceful shutdown

## Metrics Collected

The agent reports these metrics to the gateway:

| Metric | Source | Description |
|--------|--------|-------------|
| cpu_utilization_pct | psutil | Current CPU usage % |
| memory_utilization_pct | psutil | Current memory usage % |
| total_cpu_available | psutil | Total CPU cores * 1000 (millicores) |
| total_memory_available | psutil | Total memory in MB |
| total_disk_space_bytes | psutil | Root disk total |
| total_disk_free_bytes | psutil | Root disk free |
| free_gpu_count | nvidia-ml-py | Available GPUs |

## GPU Detection

On systems with NVIDIA GPUs:

```python
# Uses nvidia-ml-py (pynvml)
import pynvml
pynvml.nvmlInit()
device_count = pynvml.nvmlDeviceGetCount()
```

Falls back to 0 GPUs if:
- nvidia-ml-py not installed
- No NVIDIA drivers
- No GPUs present

## Error Handling

### Registration Errors

| Error | Cause | Action |
|-------|-------|--------|
| 403 Invalid token | Token expired or wrong | Get new token |
| 500 Invalid pool name | Pool doesn't exist | Check pool config |
| Connection refused | Tunnel not running | Start SSH tunnel |

### Keepalive Errors

| Error | Cause | Action |
|-------|-------|--------|
| machine_not_found | TTL expired | Re-register |
| Connection error | Network issue | Retry (auto) |

The agent automatically retries keepalive on transient errors.

## Development

### Running Tests

```bash
cd backend/beta9/cmd/agent
python -m pytest tests/
```

### Debug Mode

```bash
python -m cmd.agent --debug ...
```

Enables:
- Verbose logging
- Request/response dumps
- Stack traces on errors

### Dry Run

```bash
python -m cmd.agent --dry-run ...
```

Shows configuration without making any API calls.

## Platforms

### Tested

| Platform | Status | Notes |
|----------|--------|-------|
| Ubuntu 22.04 | Working | Primary target |
| Debian 12 | Working | |
| macOS 14 | Working | For development/testing |

### GPU Support

| GPU | Driver | Status |
|-----|--------|--------|
| NVIDIA A100 | 535+ | Working |
| NVIDIA H100 | 535+ | Working |
| NVIDIA RTX 4090 | 535+ | Working |
| AMD | - | Not supported |
| Intel | - | Not supported |

## Comparison with Beam Agent

| Feature | Beam Agent | Python Agent |
|---------|------------|--------------|
| Source | Closed | Open |
| Size | ~70MB | ~1MB (deps) |
| Language | Go (compiled) | Python |
| Tailscale | Required | Optional |
| SSH Tunnel | Not supported | Supported |
| k3s Install | Built-in | Phase 2 |
| GPU Detection | Built-in | Built-in |

## Troubleshooting

### Agent won't start

```bash
# Check Python version
python --version  # Need 3.10+

# Check dependencies
pip list | grep -E "httpx|psutil|click"

# Run with debug
python -m cmd.agent --debug --dry-run ...
```

### Registration fails

```bash
# Test gateway connectivity
curl http://localhost:1994/api/v1/health

# Check token validity (get new one if needed)
beta9 machine create --pool gpu
```

### Keepalive fails with "machine_not_found"

The machine expired. Re-run the agent to re-register:

```bash
python -m cmd.agent --token "..." --pool-name gpu ...
```

## See Also

- [External Workers Guide](../../docs/external-workers/README.md)
- [Registration Flow](../../docs/external-workers/registration-flow.md)
- [Keepalive Requirements](../../docs/external-workers/keepalive-requirements.md)
- [API Reference](../../docs/api-reference/machine-api.md)
