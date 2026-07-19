package apiv1

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"

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
	inferenceRegistry InferenceRegistry
}

// InferenceRegistry mirrors the subset of gateway.ModelRegistry's methods
// this package needs. apiv1 can't import gateway (gateway already imports
// apiv1), so this narrow interface breaks the cycle — but it must use the
// SAME concrete parameter types gateway.ModelRegistry actually implements
// (pkg/types.NodeInferenceInfo / pkg/types.ModelInfo, both already shared
// and importable here). The previous version declared RegisterNode(any) /
// UpdateNodeModels(nodeID string, any): Go requires exact signature matches
// for interface satisfaction, so *gateway.ModelRegistry never satisfied it
// and `inferenceRegistry.(InferenceRegistry)` below panicked on every
// gateway startup (P0, beta9 security baseline 2026-04-20 onward).
type InferenceRegistry interface {
	RegisterNode(info *types.NodeInferenceInfo)
	UpdateHeartbeat(nodeID string)
	UpdateNodeModels(nodeID string, models map[string]*types.ModelInfo)
}

func NewMachineGroup(g *echo.Group, providerRepo repository.ProviderRepository, tailscale *network.Tailscale, config types.AppConfig, workerRepo repository.WorkerRepository, inferenceRegistry any) *MachineGroup {
	// Non-panicking assertion as defense in depth: if a caller ever passes
	// something that doesn't satisfy InferenceRegistry, disable inference
	// keepalive/registration for this group instead of crashing the gateway.
	registry, ok := inferenceRegistry.(InferenceRegistry)
	if !ok && inferenceRegistry != nil {
		log.Warn().
			Str("type", fmt.Sprintf("%T", inferenceRegistry)).
			Msg("inferenceRegistry does not implement apiv1.InferenceRegistry; inference keepalive/registration disabled for this machine group")
	}
	group := &MachineGroup{routerGroup: g,
		providerRepo:      providerRepo,
		tailscale:         tailscale,
		config:            config,
		workerRepo:        workerRepo,
		inferenceRegistry: registry,
	}

	g.GET("/:workspaceId/gpus", auth.WithWorkspaceAuth(group.GPUCounts))
	// RegisterMachine/MachineKeepalive/GetConfig/ListPoolMachines all do
	// `cc, _ := ctx.(*auth.HttpAuthContext)` and immediately dereference
	// `cc.AuthInfo...` with no ok-check. Unwrapped, a caller with no token
	// hits fail-open AuthMiddleware (ctx stays plain echo.Context), so cc
	// is nil and every one of these routes nil-derefs on the first line
	// (security baseline P1: "nil-deref panic on unauth /v1/machine/*").
	// auth.WithAuth guarantees ctx is *HttpAuthContext before the handler
	// runs, which both fixes the panic and requires real authentication.
	g.POST("/register", auth.WithAuth(group.RegisterMachine))
	g.POST("/keepalive", auth.WithAuth(group.MachineKeepalive))
	g.GET("/config", auth.WithAuth(group.GetConfig))
	g.GET("/list", auth.WithAuth(group.ListPoolMachines))
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

// tailscaleCGNAT / isTailscaleAddr duplicate pkg/gateway's inference_handlers.go
// helper of the same purpose — apiv1 can't import gateway (gateway already
// imports apiv1), so this is a small, intentional duplication rather than a
// new shared package for one four-line check. If a third caller ever needs
// it, move both copies to pkg/network.
var tailscaleCGNAT = func() *net.IPNet {
	_, n, err := net.ParseCIDR("100.64.0.0/10")
	if err != nil {
		panic(err)
	}
	return n
}()

func isTailscaleAddr(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && tailscaleCGNAT.Contains(parsed)
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
		if request.Inference.Status == "running" && request.Inference.IP != "" && !isTailscaleAddr(request.Inference.IP) {
			// P1 (beta9 security baseline): a compromised-but-authenticated
			// worker token could otherwise redirect all chat/embed traffic
			// for its machine_id to an arbitrary IP via keepalive. Same
			// CGNAT check as /inference/nodes/register in the gateway package.
			return HTTPBadRequest("inference.ip must be within the Tailscale CGNAT range (100.64.0.0/10)")
		}
		if request.Inference.Status == "running" {
			// Register if needed (idempotent usually) or just update heartbeat
			// Since we don't have full VRAM info here (it's in metrics but flat),
			// we construct a best-effort update.

			// Transform models list to map
			modelsMap := make(map[string]*types.ModelInfo)
			for _, m := range request.Inference.Models {
				modelsMap[m] = &types.ModelInfo{
					Name:      m,
					LoadState: types.LoadStateReady, // Assume ready if reported
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
			info := &types.NodeInferenceInfo{
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
