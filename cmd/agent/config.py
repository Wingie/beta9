"""Configuration for Beta9 agent."""

from dataclasses import dataclass, field
from typing import Optional
import os


@dataclass
class AgentConfig:
    """Configuration for the Beta9 agent.

    Matches CLI arguments from the closed-source agent:
    - --token: Registration token from `beta9 machine create`
    - --machine-id: Unique machine identifier (8 hex chars)
    - --pool-name: Worker pool name (e.g., "gpu", "external")
    - --provider-name: Provider name (default: "generic")
    - --gateway-host: Gateway HTTP host
    - --gateway-port: Gateway HTTP port (default: 1994)

    Additional options for SSH tunnel mode:
    - --k3s-token: Pre-existing k3s token (skip installation)
    """

    # Required: from beta9 machine create
    token: str
    machine_id: str
    pool_name: str

    # Gateway connection
    gateway_host: str = "localhost"
    gateway_port: int = 1994
    gateway_scheme: str = "http"

    # Provider info
    provider_name: str = "generic"

    # k3s configuration (for Linux)
    k3s_token: Optional[str] = None
    k3s_version: str = "v1.28.5+k3s1"

    # Timing
    keepalive_interval: int = 60  # seconds
    registration_timeout: int = 30  # seconds

    # Agent behavior
    debug: bool = False
    dry_run: bool = False  # Don't actually register, just log

    @property
    def gateway_url(self) -> str:
        """Full gateway URL for API calls."""
        return f"{self.gateway_scheme}://{self.gateway_host}:{self.gateway_port}"

    @property
    def register_url(self) -> str:
        """URL for machine registration endpoint."""
        return f"{self.gateway_url}/api/v1/machine/register"

    @property
    def keepalive_url(self) -> str:
        """URL for machine keepalive endpoint."""
        return f"{self.gateway_url}/api/v1/machine/keepalive"

    @classmethod
    def from_env(cls) -> "AgentConfig":
        """Create config from environment variables.

        Environment variables:
        - BETA9_TOKEN: Registration token
        - BETA9_MACHINE_ID: Machine ID
        - BETA9_POOL_NAME: Pool name
        - BETA9_GATEWAY_HOST: Gateway host
        - BETA9_GATEWAY_PORT: Gateway port
        - BETA9_PROVIDER_NAME: Provider name
        - BETA9_K3S_TOKEN: Pre-existing k3s token
        - BETA9_DEBUG: Enable debug logging
        """
        return cls(
            token=os.environ.get("BETA9_TOKEN", ""),
            machine_id=os.environ.get("BETA9_MACHINE_ID", ""),
            pool_name=os.environ.get("BETA9_POOL_NAME", "external"),
            gateway_host=os.environ.get("BETA9_GATEWAY_HOST", "localhost"),
            gateway_port=int(os.environ.get("BETA9_GATEWAY_PORT", "1994")),
            provider_name=os.environ.get("BETA9_PROVIDER_NAME", "generic"),
            k3s_token=os.environ.get("BETA9_K3S_TOKEN"),
            debug=os.environ.get("BETA9_DEBUG", "").lower() in ("1", "true", "yes"),
        )

    def validate(self) -> list[str]:
        """Validate configuration.

        Returns:
            List of validation error messages (empty if valid)
        """
        errors = []

        if not self.token:
            errors.append("token is required (from beta9 machine create)")

        if not self.machine_id:
            errors.append("machine_id is required")
        elif len(self.machine_id) != 8:
            errors.append(f"machine_id should be 8 hex chars, got: {self.machine_id}")

        if not self.pool_name:
            errors.append("pool_name is required")

        if self.gateway_port < 1 or self.gateway_port > 65535:
            errors.append(f"gateway_port must be 1-65535, got: {self.gateway_port}")

        return errors
