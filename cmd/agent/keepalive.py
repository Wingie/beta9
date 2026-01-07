"""Keepalive loop for Beta9 agent.

Sends periodic keepalive updates to transition machine status
from registered → ready and maintain the ready status.
"""

import logging
import threading
import time
from typing import Callable, Optional

import httpx

from .config import AgentConfig
from .metrics import collect_metrics, MachineMetrics

logger = logging.getLogger("beta9-agent")

# Agent version reported in keepalive
AGENT_VERSION = "0.1.0-python"


class KeepaliveLoop:
    """Manages periodic keepalive updates to the gateway."""

    def __init__(self, config: AgentConfig):
        """Initialize keepalive loop.

        Args:
            config: Agent configuration
        """
        self.config = config
        self._stop_event = threading.Event()
        self._thread: Optional[threading.Thread] = None
        self._consecutive_failures = 0
        self._max_failures = 3

    def start(self) -> None:
        """Start the keepalive loop in a background thread."""
        if self._thread and self._thread.is_alive():
            logger.warning("Keepalive loop already running")
            return

        self._stop_event.clear()
        self._thread = threading.Thread(
            target=self._run_loop,
            name="keepalive-loop",
            daemon=True,
        )
        self._thread.start()
        logger.info(f"Started keepalive loop (interval: {self.config.keepalive_interval}s)")

    def stop(self) -> None:
        """Stop the keepalive loop."""
        self._stop_event.set()
        if self._thread:
            self._thread.join(timeout=5)
            logger.info("Stopped keepalive loop")

    def _run_loop(self) -> None:
        """Main keepalive loop."""
        # Send first keepalive immediately
        self._send_keepalive()

        while not self._stop_event.is_set():
            # Wait for interval or stop signal
            if self._stop_event.wait(timeout=self.config.keepalive_interval):
                break

            self._send_keepalive()

    def _send_keepalive(self) -> bool:
        """Send a single keepalive update.

        Returns:
            True if successful, False otherwise
        """
        metrics = collect_metrics()

        payload = {
            "machine_id": self.config.machine_id,
            "provider_name": self.config.provider_name,
            "pool_name": self.config.pool_name,
            "agent_version": AGENT_VERSION,
            "metrics": metrics.to_dict(),
        }

        logger.debug(f"Sending keepalive: cpu={metrics.cpu_utilization_pct:.1f}%, mem={metrics.memory_utilization_pct:.1f}%")

        if self.config.dry_run:
            logger.info("Dry run - skipping keepalive")
            return True

        try:
            headers = {
                "Authorization": f"Bearer {self.config.token}",
                "Content-Type": "application/json",
            }

            with httpx.Client(timeout=10) as client:
                response = client.post(
                    self.config.keepalive_url,
                    json=payload,
                    headers=headers,
                )

                if response.status_code == 200:
                    self._consecutive_failures = 0
                    logger.debug(f"Keepalive successful for machine {self.config.machine_id}")
                    return True

                else:
                    self._consecutive_failures += 1
                    logger.warning(
                        f"Keepalive failed: {response.status_code} - {response.text} "
                        f"(failure {self._consecutive_failures}/{self._max_failures})"
                    )
                    return False

        except httpx.ConnectError as e:
            self._consecutive_failures += 1
            logger.warning(f"Keepalive connection failed: {e}")
            return False

        except Exception as e:
            self._consecutive_failures += 1
            logger.error(f"Keepalive error: {e}")
            return False

    @property
    def is_healthy(self) -> bool:
        """Check if keepalive loop is healthy.

        Returns:
            True if recent keepalives succeeded
        """
        return self._consecutive_failures < self._max_failures


def send_single_keepalive(config: AgentConfig) -> bool:
    """Send a single keepalive update (for testing).

    Args:
        config: Agent configuration

    Returns:
        True if successful
    """
    loop = KeepaliveLoop(config)
    return loop._send_keepalive()
