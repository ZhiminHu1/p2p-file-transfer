package peer

import (
	"fmt"
	"strings"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"
	"time"
)

// ANSI color codes for terminal output
const (
	Reset   = "\033[0m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Gray    = "\033[90m"
	Bold    = "\033[1m"
)

// ProgressRenderer handles the rendering of download progress to the terminal
type ProgressRenderer struct {
	tracker     *DownloadTracker
	stopChan    chan struct{}
	refreshRate time.Duration
	useColors   bool
	width       int
	lastLineLen int
}

// NewProgressRenderer creates a new progress renderer
func NewProgressRenderer(tracker *DownloadTracker, useColors bool) *ProgressRenderer {
	return &ProgressRenderer{
		tracker:     tracker,
		stopChan:    make(chan struct{}),
		refreshRate: 200 * time.Millisecond,
		useColors:   useColors,
		width:       40, // Progress bar width
		lastLineLen: 0,
	}
}

// SetRefreshRate sets the refresh rate for the progress bar
func (pr *ProgressRenderer) SetRefreshRate(rate time.Duration) {
	pr.refreshRate = rate
}

// SetWidth sets the width of the progress bar
func (pr *ProgressRenderer) SetWidth(width int) {
	pr.width = width
}

// Start begins the render loop
func (pr *ProgressRenderer) Start() {
	// Initial render
	pr.Render()

	ticker := time.NewTicker(pr.refreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pr.tracker.UpdateSpeed()
			pr.Render()
		case <-pr.stopChan:
			return
		}
	}
}

// Stop signals the renderer to stop (does not wait for completion)
func (pr *ProgressRenderer) Stop() {
	close(pr.stopChan)
}

// StopAndWait stops the renderer and waits for final render
func (pr *ProgressRenderer) StopAndWait() {
	close(pr.stopChan)
	tracker := pr.tracker
	if tracker == nil {
		logger.Sugar.Warn("[PeerServer] No tracker to stop")
		return
	}
	if tracker.completedSize == tracker.FileSize {
		// 渲染最终下载成功的状态
		pr.RenderFinal()
	} else {
		// 渲染失败的状态
		pr.RenderError()
	}
}

// Render renders the current progress to the terminal
func (pr *ProgressRenderer) Render() {
	completed, total, speed, peerCount, failed := pr.tracker.GetProgress()
	eta := pr.tracker.GetETA()
	bytesDownloaded := pr.tracker.GetBytesDownloaded()
	fileSize := pr.tracker.GetFileSize()

	// Calculate byte percentage for more accurate progress
	bytePercent := float64(bytesDownloaded) / float64(fileSize) * 100

	// Build progress bar - use byte percentage for smoother updates
	barWidth := pr.width
	filled := int(float64(barWidth) * bytePercent / 100)
	if filled > barWidth {
		filled = barWidth
	}

	// Use different characters for the bar
	barFilled := strings.Repeat("█", filled)
	barEmpty := strings.Repeat("░", barWidth-filled)
	bar := barFilled + barEmpty

	// Format speed
	speedStr := formatBytes(speed)

	// Format ETA
	etaStr := formatETA(eta)

	// Build output line
	var line string
	if pr.useColors {
		line = fmt.Sprintf("\r%s[%s]%s [%s]%s %.1f%% (%d/%d chunks) | %s/s | %d peers | ETA: %s",
			Cyan, pr.tracker.FileName, Reset,
			Green+bar+Reset,
			Yellow, bytePercent, completed, total,
			Blue+speedStr+Reset, peerCount, etaStr,
		)
	} else {
		line = fmt.Sprintf("\r[%s] [%s] %.1f%% (%d/%d chunks) | %s/s | %d peers | ETA: %s",
			pr.tracker.FileName, bar, bytePercent, completed, total,
			speedStr, peerCount, etaStr,
		)
	}

	// Add failed count if any
	if failed > 0 {
		if pr.useColors {
			line += Red + fmt.Sprintf(" | %d failed", failed) + Reset
		} else {
			line += fmt.Sprintf(" | %d failed", failed)
		}
	}

	pr.lastLineLen = len(line)
	fmt.Print(line)
}

// RenderFinal renders the final completed state
func (pr *ProgressRenderer) RenderFinal() {
	_, total, _, _, _ := pr.tracker.GetProgress()
	elapsed := pr.tracker.GetElapsedTime()

	// Clear the previous line completely
	fmt.Print("\r\033[K")

	var line string
	if pr.useColors {
		line = fmt.Sprintf("%s[%s]%s [%s]%s 100%% (%d/%d chunks)%s | Completed in %s\n",
			Cyan, pr.tracker.FileName, Reset,
			Green+strings.Repeat("█", pr.width)+Reset,
			Green, total, total, Reset,
			formatDuration(elapsed),
		)
	} else {
		line = fmt.Sprintf("[%s] [%s] 100%% (%d/%d chunks) | Completed in %s\n",
			pr.tracker.FileName, strings.Repeat("█", pr.width),
			total, total, formatDuration(elapsed),
		)
	}

	fmt.Print(line)

}

// RenderError renders an error state
func (pr *ProgressRenderer) RenderError() {
	// Clear the previous line completely
	fmt.Print("\r\033[K")

	completed, total, _, _, failed := pr.tracker.GetProgress()

	var line string
	if pr.useColors {
		line = fmt.Sprintf("%s[%s]%s [%s] %.1f%% | %sDownload failed%s: %d/%d completed, %d failed\n",
			Cyan, pr.tracker.FileName, Reset,
			Red+"✗"+Reset,
			float64(completed)/float64(total)*100,
			Red, Bold, completed, total, failed,
		)
	} else {
		line = fmt.Sprintf("[%s] [✗] %.1f%% | Download failed: %d/%d completed, %d failed\n",
			pr.tracker.FileName,
			float64(completed)/float64(total)*100,
			completed, total, failed,
		)
	}

	fmt.Print(line)
}

// formatBytes formats a byte count into a human-readable string
func formatBytes(bytes float64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%.1f B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", bytes/float64(div), "KMGTPE"[exp])
}

// formatETA formats an estimated time into a human-readable string
func formatETA(eta time.Duration) string {
	if eta <= 0 {
		return "∞"
	}
	if eta < time.Second {
		return "<1s"
	}
	if eta < time.Minute {
		return fmt.Sprintf("%ds", eta/time.Second)
	}
	if eta < time.Hour {
		mins := eta / time.Minute
		secs := (eta % time.Minute) / time.Second
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := eta / time.Hour
	mins := (eta % time.Hour) / time.Minute
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", d/time.Second)
	}
	if d < time.Hour {
		mins := d / time.Minute
		secs := (d % time.Minute) / time.Second
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := d / time.Hour
	mins := (d % time.Hour) / time.Minute
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// IsTerminalSupported checks if the current environment supports terminal codes
// This is a simple check - in a real application you might use more sophisticated detection
func IsTerminalSupported() bool {
	// For now, always return true on non-Windows, or check environment
	// In production, you'd use sys/unix.IoctlGetTermios or similar
	return true
}
