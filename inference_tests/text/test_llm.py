import pytest

MODELS = {
    "llama32": "llama3.2:latest",
    "qwen25": "qwen2.5:latest",
    "phi4": "phi4:latest"
}

def test_llama32_qa(inference_client, ensure_model):
    """Test simple Q&A with Llama 3.2"""
    model = MODELS["llama32"]
    if not ensure_model(model):
        pytest.skip(f"Model {model} could not be pulled")
        
    result = inference_client.chat(
        model=model,
        messages=[{"role": "user", "content": "What is the capital of France? Answer in one word."}]
    )
    
    assert result.ok, f"Inference failed: {result.error}"
    assert "Paris" in result.content

def test_qwen25_math(inference_client, ensure_model):
    """Test math reasoning with Qwen 2.5"""
    model = MODELS["qwen25"]
    if not ensure_model(model):
        pytest.skip(f"Model {model} could not be pulled")
        
    result = inference_client.chat(
        model=model,
        messages=[{"role": "user", "content": "Calculate 15 * 23. Show just the result."}]
    )
    
    assert result.ok, f"Inference failed: {result.error}"
    assert "345" in result.content

def test_phi4_listing(inference_client, ensure_model):
    """Test instruction following with Phi-4"""
    model = MODELS["phi4"]
    if not ensure_model(model):
        pytest.skip(f"Model {model} could not be pulled")
        
    result = inference_client.chat(
        model=model,
        messages=[{"role": "user", "content": "List 3 colors, comma separated."}]
    )
    
    assert result.ok, f"Inference failed: {result.error}"
    assert "," in result.content
