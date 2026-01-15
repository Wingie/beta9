# Beta9 Inference Tests (Python SDK)

This directory contains Pytest-based tests for the Beta9 inference system, organized by modality.

## Structure

```
inference_tests/
├── text/           # Text generation tests (LLMs, Code)
│   ├── test_llm.py
│   └── test_swe.py
├── img/            # Image understanding tests (VLMs)
│   └── test_vlm.py
├── video/          # Video analysis tests (Placeholder)
│   └── test_video.py
├── conftest.py     # Setup and fixtures (SDK path, model pulling)
└── utils.py        # Helpers
```

## Prerequisites

- Python 3.8+
- `pytest`
- `httpx` (part of beta9 sdk dependencies)
- Beta9 SDK (automatically added to path by conftest.py)

## Running Tests

Run all tests (from `backend/beta9/sdk` directory):
```bash
cd backend/beta9/sdk
uv run --group dev pytest ../inference_tests
```

Run specific modality:
```bash
uv run --group dev pytest ../inference_tests/text
uv run --group dev pytest ../inference_tests/img
```

Run specific test file:
```bash
uv run --group dev pytest ../inference_tests/text/test_llm.py
```

## Configuration

Environment variables can be set to override defaults:

- `BETA9_INFERENCE_HOST`: Host of the inference server (default: 100.100.74.117)
- `BETA9_INFERENCE_PORT`: Port of the inference server (default: 11434)

Example:
```bash
BETA9_INFERENCE_HOST=100.100.74.117 uv run --group dev pytest ../inference_tests/text/test_llm.py
```
