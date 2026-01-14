package apiv1

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/beam-cloud/beta9/pkg/auth"
	"github.com/beam-cloud/beta9/pkg/network"
	"github.com/beam-cloud/beta9/pkg/providers"
	"github.com/beam-cloud/beta9/pkg/repository"
	"github.com/beam-cloud/beta9/pkg/scheduler"
	"github.com/beam-cloud/beta9/pkg/types"
)

type RegisterMachineRequest struct {
	Token        string `json:"token"`
	MachineID    string `json:"machine_id"`
	HostName     string `json:"hostname"`
	ProviderName string `json:"provider_name"`
	PoolName     string `json:"pool_name"`
	Cpu          string `json:"cpu"`
	Memory       string `json:"memory"`
	GpuCount     string `json:"gpu_count"`
	PrivateIP    string `json:"private_ip"`
}

type MachineKeepaliveRequest struct {
	MachineID    string                        `json:"machine_id"`
	ProviderName string                        `json:"provider_name"`
	PoolName     string                        `json:"pool_name"`
	AgentVersion string                        `json:"agent_version"`
	Metrics      *types.ProviderMachineMetrics `json:"metrics"`
	Inference    *InferenceStatus              `json:"inference,omitempty"`
}

type InferenceStatus struct {
	Status string   `json:"status"` // stopped, starting, running, error
	IP     string   `json:"ip,omitempty"`
	Port   int      `json:"port,omitempty"`
	Models []string `json:"models,omitempty"`
}

type MachineGroup struct {
	providerRepo      repository.ProviderRepository
	tailscale         *network.Tailscale
	routerGroup       *echo.Group
	config            types.AppConfig
	workerRepo        repository.WorkerRepository
	inferenceRegistry interface {
		RegisterNode(info interface{})
		UpdateHeartbeat(nodeID string)
		UpdateNodeModels(nodeID string, models interface{})
	}
}

// Interface for model registry to avoid circular imports if possible,
// or we need to import it. Since gateway imports apiv1, apiv1 cannot import gateway.
// We'll define the interface here or pass it as 'any' and cast it, or better, move model registry to a shared package.
// For now, let's use a narrow interface matching methods we need.
type InferenceRegistry interface {
	RegisterNode(info any)
	UpdateHeartbeat(nodeID string)
	UpdateNodeModels(nodeID string, models any)
}

func NewMachineGroup(g *echo.Group, providerRepo repository.ProviderRepository, tailscale *network.Tailscale, config types.AppConfig, workerRepo repository.WorkerRepository, inferenceRegistry any) *MachineGroup {
	group := &MachineGroup{routerGroup: g,
		providerRepo:      providerRepo,
		tailscale:         tailscale,
		config:            config,
		workerRepo:        workerRepo,
		inferenceRegistry: inferenceRegistry.(InferenceRegistry),
	}

	g.GET("/:workspaceId/gpus", auth.WithWorkspaceAuth(group.GPUCounts))
	g.POST("/register", group.RegisterMachine)
	g.POST("/keepalive", group.MachineKeepalive)
	g.GET("/config", group.GetConfig)
	g.GET("/list", group.ListPoolMachines)
	return group
}

func (g *MachineGroup) ListPoolMachines(ctx echo.Context) error {
	cc, _ := ctx.(*auth.HttpAuthContext)
	if (cc.AuthInfo.Token.TokenType != types.TokenTypeMachine) && (cc.AuthInfo.Token.TokenType != types.TokenTypeWorker) {
		return HTTPForbidden("Invalid token")
	}

	poolName := ctx.QueryParam("pool_name")
	providerName := ctx.QueryParam("provider_name")

	machines, err := g.providerRepo.ListAllMachines(providerName, poolName, false)
	if err != nil {
		return HTTPInternalServerError("Failed to get neighbors")
	}

	availableMachines := make([]*types.ProviderMachine, 0)
	for _, machine := range machines {
		if machine.State.Status != types.MachineStatusReady {
			continue
		}

		availableMachines = append(availableMachines, machine)
	}

	return ctx.JSON(http.StatusOK, availableMachines)
}

