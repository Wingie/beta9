package agent

import (
	"sync"
	"time"
)

// JobStatus represents the state of a job
type JobStatus string

const (
	JobStatusPending   JobStatus = "PENDING"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
)

// JobInfo tracks a single job/pod
type JobInfo struct {
	PodName   string
	TaskID    string
	FuncName  string
	Status    JobStatus
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	CPU       string // e.g., "45%"
	Memory    string // e.g., "128Mi"
	ExitCode  int
}

// AgentState holds the current state of the agent for the TUI
type AgentState struct {
	mu sync.RWMutex

	// Machine info
	MachineID string
	PoolName  string
	Gateway   string
	Status    string // READY, BUSY, UNHEALTHY

	// Metrics
	CPUPercent    float64
	MemoryPercent float64
	GPUCount      int

	// Timing
	StartTime       time.Time
	LastHeartbeat   time.Time
	HeartbeatStatus string // ok, failed

	// Jobs
	Jobs        []JobInfo
	RunningJobs int
	TotalJobs   int
}

// NewAgentState creates a new agent state
func NewAgentState(machineID, poolName, gateway string) *AgentState {
	return &AgentState{
		MachineID: machineID,
		PoolName:  poolName,
		Gateway:   gateway,
		Status:    "STARTING",
		StartTime: time.Now(),
		Jobs:      make([]JobInfo, 0),
	}
}

// UpdateMetrics updates CPU/Memory metrics
func (s *AgentState) UpdateMetrics(cpu, memory float64, gpus int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CPUPercent = cpu
	s.MemoryPercent = memory
	s.GPUCount = gpus
}

// UpdateHeartbeat records a heartbeat result
func (s *AgentState) UpdateHeartbeat(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastHeartbeat = time.Now()
	if success {
		s.HeartbeatStatus = "ok"
		if s.Status != "BUSY" {
			s.Status = "READY"
		}
	} else {
		s.HeartbeatStatus = "failed"
		s.Status = "UNHEALTHY"
	}
}

// AddJob adds or updates a job in the list
func (s *AgentState) AddJob(job JobInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if job already exists (update it)
	for i, existing := range s.Jobs {
		if existing.PodName == job.PodName {
			s.Jobs[i] = job
			s.updateJobCounts()
			return
		}
	}

	// Add new job at the front
	s.Jobs = append([]JobInfo{job}, s.Jobs...)

	// Keep only last 20 jobs
	if len(s.Jobs) > 20 {
		s.Jobs = s.Jobs[:20]
	}

	s.updateJobCounts()
}

// updateJobCounts updates running/total job counters
func (s *AgentState) updateJobCounts() {
	running := 0
	for _, job := range s.Jobs {
		if job.Status == JobStatusRunning || job.Status == JobStatusPending {
			running++
		}
	}
	s.RunningJobs = running
	s.TotalJobs = len(s.Jobs)
	if running > 0 {
		s.Status = "BUSY"
	} else if s.HeartbeatStatus == "ok" {
		s.Status = "READY"
	}
}

// Uptime returns the agent uptime
func (s *AgentState) Uptime() time.Duration {
	return time.Since(s.StartTime)
}

// TimeSinceHeartbeat returns time since last heartbeat
func (s *AgentState) TimeSinceHeartbeat() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.LastHeartbeat.IsZero() {
		return 0
	}
	return time.Since(s.LastHeartbeat)
}

// GetJobs returns a copy of the jobs list
func (s *AgentState) GetJobs() []JobInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]JobInfo, len(s.Jobs))
	copy(jobs, s.Jobs)
	return jobs
}

// GetSnapshot returns a snapshot of the state for rendering
func (s *AgentState) GetSnapshot() AgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := *s
	snapshot.Jobs = make([]JobInfo, len(s.Jobs))
	copy(snapshot.Jobs, s.Jobs)
	return snapshot
}
