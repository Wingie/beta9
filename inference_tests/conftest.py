import os
import sys
import pytest

# Add SDK to path
SDK_PATH = os.path.abspath(os.path.join(os.path.dirname(__file__), "../sdk/src"))
if SDK_PATH not in sys.path:
    sys.path.insert(0, SDK_PATH)

from beta9 import inference

@pytest.fixture(scope="session")
def inference_client():
    host = os.environ.get('BETA9_INFERENCE_HOST', '100.100.74.117')
    port = int(os.environ.get('BETA9_INFERENCE_PORT', '11434'))
    
    # Configure global inference settings 
    # (assuming the SDK uses global config or we can configure it here)
    # Based on inference.py, it reads env vars. 
    os.environ['BETA9_INFERENCE_HOST'] = host
    os.environ['BETA9_INFERENCE_PORT'] = str(port)
    
    return inference

@pytest.fixture(scope="session")
def ensure_model(inference_client):
    """Fixture to ensure a model exists before testing."""
    def _ensure(model_name):
        # List models
        models = inference_client.list_models()
        model_names = [m.name for m in models]
        
        # Check if model exists (exact or prefix match)
        found = False
        for m in model_names:
            if m == model_name or m.split(':')[0] == model_name.split(':')[0]:
                found = True
                break
        
        if not found:
            print(f"\nPulling model {model_name}...")
            # Direct API call to pull since SDK doesn't expose it
            import httpx
            # Use the client's base_url which is private in the SDK, so reconstruct it
            base_url = f"http://{os.environ['BETA9_INFERENCE_HOST']}:{os.environ['BETA9_INFERENCE_PORT']}"
            
            try:
                # Trigger pull (stream=False to wait for completion)
                resp = httpx.post(f"{base_url}/api/pull", json={"name": model_name, "stream": False}, timeout=1200.0)
                resp.raise_for_status()
                print(f"Successfully pulled {model_name}")
            except Exception as e:
                print(f"Failed to pull {model_name}: {e}")
                return False
                
        return True
    return _ensure
