# FlowState Fork Differences

This document describes modifications made to the upstream [beam-cloud/beta9](https://github.com/beam-cloud/beta9) repository.

## Why We Forked

### 1. Closed-Source Agent Dependency

The upstream Beta9 requires a proprietary agent binary:

```bash
# From pkg/providers/ec2.go
curl -L -o agent https://release.beam.cloud/agent/agent
```

- Binary size: ~70MB
- Not open source
- No visibility into what it does
- Cannot modify or extend

### 2. Mandatory Tailscale VPN

The upstream architecture assumes Tailscale VPN for network connectivity:

- Requires Tailscale account
- Requires control server (tailscale.com or headscale)
- Complex network setup
- Not suitable for all environments

### Our Use Case

We needed to:
- Connect external GPU workers (Lambda Labs, local machines) to self-hosted Beta9
- Use simple SSH tunnels instead of VPN
- Have full visibility and control over the agent code
- Work in environments where VPN is not feasible

---

## Changes Made

### 1. Added Machine Keepalive Endpoint

**File:** `pkg/api/v1/machine.go`

**Problem:** The upstream gateway has a `SetMachineKeepAlive()` function in the repository layer, but **no HTTP endpoint** to call it. This means external agents cannot transition machines from "registered" to "ready" status.

**Solution:** Added `POST /api/v1/machine/keepalive` endpoint.

```go
// Added route in NewMachineGroup()
g.POST("/keepalive", group.MachineKeepalive)

// Added handler
func (g *MachineGroup) MachineKeepalive(ctx echo.Context) error {
    // Validates token
    // Calls providerRepo.SetMachineKeepAlive()
    // Returns machine state
}
```

**Behavior:**
- Accepts machine_id, pool_name, agent_version, metrics
- Transitions machine status to "ready"
- Refreshes 5-minute TTL
- Stores metrics in Redis

### 2. Fixed Tailscale Dependency in Registration

**File:** `pkg/api/v1/machine.go`

**Problem:** `RegisterMachine()` called `GetRemoteConfig()` which requires Tailscale to resolve Redis hostnames via Tailscale DNS. For SSH tunnel workers without Tailscale, registration failed with:

```
"Unable to create remote config"
```

**Solution:** Return null config instead of failing.

```go
// Before
remoteConfig, err := providers.GetRemoteConfig(g.config, g.tailscale)
if err != nil {
    return HTTPInternalServerError("Unable to create remote config")
}

// After
remoteConfig, err := providers.GetRemoteConfig(g.config, g.tailscale)
if err != nil {
    // Return nil config - external workers via SSH tunnel don't need it
    remoteConfig = nil
}
```

**Impact:** SSH tunnel workers can now register without Tailscale. They receive `config: null` which is fine since they connect via tunnel, not Tailscale.

### 3. Added Python Agent

**Location:** `beta9_agent/`

**Purpose:** Open-source replacement for the closed-source Beam agent binary.

**Files:**
```
beta9_agent/
├── __init__.py
├── main.py            # Entry point, CLI
├── config.py          # Configuration
├── registration.py    # POST /register
├── keepalive.py       # Keepalive loop
├── metrics.py         # System metrics
├── utils.py           # Utilities
└── requirements.txt   # Dependencies
```

**Features:**
- HTTP-based registration (not gRPC)
- SSH tunnel connectivity
- System metrics collection
- Graceful shutdown
- Debug mode

### 4. Enhanced API Responses

**File:** `pkg/api/v1/machine.go`

**Change:** API endpoints now return structured `machine_state` objects.

```json
// Register response
{
  "config": null,
  "machine_state": {
    "machine_id": "1165a9b6",
    "status": "registered",
    "ttl_seconds": 300
  }
}

// Keepalive response
{
  "status": "ok",
  "machine_state": {
    "machine_id": "1165a9b6",
    "status": "ready",
    "last_keepalive": "1767804357"
  }
}
```

---

## Upstream Compatibility

### What Still Works

- **Tailscale mode** - If Tailscale is configured, everything works as upstream
- **Original agent** - The closed-source agent still works with our gateway
- **SDK** - Python SDK fully compatible
- **Cloud providers** - EC2, OCI, Lambda Labs providers unchanged

### What's Different

| Feature | Upstream | FlowState Fork |
|---------|----------|----------------|
| Agent | Closed-source binary | Python agent available |
| Network | Tailscale required | SSH tunnel supported |
| Keepalive | No endpoint | Endpoint added |
| Registration | Fails without Tailscale | Works with null config |
| API responses | Minimal | Enhanced with state info |

---

## Migration from Upstream

If you're running upstream Beta9:

### 1. Pull Fork Changes

```bash
git remote add flowstate https://github.com/Wingie/flowstate-agents.git
git fetch flowstate
git cherry-pick <keepalive-commit>
git cherry-pick <registration-fix-commit>
```

### 2. Rebuild Gateway

```bash
cd backend/beta9
podman build . -f docker/Dockerfile.gateway -t your-registry/beta9-gateway:latest
podman push your-registry/beta9-gateway:latest
kubectl rollout restart deployment gateway -n beta9
```

### 3. (Optional) Use Python Agent

```bash
cd backend/beta9/beta9_agent
pip install -r requirements.txt
python -m beta9_agent --token ... --pool-name gpu
```

---

## Contributing Back

These changes could be contributed upstream as:

1. **PR: Add keepalive HTTP endpoint**
   - Enables third-party agents
   - Documents the endpoint
   - Adds OpenAPI spec

2. **PR: Make Tailscale optional in registration**
   - Returns null config when unavailable
   - Documents SSH tunnel mode
   - No breaking changes

3. **PR: Add Python agent reference implementation**
   - Alternative to closed-source binary
   - Educational value
   - Community contribution

---

## Files Modified

| File | Change |
|------|--------|
| `pkg/api/v1/machine.go` | Added keepalive endpoint, fixed Tailscale error |
| `pkg/api/v1/machine_types.go` | New file: response types |

## Files Added

| File | Purpose |
|------|---------|
| `beta9_agent/*` | Python agent implementation |
| `docs/self-hosting/*` | Self-hosting documentation |
| `docs/external-workers/*` | External worker documentation |
| `docs/api-reference/machine-api.md` | API documentation |
| `docs/flowstate-fork.md` | This document |

---

## Version Tracking

| Date | Change | Commit |
|------|--------|--------|
| 2025-01-07 | Added keepalive endpoint | `pkg/api/v1/machine.go` |
| 2025-01-07 | Fixed Tailscale dependency | `pkg/api/v1/machine.go` |
| 2025-01-08 | Added documentation | `docs/*` |
| 2025-01-08 | Enhanced API responses | `pkg/api/v1/machine.go` |

---

## Contact

For questions about the fork:
- GitHub: https://github.com/Wingie/flowstate-agents
- Issues: Create an issue in the repository
