# Infrastructure Requirements

Detailed specifications for self-hosting Beta9.

## Control Plane Components

| Component | Minimum Version | Recommended | Purpose |
|-----------|-----------------|-------------|---------|
| PostgreSQL | 13+ | 15+ | Metadata storage (users, stubs, deployments) |
| Redis | 6+ | 7+ | State management, pub/sub, caching |
| JuiceFS | 1.0+ | 1.1+ | Distributed filesystem for volumes |
| k3s/k8s | 1.28+ | 1.29+ | Container orchestration |

## Compute Requirements

### Gateway Node

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 2 cores | 4 cores |
| Memory | 4GB | 8GB |
| Disk | 20GB | 50GB |

### Worker Nodes (Local Pools)

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 2 cores | 8+ cores |
| Memory | 4GB | 16GB+ |
| Disk | 50GB | 100GB+ |

### External GPU Workers

| Resource | Minimum | Notes |
|----------|---------|-------|
| CPU | 4 cores | For k3s overhead |
| Memory | 8GB | Plus GPU memory |
| GPU | 1x | NVIDIA with drivers |
| Network | 100Mbps | SSH tunnel to gateway |

## Network Requirements

### Ports

| Port | Protocol | Service | Access |
|------|----------|---------|--------|
| 1993 | TCP | gRPC API | Internal/External |
| 1994 | TCP | HTTP API | Internal/External |
| 6443 | TCP | k3s API | External workers only |
| 5432 | TCP | PostgreSQL | Internal only |
| 6379 | TCP | Redis | Internal only |
| 9090 | TCP | Metrics | Internal only |

### NodePort Mappings (k3s)

| Service | Internal Port | NodePort |
|---------|---------------|----------|
| Gateway HTTP | 1994 | 31994 |
| Gateway gRPC | 1993 | 31993 |
| LocalStack S3 | 4566 | 31566 |

## Storage Requirements

### Persistent Volumes

| PVC | Size | Purpose |
|-----|------|---------|
| PostgreSQL | 10Gi | Database storage |
| Redis | 1Gi | State persistence |
| JuiceFS Redis | 1Gi | Filesystem metadata |
| Container Images | 50Gi+ | Local image cache |

### JuiceFS Backend

JuiceFS requires object storage for data. Options:

- **LocalStack S3** - Development/testing
- **AWS S3** - Production
- **MinIO** - Self-hosted alternative
- **Any S3-compatible** - Backblaze B2, Wasabi, etc.

## Database Schema

PostgreSQL tables (see [datamodels/backend.md](../datamodels/backend.md)):

- `workspace` - Multi-tenancy
- `token` - API authentication
- `object` - File/image storage
- `stub` - Function definitions
- `deployment` - Running deployments
- `task` - Background jobs
- `volume` - Persistent volumes
- `secret` - Encrypted secrets

## Redis Keys

Key prefixes used by Beta9:

| Prefix | Purpose | TTL |
|--------|---------|-----|
| `provider:machine:*` | Machine state | 300s (5 min) |
| `provider:machine:*:metrics` | Machine metrics | 300s |
| `worker:*` | Worker state | 60s |
| `container:*` | Container state | Variable |
| `scheduler:*` | Scheduling locks | Variable |

## Scaling Considerations

### Single Node (Development)

- All components on one machine
- k3d or single k3s node
- LocalStack for S3
- Suitable for testing and small workloads

### Multi-Node (Production)

- Dedicated database nodes
- Multiple gateway replicas behind load balancer
- Separate worker pools for different GPU types
- External S3 storage (AWS, MinIO)
- Monitoring with Prometheus/Grafana

### High Availability

For HA deployments:

- PostgreSQL with streaming replication
- Redis Sentinel or Redis Cluster
- Multiple gateway replicas
- Network load balancer
- Cross-zone worker distribution

## External Worker Requirements

For connecting external GPU machines:

| Requirement | Details |
|-------------|---------|
| OS | Linux (Debian/Ubuntu recommended) |
| Python | 3.10+ |
| NVIDIA Drivers | Latest stable |
| CUDA | Compatible with your GPU |
| Network | SSH access to gateway host OR Tailscale |
| k3s | v1.28.5+k3s1 (auto-installed by agent) |

See [External Workers Guide](../external-workers/README.md) for setup instructions.
