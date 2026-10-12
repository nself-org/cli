package commands

// Purpose: doctor checks for the local environment: Docker install/running/
// compose version, git, disk space, memory, and network reachability. Inputs
// are a context and a verbose flag; outputs are doctorCheckResult values.
// Constraints: split out of doctor.go (CLI-R12). The compose version check
// also warns below the minimum compose version (D-0548). The check reads the
// version through the dockerComposeVersion seam so tests run no subprocess.

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/maintenance"
)

// dockerComposeVersion reads the installed docker compose version. A package
// variable so tests can stub it.
var dockerComposeVersion = docker.ComposeVersion

// checkDockerInstalled verifies the docker binary is on PATH.
func checkDockerInstalled(verbose bool) doctorCheckResult {
	name := "Docker installed"
	path, err := exec.LookPath("docker")
	if err != nil {
		printCheck("fail", name, "docker not found in PATH", verbose)
		return doctorCheckResult{Name: name, Status: "fail", Message: "docker not found in PATH"}
	}
	detail := ""
	if verbose {
		detail = path
	}
	printCheck("pass", name, "docker found", verbose)
	return doctorCheckResult{Name: name, Status: "pass", Message: "docker found", Detail: detail}
}

// checkDockerRunning verifies the Docker daemon is responsive.
func checkDockerRunning(ctx context.Context, verbose bool) doctorCheckResult {
	name := "Docker daemon running"
	cmd := exec.CommandContext(ctx, "docker", "info")
	out, err := cmd.CombinedOutput()
	if err != nil {
		printCheck("fail", name, "Docker daemon is not running", verbose)
		return doctorCheckResult{Name: name, Status: "fail", Message: "Docker daemon is not running"}
	}
	detail := ""
	if verbose {
		// Extract server version from docker info output
		for _, line := range strings.Split(string(out), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Server Version:") {
				detail = strings.TrimSpace(strings.TrimPrefix(trimmed, "Server Version:"))
				break
			}
		}
	}
	printCheck("pass", name, "Docker daemon is responsive", verbose)
	return doctorCheckResult{Name: name, Status: "pass", Message: "Docker daemon is responsive", Detail: detail}
}

// checkDockerComposeVersion verifies docker compose v2 is available, reports
// its version, and warns when it is older than docker.MinComposeVersion.
func checkDockerComposeVersion(ctx context.Context, verbose bool) doctorCheckResult {
	name := "Docker Compose v2"
	version, err := dockerComposeVersion(ctx)
	if err != nil {
		printCheck("fail", name, "docker compose not available", verbose)
		return doctorCheckResult{Name: name, Status: "fail", Message: "docker compose not available"}
	}
	switch docker.CheckComposeVersion(version) {
	case docker.ComposeTooOld:
		msg := docker.ComposeTooOldMessage(version)
		printCheck("warn", name, msg, verbose)
		return doctorCheckResult{Name: name, Status: "warn", Message: msg, Detail: version}
	case docker.ComposeUnknown:
		msg := fmt.Sprintf("docker compose %s (version not recognised; the minimum is %s)", version, docker.MinComposeVersion)
		printCheck("pass", name, msg, verbose)
		return doctorCheckResult{Name: name, Status: "pass", Message: msg, Detail: version}
	}
	msg := fmt.Sprintf("docker compose %s", version)
	printCheck("pass", name, msg, verbose)
	return doctorCheckResult{Name: name, Status: "pass", Message: msg, Detail: version}
}

// checkGitInstalled verifies git is on PATH.
func checkGitInstalled(verbose bool) doctorCheckResult {
	name := "Git installed"
	path, err := exec.LookPath("git")
	if err != nil {
		printCheck("warn", name, "git not found in PATH", verbose)
		return doctorCheckResult{Name: name, Status: "warn", Message: "git not found in PATH"}
	}
	detail := ""
	if verbose {
		detail = path
	}
	printCheck("pass", name, "git found", verbose)
	return doctorCheckResult{Name: name, Status: "pass", Message: "git found", Detail: detail}
}

