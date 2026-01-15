# b9agent

A Go-based agent for connecting external machines to a Beta9 cluster as workers.

## Features

- **Persistent Configuration**: Store credentials in `~/.b9agent/config.yaml`
- **TUI Dashboard**: Real-time status display with job monitoring
- **Job Tracking**: Watch k3s pods to see running/completed jobs
- **Cross-Platform**: Builds for Linux, macOS (Intel & Apple Silicon)

## Installation

### Quick Install (macOS/Linux)

```bash
cd cmd/agent-go
make install
```

This builds the binary and installs it to `/usr/local/bin/b9agent`.

### Manual Build

```bash
make build           # Build for current platform
make build-all       # Build for all platforms
```

### Uninstall

```bash
make uninstall
```

## Usage

### First Time Setup

```bash
# Interactive setup
b9agent init

# Non-interactive setup (for scripts)
b9agent init \
  --gateway 100.72.101.23 \
  --token "your-registration-token" \
  --machine-id "abcd1234" \
  --pool external \
  --hostname "100.x.x.x"
```

### Running the Agent

```bash
# Start with TUI dashboard
b9agent

# Start without TUI (log mode)
b9agent --tui=false

# Run once and exit
b9agent --once
```

### View Configuration

```bash
b9agent config
```

## TUI Dashboard

The agent displays a real-time dashboard showing:

```
╔══ b9agent: 159ecc90 ══════════════════════════════════════════════════════════╗
║ Status: READY │ Gateway: 100.72.101.23 │ Pool: external │ Uptime: 2h 34m      ║
║ CPU: 18.2% │ Memory: 56.1% │ GPUs: 0 │ Last Heartbeat: 3s ago                 ║
╠═══════════════════════════════════════════════════════════════════════════════╣
║ WORKER PODS                                                                   ║
╟───────────────────────────────────────────────────────────────────────────────╢
║ worker-abc123   RUNNING     hello_beta9:hello     12s                         ║
║ worker-def456   COMPLETED   hello_beta9:hello    847ms  (2 min ago)           ║
╚═══════════════════════════════════════════════════════════════════════════════╝
Press Ctrl+C to quit
```

### Status Colors

- **READY** (green): Agent is connected and ready for jobs
- **BUSY** (yellow): Jobs are currently running
- **UNHEALTHY** (red): Heartbeat failures detected

## Configuration

Configuration is stored in `~/.b9agent/config.yaml`:

```yaml
gateway:
  host: "100.72.101.23"
  port: 1994
machine:
  id: "159ecc90"
  token: "your-registration-token"
  hostname: "100.x.x.x"
pool: "external"
k3s:
  token: "k3s-bearer-token"
```

### Configuration Precedence

1. CLI flags (highest priority)
2. Environment variables
3. Config file (`~/.b9agent/config.yaml`)
4. Defaults (lowest priority)

### Environment Variables

| Variable | Description |
|----------|-------------|
| `BETA9_TOKEN` | Registration token |
| `BETA9_MACHINE_ID` | Machine ID |
| `BETA9_GATEWAY_HOST` | Gateway hostname |
| `BETA9_GATEWAY_PORT` | Gateway port (default: 1994) |
| `BETA9_POOL_NAME` | Pool name (default: external) |
| `BETA9_HOSTNAME` | Hostname for gateway to reach k3s |
| `B9AGENT_CONFIG` | Custom config file path |

## Sentry Integration

We strongly recommend using [Sentry](https://sentry.io) for error tracking. It offers a generous free tier and significantly helps in debugging production issues.

The agent, gateway, and SDK are instrumented to report errors if the `SENTRY_DSN` environment variable is set.

To enable it:
1. Create a Sentry project and get your DSN.
2. Add `SENTRY_DSN=https://...@sentry.io/...` to your `.env` file or export it in your shell.
3. Restart the agent/gateway.

## Prerequisites

- **kubectl**: Must be configured to access your local k3s cluster
- **Tailscale** (optional): For secure networking to the gateway
- **Rancher Desktop** or **k3s**: Local Kubernetes cluster

## Development

### Run Tests

```bash
make test
make test-coverage
```

### Clean Build Artifacts

```bash
make clean
```

## Architecture

```
┌─────────────────┐     HTTP/1994      ┌──────────────────┐
│   b9agent       │ ─────────────────> │   Beta9 Gateway  │
│   (external)    │                    │   (OCI k3s)      │
└─────────────────┘                    └──────────────────┘
        │
        │ kubectl
        ▼
┌─────────────────┐
│   Local k3s     │
│   (Rancher)     │
└─────────────────┘
```

The agent:
1. Registers with the gateway via HTTP
2. Sends keepalive heartbeats with metrics
3. Monitors local k3s for worker pods
4. Displays status in the TUI dashboard

## Troubleshooting

### "No config file found"

Run `b9agent init` to create the configuration.

### "Connection refused"

Check that:
1. Gateway is reachable: `curl http://<gateway>:1994/api/v1/health`
2. Tailscale is connected (if using)

### "Keepalive failed"

The gateway may have rejected the token. Get a new token:
```bash
ssh flow "beta9 machine create --pool external"
```

## License

Part of the Beta9 project.
