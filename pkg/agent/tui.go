package agent

import (
	"fmt"
	"strings"
	"time"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// Box drawing characters
const (
	boxTopLeft     = "╔"
	boxTopRight    = "╗"
	boxBottomLeft  = "╚"
	boxBottomRight = "╝"
	boxHorizontal  = "═"
	boxVertical    = "║"
	boxMidLeft     = "╠"
	boxMidRight    = "╣"
	boxMidHoriz    = "╟"
	boxMidVert     = "─"
)

// TUI handles terminal rendering
type TUI struct {
	width int
}

// NewTUI creates a new TUI renderer
func NewTUI() *TUI {
	return &TUI{
		width: 80, // Default width
	}
}

// Clear clears the terminal screen
func (t *TUI) Clear() {
	fmt.Print("\033[H\033[2J")
}

// Render renders the current state to terminal
func (t *TUI) Render(state *AgentState) string {
	var sb strings.Builder
	snapshot := state.GetSnapshot()

	// Header line
	sb.WriteString(t.renderLine(boxTopLeft, fmt.Sprintf(" Beta9 Agent: %s ", snapshot.MachineID), boxTopRight))

	// Status line
	statusColor := t.statusColor(snapshot.Status)
	statusLine := fmt.Sprintf(" Status: %s%s%s │ Gateway: %s │ Pool: %s │ Uptime: %s ",
		statusColor, snapshot.Status, colorReset,
		snapshot.Gateway,
		snapshot.PoolName,
		t.formatDuration(snapshot.Uptime()))
	sb.WriteString(t.renderLine(boxVertical, statusLine, boxVertical))

	// Metrics line
	heartbeatAgo := "never"
	if !snapshot.LastHeartbeat.IsZero() {
		heartbeatAgo = t.formatDuration(snapshot.TimeSinceHeartbeat()) + " ago"
	}
	metricsLine := fmt.Sprintf(" CPU: %.1f%% │ Memory: %.1f%% │ GPUs: %d │ Last Heartbeat: %s ",
		snapshot.CPUPercent,
		snapshot.MemoryPercent,
		snapshot.GPUCount,
		heartbeatAgo)
	sb.WriteString(t.renderLine(boxVertical, metricsLine, boxVertical))

	// Separator
	sb.WriteString(t.renderLine(boxMidLeft, "", boxMidRight))

	// Jobs header
	sb.WriteString(t.renderLine(boxVertical, " WORKER PODS ", boxVertical))
	sb.WriteString(t.renderLine(boxMidHoriz, "", boxMidHoriz))

	// Jobs list
	if len(snapshot.Jobs) == 0 {
		sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" %sNo jobs yet%s ", colorDim, colorReset), boxVertical))
	} else {
		for i, job := range snapshot.Jobs {
			if i >= 10 { // Show max 10 jobs
				sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" %s... and %d more%s ", colorDim, len(snapshot.Jobs)-10, colorReset), boxVertical))
				break
			}
			sb.WriteString(t.renderLine(boxVertical, t.formatJob(job), boxVertical))
		}
	}

	// Footer
	sb.WriteString(t.renderLine(boxBottomLeft, "", boxBottomRight))
	sb.WriteString(fmt.Sprintf("%sPress Ctrl+C to quit%s\n", colorDim, colorReset))

	return sb.String()
}

// renderLine renders a single line with box characters
func (t *TUI) renderLine(left, content, right string) string {
	// Strip ANSI codes for length calculation
	cleanContent := stripANSI(content)
	padding := t.width - len(cleanContent) - 2 // -2 for left/right borders

	if padding < 0 {
		padding = 0
	}

	fillChar := " "
	if left == boxMidLeft || left == boxMidHoriz {
		fillChar = boxHorizontal
		if left == boxMidHoriz {
			fillChar = boxMidVert
		}
	}
	if left == boxTopLeft || left == boxBottomLeft {
		fillChar = boxHorizontal
	}

	return left + content + strings.Repeat(fillChar, padding) + right + "\n"
}

// statusColor returns ANSI color for status
func (t *TUI) statusColor(status string) string {
	switch status {
	case "READY":
		return colorGreen + colorBold
	case "BUSY":
		return colorYellow + colorBold
	case "UNHEALTHY":
		return colorRed + colorBold
	case "STARTING":
		return colorCyan + colorBold
	default:
		return colorWhite
	}
}

// formatDuration formats a duration for display
func (t *TUI) formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// formatJob formats a job for display
func (t *TUI) formatJob(job JobInfo) string {
	statusColor := t.jobStatusColor(job.Status)

	// Calculate duration
	duration := job.Duration
	if duration == 0 && !job.StartTime.IsZero() {
		if job.EndTime.IsZero() {
			duration = time.Since(job.StartTime)
		} else {
			duration = job.EndTime.Sub(job.StartTime)
		}
	}

	// Format age for completed jobs
	age := ""
	if job.Status == JobStatusCompleted || job.Status == JobStatusFailed {
		if !job.EndTime.IsZero() {
			age = fmt.Sprintf(" (%s ago)", t.formatDuration(time.Since(job.EndTime)))
		}
	}

	// Truncate function name if too long
	funcName := job.FuncName
	if len(funcName) > 25 {
		funcName = funcName[:22] + "..."
	}

	return fmt.Sprintf(" %-15s %s%-10s%s %-25s %8s%s ",
		truncate(job.PodName, 15),
		statusColor, job.Status, colorReset,
		funcName,
		t.formatDuration(duration),
		age)
}

// jobStatusColor returns ANSI color for job status
func (t *TUI) jobStatusColor(status JobStatus) string {
	switch status {
	case JobStatusRunning:
		return colorGreen
	case JobStatusCompleted:
		return colorBlue
	case JobStatusFailed:
		return colorRed
	case JobStatusPending:
		return colorYellow
	default:
		return colorWhite
	}
}

// truncate truncates a string to max length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	result := s
	for {
		start := strings.Index(result, "\033[")
		if start == -1 {
			break
		}
		end := strings.IndexByte(result[start:], 'm')
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+1:]
	}
	return result
}
