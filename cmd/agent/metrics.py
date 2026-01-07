"""System metrics collection for Beta9 agent.

Collects metrics matching the ProviderMachineMetrics struct:
- CPU: total available (millicores), utilization %
- Memory: total available (bytes), utilization %
- Disk: total space, free space
- GPU: count of free GPUs
"""

import logging
import subprocess
from dataclasses import dataclass
from typing import Optional

try:
    import psutil
except ImportError:
    psutil = None

logger = logging.getLogger("beta9-agent")


@dataclass
class MachineMetrics:
    """System metrics for keepalive updates.

    Matches pkg/types/provider.go ProviderMachineMetrics struct.
    """
    total_cpu_available: int      # millicores (e.g., 8000 for 8 cores)
    total_memory_available: int   # bytes
    total_disk_space_bytes: int
    cpu_utilization_pct: float
    memory_utilization_pct: float
    total_disk_free_bytes: int
    worker_count: int = 0
    container_count: int = 0
    free_gpu_count: int = 0
    cache_usage_pct: float = 0.0
    cache_capacity: int = 0
    cache_memory_usage: int = 0
    cache_cpu_usage: float = 0.0

    def to_dict(self) -> dict:
        """Convert to API-compatible dict."""
        return {
            "total_cpu_available": self.total_cpu_available,
            "total_memory_available": self.total_memory_available,
            "total_disk_space_bytes": self.total_disk_space_bytes,
            "cpu_utilization_pct": self.cpu_utilization_pct,
            "memory_utilization_pct": self.memory_utilization_pct,
            "total_disk_free_bytes": self.total_disk_free_bytes,
            "worker_count": self.worker_count,
            "container_count": self.container_count,
            "free_gpu_count": self.free_gpu_count,
            "cache_usage_pct": self.cache_usage_pct,
            "cache_capacity": self.cache_capacity,
            "cache_memory_usage": self.cache_memory_usage,
            "cache_cpu_usage": self.cache_cpu_usage,
        }


def collect_metrics() -> MachineMetrics:
    """Collect current system metrics.

    Returns:
        MachineMetrics with current system state
    """
    if psutil is None:
        logger.warning("psutil not installed, returning mock metrics")
        return _mock_metrics()

    # CPU
    cpu_count = psutil.cpu_count() or 1
    cpu_millicores = cpu_count * 1000
    cpu_percent = psutil.cpu_percent(interval=0.1)

    # Memory
    mem = psutil.virtual_memory()
    total_memory = mem.total
    mem_percent = mem.percent

    # Disk (root partition)
    disk = psutil.disk_usage("/")
    disk_total = disk.total
    disk_free = disk.free

    # GPU
    gpu_count = detect_gpu_count()

    return MachineMetrics(
        total_cpu_available=cpu_millicores,
        total_memory_available=total_memory,
        total_disk_space_bytes=disk_total,
        cpu_utilization_pct=cpu_percent,
        memory_utilization_pct=mem_percent,
        total_disk_free_bytes=disk_free,
        free_gpu_count=gpu_count,
    )


def detect_gpu_count() -> int:
    """Detect number of NVIDIA GPUs.

    Returns:
        Number of GPUs, 0 if none or nvidia-smi not available
    """
    try:
        result = subprocess.run(
            ["nvidia-smi", "--query-gpu=name", "--format=csv,noheader"],
            capture_output=True,
            text=True,
            timeout=5,
        )
        if result.returncode == 0:
            lines = [l for l in result.stdout.strip().split("\n") if l]
            return len(lines)
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    return 0


def get_cpu_string() -> str:
    """Get CPU as Beta9 string format (e.g., "8000m").

    Returns:
        CPU string in millicores format
    """
    if psutil is None:
        return "4000m"

    cpu_count = psutil.cpu_count() or 1
    return f"{cpu_count * 1000}m"


def get_memory_string() -> str:
    """Get memory as Beta9 string format (e.g., "16Gi").

    Returns:
        Memory string in Gi format
    """
    if psutil is None:
        return "8Gi"

    mem = psutil.virtual_memory()
    gi = mem.total / (1024 * 1024 * 1024)
    return f"{int(gi)}Gi"


def get_private_ip() -> str:
    """Get private IP address.

    Returns:
        Private IP or "127.0.0.1" if not found
    """
    if psutil is None:
        return "127.0.0.1"

    for iface, addrs in psutil.net_if_addrs().items():
        # Skip loopback
        if iface.startswith("lo"):
            continue

        for addr in addrs:
            # IPv4 only
            if addr.family.name == "AF_INET":
                ip = addr.address
                # Skip loopback and link-local
                if not ip.startswith("127.") and not ip.startswith("169.254."):
                    return ip

    return "127.0.0.1"


def _mock_metrics() -> MachineMetrics:
    """Return mock metrics for testing without psutil."""
    return MachineMetrics(
        total_cpu_available=4000,
        total_memory_available=8 * 1024 * 1024 * 1024,  # 8Gi
        total_disk_space_bytes=100 * 1024 * 1024 * 1024,  # 100Gi
        cpu_utilization_pct=10.0,
        memory_utilization_pct=20.0,
        total_disk_free_bytes=80 * 1024 * 1024 * 1024,  # 80Gi
        free_gpu_count=0,
    )
