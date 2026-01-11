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

// ANSI screen control codes
const (
	enterAltScreen = "\033[?1049h" // Switch to alternate screen buffer
	exitAltScreen  = "\033[?1049l" // Restore original screen buffer
	clearScreen    = "\033[2J"     // Clear entire screen
	moveCursorHome = "\033[H"      // Move cursor to home (0,0)
	hideCursor     = "\033[?25l"   // Hide cursor
	showCursor     = "\033[?25h"   // Show cursor
	clearToEnd     = "\033[J"      // Clear from cursor to end of screen
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
	width     int
	lastLines int // Track lines rendered for proper clearing
}

// NewTUI creates a new TUI renderer
func NewTUI() *TUI {
	return &TUI{
		width: 80, // Default width
	}
}

// EnterFullScreen switches to alternate screen buffer for clean TUI
func (t *TUI) EnterFullScreen() {
	fmt.Print(enterAltScreen) // Switch to alternate buffer
	fmt.Print(hideCursor)     // Hide cursor
	fmt.Print(clearScreen)    // Clear screen
	fmt.Print(moveCursorHome) // Move to top-left
}

// ExitFullScreen restores original screen buffer
func (t *TUI) ExitFullScreen() {
	fmt.Print(showCursor)    // Show cursor
	fmt.Print(exitAltScreen) // Restore original buffer
}

// Clear clears the terminal screen (used once at startup)
func (t *TUI) Clear() {
	fmt.Print(moveCursorHome + clearScreen)
}

// MoveCursorHome moves cursor to top-left without clearing
func (t *TUI) MoveCursorHome() {
	fmt.Print(moveCursorHome)
}

// ClearToEnd clears from cursor to end of screen
func (t *TUI) ClearToEnd() {
	fmt.Print(clearToEnd)
}

// HideCursor hides the terminal cursor
func (t *TUI) HideCursor() {
	fmt.Print(hideCursor)
}

// ShowCursor shows the terminal cursor
func (t *TUI) ShowCursor() {
	fmt.Print(showCursor)
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
			if i >= 5 { // Show max 5 jobs
				sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" %s... and %d more%s ", colorDim, len(snapshot.Jobs)-5, colorReset), boxVertical))
				break
			}
			sb.WriteString(t.renderLine(boxVertical, t.formatJob(job), boxVertical))
		}
	}

	// Separator
	sb.WriteString(t.renderLine(boxMidLeft, "", boxMidRight))

	// Inference status header
	sb.WriteString(t.renderLine(boxVertical, " INFERENCE ", boxVertical))
	sb.WriteString(t.renderLine(boxMidHoriz, "", boxMidHoriz))

	// Inference status content
	inferenceColor := t.inferenceStatusColor(snapshot.InferenceStatus)
	if snapshot.InferenceStatus == "stopped" {
		sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" Status: %s%s%s │ Waiting for start command ",
			inferenceColor, snapshot.InferenceStatus, colorReset), boxVertical))
	} else {
		endpoint := fmt.Sprintf("%s:%d", snapshot.InferenceIP, snapshot.InferencePort)
		if snapshot.InferenceIP == "" {
			endpoint = "not configured"
		}
		sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" Status: %s%s%s │ Endpoint: %s ",
			inferenceColor, snapshot.InferenceStatus, colorReset, endpoint), boxVertical))
	}

	// Models list
	if len(snapshot.InferenceModels) > 0 {
		models := strings.Join(snapshot.InferenceModels, ", ")
		if len(models) > 50 {
			models = models[:47] + "..."
		}
		sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" Models: %s ", models), boxVertical))
	}

	// Separator for logs
	sb.WriteString(t.renderLine(boxMidLeft, "", boxMidRight))

	// Logs header
	sb.WriteString(t.renderLine(boxVertical, " LOGS ", boxVertical))
	sb.WriteString(t.renderLine(boxMidHoriz, "", boxMidHoriz))

	// Logs content
	if len(snapshot.Logs) == 0 {
		sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" %sNo recent logs%s ", colorDim, colorReset), boxVertical))
	} else {
		for _, logLine := range snapshot.Logs {
			sb.WriteString(t.renderLine(boxVertical, fmt.Sprintf(" %s%s%s ", colorDim, logLine, colorReset), boxVertical))
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

// inferenceStatusColor returns ANSI color for inference status
func (t *TUI) inferenceStatusColor(status string) string {
	switch status {
	case "running":
		return colorGreen + colorBold
	case "starting":
		return colorYellow + colorBold
	case "stopped":
		return colorDim
	case "error":
		return colorRed + colorBold
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
