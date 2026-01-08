# Machine Registration Flow

Understanding the machine lifecycle and registration API.

## State Machine

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│    ┌─────────┐         ┌────────────┐         ┌─────────────────┐  │
│    │ pending │ ──────> │ registered │ ──────> │     ready       │  │
│    └─────────┘         └────────────┘         └─────────────────┘  │
│         │                    │                        │            │
│         │                    │                        │            │
│    beta9 machine         POST                   POST /keepalive    │
│    create               /register               (first one)        │
│                                                                     │
│    TTL: 1 hour          TTL: 5 min              TTL: 5 min         │
│    (pending expiry)     (keepalive expiry)      (refreshed)        │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## States Explained

### Pending

- Created by `beta9 machine create`
- Token generated, waiting for agent to register
- **TTL**: 1 hour (`MachinePendingExpirationS = 3600`)
- If not registered within 1 hour, entry expires

### Registered

- Agent called `POST /api/v1/machine/register`
- Machine specs recorded (CPU, memory, GPU)
- **TTL**: 5 minutes (`MachineKeepaliveExpirationS = 300`)
- Must send keepalive to transition to "ready"

### Ready

- Agent sent first `POST /api/v1/machine/keepalive`
- Machine available for workload scheduling
- **TTL**: 5 minutes (refreshed with each keepalive)
- Gateway can deploy worker pods to this machine

## Registration API

### POST /api/v1/machine/register

Register a machine with the gateway.

**Headers:**
```
Authorization: Bearer <machine_token>
Content-Type: application/json
```

**Request Body:**
```json
{
  "token": "<machine_token>",
  "machine_id": "1165a9b6",
  "hostname": "gpu-worker-1",
  "provider_name": "generic",
  "pool_name": "gpu",
  "cpu": "8000m",
  "memory": "16Gi",
  "gpu_count": "1",
  "private_ip": "192.168.1.100"
}
```

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| token | string | yes | Machine token from `beta9 machine create` |
| machine_id | string | yes | 8 character hex ID |
| hostname | string | yes | Machine hostname for identification |
| provider_name | string | no | Default: "generic" |
| pool_name | string | yes | Worker pool name |
| cpu | string | yes | CPU in millicores (e.g., "8000m" = 8 cores) |
| memory | string | yes | Memory size (e.g., "16Gi") |
| gpu_count | string | yes | Number of GPUs |
| private_ip | string | yes | Machine's private IP address |

**Response (200 OK):**
```json
{
  "config": null,
  "machine_state": {
    "machine_id": "1165a9b6",
    "status": "registered",
    "pool_name": "gpu",
    "ttl_seconds": 300
  }
}
```

**Note:** `config: null` is normal for SSH tunnel mode. Tailscale mode returns remote configuration.

**Errors:**

| Code | Message | Cause |
|------|---------|-------|
| 400 | Invalid payload | Malformed JSON or missing fields |
| 403 | Invalid token | Wrong or expired token |
| 500 | Invalid pool name | Pool doesn't exist in config |
| 500 | Failed to register machine | Redis error |

## Keepalive API

### POST /api/v1/machine/keepalive

Send heartbeat to maintain "ready" status.

**Headers:**
```
Authorization: Bearer <machine_token>
Content-Type: application/json
```

**Request Body:**
```json
{
  "machine_id": "1165a9b6",
  "provider_name": "generic",
  "pool_name": "gpu",
  "agent_version": "0.1.0-python",
  "metrics": {
    "cpu_utilization_pct": 15.5,
    "memory_utilization_pct": 42.3,
    "total_cpu_available": 8000,
    "total_memory_available": 16384,
    "free_gpu_count": 1
  }
}
```

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| machine_id | string | yes | Machine ID from registration |
| provider_name | string | no | Default: "generic" |
| pool_name | string | yes | Worker pool name |
| agent_version | string | no | Agent version for tracking |
| metrics | object | no | System metrics |

**Metrics Object:**

| Field | Type | Description |
|-------|------|-------------|
| cpu_utilization_pct | float | CPU usage percentage |
| memory_utilization_pct | float | Memory usage percentage |
| total_cpu_available | int | Total CPU millicores |
| total_memory_available | int | Total memory MB |
| free_gpu_count | int | Available GPUs |

**Response (200 OK):**
```json
{
  "status": "ok",
  "machine_state": {
    "machine_id": "1165a9b6",
    "status": "ready",
    "last_keepalive": "1767804357",
    "ttl_seconds": 300,
    "agent_version": "0.1.0-python"
  }
}
```

**Errors:**

| Code | Error Code | Message | Cause |
|------|------------|---------|-------|
| 400 | - | machine_id and pool_name are required | Missing fields |
| 403 | invalid_token | Invalid token | Wrong token |
| 500 | machine_not_found | Machine state expired | TTL expired, re-register |

## Complete Registration Example

```bash
# Step 1: Create machine (on gateway)
beta9 machine create --pool gpu
# Token: abc123...
# Machine ID: 1165a9b6

# Step 2: Register (from worker, through tunnel)
curl -X POST http://localhost:1994/api/v1/machine/register \
  -H "Authorization: Bearer abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "token": "abc123...",
    "machine_id": "1165a9b6",
    "hostname": "my-gpu-worker",
    "provider_name": "generic",
    "pool_name": "gpu",
    "cpu": "8000m",
    "memory": "16Gi",
    "gpu_count": "1",
    "private_ip": "192.168.1.100"
  }'

# Response: {"config":null,"machine_state":{"machine_id":"1165a9b6","status":"registered",...}}

# Step 3: First keepalive (transitions to ready)
curl -X POST http://localhost:1994/api/v1/machine/keepalive \
  -H "Authorization: Bearer abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "machine_id": "1165a9b6",
    "pool_name": "gpu",
    "agent_version": "0.1.0"
  }'

# Response: {"status":"ok","machine_state":{"status":"ready",...}}

# Step 4: Verify (on gateway)
beta9 machine list
# Shows status: ready
```

## Redis Keys

Machine state is stored in Redis with these keys:

```
provider:machine:{provider}:{pool}:{id}          # Hash: machine state
provider:machine:{provider}:{pool}:{id}:metrics  # Hash: machine metrics
provider:machine:{provider}:{pool}:{id}:lock     # Lock for concurrency
provider:machine:{provider}:{pool}:machine_index # Set: all machines in pool
```

Example:
```
provider:machine:generic:gpu:1165a9b6
provider:machine:generic:gpu:1165a9b6:metrics
provider:machine:generic:gpu:machine_index
```

## Checking Machine State

```bash
# Via beta9 CLI
beta9 machine list

# Via Redis directly
kubectl exec -n beta9 $(kubectl get pods -n beta9 -l app.kubernetes.io/name=redis -o jsonpath='{.items[0].metadata.name}') -- \
  redis-cli hgetall provider:machine:generic:gpu:1165a9b6
```

## Gateway Logs

Registration events appear in gateway logs:

```
# Successful registration
INF request method=POST path=/api/v1/machine/register status=200

# Pool becomes healthy
INF pool is healthy, resuming pool sizing pool_name=gpu

# Machine detected
INF using existing machine hostname=my-gpu-worker.beta9.headscale.internal machine_id=1165a9b6
```
