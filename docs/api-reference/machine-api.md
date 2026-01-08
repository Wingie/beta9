# Machine API Reference

REST API endpoints for machine registration and management.

## Base URL

```
/api/v1/machine
```

## Authentication

All endpoints require Bearer token authentication:

```
Authorization: Bearer <machine_token>
```

Token types accepted: `TokenTypeMachine` or `TokenTypeWorker`

Tokens are generated via `beta9 machine create --pool <pool_name>`.

---

## Endpoints

### POST /register

Register a machine with the gateway.

**URL:** `POST /api/v1/machine/register`

**Headers:**
```
Authorization: Bearer <machine_token>
Content-Type: application/json
```

**Request Body:**

```json
{
  "token": "swwMV_WA4C...",
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

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| token | string | Yes | Machine token from `beta9 machine create` |
| machine_id | string | Yes | 8 character hexadecimal identifier |
| hostname | string | Yes | Machine hostname for identification |
| provider_name | string | No | Provider identifier (default: "generic") |
| pool_name | string | Yes | Worker pool name (must exist in config) |
| cpu | string | Yes | CPU capacity in millicores (e.g., "8000m" = 8 cores) |
| memory | string | Yes | Memory capacity (e.g., "16Gi", "32768Mi") |
| gpu_count | string | Yes | Number of GPUs (e.g., "1", "0") |
| private_ip | string | Yes | Machine's private IP address |

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

| Field | Type | Description |
|-------|------|-------------|
| config | object\|null | Remote config (null for SSH tunnel mode) |
| machine_state.machine_id | string | Registered machine ID |
| machine_state.status | string | Current status ("registered") |
| machine_state.pool_name | string | Pool name |
| machine_state.ttl_seconds | int | Seconds until expiry without keepalive |

**Error Responses:**

| Code | Message | Cause |
|------|---------|-------|
| 400 | Invalid payload | Malformed JSON or missing required fields |
| 403 | Invalid token | Token doesn't exist, expired, or wrong type |
| 500 | Invalid machine cpu value | CPU format invalid (use "8000m") |
| 500 | Invalid machine memory value | Memory format invalid (use "16Gi") |
| 500 | Invalid gpu count | gpu_count not a valid integer string |
| 500 | Invalid pool name | Pool doesn't exist in gateway config |
| 500 | Failed to register machine | Redis error |

**Example:**

```bash
curl -X POST http://localhost:1994/api/v1/machine/register \
  -H "Authorization: Bearer swwMV_WA4C_FxN-_UY5UiRgdg2dTM1GUDLVUzO9c6fzyOs0BUJuXKLfxb-QcZkwAnlOGscu0U3WYmF1eQpcSBg==" \
  -H "Content-Type: application/json" \
  -d '{
    "token": "swwMV_WA4C_FxN-_UY5UiRgdg2dTM1GUDLVUzO9c6fzyOs0BUJuXKLfxb-QcZkwAnlOGscu0U3WYmF1eQpcSBg==",
    "machine_id": "1165a9b6",
    "hostname": "gpu-worker-1",
    "provider_name": "generic",
    "pool_name": "gpu",
    "cpu": "8000m",
    "memory": "16Gi",
    "gpu_count": "1",
    "private_ip": "192.168.1.100"
  }'
