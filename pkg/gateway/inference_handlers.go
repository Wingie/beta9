package gateway

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// ============================================================================
// Inference HTTP Handlers - API endpoints for inference routing
// ============================================================================

// InferenceService provides HTTP handlers for inference
type InferenceService struct {
	router   *InferenceRouter
	registry *ModelRegistry
	ctx      context.Context
}

// NewInferenceService creates a new InferenceService
func NewInferenceService(ctx context.Context, registry *ModelRegistry) *InferenceService {
	return &InferenceService{
		router:   NewInferenceRouter(registry),
		registry: registry,
		ctx:      ctx,
	}
}

// RegisterRoutes registers inference routes on the Echo router
func (s *InferenceService) RegisterRoutes(g *echo.Group) {
	// OpenAI-compatible endpoints
	g.POST("/chat/completions", s.handleChat)
	g.POST("/embeddings", s.handleEmbed)

	// Model management
	g.GET("/models", s.handleListModels)
	g.POST("/models/:model/load", s.handleLoadModel)
	g.POST("/models/:model/unload", s.handleUnloadModel)

	// Node management
	g.GET("/nodes", s.handleListNodes)
	g.POST("/nodes/register", s.handleRegisterNode)
	g.POST("/nodes/:nodeId/heartbeat", s.handleHeartbeat)

	// Health
	g.GET("/health", s.handleHealth)
}

// handleChat handles POST /v1/inference/chat/completions
func (s *InferenceService) handleChat(c echo.Context) error {
	var req ChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	if req.Model == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "model is required",
		})
	}

	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "messages is required",
		})
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	resp, err := s.router.Chat(ctx, &req)
	if err != nil {
		log.Error().Err(err).Str("model", req.Model).Msg("Chat request failed")
		if routerErr, ok := err.(*RouterError); ok {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": routerErr.Message,
				"code":  routerErr.Code,
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// handleEmbed handles POST /v1/inference/embeddings
func (s *InferenceService) handleEmbed(c echo.Context) error {
	var req EmbedRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	if req.Model == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "model is required",
		})
	}

	if len(req.Input) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "input is required",
		})
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	resp, err := s.router.Embed(ctx, &req)
	if err != nil {
		log.Error().Err(err).Str("model", req.Model).Msg("Embed request failed")
		if routerErr, ok := err.(*RouterError); ok {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": routerErr.Message,
				"code":  routerErr.Code,
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// handleListModels handles GET /v1/inference/models
func (s *InferenceService) handleListModels(c echo.Context) error {
	models := s.router.ListModels()

	// Convert to OpenAI-compatible format
	data := make([]map[string]interface{}, 0)
	for model, locations := range models {
		data = append(data, map[string]interface{}{
			"id":         model,
			"object":     "model",
			"created":    time.Now().Unix(),
			"owned_by":   "cluster",
			"locations":  locations,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"object": "list",
		"data":   data,
	})
}

// handleLoadModel handles POST /v1/inference/models/:model/load
func (s *InferenceService) handleLoadModel(c echo.Context) error {
	model := c.Param("model")
	nodeID := c.QueryParam("node")

	if nodeID == "" {
		// Find best node with capacity
		node := s.registry.FindAnyNodeWithCapacity(0, "")
		if node == nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "no available nodes with capacity",
			})
		}
		nodeID = node.NodeID
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	if err := s.router.LoadModel(ctx, nodeID, model); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "loaded",
		"model":  model,
		"node":   nodeID,
	})
}

// handleUnloadModel handles POST /v1/inference/models/:model/unload
func (s *InferenceService) handleUnloadModel(c echo.Context) error {
	model := c.Param("model")
	nodeID := c.QueryParam("node")

	if nodeID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "node query parameter is required",
		})
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	if err := s.router.UnloadModel(ctx, nodeID, model); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "unloaded",
		"model":  model,
		"node":   nodeID,
	})
}

