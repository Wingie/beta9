"""Machine registration client for Beta9 gateway.

Handles POST /api/v1/machine/register to transition machine from
pending → registered status.
"""

import logging
from dataclasses import dataclass
from typing import Optional

import httpx

from .config import AgentConfig
from .metrics import get_cpu_string, get_memory_string, get_private_ip, detect_gpu_count

logger = logging.getLogger("beta9-agent")


@dataclass
class RegistrationResult:
    """Result of machine registration."""
    success: bool
    config: Optional[dict] = None
    error: Optional[str] = None


def register_machine(config: AgentConfig) -> RegistrationResult:
    """Register machine with Beta9 gateway.

    Sends POST /api/v1/machine/register with machine details.

    Args:
        config: Agent configuration

    Returns:
        RegistrationResult with success status and gateway config
    """
    hostname = f"machine-{config.machine_id}"

    payload = {
        "token": config.k3s_token or "mock-k3s-token",  # k3s token, not registration token
        "machine_id": config.machine_id,
        "hostname": hostname,
        "provider_name": config.provider_name,
        "pool_name": config.pool_name,
        "cpu": get_cpu_string(),
        "memory": get_memory_string(),
        "gpu_count": str(detect_gpu_count()),
        "private_ip": get_private_ip(),
    }

    logger.info(f"Registering machine {config.machine_id} with gateway {config.gateway_url}")
    logger.debug(f"Registration payload: {payload}")

    if config.dry_run:
        logger.info("Dry run mode - skipping actual registration")
        return RegistrationResult(success=True, config={"dry_run": True})

    try:
        headers = {
            "Authorization": f"Bearer {config.token}",
            "Content-Type": "application/json",
        }

        with httpx.Client(timeout=config.registration_timeout) as client:
            response = client.post(
                config.register_url,
                json=payload,
                headers=headers,
            )

            if response.status_code == 200:
                data = response.json()
                logger.info(f"Machine {config.machine_id} registered successfully")
                return RegistrationResult(
                    success=True,
                    config=data.get("config"),
                )

            elif response.status_code == 403:
                return RegistrationResult(
                    success=False,
                    error="Invalid token - ensure token is from 'beta9 machine create'",
                )

            elif response.status_code == 400:
                return RegistrationResult(
                    success=False,
                    error=f"Bad request: {response.text}",
                )

            else:
                return RegistrationResult(
                    success=False,
                    error=f"Unexpected status {response.status_code}: {response.text}",
                )

    except httpx.ConnectError as e:
        return RegistrationResult(
            success=False,
            error=f"Connection failed to {config.gateway_url}: {e}",
        )

    except httpx.TimeoutException:
        return RegistrationResult(
            success=False,
            error=f"Timeout connecting to {config.gateway_url}",
        )

    except Exception as e:
        return RegistrationResult(
            success=False,
            error=f"Registration failed: {e}",
        )
