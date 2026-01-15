import pytest

MODELS = {
    "llava": "llava:latest",
    "llava13b": "llava:13b"
}

# 1x1 pixel red dot png
TEST_IMAGE_BASE64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

def test_llava_image_desc(inference_client, ensure_model):
    """Test image description with LLaVA"""
    model = MODELS["llava"]
    if not ensure_model(model):
        pytest.skip(f"Model {model} could not be pulled")
        
    result = inference_client.chat(
        model=model,
        messages=[{
            "role": "user", 
            "content": "What is in this image?",
            "images": [TEST_IMAGE_BASE64]
        }]
    )
    
    assert result.ok, f"Inference failed: {result.error}"
    assert len(result.content) > 0
    # Note: It's hard to assert specific content for a 1x1 pixel, but we ensure it generates something.
