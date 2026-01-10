# Troubleshooting External Workers

Common issues and solutions for external worker connectivity.

## Quick Diagnostics

```bash
# 1. Check tunnel is working
curl http://localhost:1994/api/v1/health
# Expected: {"status":"ok"}

# 2. Check machine status
beta9 machine list

# 3. Check gateway logs
kubectl logs -n beta9 -l app=gateway --tail=50

# 4. Check Redis for machine state
kubectl exec -n beta9 $REDIS_POD -- redis-cli keys "provider:machine:*"
```

## Registration Issues

### Error: "Invalid token" (403)

**Symptoms:**
```json
{"message": "Invalid token"}
```

**Causes:**
1. Token expired (pending tokens expire after 1 hour)
2. Token already used by different machine
3. Typo in token

**Solutions:**
```bash
# Generate new token
beta9 machine create --pool gpu

# Use new token immediately
python -m beta9_agent --token "NEW_TOKEN" ...
```

### Error: "Invalid pool name" (500)

**Symptoms:**
```json
{"message": "Invalid pool name"}
```

**Causes:**
1. Pool doesn't exist in gateway config
2. Pool name typo

**Solutions:**
```bash
# Check available pools
beta9 pool list

# Or check gateway config
kubectl get configmap -n beta9 gateway-config -o yaml | grep -A 20 "pools:"
```

### Error: "Invalid payload" (400)

**Symptoms:**
```json
{"message": "Invalid payload"}
```

**Causes:**
1. Malformed JSON
2. Missing required fields
3. Wrong data types

**Solutions:**
```bash
# Verify JSON is valid
echo '{"machine_id":"test"}' | jq .

# Check all required fields are present:
# - machine_id, hostname, pool_name, cpu, memory, gpu_count, private_ip
```

## Keepalive Issues

### Error: "machine_not_found" (500)

**Symptoms:**
```json
{
  "error_code": "machine_not_found",
  "message": "Machine state expired or never registered"
}
```

**Causes:**
1. TTL expired (no keepalive for 5+ minutes)
2. Machine never registered
3. Wrong machine_id or pool_name

**Solutions:**
```bash
# Re-register the machine
curl -X POST http://localhost:1994/api/v1/machine/register ...

# Then immediately send keepalive
curl -X POST http://localhost:1994/api/v1/machine/keepalive ...
```

### Machine Keeps Expiring

**Symptoms:**
- Machine repeatedly disappears from `beta9 machine list`
- Logs show repeated registrations

**Causes:**
1. Keepalive interval too long (>5 min)
2. Network interruptions
3. Agent crashing

**Solutions:**
```bash
# Check agent is running
ps aux | grep agent

# Check keepalive interval (should be ~60s)
# In agent logs, look for keepalive timestamps

# Test manual keepalive
curl -X POST http://localhost:1994/api/v1/machine/keepalive \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"machine_id":"YOUR_ID","pool_name":"gpu"}'
```

## Network Issues

### Tunnel Connection Refused

**Symptoms:**
```
curl: (7) Failed to connect to localhost port 1994: Connection refused
```

**Causes:**
1. SSH tunnel not running
2. Gateway not running
3. Wrong port

**Solutions:**
```bash
# Check if tunnel is running
ps aux | grep "ssh.*1994"

# Restart tunnel
ssh -fNL 1994:localhost:31994 gateway-host

# Verify gateway is running
kubectl get pods -n beta9 -l app=gateway
```

### Tunnel Keeps Dropping

**Symptoms:**
- Intermittent connectivity
- Agent logs show connection errors

**Solutions:**
```bash
# Use autossh for auto-reconnect
autossh -M 0 -f -N -L 1994:localhost:31994 gateway-host

# Add SSH keepalive in ~/.ssh/config
Host gateway-host
    ServerAliveInterval 30
    ServerAliveCountMax 3
```

### Gateway Unreachable from Tunnel

**Symptoms:**
- Tunnel connects but health check fails
- `curl: (52) Empty reply from server`

**Causes:**
1. Wrong NodePort
2. Gateway pod not ready
3. Service not exposed

