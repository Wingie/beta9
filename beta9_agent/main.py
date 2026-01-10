#!/usr/bin/env python3
"""Beta9 Agent - Open-source replacement for closed-source agent binary.

Usage:
    python -m beta9_agent --token TOKEN --machine-id ID --pool-name POOL

Or with environment variables:
    BETA9_TOKEN=xxx BETA9_MACHINE_ID=abc123 python -m beta9_agent
"""

import signal
import sys
import time
from typing import Optional

import click

from . import __version__
from .config import AgentConfig
from .keepalive import KeepaliveLoop
from .registration import register_machine
from .utils import setup_logging, generate_machine_id


# Global for signal handling
_keepalive_loop: Optional[KeepaliveLoop] = None


def signal_handler(signum, frame):
    """Handle shutdown signals gracefully."""
    logger = setup_logging()
    logger.info(f"Received signal {signum}, shutting down...")

    if _keepalive_loop:
        _keepalive_loop.stop()

    sys.exit(0)


@click.command()
@click.option("--token", envvar="BETA9_TOKEN", required=True, help="Registration token from 'beta9 machine create'")
@click.option("--machine-id", envvar="BETA9_MACHINE_ID", default=None, help="Unique machine ID (8 hex chars, auto-generated if not provided)")
@click.option("--pool-name", envvar="BETA9_POOL_NAME", default="external", help="Worker pool name")
@click.option("--provider-name", envvar="BETA9_PROVIDER_NAME", default="generic", help="Provider name")
@click.option("--gateway-host", envvar="BETA9_GATEWAY_HOST", default="localhost", help="Gateway HTTP host")
@click.option("--gateway-port", envvar="BETA9_GATEWAY_PORT", default=1994, type=int, help="Gateway HTTP port")
@click.option("--hostname", envvar="BETA9_HOSTNAME", default=None, help="Hostname/IP for gateway to reach this machine's k3s API (e.g., Tailscale IP)")
@click.option("--k3s-token", envvar="BETA9_K3S_TOKEN", default=None, help="k3s bearer token for API authentication")
@click.option("--keepalive-interval", default=60, type=int, help="Keepalive interval in seconds")
@click.option("--debug", is_flag=True, help="Enable debug logging")
@click.option("--dry-run", is_flag=True, help="Don't actually register, just log")
@click.option("--once", is_flag=True, help="Run once (register + single keepalive) then exit")
@click.option("--version", is_flag=True, help="Show version and exit")
def main(
    token: str,
    machine_id: Optional[str],
    pool_name: str,
    provider_name: str,
    gateway_host: str,
    gateway_port: int,
    hostname: Optional[str],
    k3s_token: Optional[str],
    keepalive_interval: int,
    debug: bool,
    dry_run: bool,
    once: bool,
    version: bool,
):
    """Beta9 Agent - Connect external workers to Beta9 control plane."""
    global _keepalive_loop

    if version:
        click.echo(f"beta9-agent {__version__}")
        return

    # Generate machine ID if not provided
    if not machine_id:
        machine_id = generate_machine_id()

    # Setup logging
    logger = setup_logging(debug=debug)
    logger.info(f"Beta9 Agent v{__version__}")
    logger.info(f"Machine ID: {machine_id}")
    logger.info(f"Pool: {pool_name}")
    logger.info(f"Gateway: {gateway_host}:{gateway_port}")

    # Create config
    config = AgentConfig(
        token=token,
        machine_id=machine_id,
        pool_name=pool_name,
        provider_name=provider_name,
        gateway_host=gateway_host,
        gateway_port=gateway_port,
        hostname=hostname,
        k3s_token=k3s_token,
        keepalive_interval=keepalive_interval,
        debug=debug,
        dry_run=dry_run,
    )

    # Validate config
    errors = config.validate()
    if errors:
        for error in errors:
            logger.error(f"Config error: {error}")
        sys.exit(1)

    # Setup signal handlers
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    # Step 1: Register machine
    logger.info("Registering machine with gateway...")
    result = register_machine(config)

    if not result.success:
        logger.error(f"Registration failed: {result.error}")
        sys.exit(1)

    logger.info("Machine registered successfully")
    if result.config:
        logger.debug(f"Gateway config: {result.config}")

    # Step 2: Handle --once mode (single keepalive then exit)
    if once:
        from .keepalive import send_single_keepalive
        logger.info("Running in --once mode, sending single keepalive...")
        success = send_single_keepalive(config)
        if success:
            logger.info("Single keepalive sent successfully")
        else:
            logger.warning("Single keepalive failed (may be expected if endpoint not deployed)")
        logger.info("Agent complete (--once mode)")
        return

    # Step 3: Start keepalive loop
    logger.info("Starting keepalive loop...")
    _keepalive_loop = KeepaliveLoop(config)
    _keepalive_loop.start()

    # Step 4: Main loop - monitor health
    try:
        while True:
            if not _keepalive_loop.is_healthy:
                logger.error("Keepalive loop unhealthy, exiting...")
                break

            time.sleep(10)

    except KeyboardInterrupt:
        logger.info("Interrupted by user")

    finally:
        if _keepalive_loop:
            _keepalive_loop.stop()

    logger.info("Agent shutdown complete")


if __name__ == "__main__":
    main()