func (g *MachineGroup) GetConfig(ctx echo.Context) error {
	cc, _ := ctx.(*auth.HttpAuthContext)
	if (cc.AuthInfo.Token.TokenType != types.TokenTypeMachine) && (cc.AuthInfo.Token.TokenType != types.TokenTypeWorker) {
		return HTTPForbidden("Invalid token")
	}

	remoteConfig, err := providers.GetRemoteConfig(g.config, g.tailscale)
	if err != nil {
		return HTTPInternalServerError("Unable to create remote config")
	}

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"config": remoteConfig,
	})
}

func (g *MachineGroup) RegisterMachine(ctx echo.Context) error {
	cc, _ := ctx.(*auth.HttpAuthContext)
	if (cc.AuthInfo.Token.TokenType != types.TokenTypeMachine) && (cc.AuthInfo.Token.TokenType != types.TokenTypeWorker) {
		return HTTPForbidden("Invalid token")
	}

	var request RegisterMachineRequest
	if err := ctx.Bind(&request); err != nil {
		return HTTPBadRequest("Invalid payload")
	}

	// Get remote config for Tailscale-connected workers
	// For external pools using SSH tunnel, this may fail - that's OK
	remoteConfig, err := providers.GetRemoteConfig(g.config, g.tailscale)
	if err != nil {
		// Return nil config - external workers via SSH tunnel don't need it
		remoteConfig = nil
	}

	cpu, err := scheduler.ParseCPU(request.Cpu)
	if err != nil {
		return HTTPInternalServerError("Invalid machine cpu value")
	}

	memory, err := scheduler.ParseMemory(request.Memory)
	if err != nil {
		return HTTPInternalServerError("Invalid machine memory value")
	}

	gpuCount, err := strconv.ParseUint(request.GpuCount, 10, 32)
	if err != nil {
		return HTTPInternalServerError("Invalid gpu count")
	}

	// Determine hostname format based on connectivity mode
	var hostName string
	if g.config.Tailscale.DirectRedisHost != "" {
		// Direct mode: use hostname as-is (IP address for direct k8s access)
		hostName = request.HostName
	} else {
		// Tailscale MagicDNS mode: append Tailscale hostname suffix
		hostName = fmt.Sprintf("%s.%s", request.HostName, g.config.Tailscale.HostName)
		// If user is != "", add it into hostname (for self-managed control servers like headscale)
		if g.config.Tailscale.User != "" {
			hostName = fmt.Sprintf("%s.%s.%s", request.HostName, g.config.Tailscale.User, g.config.Tailscale.HostName)
		}
	}

	poolConfig, ok := g.config.Worker.Pools[request.PoolName]
	if !ok {
		return HTTPInternalServerError("Invalid pool name")
	}

	err = g.providerRepo.RegisterMachine(request.ProviderName, request.PoolName, request.MachineID, &types.ProviderMachineState{
		MachineId: request.MachineID,
		Token:     request.Token,
		HostName:  hostName,
		Cpu:       cpu,
		Memory:    memory,
		GpuCount:  uint32(gpuCount),
		PrivateIP: request.PrivateIP,
	}, &poolConfig)
	if err != nil {
		return HTTPInternalServerError("Failed to register machine")
	}

	return ctx.JSON(http.StatusOK, RegisterMachineResponse{
		Config: remoteConfig,
		MachineState: &MachineStateResponse{
			MachineID:  request.MachineID,
			Status:     types.MachineStatusRegistered,
			PoolName:   request.PoolName,
			TTLSeconds: types.MachineKeepaliveExpirationS,
		},
	})
}

// Helper structs for reflection/dynamic typing without importing gateway
type NodeInferenceInfo struct {
	NodeID        string                `json:"node_id"`
	TailscaleIP   string                `json:"tailscale_ip"`
	Port          int                   `json:"port"`
	GPUType       string                `json:"gpu_type"`
	TotalVRAM     int64                 `json:"total_vram_mb"`
	AvailableVRAM int64                 `json:"available_vram_mb"`
	Models        map[string]*ModelInfo `json:"models"`
}

type ModelInfo struct {
	Name      string `json:"name"`
	LoadState string `json:"load_state"`
	LastUsed  any    `json:"last_used"`
	LoadedAt  any    `json:"loaded_at"`
}