**Solutions:**
```bash
# Check gateway service ports
kubectl get svc -n beta9 gateway

# Check NodePort
kubectl get svc -n beta9 beta9-gateway-nodeport

# Port forward directly to test
kubectl port-forward svc/gateway 1994:1994 -n beta9
curl http://localhost:1994/api/v1/health
```

## Gateway Issues

### Pool Shows "Degraded"

**Symptoms:**
```
WRN pool is degraded, skipping pool sizing pool_name=gpu
```

**Causes:**
1. No machines in "ready" state
2. All machines expired
3. Pool misconfigured

**Solutions:**
```bash
# Check machine status
beta9 machine list

# Check Redis for machines
kubectl exec -n beta9 $REDIS_POD -- redis-cli smembers provider:machine:generic:gpu:machine_index

# Register and keepalive a machine
```

### "provider not implemented" Error

**Symptoms:**
```
ERR failed to add worker error="provider not implemented" pool_name=gpu
```

**Explanation:**
This is **expected** for external pools. External pools expect workers to self-provision via the agent, not be provisioned by the gateway.

The pool is healthy if you see:
```
INF pool is healthy, resuming pool sizing pool_name=gpu
INF using existing machine hostname=... machine_id=...
```

### Gateway Can't Reach Worker k3s

**Symptoms:**
- Workloads don't run on external worker
- Gateway logs show k8s API errors

**Causes:**
1. k3s not installed on worker
2. Reverse tunnel not set up
3. Tailscale not configured

**Solutions:**
```bash
# For SSH tunnel mode: Set up reverse tunnel
ssh -R 6443:localhost:6443 gateway-host

# For Tailscale mode: Verify tailscale is connected
tailscale status
```

## Agent Issues

### Agent Crashes on Startup

**Symptoms:**
- Python traceback
- Missing dependencies

**Solutions:**
```bash
# Install dependencies
pip install -r beta9_agent/requirements.txt

# Check Python version (3.10+ required)
python --version

# Run with debug
python -m beta9_agent --debug ...
```

### Agent Can't Detect GPUs

**Symptoms:**
- gpu_count shows 0
- No NVIDIA devices found

**Solutions:**
```bash
# Check NVIDIA driver
nvidia-smi

# Check CUDA
nvcc --version

# Install nvidia-ml-py for Python detection
pip install nvidia-ml-py
```

## Debugging Commands

### Full System Check

```bash
#!/bin/bash
echo "=== Tunnel Check ==="
curl -s http://localhost:1994/api/v1/health || echo "FAIL: Gateway unreachable"

echo -e "\n=== Gateway Pods ==="
kubectl get pods -n beta9 -l app=gateway

echo -e "\n=== Machine List ==="
beta9 machine list 2>/dev/null || echo "beta9 CLI not available"

echo -e "\n=== Redis Machine Keys ==="
kubectl exec -n beta9 $(kubectl get pods -n beta9 -l app.kubernetes.io/name=redis -o jsonpath='{.items[0].metadata.name}') -- redis-cli keys "provider:machine:*" 2>/dev/null

echo -e "\n=== Gateway Logs (last 20 lines) ==="
kubectl logs -n beta9 -l app=gateway --tail=20 2>/dev/null
```

### Verbose Agent Run

```bash
python -m beta9_agent \
    --token "$TOKEN" \
    --machine-id "$MACHINE_ID" \
    --pool-name gpu \
    --gateway-host localhost \
    --gateway-port 1994 \
    --debug \
    --once
```

### Manual API Test

```bash
# Set variables
TOKEN="your-token-here"
MACHINE_ID="test1234"

# Test register
curl -v -X POST http://localhost:1994/api/v1/machine/register \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"token\": \"$TOKEN\",
    \"machine_id\": \"$MACHINE_ID\",
    \"hostname\": \"test-worker\",
    \"provider_name\": \"generic\",
    \"pool_name\": \"gpu\",
    \"cpu\": \"8000m\",
    \"memory\": \"16Gi\",
    \"gpu_count\": \"1\",
    \"private_ip\": \"192.168.1.100\"
  }"

# Test keepalive
curl -v -X POST http://localhost:1994/api/v1/machine/keepalive \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"machine_id\": \"$MACHINE_ID\",
    \"pool_name\": \"gpu\",
    \"agent_version\": \"test\"
  }"
```
