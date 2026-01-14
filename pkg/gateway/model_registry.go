package gateway

import (
	"sync"
	"time"

	"github.com/beam-cloud/beta9/pkg/types"
)

// ============================================================================
// Model Registry - Tracks model availability across nodes
// ============================================================================

// ModelRegistry tracks model availability across all nodes
type ModelRegistry struct {
	nodes map[string]*types.NodeInferenceInfo
	mu    sync.RWMutex

	// HeartbeatTimeout is how long before a node is considered unhealthy
	HeartbeatTimeout time.Duration
}

// NewModelRegistry creates a new ModelRegistry
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		nodes:            make(map[string]*types.NodeInferenceInfo),
		HeartbeatTimeout: 30 * time.Second,
	}
}

// RegisterNode registers or updates a node's inference capability
func (r *ModelRegistry) RegisterNode(info *types.NodeInferenceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	info.LastHeartbeat = time.Now()
	info.Healthy = true

	// Preserve existing model stats if updating
	if existing, ok := r.nodes[info.NodeID]; ok {
		for name, existingModel := range existing.Models {
			if newModel, exists := info.Models[name]; exists {
				// Preserve stats that node doesn't send
				newModel.RequestCount = existingModel.RequestCount
				if newModel.LoadedAt.IsZero() && existingModel.LoadState == types.LoadStateReady {
					newModel.LoadedAt = existingModel.LoadedAt
				}
			}
		}
	}

	r.nodes[info.NodeID] = info
}

// UpdateNodeModels updates model states for a node
func (r *ModelRegistry) UpdateNodeModels(nodeID string, models map[string]*types.ModelInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node, ok := r.nodes[nodeID]; ok {
		for name, model := range models {
			if existing, exists := node.Models[name]; exists {
				// Preserve request count
				model.RequestCount = existing.RequestCount
			}
			node.Models[name] = model
		}
		node.LastHeartbeat = time.Now()
		node.Healthy = true
	}
}

// UpdateHeartbeat updates the heartbeat timestamp for a node
func (r *ModelRegistry) UpdateHeartbeat(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node, ok := r.nodes[nodeID]; ok {
		node.LastHeartbeat = time.Now()
		node.Healthy = true
	}
}

// RemoveNode removes a node from the registry
func (r *ModelRegistry) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
}

// FindNodeForModel finds the best node to serve a specific model
// Returns nil if no suitable node is found
func (r *ModelRegistry) FindNodeForModel(model string, preferGPU string) *types.NodeInferenceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var bestNode *types.NodeInferenceInfo
	var bestScore int

	for _, node := range r.nodes {
		if !r.isNodeHealthy(node) {
			continue
		}

		modelInfo, hasModel := node.Models[model]
		if !hasModel {
			continue
		}

		// Calculate score based on:
		// - Model state (ready > loading > idle)
		// - GPU preference match
		// - Recent usage (warm cache)
		// - Available VRAM
		score := 0

		switch modelInfo.LoadState {
		case types.LoadStateReady:
			score += 100 // Strongly prefer already-loaded models
		case types.LoadStateLoading:
			score += 10 // Slightly prefer loading over idle
		case types.LoadStateIdle:
			score += 1
		case types.LoadStateError:
			continue // Skip nodes with error state
		}

		// GPU preference bonus
		if preferGPU != "" && node.GPUType == preferGPU {
			score += 50
		}

		// Recency bonus (used in last 5 minutes)
		if time.Since(modelInfo.LastUsed) < 5*time.Minute {
			score += 20
		}

		// VRAM availability bonus
		if node.AvailableVRAM > 0 {
			score += int(node.AvailableVRAM / 1024) // Bonus per GB available
		}

		if score > bestScore {
			bestScore = score
			bestNode = node
		}
	}

	return bestNode
}

