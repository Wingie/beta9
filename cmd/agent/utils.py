"""Logging and utility helpers for Beta9 agent."""

import logging
import sys
from typing import Optional


def setup_logging(debug: bool = False) -> logging.Logger:
    """Configure logging for the agent.

    Args:
        debug: Enable debug-level logging

    Returns:
        Configured logger instance
    """
    level = logging.DEBUG if debug else logging.INFO

    # Create formatter matching Beta9 gateway style
    formatter = logging.Formatter(
        fmt="%(asctime)s %(levelname)s [%(name)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S"
    )

    # Configure root logger
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(formatter)

    logger = logging.getLogger("beta9-agent")
    logger.setLevel(level)
    logger.addHandler(handler)

    return logger


def generate_machine_id() -> str:
    """Generate a unique machine ID (8 hex chars).

    Uses first 8 chars of a UUID4 to match Beta9's format.
    Example: "543b6042"
    """
    import uuid
    return uuid.uuid4().hex[:8]


def parse_cpu_string(cpu_str: str) -> int:
    """Parse CPU string to millicores.

    Args:
        cpu_str: CPU string like "8000m" or "8"

    Returns:
        CPU in millicores (e.g., 8000)
    """
    if cpu_str.endswith("m"):
        return int(cpu_str[:-1])
    return int(float(cpu_str) * 1000)


def parse_memory_string(mem_str: str) -> int:
    """Parse memory string to bytes.

    Args:
        mem_str: Memory string like "16Gi", "1024Mi", "1073741824"

    Returns:
        Memory in bytes
    """
    if mem_str.endswith("Gi"):
        return int(float(mem_str[:-2]) * 1024 * 1024 * 1024)
    elif mem_str.endswith("Mi"):
        return int(float(mem_str[:-2]) * 1024 * 1024)
    elif mem_str.endswith("Ki"):
        return int(float(mem_str[:-2]) * 1024)
    return int(mem_str)


def format_cpu_millicores(millicores: int) -> str:
    """Format millicores to Beta9 CPU string.

    Args:
        millicores: CPU in millicores (e.g., 8000)

    Returns:
        CPU string like "8000m"
    """
    return f"{millicores}m"


def format_memory_gi(bytes_val: int) -> str:
    """Format bytes to Beta9 memory string.

    Args:
        bytes_val: Memory in bytes

    Returns:
        Memory string like "16Gi"
    """
    gi = bytes_val / (1024 * 1024 * 1024)
    return f"{gi:.0f}Gi"