```

---

### POST /keepalive

Send heartbeat to maintain machine in "ready" state.

**URL:** `POST /api/v1/machine/keepalive`

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

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| machine_id | string | Yes | Machine ID from registration |
| provider_name | string | No | Provider identifier (default: "generic") |
| pool_name | string | Yes | Worker pool name |
| agent_version | string | No | Agent version for tracking |
| metrics | object | No | Current system metrics |

**Metrics Object:**

| Field | Type | Description |
|-------|------|-------------|
| cpu_utilization_pct | float | Current CPU usage (0-100) |
| memory_utilization_pct | float | Current memory usage (0-100) |
| total_cpu_available | int | Total CPU in millicores |
| total_memory_available | int | Total memory in MB |
| total_disk_space_bytes | int | Total disk space in bytes |
| total_disk_free_bytes | int | Free disk space in bytes |
| free_gpu_count | int | Number of available GPUs |
| worker_count | int | Number of running workers |
| container_count | int | Number of running containers |
| cache_usage_pct | float | Image cache usage (0-100) |
| cache_capacity | int | Cache capacity in bytes |
| cache_memory_usage | int | Cache memory usage in bytes |
| cache_cpu_usage | float | Cache CPU usage |

**Response (200 OK):**

```json
{
  "status": "ok",
  "machine_state": {
    "machine_id": "1165a9b6",
    "status": "ready",
    "last_keepalive": "1767804357",
    "last_worker_seen": "",
    "ttl_seconds": 300,
    "agent_version": "0.1.0-python"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| status | string | "ok" on success |
| machine_state.machine_id | string | Machine ID |
| machine_state.status | string | Current status ("ready" after first keepalive) |
| machine_state.last_keepalive | string | Unix timestamp of last keepalive |
| machine_state.last_worker_seen | string | Unix timestamp of last worker activity |
| machine_state.ttl_seconds | int | TTL (always 300) |
| machine_state.agent_version | string | Reported agent version |

**Error Responses:**

| Code | Error Code | Message | Cause |
|------|------------|---------|-------|
| 400 | - | machine_id and pool_name are required | Missing required fields |
| 403 | invalid_token | Invalid token | Token invalid or expired |
| 500 | machine_not_found | Machine state expired or never registered | TTL expired |

**Example:**

```bash
curl -X POST http://localhost:1994/api/v1/machine/keepalive \
  -H "Authorization: Bearer swwMV_WA4C_FxN-_UY5UiRgdg2dTM1GUDLVUzO9c6fzyOs0BUJuXKLfxb-QcZkwAnlOGscu0U3WYmF1eQpcSBg==" \
  -H "Content-Type: application/json" \
  -d '{
    "machine_id": "1165a9b6",
    "pool_name": "gpu",
    "agent_version": "0.1.0-python",
    "metrics": {
      "cpu_utilization_pct": 15.5,
      "memory_utilization_pct": 42.3
    }
  }'
```

---

### GET /config

Get gateway configuration for workers.

**URL:** `GET /api/v1/machine/config`

**Headers:**
```
Authorization: Bearer <machine_token>
```

**Response (200 OK):**

```json
{
  "config": {
    "gateway": {
      "host": "gateway.tailnet.internal",
      "grpc_port": 1993,
      "http_port": 1994
    },
    "redis": {
      "addrs": ["redis-master.tailnet.internal:6379"]
    }
  }
}
```

**Note:** Returns null config if Tailscale is not configured.

---

### GET /list

List machines in a pool.

**URL:** `GET /api/v1/machine/list?pool_name=<pool>&provider_name=<provider>`

**Headers:**
```
Authorization: Bearer <machine_token>
```

**Query Parameters:**

| Parameter | Required | Description |
|-----------|----------|-------------|
| pool_name | No | Filter by pool name |
| provider_name | No | Filter by provider name |

**Response (200 OK):**

```json
[
  {
    "state": {
      "machine_id": "1165a9b6",
      "pool_name": "gpu",
      "status": "ready",
      "hostname": "gpu-worker-1.beta9.headscale.internal",
      "cpu": 8000,
      "memory": 16384,
      "gpu_count": 1,
      "agent_version": "0.1.0-python"
    },
    "metrics": {
      "cpu_utilization_pct": 15.5,
      "memory_utilization_pct": 42.3,
      "free_gpu_count": 1
    }
  }
]
```

Only returns machines with status "ready".

---

### GET /:workspaceId/gpus

Get GPU counts for a workspace.

**URL:** `GET /api/v1/machine/:workspaceId/gpus`

**Headers:**
```
Authorization: Bearer <workspace_token>
```

**Note:** Requires workspace authentication, not machine token.

**Response (200 OK):**

```json
{
  "A100": 4,
  "H100": 2,
  "RTX4090": 0
}
```

---

## Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad Request - Invalid input |
| 403 | Forbidden - Invalid or missing token |
| 404 | Not Found - Endpoint doesn't exist |
| 500 | Internal Server Error - Server-side error |

## Rate Limits

No explicit rate limits, but recommended:

| Operation | Recommended Interval |
|-----------|---------------------|
| Register | Once per machine startup |
| Keepalive | Every 60 seconds |
| List | As needed |
| Config | Once per machine startup |

## Machine States

| State | Description | TTL |
|-------|-------------|-----|
| pending | Token created, awaiting registration | 1 hour |
| registered | Machine registered, awaiting first keepalive | 5 min |
| ready | Machine active and accepting workloads | 5 min (refreshed) |

## Constants

| Constant | Value | Location |
|----------|-------|----------|
| MachinePendingExpirationS | 3600 (1 hour) | pkg/types/provider.go |
| MachineKeepaliveExpirationS | 300 (5 min) | pkg/types/provider.go |

## OpenAPI Schema

The OpenAPI/Swagger schema is available at `docs/openapi/`.

Note: The keepalive endpoint was added in the FlowState fork and may not be in the upstream OpenAPI spec.
