import pytest

MODELS = {
    "nebius": "VolkerMauel/nebius-SWE-rebench-openhands-Qwen3-30B-A3B-GGUF:Q4_K_M"
}

def test_nebius_bugfix(inference_client, ensure_model):
    """Test code bug fixing with Nebius SWE-bench model"""
    model = MODELS["nebius"]
    if not ensure_model(model):
        pytest.skip(f"Model {model} could not be pulled")
        
    buggy_code = "def add(a, b): return a - b"
    prompt = f"Fix this Python bug: {buggy_code}. The function should add, not subtract."
    
    result = inference_client.chat(
        model=model,
        messages=[{"role": "user", "content": prompt}]
    )
    
    assert result.ok, f"Inference failed: {result.error}"
    content = result.content.lower()
    
    # Check if it suggests using + or addition
    assert "+" in content or "add" in content or "sum" in content
