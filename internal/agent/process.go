package agent

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"syscall"
	"time"
)

// stopProcess sends SIGTERM, waits up to 10s, then SIGKILL.
func stopProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		slog.Debug("SIGTERM failed", "pid", pid, "error", err)
		return
	}

	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(10 * time.Second):
		slog.Warn("process did not exit after SIGTERM, sending SIGKILL", "pid", pid)
		proc.Signal(syscall.SIGKILL)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
