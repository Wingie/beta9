
import functools
from typing import Optional, List

import click
from rich.table import Table, Column, box
from rich.console import Console

from .. import terminal
from ..config import get_config_context, DEFAULT_CONTEXT_NAME, ConfigContext
from .extraclick import ClickManagementGroup, ClickCommonGroup, config_context_option
from .. import inference

console = Console()

def pass_config(func):
    """
    Decorator that sets ConfigContext as the first argument.
    """
    @config_context_option
    @functools.wraps(func)
    def decorator(context: Optional[str] = None, *args, **kwargs):
        ctx = click.get_current_context()

        context = context or ctx.params.get("context", None)

        if context is None and hasattr(ctx, "parent") and hasattr(ctx.parent, "params"):
            context = ctx.parent.params.get("context", None)

        if (
            context is None
            and hasattr(ctx, "parent")
            and hasattr(ctx.parent, "parent")
            and hasattr(ctx.parent.parent, "params")
        ):
            context = ctx.parent.parent.params.get("context", "")

        config = get_config_context(context or DEFAULT_CONTEXT_NAME)
        
        # Configure the inference client with the gateway details
        inference.configure(
            host=config.gateway_host, 
            port=config.gateway_port
        )
        
        return func(config, *args, **kwargs)

    return decorator


@click.group(cls=ClickCommonGroup)
def common(**_):
    pass


@click.group(
    name="inference",
    help="Manage inference nodes and models.",
    cls=ClickManagementGroup,
)
def management():
    pass


@management.command(
    name="health",
    help="Check inference service health.",
)
@pass_config
def check_health(config: ConfigContext):
    """Check if the inference service is healthy."""
    if inference.health():
        terminal.success("Inference service is healthy")
    else:
        terminal.error("Inference service is unhealthy or unreachable")


@management.command(
    name="nodes",
    help="List all inference nodes.",
)
@pass_config
def list_inference_nodes(config: ConfigContext):
    """List all registered inference nodes."""
    # The client doesn't currently support listing nodes directly in the convenience methods,
    # so we'll access the client instance or endpoint directly if needed.
    # However, for now, let's see if we can add it to the client or if we missed it.
    # Looking at inference.py, there is NO list_nodes method on the client.
    # We should add it to the client first if we want to be consistent, OR use raw httpx here.
    # Given the instruction to use the SDK client, maybe we should stick to what's available
    # or extend the client. User said "put everything you think is needed".
    # I'll implement list_nodes here using the client's internal http client if possible, 
    # or just raw httpx using config for now to avoid modifying the SDK core if not strictly needed yet.
    # BUT, modifying the SDK core is better.
    # For this step, I'll use the raw URL approach inside this function but styled like the client.
    
    # Actually, let's just use httpx here for the 'admin' commands that might not be in the user-facing SDK yet.
    import httpx
    
    url = f"http://{config.gateway_host}:{config.gateway_port}/api/v1/inference/nodes"
    try:
        resp = httpx.get(url, timeout=5.0)
        resp.raise_for_status()
        data = resp.json()
        
        nodes = data.get("nodes", [])
        
        table = Table(
            Column("Node ID"),
            Column("Tailscale IP"),
            Column("GPU"),
            Column("VRAM (MB)"),
            Column("Models"),
            Column("Healthy"),
            box=box.SIMPLE,
        )
        
        for node in nodes:
            vram = f"{node.get('available_vram_mb', 0)} / {node.get('total_vram_mb', 0)}"
            models = len(node.get('models', {}))
            healthy = "✅" if node.get('healthy') else "❌"
            
            table.add_row(
                node.get("node_id"),
                node.get("tailscale_ip"),
                node.get("gpu_type"),
                vram,
                str(models),
                healthy,
            )
            
        terminal.print(table)
    except Exception as e:
        terminal.error(f"Failed to list nodes: {e}")


@management.command(
    name="models",
    help="List available models.",
)
@pass_config
def list_available_models(config: ConfigContext):
    """List all available models."""
    models = inference.list_models()
    
    if not models:
        terminal.warn("No models found.")
        return

    table = Table(
        Column("Model Name"),
        Column("Size (GB)"),
        Column("Load State"),
        Column("Last Used"),
        box=box.SIMPLE,
    )
    
    for m in models:
        table.add_row(
            m.name,
            str(m.size_gb),
            m.load_state,
            str(m.last_used or "-"),
        )
        
    terminal.print(table)


@management.command(
    name="chat",
    help="Run a chat completion.",
)
@click.option("--model", required=True, help="Model to use")
@click.option("--message", required=True, help="User message")
@click.option("--temperature", default=0.7, help="Temperature")
@pass_config
def run_chat(config: ConfigContext, model: str, message: str, temperature: float):
    """Run a chat completion."""
    terminal.detail(f"Sending chat request to model '{model}'...")
    
    options = inference.InferenceOptions(temperature=temperature)
    result = inference.chat(
        model=model,
        messages=[{"role": "user", "content": message}],
        options=options
    )
    
    if result.ok:
        terminal.success("Response:")
        terminal.print(result.content)
        if result.usage:
            terminal.dim(f"\nUsage: {result.usage}")
    else:
        terminal.error(f"Chat failed: {result.error}")
