package plugin

// Purpose: process-start path for a plugin runtime (Start, and the grace-period helper Stop depends on).
// Inputs: a plugin directory and name, resolved against the plugin's manifest entry point.
// Outputs: a running background process with its PID and state recorded under ~/.nself/runtime/.
// Constraints: split out of runtime.go as a pure move (CLI-R12); no behavior change.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Start launches a plugin process in the background. It locates the plugin's
// entry point inside pluginDir, starts it, writes the PID to
// ~/.nself/runtime/pids/{name}.pid, and records the state in
// ~/.nself/runtime/states/{name}.state.
func Start(ctx context.Context, pluginDir string, name string) error {
	// Check if already running.
	if pid, err := readPID(name); err == nil && isProcessRunning(pid) {
		return fmt.Errorf("plugin %s is already running (pid %d)", name, pid)
	}

	// Set state to starting.
	if err := writeState(name, "starting"); err != nil {
		return fmt.Errorf("setting starting state for %s: %w", name, err)
	}

	// Determine entry point from manifest if available.
	entryPoint := ""
	manifestPath := filepath.Join(pluginDir, "plugin.json")
	var manifest *PluginManifest
	if m, err := parseManifest(manifestPath); err == nil {
		manifest = m
		if m.EntryPoint != "" {
			entryPoint = filepath.Join(pluginDir, m.EntryPoint)
		}
	}

	// Build the command to execute.
	var cmd *exec.Cmd
	if entryPoint != "" {
		// Detect runtime from file extension.
		switch {
		case strings.HasSuffix(entryPoint, ".js"):
			cmd = exec.CommandContext(ctx, "node", entryPoint)
		case strings.HasSuffix(entryPoint, ".ts"):
			cmd = exec.CommandContext(ctx, "npx", "tsx", entryPoint)
		default:
			// Assume a binary.
			cmd = exec.CommandContext(ctx, entryPoint)
		}
	} else {
		// Fallback: look for an executable named after the plugin.
		binPath := filepath.Join(pluginDir, name)
		if _, err := os.Stat(binPath); err != nil {
			_ = writeState(name, "failed")
			// A plugin shipping docker-compose.plugin.yml is a compose
			// service, not a background process: it has no entry point by
			// design and `nself build` + `nself start` are what run it.
			// Saying only "no entry point found" reads like a broken install
			// and sends the caller looking for a missing binary.
			if _, cErr := os.Stat(filepath.Join(pluginDir, "docker-compose.plugin.yml")); cErr == nil {
				return fmt.Errorf(
					"plugin %s runs as a compose service, not a background process; "+
						"run `nself build && nself start` to bring it up", name)
			}
			return fmt.Errorf("no entry point found for plugin %s", name)
		}
		cmd = exec.CommandContext(ctx, binPath)
	}

	cmd.Dir = pluginDir

	// Load plugin .env if present.
	envFile := filepath.Join(pluginDir, ".env")
	cmd.Env = os.Environ()
	if data, err := os.ReadFile(envFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			cmd.Env = append(cmd.Env, line)
		}
	}

	// Redirect stdout and stderr to the log file.
	logDir := filepath.Dir(logPath(name))
	if err := os.MkdirAll(logDir, 0755); err != nil {
		_ = writeState(name, "failed")
		return fmt.Errorf("creating log directory: %w", err)
	}

	logFile, err := os.OpenFile(logPath(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		_ = writeState(name, "failed")
		return fmt.Errorf("opening log file for %s: %w", name, err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process in a new process group so we can kill children later.
	// setNewProcessGroup is OS-specific (see runtime_unix.go / runtime_windows.go).
	setNewProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = writeState(name, "failed")
		return fmt.Errorf("starting plugin %s: %w", name, err)
	}

	pid := cmd.Process.Pid
	_ = logFile.Close()

	// Write PID file.
	if err := writePID(name, pid); err != nil {
		_ = writeState(name, "failed")
		return fmt.Errorf("writing pid for %s: %w", name, err)
	}

	// Poll for the plugin to become healthy (or confirm the process is alive
	// when no health endpoint is available). Timeout: 10 seconds at 500ms
	// intervals.
	const (
		healthTimeout  = 10 * time.Second
		healthInterval = 500 * time.Millisecond
	)

	deadline := time.Now().Add(healthTimeout)
	var lastErr error
	healthy := false

	for time.Now().Before(deadline) {
		// Check context cancellation first.
		select {
		case <-ctx.Done():
			_ = writeState(name, "failed")
			_ = os.Remove(pidPath(name))
			return fmt.Errorf("plugin %s start cancelled: %w", name, ctx.Err())
		default:
		}

		// If the process has already exited, fail fast.
		if !isProcessRunning(pid) {
			_ = writeState(name, "failed")
			_ = os.Remove(pidPath(name))
			return fmt.Errorf("plugin %s exited immediately after start", name)
		}

		// If the manifest declared a port, verify via HTTP health check.
		if manifest != nil && manifest.Port > 0 {
			ok, err := health(ctx, name, manifest.Port)
			if ok {
				healthy = true
				break
			}
			if err != nil {
				lastErr = err
			} else {
				lastErr = fmt.Errorf("plugin %s health endpoint returned non-200", name)
			}
		} else {
			// No port declared — process alive is sufficient.
			healthy = true
			break
		}

		time.Sleep(healthInterval)
	}

	// If we exhausted the deadline without a healthy response, report failure.
	if !healthy {
		if lastErr == nil {
			lastErr = fmt.Errorf("timed out waiting for plugin to become healthy")
		}
		_ = writeState(name, "failed")
		_ = os.Remove(pidPath(name))
		return fmt.Errorf("plugin %s failed health check after start: %w", name, lastErr)
	}

	if err := writeState(name, "running"); err != nil {
		return fmt.Errorf("setting running state for %s: %w", name, err)
	}

	// Check plugin interface contract compliance (non-blocking).
	// Logs warnings for non-compliant plugins but does not prevent startup.
	if manifest != nil && manifest.Port > 0 {
		CheckCompliance(ctx, name, manifest.Port)
	}

	return nil
}

// gracePeriod returns the SIGTERM-to-SIGKILL grace window for plugin shutdown.
// It reads DOCKER_STOP_GRACE_PERIOD env var (e.g. "30s", "10s") to match the
// compose layer's stop_grace_period setting. Defaults to 30s if unset or invalid.
// Bounded to [1s, 120s] to prevent accidental runaway or instant-kill values.
// DEP-03: fixes hardcoded 5s that conflicted with compose 30s stop_grace_period.
func gracePeriod() time.Duration {
	const defaultGrace = 30 * time.Second
	const maxGrace = 120 * time.Second
	const minGrace = 1 * time.Second

	raw := strings.TrimSpace(os.Getenv("DOCKER_STOP_GRACE_PERIOD"))
	if raw == "" {
		return defaultGrace
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		// Invalid value: fall back to default with a silent warning.
		// The caller will apply the default; misconfiguration is non-fatal.
		return defaultGrace
	}
	if d < minGrace {
		return minGrace
	}
	if d > maxGrace {
		return maxGrace
	}
	return d
}

// Stop gracefully shuts down a running plugin. It sends SIGTERM first, waits
// up to gracePeriod() for the process to exit, then sends SIGKILL if it is
// still alive. The grace window respects DOCKER_STOP_GRACE_PERIOD env var
// to match the compose layer's stop_grace_period (default 30s). PID and
// state files are cleaned up.
