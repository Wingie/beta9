# Self-Hosting Beta9

This guide covers deploying Beta9 on your own infrastructure for running serverless GPU workloads.

## What is Beta9?

Beta9 is an open-source runtime for serverless AI workloads. It provides:

- **Fast container startup** - Custom runtime launches containers in under a second
- **Auto-scaling** - Scale from zero to hundreds of containers
- **GPU support** - Run on cloud GPUs or bring your own
- **Distributed storage** - JuiceFS-backed volume mounts

## Architecture Overview

```
                    ┌─────────────────────────────────────────────┐
                    │              Beta9 Gateway                  │
                    │  (HTTP/gRPC API, Scheduler, Pool Manager)  │
                    └─────────────────┬───────────────────────────┘
                                      │
           ┌──────────────────────────┼──────────────────────────┐
           │                          │                          │
           ▼                          ▼                          ▼
    ┌─────────────┐           ┌─────────────┐           ┌─────────────┐
    │ PostgreSQL  │           │    Redis    │           │   JuiceFS   │
    │  (metadata) │           │   (state)   │           │  (storage)  │
    └─────────────┘           └─────────────┘           └─────────────┘
                                      │
           ┌──────────────────────────┼──────────────────────────┐
           │                          │                          │
           ▼                          ▼                          ▼
    ┌─────────────┐           ┌─────────────┐           ┌─────────────┐
    │ Local Pool  │           │External Pool│           │  GPU Pool   │
    │  (k3s pods) │           │(SSH tunnel) │           │(Lambda/OCI) │
    └─────────────┘           └─────────────┘           └─────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes cluster (k3s recommended) or k3d for local development
- Helm 3.x
- 4GB RAM minimum, 8GB recommended

### Install with Helm

```bash
# Add the Beta9 Helm repository (if published)
# Or use local chart from deploy/charts/beta9/

# Create namespace
kubectl create namespace beta9

# Install with default values
helm install beta9 ./deploy/charts/beta9 -n beta9

# Or with custom values
helm install beta9 ./deploy/charts/beta9 -n beta9 -f my-values.yaml
```

### Verify Installation

```bash
# Check pods are running
kubectl get pods -n beta9

# Expected output:
# NAME                       READY   STATUS    RESTARTS   AGE
# gateway-xxx                1/1     Running   0          1m
# postgresql-0               1/1     Running   0          1m
# redis-master-0             1/1     Running   0          1m

# Check gateway health
kubectl port-forward svc/gateway 1994:1994 -n beta9
curl http://localhost:1994/api/v1/health
# {"status":"ok"}
```

## Deployment Options

### Local Development (k3d)

For local testing and development:

```bash
# Create k3d cluster with local registry
k3d cluster create beta9 --registry-create registry.localhost:5000

# Apply local kustomize overlay
kubectl apply -k manifests/kustomize/overlays/cluster-dev
```

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for full local development setup.

### Single Node Production

For small deployments or testing:

1. Install k3s on a Linux server
2. Deploy Beta9 with Helm
3. Configure external access via NodePort or LoadBalancer

### Multi-Node Production

For production workloads:

1. Set up k8s cluster with multiple nodes
2. Configure persistent storage (Longhorn, EBS, etc.)
3. Deploy Beta9 with production values
4. Set up monitoring with Prometheus/Grafana

## Infrastructure Requirements

See [requirements.md](requirements.md) for detailed infrastructure specifications.

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| PostgreSQL | 13+ | 15+ |
| Redis | 6+ | 7+ |
| Kubernetes | 1.28+ | 1.29+ |
| Memory | 4GB | 8GB+ |
| Storage | 50GB | 100GB+ |

## Configuration

The gateway is configured via `config.yaml`. Key sections:

```yaml
gateway:
  http:
    port: 1994
    externalPort: 1994
  grpc:
    port: 1993
    externalPort: 1993

database:
  postgres:
    host: postgresql
    port: 5432
    name: beta9

redis:
  mode: single
  addrs:
    - redis-master:6379

worker:
  pools:
    default:
      mode: local
      poolSizing:
        defaultWorkerCpu: 1000m
        defaultWorkerMemory: 1Gi
```

See the [Helm values](../../deploy/charts/beta9/values.yaml) for all configuration options.

## Next Steps

- [Infrastructure Requirements](requirements.md) - Detailed specs
- [External Workers](../external-workers/README.md) - Connect GPU machines
- [API Reference](../api-reference/machine-api.md) - Machine management API
- [FlowState Fork](../flowstate-fork.md) - Fork-specific features
