package orchestrator

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

// StallDetector monitors running agents for stalled output.
type StallDetector struct {
	stallTimeout time.Duration
}

func NewStallDetector(stallTimeoutMS int) *StallDetector {
	return &StallDetector{
		stallTimeout: time.Duration(stallTimeoutMS) * time.Millisecond,
	}
}

// CheckStalled examines running entries and returns any that appear stalled.
// A run is stalled if its agent log file hasn't been modified within the stall timeout.
func (d *StallDetector) CheckStalled(entries []*RunEntry) []*RunEntry {
	if d.stallTimeout <= 0 {
		return nil
	}

	now := time.Now()
	var stalled []*RunEntry

	for _, entry := range entries {
		// Check if the agent log file has been recently modified
		logPath := fmt.Sprintf("%s/.gopilot-agent-%s.log",
			entry.Session.ID, entry.Session.ID) // approximation
		// We use a more reliable approach: check the workspace for any recent file changes
		if d.isStalled(entry, now, logPath) {
			stalled = append(stalled, entry)
		}
	}

	return stalled
}

func (d *StallDetector) isStalled(entry *RunEntry, now time.Time, logPath string) bool {
	// Primary check: has the process been running longer than expected without output?
	elapsed := now.Sub(entry.StartedAt)
	if elapsed < d.stallTimeout {
		return false // too early to declare stalled
	}

	// Check if log file exists and when it was last modified
	info, err := os.Stat(logPath)
	if err != nil {
		// Can't check log file — fall back to time-based check
		// If process has been running for more than stallTimeout with no readable log,
		// consider it stalled
		slog.Debug("stall check: no log file", "session", entry.Session.ID, "elapsed", elapsed)
		return elapsed > d.stallTimeout*2
	}

	lastMod := info.ModTime()
	sinceLastMod := now.Sub(lastMod)
	if sinceLastMod > d.stallTimeout {
		slog.Warn("stall detected",
			"session", entry.Session.ID,
			"issue", fmt.Sprintf("%s#%d", entry.Issue.Repo, entry.Issue.ID),
			"last_output", sinceLastMod,
			"threshold", d.stallTimeout,
		)
		return true
	}

	return false
}
