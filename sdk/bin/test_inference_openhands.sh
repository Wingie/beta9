#!/bin/bash

# Configuration
GATEWAY_HOST=${BETA9_GATEWAY_HOST:-"localhost"}
GATEWAY_PORT=${BETA9_GATEWAY_PORT:-"9090"} # Default gateway port
MODEL_NAME=${1:-"VolkerMauel/nebius-SWE-rebench-openhands-Qwen3-30B-A3B-GGUF"}

echo "Testing inference on Beta9 Gateway at $GATEWAY_HOST:$GATEWAY_PORT"
echo "Model: $MODEL_NAME"
echo ""

# 1. Check Inference Health
echo "1. Checking Inference Service Health..."
curl -s "http://$GATEWAY_HOST:$GATEWAY_PORT/api/v1/inference/health" | jq .
echo ""

# 2. List Nodes
echo "2. Listing Inference Nodes..."
curl -s "http://$GATEWAY_HOST:$GATEWAY_PORT/api/v1/inference/nodes" | jq .
echo ""

# 3. List Models
echo "3. Listing Available Models..."
curl -s "http://$GATEWAY_HOST:$GATEWAY_PORT/api/v1/inference/models" | jq .
echo ""

# 4. Run Chat Completion (Streaming disabled for curl simplicity)
echo "4. Running Chat Completion..."
start_time=$(date +%s)

response=$(curl -s -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/api/v1/inference/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"$MODEL_NAME"'",
    "messages": [
      {"role": "user", "content": "Hello! How are you?"}
    ],
    "temperature": 0.7
  }')

end_time=$(date +%s)
duration=$((end_time - start_time))

echo "Response received in ${duration}s:"
echo "$response" | jq .
echo ""

# 5. Run Embedding
echo "5. Running Embedding..."
curl -s -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/api/v1/inference/embeddings" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"$MODEL_NAME"'",
    "input": ["Hello world", "Beta9 is awesome"]
  }' | jq .
echo ""

echo "Test complete."

# 6. Test CLI commands
echo "6. Testing CLI Commands..."
beta9 inference health --context default
beta9 inference nodes --context default
beta9 inference models --context default
beta9 inference chat --model "$MODEL_NAME" --message "Hello from CLI" --context default
