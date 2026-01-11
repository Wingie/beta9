#!/usr/bin/env python3
"""
Test basic inference job via the Beta9 SDK.

Prerequisites:
    1. Ollama running: `ollama serve`
    2. Model pulled: `ollama pull llama3.2`

Usage:
    python test_job.py
"""

import sys
import os

# Add SDK to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "sdk", "src"))

from beta9 import inference


def test_health():
    """Test inference server is healthy."""
    print("Testing health check...")
    healthy = inference.health()
    print(f"  Server healthy: {healthy}")
    return healthy


def test_list_models():
    """List available models."""
    print("\nListing available models...")
    models = inference.list_models()
    if not models:
        print("  No models found (or server unreachable)")
        return False

    for m in models:
        print(f"  - {m.name} ({m.size_gb:.1f} GB)")
    return True


def test_chat():
    """Test chat completion."""
    print("\nTesting chat completion with llama3.2...")

    result = inference.chat(
        model="llama3.2",
        messages=[
            {"role": "system", "content": "You are a helpful assistant. Be brief."},
            {"role": "user", "content": "What is 2 + 2?"},
        ],
    )

    if result.error:
        print(f"  Error: {result.error}")
        return False

    print(f"  Response: {result.content[:200]}...")
    print(f"  Tokens: {result.usage}")
    return True


def test_generate():
    """Test text generation."""
    print("\nTesting text generation with llama3.2...")

    result = inference.generate(
        model="llama3.2",
        prompt="The capital of France is",
    )

    if result.error:
        print(f"  Error: {result.error}")
        return False

    print(f"  Response: {result.content[:200]}...")
    return True


def main():
    print("=" * 50)
    print("Beta9 Inference Test")
    print("=" * 50)

    # Configure to use local Ollama
    inference.configure(host="localhost", port=11434)

    # Run tests
    tests = [
        ("Health Check", test_health),
        ("List Models", test_list_models),
        ("Chat Completion", test_chat),
        ("Text Generation", test_generate),
    ]

    passed = 0
    failed = 0

    for test_name, test_fn in tests:
        try:
            if test_fn():
                passed += 1
            else:
                failed += 1
        except Exception as e:
            print(f"  {test_name} Exception: {e}")
            failed += 1

    print("\n" + "=" * 50)
    print(f"Results: {passed} passed, {failed} failed")
    print("=" * 50)

    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
