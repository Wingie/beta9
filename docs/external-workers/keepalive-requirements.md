# Keepalive Requirements

Understanding the TTL-based machine lifecycle and keepalive behavior.

## Critical Concept: 5-Minute TTL

Machine state in Beta9 is stored in Redis with a **5-minute TTL** (Time To Live).

```
┌────────────────────────────────────────────────────────────────────┐
│                                                                    │
│  Register ──> Redis key created ──> TTL = 300 seconds             │
│                                                                    │
│  Keepalive ──> TTL refreshed ──> TTL = 300 seconds (reset)        │
│                                                                    │
│  No keepalive for 5 min ──> Key expires ──> Machine GONE          │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

## What Happens on TTL Expiry

When the 5-minute TTL expires without a keepalive:

1. **Redis keys automatically deleted**
   - Machine state key expires
   - Metrics key expires
   - Machine removed from pool index

2. **Gateway detects missing machine**
   - Pool becomes "degraded" (no healthy machines)
   - Logs: "pool is degraded, skipping pool sizing"

3. **Workloads cannot be scheduled**
   - No available machines in pool
   - Pending tasks wait for capacity

4. **Recovery requires re-registration**
   - Agent must call `/register` again
   - Then send keepalive to become "ready"

## Recommended Keepalive Interval

| Setting | Value | Reasoning |
|---------|-------|-----------|
| TTL | 300 seconds | Set by gateway (hardcoded) |
| **Keepalive interval** | **60 seconds** | 5x safety margin |
| Minimum safe | 120 seconds | 2x margin, risky |
| Maximum safe | 240 seconds | 80% of TTL, very risky |

**Use 60-second intervals** to handle:
- Network latency
- Temporary connectivity issues
- Gateway restarts
- Clock drift

## Keepalive Payload

### Minimal (Required Fields Only)

```json
{
  "machine_id": "1165a9b6",
  "pool_name": "gpu"
}
```

### Full (With Metrics)

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
    "total_disk_space_bytes": 500000000000,
    "total_disk_free_bytes": 300000000000,
    "free_gpu_count": 1,
    "worker_count": 2,
    "container_count": 5
  }
}
```

## Metrics Reference

| Field | Type | Description |
|-------|------|-------------|
| cpu_utilization_pct | float | CPU usage 0-100% |
| memory_utilization_pct | float | Memory usage 0-100% |
| total_cpu_available | int | Total CPU millicores |
| total_memory_available | int | Total memory in MB |
| total_disk_space_bytes | int | Total disk space |
| total_disk_free_bytes | int | Free disk space |
| free_gpu_count | int | Available GPUs |
| worker_count | int | Running worker pods |
| container_count | int | Running containers |
| cache_usage_pct | float | Image cache usage |
| cache_capacity | int | Cache capacity bytes |

## Implementation in Python Agent

The agent's keepalive loop:

```python
# From beta9_agent/keepalive.py

async def keepalive_loop(config: AgentConfig, interval: int = 60):
    """Send keepalive every interval seconds."""

    while True:
        try:
            metrics = collect_metrics()  # CPU, memory, GPU

            response = await send_keepalive(
                config=config,
                metrics=metrics
            )

            if response.get("status") == "ok":
                logger.info(f"Keepalive OK, status: {response.get('machine_state', {}).get('status')}")
            else:
                logger.warning(f"Keepalive issue: {response}")

        except Exception as e:
            logger.error(f"Keepalive failed: {e}")
            # Don't exit - keep trying

        await asyncio.sleep(interval)
```

## Monitoring Keepalive Health

### Check Last Keepalive Time

```bash
# Via Redis
kubectl exec -n beta9 $REDIS_POD -- redis-cli hget \
  provider:machine:generic:gpu:1165a9b6 last_keepalive

# Returns Unix timestamp, e.g., 1767804357
```

### Check TTL Remaining

```bash
kubectl exec -n beta9 $REDIS_POD -- redis-cli ttl \
  provider:machine:generic:gpu:1165a9b6

# Returns seconds remaining, e.g., 247
```

### Gateway Logs

```bash
# Successful keepalive
kubectl logs -n beta9 -l app=gateway | grep keepalive

# Look for:
# INF request method=POST path=/api/v1/machine/keepalive status=200
```

## Failure Scenarios

### Scenario 1: Network Blip

```
Timeline:
0:00  - Keepalive sent (TTL = 300s)
1:00  - Keepalive sent (TTL = 300s)
2:00  - Network down, keepalive fails
3:00  - Network down, keepalive fails
4:00  - Network up, keepalive sent (TTL = 300s)

Result: OK - Machine stays registered
```

### Scenario 2: Extended Outage

```
Timeline:
0:00  - Keepalive sent (TTL = 300s)
1:00  - Network down
...
6:00  - TTL expires, machine removed
7:00  - Network up, keepalive fails (machine_not_found)

Result: Must re-register
```

### Scenario 3: Gateway Restart

```
Timeline:
0:00  - Keepalive sent
1:00  - Gateway restarts (takes 30s)
1:30  - Gateway back up
2:00  - Keepalive sent

Result: OK - Redis state persists across gateway restarts
```

## Troubleshooting

### Error: "machine_not_found"

```json
{
  "error_code": "machine_not_found",
  "message": "Machine state expired or never registered",
  "suggestion": "Call POST /api/v1/machine/register before keepalive"
}
```

**Cause**: TTL expired or machine never registered

**Fix**: Re-register the machine, then resume keepalive

### Machine Keeps Disappearing

1. Check agent logs for keepalive errors
2. Verify network connectivity to gateway
3. Check system clock (NTP sync)
4. Increase logging: `--debug` flag

### Gateway Shows Pool Degraded

```
WRN pool is degraded, skipping pool sizing pool_name=gpu
```

**Cause**: No machines in "ready" state

**Fix**: Ensure at least one machine is registered and sending keepalives

## Constants Reference

From `pkg/types/provider.go`:

```go
const (
    MachineStatusRegistered     MachineStatus = "registered"
    MachineStatusPending        MachineStatus = "pending"
    MachineStatusReady          MachineStatus = "ready"
    MachinePendingExpirationS   int = 3600  // 1 hour (pending state)
    MachineKeepaliveExpirationS int = 300   // 5 minutes (registered/ready)
)
```
