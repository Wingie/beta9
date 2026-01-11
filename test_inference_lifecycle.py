#!/usr/bin/env python3
"""
Test inference model lifecycle (load/unload) via the Beta9 SDK.

This tests:
1. Cold start (model not loaded)
2. Warm requests (model already loaded)
3. Model unloading
4. Model listing/status

Prerequisites:
    1. Ollama running: `ollama serve`
    2. Model pulled: `ollama pull llama3.2`

Usage:
    python test_inference_lifecycle.py
"""

import sys
import os
import time

# Add SDK to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "sdk", "src"))

from beta9 import inference


def measure_time(fn):
    """Measure execution time of a function."""
    start = time.time()
    result = fn()
    elapsed = time.time() - start
    return result, elapsed


def test_cold_start():
    """Test cold start latency."""
    print("\n--- Cold Start Test ---")

    # First, unload the model
    print("Unloading model...")
    inference.unload_model("llama3.2")
    time.sleep(2)  # Give time to unload

    # Now make a request (should trigger model load)
    print("Making cold request (model will load)...")

    def cold_request():
        return inference.chat(
            model="llama3.2",
            messages=[{"role": "user", "content": "Say hello"}],
        )

    result, elapsed = measure_time(cold_request)

    if result.error:
        print(f"  Error: {result.error}")
        return False

    print(f"  Cold start latency: {elapsed:.2f}s")
    print(f"  Response: {result.content[:100]}...")
    return True


def test_warm_requests():
    """Test warm request latency (model already loaded)."""
    print("\n--- Warm Request Test ---")

    # Pre-load the model
    print("Pre-loading model...")
    inference.load_model("llama3.2", keep_alive=-1)
    time.sleep(1)

    # Make multiple requests
    latencies = []
    for i in range(3):
        def warm_request():
            return inference.chat(
                model="llama3.2",
                messages=[{"role": "user", "content": f"Count to {i+1}"}],
            )

        result, elapsed = measure_time(warm_request)

        if result.error:
            print(f"  Request {i+1} Error: {result.error}")
            return False

        latencies.append(elapsed)
        print(f"  Request {i+1}: {elapsed:.2f}s - {result.content[:50]}...")

    avg_latency = sum(latencies) / len(latencies)
    print(f"\n  Average warm latency: {avg_latency:.2f}s")
    return True


def test_model_switching():
    """Test switching between models."""
    print("\n--- Model Switch Test ---")

    # This test requires two models to be available
    models = inference.list_models()
    model_names = [m.name for m in models]

    if len(model_names) < 2:
        print("  Skipping: Need at least 2 models for this test")
        print(f"  Available models: {model_names}")
        return True  # Not a failure, just skip

    model1 = model_names[0]
    model2 = model_names[1]

    print(f"  Testing switch between {model1} and {model2}")

    # Request model1
    result1 = inference.chat(
        model=model1,
        messages=[{"role": "user", "content": "Hi"}],
    )
    print(f"  {model1}: {'OK' if result1.ok else result1.error}")

    # Request model2
    result2 = inference.chat(
        model=model2,
        messages=[{"role": "user", "content": "Hi"}],
    )
    print(f"  {model2}: {'OK' if result2.ok else result2.error}")

    return result1.ok and result2.ok


def test_model_status():
    """Test model status reporting."""
    print("\n--- Model Status Test ---")

    models = inference.list_models()
    print(f"  Found {len(models)} models:")

    for m in models:
        print(f"    - {m.name}")
        print(f"      State: {m.load_state}")
        print(f"      Size: {m.size_gb:.2f} GB")
        if m.last_used:
            print(f"      Last used: {m.last_used}")

    return True


def test_load_unload():
    """Test explicit load/unload."""
    print("\n--- Load/Unload Test ---")

    model = "llama3.2"

    # Load
    print(f"  Loading {model}...")
    load_ok = inference.load_model(model, keep_alive=-1)
    print(f"    Load result: {'OK' if load_ok else 'FAILED'}")

    # Verify it's loaded with a quick request
    result = inference.generate(model=model, prompt="Test")
    print(f"    Verify load: {'OK' if result.ok else result.error}")

    # Unload
    print(f"  Unloading {model}...")
    unload_ok = inference.unload_model(model)
    print(f"    Unload result: {'OK' if unload_ok else 'FAILED'}")

    return load_ok and result.ok


def main():
    print("=" * 50)
    print("Beta9 Inference Lifecycle Test")
    print("=" * 50)

    # Configure to use local Ollama
    inference.configure(host="localhost", port=11434, timeout=120)

    # Check server health first
    if not inference.health():
        print("ERROR: Inference server not responding")
        print("Make sure Ollama is running: `ollama serve`")
        return 1

    # Run tests
    tests = [
        ("Model Status", test_model_status),
        ("Warm Requests", test_warm_requests),
        ("Load/Unload", test_load_unload),
        ("Cold Start", test_cold_start),
        ("Model Switching", test_model_switching),
    ]

    passed = 0
    failed = 0

    for name, test_fn in tests:
        print(f"\n{'='*50}")
        print(f"Running: {name}")
        try:
            if test_fn():
                print(f"Result: PASSED")
                passed += 1
            else:
                print(f"Result: FAILED")
                failed += 1
        except Exception as e:
            print(f"Result: ERROR - {e}")
            failed += 1

    print("\n" + "=" * 50)
    print(f"Summary: {passed} passed, {failed} failed")
    print("=" * 50)

    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