// handleListNodes handles GET /v1/inference/nodes
func (s *InferenceService) handleListNodes(c echo.Context) error {
	nodes := s.registry.ListNodes()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"nodes": nodes,
		"stats": s.registry.Stats(),
	})
}

// NodeRegistrationRequest is the payload for node registration
type NodeRegistrationRequest struct {
	NodeID        string                `json:"node_id"`
	TailscaleIP   string                `json:"tailscale_ip"`
	Port          int                   `json:"port"`
	GPUType       string                `json:"gpu_type"`
	TotalVRAM     int64                 `json:"total_vram_mb"`
	AvailableVRAM int64                 `json:"available_vram_mb"`
	Models        map[string]*ModelInfo `json:"models"`
}

// handleRegisterNode handles POST /v1/inference/nodes/register
func (s *InferenceService) handleRegisterNode(c echo.Context) error {
	var req NodeRegistrationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	if req.NodeID == "" || req.TailscaleIP == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "node_id and tailscale_ip are required",
		})
	}

	if req.Port == 0 {
		req.Port = 11434 // Default Ollama port
	}

	if req.Models == nil {
		req.Models = make(map[string]*ModelInfo)
	}

	info := &NodeInferenceInfo{
		NodeID:        req.NodeID,
		TailscaleIP:   req.TailscaleIP,
		Port:          req.Port,
		GPUType:       req.GPUType,
		TotalVRAM:     req.TotalVRAM,
		AvailableVRAM: req.AvailableVRAM,
		Models:        req.Models,
	}

	s.registry.RegisterNode(info)

	log.Info().
		Str("node_id", req.NodeID).
		Str("tailscale_ip", req.TailscaleIP).
		Str("gpu_type", req.GPUType).
		Msg("Node registered for inference")

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "registered",
		"node_id": req.NodeID,
	})
}

// HeartbeatRequest is the payload for heartbeats
type HeartbeatRequest struct {
	Models        map[string]*ModelInfo `json:"models,omitempty"`
	AvailableVRAM int64                 `json:"available_vram_mb,omitempty"`
}

// handleHeartbeat handles POST /v1/inference/nodes/:nodeId/heartbeat
func (s *InferenceService) handleHeartbeat(c echo.Context) error {
	nodeID := c.Param("nodeId")

	var req HeartbeatRequest
	if err := c.Bind(&req); err != nil {
		// Body is optional
		req = HeartbeatRequest{}
	}

	// Update heartbeat
	s.registry.UpdateHeartbeat(nodeID)

	// Update models if provided
	if len(req.Models) > 0 {
		s.registry.UpdateNodeModels(nodeID, req.Models)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// handleHealth handles GET /v1/inference/health
func (s *InferenceService) handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, s.router.Health())
}

// ============================================================================
// Integration with existing Gateway
// ============================================================================

// These functions help integrate with the existing Gateway structure

// AddInferenceRoutes adds inference routes to an existing Echo group
// Call this from gateway.go in initHttp() to add inference endpoints
//
// Example usage in gateway.go:
//
//   func (g *Gateway) initHttp() error {
//       // ... existing code ...
//
//       // Add inference routes
//       g.inferenceRegistry = NewModelRegistry()
//       inferenceService := NewInferenceService(g.ctx, g.inferenceRegistry)
//       inferenceService.RegisterRoutes(g.baseRouteGroup.Group("/inference"))
//
//       // Start cleanup goroutine
//       go g.cleanupStaleInferenceNodes()
//
//       return nil
//   }
//
func AddInferenceRoutes(ctx context.Context, g *echo.Group) (*InferenceService, *ModelRegistry) {
	registry := NewModelRegistry()
	service := NewInferenceService(ctx, registry)
	service.RegisterRoutes(g)
	return service, registry
}

// StartRegistryCleanup starts a goroutine to periodically clean up stale nodes
func StartRegistryCleanup(ctx context.Context, registry *ModelRegistry) {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				removed := registry.CleanupStaleNodes()
				if removed > 0 {
					log.Info().Int("removed", removed).Msg("Cleaned up stale inference nodes")
				}
			}
		}
	}()
}
