# macOS GPU (Metal/MPS) Limitations

## Overview

Beta9 supports Apple Silicon (M1/M2/M3/M4) as external workers, but there are critical limitations when running containerized workloads on macOS.

## Critical Limitation: Containers Cannot Access Metal GPUs

**Docker/containers on macOS cannot access Metal GPUs.** This is a fundamental architectural limitation, not a missing feature.

Metal (Apple's GPU framework) requires direct hardware access, which Linux containers running in a virtual machine cannot obtain. This affects:

- Docker Desktop on macOS
- Rancher Desktop
- k3s/k3d workers on Mac
- Any container runtime on macOS

### Impact

When running Beta9 workers as containers on macOS:
- GPU acceleration is **not available**
- Workers operate in CPU-only mode
- MPS/Metal backend cannot be used
- gVisor sandboxing works but without GPU

## Current Status

### What Works
- CPU-based tasks on Mac external workers
- Container sandboxing (gVisor/runc)
- JuiceFS filesystem access
- Standard Beta9 function execution

### What Doesn't Work
- `@function(gpu="MPS")` or `@function(gpu="M1")` jobs
- PyTorch MPS backend in containers
- MLX in containers
- Any Metal-accelerated code in containers

## Workarounds

### Option 1: Run Native (No Container)
For GPU-accelerated tasks on Mac, run the code natively on macOS (not in a container):

```bash
# Native Python with MPS access
python my_mps_script.py
```

### Option 2: Use Ray for Heterogeneous Clusters
Ray can coordinate between different hardware types, handling MPS + CUDA workers:

```python
import ray

@ray.remote(num_gpus=1, accelerator_type="MPS")
def mps_task():
    import torch
    device = torch.device("mps")
    # Metal GPU accessible
```

See: [Ray on Mac Minis](https://www.doppler.com/blog/building-a-distributed-ai-system-how-to-set-up-ray-and-vllm-on-mac-minis)

### Option 3: Use MLX (Apple Native)
Apple's MLX framework is designed for Apple Silicon and has built-in distributed support:

```python
import mlx.core as mx
# Uses Metal automatically
```

See: [MLX Documentation](https://github.com/ml-explore/mlx)

## Future: Native Worker Mode

We're planning a native worker mode for macOS that:
1. Runs as a native macOS daemon (not containerized)
2. Has full Metal GPU access
3. Connects to Beta9 gateway via gRPC
4. Supports `@function(gpu="MPS")` jobs

See: `wip-specs/mps-native-worker.md` for the design document.

## References

- [PyTorch MPS Backend](https://docs.pytorch.org/docs/stable/notes/mps.html)
- [Docker macOS GPU Limitations](https://addspice.net/overcoming-gpu-access-issues-with-docker-on-macos/)
- [Apple Silicon vs NVIDIA CUDA 2025](https://scalastic.io/en/apple-silicon-vs-nvidia-cuda-ai-2025/)
- [Being GPU Poor - Heterogeneous Training](https://www.dilawar.ai/2025/07/04/Multi-Cluster%20Distributed%20Training%20on%20Heterogeneous%20Hardware/)
