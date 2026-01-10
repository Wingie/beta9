package agent

import (
	"testing"
	"time"
)

func TestNewAgentState(t *testing.T) {
	state := NewAgentState("machine123", "external", "100.72.101.23")

	if state.MachineID != "machine123" {
		t.Errorf("expected MachineID 'machine123', got '%s'", state.MachineID)
	}
	if state.PoolName != "external" {
		t.Errorf("expected PoolName 'external', got '%s'", state.PoolName)
	}
	if state.Gateway != "100.72.101.23" {
		t.Errorf("expected Gateway '100.72.101.23', got '%s'", state.Gateway)
	}
	if state.Status != "STARTING" {
		t.Errorf("expected Status 'STARTING', got '%s'", state.Status)
	}
	if len(state.Jobs) != 0 {
		t.Errorf("expected empty Jobs slice, got %d jobs", len(state.Jobs))
	}
}

func TestUpdateMetrics(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	state.UpdateMetrics(45.5, 62.3, 2)

	if state.CPUPercent != 45.5 {
		t.Errorf("expected CPUPercent 45.5, got %f", state.CPUPercent)
	}
	if state.MemoryPercent != 62.3 {
		t.Errorf("expected MemoryPercent 62.3, got %f", state.MemoryPercent)
	}
	if state.GPUCount != 2 {
		t.Errorf("expected GPUCount 2, got %d", state.GPUCount)
	}
}

func TestUpdateHeartbeat(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	// Test successful heartbeat
	state.UpdateHeartbeat(true)
	if state.HeartbeatStatus != "ok" {
		t.Errorf("expected HeartbeatStatus 'ok', got '%s'", state.HeartbeatStatus)
	}
	if state.Status != "READY" {
		t.Errorf("expected Status 'READY', got '%s'", state.Status)
	}
	if state.LastHeartbeat.IsZero() {
		t.Error("expected LastHeartbeat to be set")
	}

	// Test failed heartbeat
	state.UpdateHeartbeat(false)
	if state.HeartbeatStatus != "failed" {
		t.Errorf("expected HeartbeatStatus 'failed', got '%s'", state.HeartbeatStatus)
	}
	if state.Status != "UNHEALTHY" {
		t.Errorf("expected Status 'UNHEALTHY', got '%s'", state.Status)
	}
}

func TestUpdateHeartbeatPreservesBusyStatus(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	// Add a running job to make status BUSY
	state.AddJob(JobInfo{
		PodName: "worker-123",
		Status:  JobStatusRunning,
	})

	if state.Status != "BUSY" {
		t.Errorf("expected Status 'BUSY', got '%s'", state.Status)
	}

	// Successful heartbeat should not override BUSY
	state.UpdateHeartbeat(true)
	if state.Status != "BUSY" {
		t.Errorf("expected Status to remain 'BUSY', got '%s'", state.Status)
	}
}

func TestAddJob(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")
	state.UpdateHeartbeat(true) // Set to READY first

	// Add a running job
	job1 := JobInfo{
		PodName:   "worker-abc",
		TaskID:    "task-1",
		FuncName:  "hello:greet",
		Status:    JobStatusRunning,
		StartTime: time.Now(),
	}
	state.AddJob(job1)

	if len(state.Jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(state.Jobs))
	}
	if state.RunningJobs != 1 {
		t.Errorf("expected 1 running job, got %d", state.RunningJobs)
	}
	if state.Status != "BUSY" {
		t.Errorf("expected Status 'BUSY', got '%s'", state.Status)
	}
}

func TestAddJobUpdatesExisting(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	// Add a running job
	job1 := JobInfo{
		PodName: "worker-abc",
		Status:  JobStatusRunning,
	}
	state.AddJob(job1)

	// Update the same job to completed
	job1Updated := JobInfo{
		PodName:  "worker-abc",
		Status:   JobStatusCompleted,
		Duration: 500 * time.Millisecond,
	}
	state.AddJob(job1Updated)

	if len(state.Jobs) != 1 {
		t.Errorf("expected 1 job after update, got %d", len(state.Jobs))
	}
	if state.Jobs[0].Status != JobStatusCompleted {
		t.Errorf("expected job status COMPLETED, got %s", state.Jobs[0].Status)
	}
	if state.RunningJobs != 0 {
		t.Errorf("expected 0 running jobs, got %d", state.RunningJobs)
	}
}

func TestAddJobKeepsMax20(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	// Add 25 jobs
	for i := 0; i < 25; i++ {
		state.AddJob(JobInfo{
			PodName: "worker-" + string(rune('a'+i)),
			Status:  JobStatusCompleted,
		})
	}

	if len(state.Jobs) != 20 {
		t.Errorf("expected max 20 jobs, got %d", len(state.Jobs))
	}
}

func TestGetJobs(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	state.AddJob(JobInfo{PodName: "job1", Status: JobStatusRunning})
	state.AddJob(JobInfo{PodName: "job2", Status: JobStatusCompleted})

	jobs := state.GetJobs()
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}

	// Verify it's a copy (modifying shouldn't affect original)
	jobs[0].PodName = "modified"
	if state.Jobs[0].PodName == "modified" {
		t.Error("GetJobs should return a copy, not the original")
	}
}

func TestGetSnapshot(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")
	state.UpdateMetrics(50.0, 60.0, 1)
	state.AddJob(JobInfo{PodName: "job1", Status: JobStatusRunning})

	snapshot := state.GetSnapshot()

	if snapshot.MachineID != "m1" {
		t.Errorf("expected MachineID 'm1', got '%s'", snapshot.MachineID)
	}
	if snapshot.CPUPercent != 50.0 {
		t.Errorf("expected CPUPercent 50.0, got %f", snapshot.CPUPercent)
	}
	if len(snapshot.Jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(snapshot.Jobs))
	}

	// Verify it's a copy
	snapshot.Jobs[0].PodName = "modified"
	if state.Jobs[0].PodName == "modified" {
		t.Error("GetSnapshot should return a copy, not the original")
	}
}

func TestUptime(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	uptime := state.Uptime()
	if uptime < 10*time.Millisecond {
		t.Errorf("expected uptime >= 10ms, got %v", uptime)
	}
}

func TestTimeSinceHeartbeat(t *testing.T) {
	state := NewAgentState("m1", "pool", "gw")

	// Before any heartbeat
	if state.TimeSinceHeartbeat() != 0 {
		t.Error("expected 0 before first heartbeat")
	}

	state.UpdateHeartbeat(true)
	time.Sleep(10 * time.Millisecond)

	since := state.TimeSinceHeartbeat()
	if since < 10*time.Millisecond {
		t.Errorf("expected since >= 10ms, got %v", since)
	}
}

func TestJobStatusConstants(t *testing.T) {
	if JobStatusPending != "PENDING" {
		t.Errorf("expected PENDING, got %s", JobStatusPending)
	}
	if JobStatusRunning != "RUNNING" {
		t.Errorf("expected RUNNING, got %s", JobStatusRunning)
	}
	if JobStatusCompleted != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", JobStatusCompleted)
	}
	if JobStatusFailed != "FAILED" {
		t.Errorf("expected FAILED, got %s", JobStatusFailed)
	}
}