func (g *MachineGroup) MachineKeepalive(ctx echo.Context) error {
	cc, _ := ctx.(*auth.HttpAuthContext)
	if (cc.AuthInfo.Token.TokenType != types.TokenTypeMachine) && (cc.AuthInfo.Token.TokenType != types.TokenTypeWorker) {
		return HTTPForbidden("Invalid token")
	}

	var request MachineKeepaliveRequest
	if err := ctx.Bind(&request); err != nil {
		return HTTPBadRequest("Invalid payload")
	}

	if request.MachineID == "" || request.PoolName == "" {
		return HTTPBadRequest("machine_id and pool_name are required")
	}

	// Default provider name if not specified
	providerName := request.ProviderName
	if providerName == "" {
		providerName = "generic"
	}

	err := g.providerRepo.SetMachineKeepAlive(
		providerName,
		request.PoolName,
		request.MachineID,
		request.AgentVersion,
		request.Metrics,
	)
	if err != nil {
		return HTTPInternalServerError(fmt.Sprintf("Failed to update keepalive: %v", err))
	}

	// Update inference status if available
	if request.Inference != nil && g.inferenceRegistry != nil {
		if request.Inference.Status == "running" {
			// Register if needed (idempotent usually) or just update heartbeat
			// Since we don't have full VRAM info here (it's in metrics but flat),
			// we construct a best-effort update.

			// Transform models list to map
			modelsMap := make(map[string]*ModelInfo)
			for _, m := range request.Inference.Models {
				modelsMap[m] = &ModelInfo{
					Name:      m,
					LoadState: "ready", // Assume ready if reported
				}
			}

			// If verify registration needs more info, we might need to handle that.
			// Ideally we call UpdateHeartbeat and UpdateNodeModels
			g.inferenceRegistry.UpdateHeartbeat(request.MachineID)
			g.inferenceRegistry.UpdateNodeModels(request.MachineID, modelsMap)

			// Also ensure node is registered with IP if not already?
			// The registry.RegisterNode does upsert.
			// We can try to register it on every heartbeat to be safe or only if missing?
			// For now let's just update models/heartbeat which should be enough if registered via /inference/nodes/register
			// But wait, the agent doesn't call /inference/nodes/register explicitly in its main loop?
			// Ah, the agent doesn't seem to call /inference/nodes/register in `runWithTUI`?
			// It probably should rely on keepalive.

			// So let's do a RegisterNode call here with available info
			info := &NodeInferenceInfo{
				NodeID:      request.MachineID,
				TailscaleIP: request.Inference.IP,
				Port:        request.Inference.Port,
				GPUType:     "MPS", // TODO: Infer from metrics?
				Models:      modelsMap,
				// VRAM from metrics if available
			}
			// metrics has GPU info?
			// request.Metrics is *types.ProviderMachineMetrics
			// It has GpuCount but maybe not VRAM details easily?
			// Let's rely on update for now.

			g.inferenceRegistry.RegisterNode(info)
		}
	}

	// Fetch updated machine state to include in response
	response := KeepaliveResponse{
		Status: "ok",
	}

	machineState, err := g.providerRepo.GetMachine(providerName, request.PoolName, request.MachineID)
	if err == nil && machineState != nil && machineState.State != nil {
		response.MachineState = &MachineStateResponse{
			MachineID:      request.MachineID,
			Status:         machineState.State.Status,
			PoolName:       request.PoolName,
			LastKeepalive:  machineState.State.LastKeepalive,
			LastWorkerSeen: machineState.State.LastWorkerSeen,
			TTLSeconds:     types.MachineKeepaliveExpirationS,
			AgentVersion:   machineState.State.AgentVersion,
		}
	}

	return ctx.JSON(http.StatusOK, response)
}

func (g *MachineGroup) GPUCounts(ctx echo.Context) error {
	gpuCounts, err := g.workerRepo.GetGpuCounts()
	if err != nil {
		return HTTPInternalServerError("Unable to list GPUs")
	}

	return ctx.JSON(http.StatusOK, gpuCounts)
}
