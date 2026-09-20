package plugin

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PluginStatus describes the current state of a plugin process.
type PluginStatus struct {
	Name  string
	State string // starting, running, stopping, stopped, failed
	PID   int
}

// runtimeBase returns the base directory for plugin runtime files.
func runtimeBase() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".nself", "runtime")
	}
	return filepath.Join(home, ".nself", "runtime")
}

// pidPath returns the file path for a plugin's PID file.
func pidPath(name string) string {
	return filepath.Join(runtimeBase(), "pids", name+".pid")
}

// statePath returns the file path for a plugin's state file.
func statePath(name string) string {
	return filepath.Join(runtimeBase(), "states", name+".state")
}

// logPath returns the file path for a plugin's log file.
func logPath(name string) string {
	return filepath.Join(runtimeBase(), "logs", name+".log")
}

// writeState writes the given state string to the plugin's state file.
func writeState(name string, state string) error {
	dir := filepath.Dir(statePath(name))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}
	return os.WriteFile(statePath(name), []byte(state), 0644)
}

// writePID writes the given PID to the plugin's PID file.
func writePID(name string, pid int) error {
	dir := filepath.Dir(pidPath(name))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating pid directory: %w", err)
	}
	return os.WriteFile(pidPath(name), []byte(strconv.Itoa(pid)), 0644)
}

// readPID reads the PID from the plugin's PID file.
func readPID(name string) (int, error) {
	data, err := os.ReadFile(pidPath(name))
	if err != nil {
		return 0, fmt.Errorf("reading pid file for %s: %w", name, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing pid for %s: %w", name, err)
	}
	return pid, nil
}

// readState reads the state from the plugin's state file.
func readState(name string) string {
	data, err := os.ReadFile(statePath(name))
	if err != nil {
		return "stopped"
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "stopped"
	}
	return s
}

// isProcessRunning checks whether a process with the given PID is alive.
func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 tests for process existence without actually sending a signal.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func Stop(ctx context.Context, name string) error {
	pid, err := readPID(name)
	if err != nil {
		return fmt.Errorf("plugin %s is not running: %w", name, err)
	}

	if !isProcessRunning(pid) {
		// Process already gone. Clean up stale files.
		_ = os.Remove(pidPath(name))
		_ = writeState(name, "stopped")
		return nil
	}

	_ = writeState(name, "stopping")

	// Send graceful stop signal to the process group (SIGTERM on Unix,
	// taskkill /T on Windows). Implementation is OS-specific.
	_ = terminateProcessGroup(pid)

	// Wait up to gracePeriod() for graceful shutdown.
	// DEP-03: reads DOCKER_STOP_GRACE_PERIOD env var; defaults to 30s.
	deadline := time.Now().Add(gracePeriod())
	for time.Now().Before(deadline) {
		if !isProcessRunning(pid) {
			break
		}
		// Check context cancellation.
		select {
		case <-ctx.Done():
			// Force kill on context cancellation.
			_ = killProcessGroup(pid)
			_ = os.Remove(pidPath(name))
			_ = writeState(name, "stopped")
			return ctx.Err()
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}

	// If still running after graceful timeout, hard kill.
	if isProcessRunning(pid) {
		_ = killProcessGroup(pid)
		// Brief wait for the kill signal to take effect.
		time.Sleep(200 * time.Millisecond)
	}

	// Clean up.
	_ = os.Remove(pidPath(name))
	_ = writeState(name, "stopped")

	return nil
}

// Status returns the current status of a plugin by reading its PID and state
// files. If the PID file indicates a process that is no longer running, the
// state is corrected to "stopped".
func Status(name string) (*PluginStatus, error) {
	state := readState(name)
	pid := 0

	if p, err := readPID(name); err == nil {
		pid = p
		// Reconcile: if state claims running but the process is dead, correct it.
		if (state == "running" || state == "starting") && !isProcessRunning(pid) {
			state = "stopped"
			_ = writeState(name, "stopped")
			_ = os.Remove(pidPath(name))
			pid = 0
		}
	} else {
		// No PID file. If state claims running, correct it.
		if state == "running" || state == "starting" {
			state = "stopped"
			_ = writeState(name, "stopped")
		}
	}

	return &PluginStatus{
		Name:  name,
		State: state,
		PID:   pid,
	}, nil
}

// health performs an HTTP health check against a plugin by sending a GET
// request to http://localhost:{port}/health. It returns true if the response
// status is 200 OK.
func health(ctx context.Context, name string, port int) (bool, error) {
	url := fmt.Sprintf("http://localhost:%d/health", port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("creating health request for %s: %w", name, err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Propagate the transport error instead of flattening it into a plain
		// "not healthy". The only caller (waitForPluginHealth, runtime_start.go)
		// keeps polling either way — it stops on ok, not on err — but it stores
		// this as lastErr and puts it in the message it finally returns.
		// Returning (false, nil) made every unreachable plugin come out as the
		// generic "timed out waiting for plugin to become healthy", with no
		// indication of whether the port was refused, the request never left,
		// or the context was cancelled.
		return false, fmt.Errorf("health check for %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK, nil
}