// FindAnyNodeWithCapacity finds any healthy node with GPU capacity
func (r *ModelRegistry) FindAnyNodeWithCapacity(minVRAMMB int64, preferGPU string) *types.NodeInferenceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var bestNode *types.NodeInferenceInfo
	var bestVRAM int64

	for _, node := range r.nodes {
		if !r.isNodeHealthy(node) {
			continue
		}

		if minVRAMMB > 0 && node.AvailableVRAM < minVRAMMB {
			continue
		}

		// Prefer matching GPU type
		if preferGPU != "" && node.GPUType == preferGPU {
			if node.AvailableVRAM > bestVRAM {
				bestNode = node
				bestVRAM = node.AvailableVRAM
			}
		} else if bestNode == nil || node.AvailableVRAM > bestVRAM {
			bestNode = node
			bestVRAM = node.AvailableVRAM
		}
	}

	return bestNode
}

// ListModels returns all models across all nodes
func (r *ModelRegistry) ListModels() map[string][]*types.NodeInferenceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]*types.NodeInferenceInfo)

	for _, node := range r.nodes {
		if !r.isNodeHealthy(node) {
			continue
		}

		for modelName := range node.Models {
			result[modelName] = append(result[modelName], node)
		}
	}

	return result
}

// ListNodes returns all registered nodes
func (r *ModelRegistry) ListNodes() []*types.NodeInferenceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]*types.NodeInferenceInfo, 0, len(r.nodes))
	for _, node := range r.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetNode returns info for a specific node
func (r *ModelRegistry) GetNode(nodeID string) *types.NodeInferenceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodes[nodeID]
}

// IncrementRequestCount increments the request count for a model on a node
func (r *ModelRegistry) IncrementRequestCount(nodeID, model string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node, ok := r.nodes[nodeID]; ok {
		if modelInfo, exists := node.Models[model]; exists {
			modelInfo.RequestCount++
			modelInfo.LastUsed = time.Now()
		}
	}
}

// MarkModelLoaded marks a model as loaded on a node
func (r *ModelRegistry) MarkModelLoaded(nodeID, model string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node, ok := r.nodes[nodeID]; ok {
		if modelInfo, exists := node.Models[model]; exists {
			modelInfo.LoadState = types.LoadStateReady
			modelInfo.LoadedAt = time.Now()
			modelInfo.Error = ""
		}
	}
}

// MarkModelError marks a model as errored on a node
func (r *ModelRegistry) MarkModelError(nodeID, model, errorMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node, ok := r.nodes[nodeID]; ok {
		if modelInfo, exists := node.Models[model]; exists {
			modelInfo.LoadState = types.LoadStateError
			modelInfo.Error = errorMsg
		}
	}
}

// CleanupStaleNodes removes nodes that haven't sent heartbeats
func (r *ModelRegistry) CleanupStaleNodes() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	for nodeID, node := range r.nodes {
		if time.Since(node.LastHeartbeat) > r.HeartbeatTimeout*3 {
			delete(r.nodes, nodeID)
			removed++
		} else if time.Since(node.LastHeartbeat) > r.HeartbeatTimeout {
			node.Healthy = false
		}
	}
	return removed
}

// isNodeHealthy checks if a node is healthy (internal, no lock)
func (r *ModelRegistry) isNodeHealthy(node *types.NodeInferenceInfo) bool {
	if !node.Healthy {
		return false
	}
	return time.Since(node.LastHeartbeat) < r.HeartbeatTimeout
}

// Stats returns registry statistics
func (r *ModelRegistry) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	healthy := 0
	unhealthy := 0
	totalModels := 0
	readyModels := 0

	for _, node := range r.nodes {
		if r.isNodeHealthy(node) {
			healthy++
		} else {
			unhealthy++
		}
		for _, model := range node.Models {
			totalModels++
			if model.LoadState == types.LoadStateReady {
				readyModels++
			}
		}
	}

	return map[string]interface{}{
		"total_nodes":     len(r.nodes),
		"healthy_nodes":   healthy,
		"unhealthy_nodes": unhealthy,
		"total_models":    totalModels,
		"ready_models":    readyModels,
	}
}