// checkDiskSpace verifies at least 5 GB of free disk space.
// When --deep is active and disk usage exceeds 70%, it also appends a
// suggestion to enable the daily maintenance timer.
func checkDiskSpace(verbose bool) doctorCheckResult {
	name := "Disk space"
	freeGB, err := getFreeDiskGB()
	if err != nil {
		msg := fmt.Sprintf("unable to check disk space: %v", err)
		printCheck("warn", name, msg, verbose)
		return doctorCheckResult{Name: name, Status: "warn", Message: msg}
	}
	msg := fmt.Sprintf("%.1f GB free", freeGB)
	if freeGB < 5.0 {
		// Also check used-percent so we can surface the maintenance suggestion.
		if usage, uerr := maintenance.GetDiskUsage(); uerr == nil && usage.UsedPercent > 70 {
			detail := fmt.Sprintf("disk is %d%% full — run `nself maintenance schedule --daily` to enable automatic daily cleanup", usage.UsedPercent)
			printCheck("warn", name, msg+" (recommended: 5 GB+) — "+detail, verbose)
			return doctorCheckResult{Name: name, Status: "warn", Message: msg + " (recommended: 5 GB+)", Detail: detail}
		}
		printCheck("warn", name, msg+" (recommended: 5 GB+)", verbose)
		return doctorCheckResult{Name: name, Status: "warn", Message: msg + " (recommended: 5 GB+)"}
	}
	// Even when free space is adequate, warn if disk is >70% full.
	if usage, uerr := maintenance.GetDiskUsage(); uerr == nil && usage.UsedPercent > 70 {
		detail := fmt.Sprintf("disk is %d%% full — run `nself maintenance schedule --daily` to enable automatic daily cleanup", usage.UsedPercent)
		printCheck("warn", name, msg+" — "+detail, verbose)
		return doctorCheckResult{Name: name, Status: "warn", Message: msg, Detail: detail}
	}
	printCheck("pass", name, msg, verbose)
	return doctorCheckResult{Name: name, Status: "pass", Message: msg}
}

// checkMemory verifies at least 2 GB of total system memory.
func checkMemory(verbose bool) doctorCheckResult {
	name := "System memory"
	totalMB, err := getTotalMemoryMB()
	if err != nil {
		msg := fmt.Sprintf("unable to check memory: %v", err)
		printCheck("warn", name, msg, verbose)
		return doctorCheckResult{Name: name, Status: "warn", Message: msg}
	}
	totalGB := float64(totalMB) / 1024.0
	msg := fmt.Sprintf("%.1f GB total", totalGB)
	if totalMB < 2048 {
		printCheck("warn", name, msg+" (recommended: 2 GB+)", verbose)
		return doctorCheckResult{Name: name, Status: "warn", Message: msg + " (recommended: 2 GB+)"}
	}
	printCheck("pass", name, msg, verbose)
	return doctorCheckResult{Name: name, Status: "pass", Message: msg}
}

// checkNetwork verifies internet connectivity by pinging Docker Hub.
func checkNetwork(ctx context.Context, verbose bool) doctorCheckResult {
	name := "Network / Docker Hub"
	cmd := exec.CommandContext(ctx, "docker", "pull", "--quiet", "hello-world")
	_, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback: just try to reach the registry
		cmd2 := exec.CommandContext(ctx, "docker", "manifest", "inspect", "hello-world")
		_, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			printCheck("warn", name, "Docker Hub unreachable", verbose)
			return doctorCheckResult{Name: name, Status: "warn", Message: "Docker Hub unreachable"}
		}
	}
	printCheck("pass", name, "Docker Hub reachable", verbose)
	return doctorCheckResult{Name: name, Status: "pass", Message: "Docker Hub reachable"}
}
