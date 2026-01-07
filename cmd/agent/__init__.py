"""Beta9 Agent - Open-source replacement for closed-source agent binary.

This Python agent enables external GPU workers to connect to a Beta9 control plane
without depending on the proprietary agent binary from release.beam.cloud.

Key responsibilities:
- Register machine with gateway via POST /api/v1/machine/register
- Send keepalive updates to maintain "ready" status
- Report system metrics (CPU, memory, GPU)
- (Linux only) Install and manage k3s for worker pod deployment
"""

__version__ = "0.1.0"
