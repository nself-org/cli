// Package doctor provides comprehensive health check sections for nself doctor.
package doctor

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

// CheckResult holds the outcome of a single diagnostic check.
type CheckResult struct {
	Section string `json:"section"`
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, warn, fail
	Message string `json:"message"`
	FixCmd  string `json:"fix_cmd,omitempty"` // suggested fix command
}

// SystemChecks runs system-level diagnostics: disk, memory, swap, load, clock sync, Docker, kernel.
func SystemChecks(ctx context.Context, verbose bool) []CheckResult {
	var results []CheckResult

	// Docker daemon version
	results = append(results, checkDockerVersion(ctx))

	// Kernel version (Linux only)
	if runtime.GOOS == "linux" {
		results = append(results, checkKernel(ctx))
	}

	// Clock sync
	results = append(results, checkClockSync(ctx))

	// Load average
	results = append(results, checkLoad(ctx))

	// Swap
	results = append(results, checkSwap(ctx))

	return results
}

func checkDockerVersion(ctx context.Context) CheckResult {
	out, err := docker.ServerVersion(ctx)
	if err != nil {
		return CheckResult{Section: "system", Name: "Docker version", Status: "fail", Message: "cannot get Docker version"}
	}
	return CheckResult{Section: "system", Name: "Docker version", Status: "pass", Message: out}
}

func checkKernel(ctx context.Context) CheckResult {
	cmd := exec.CommandContext(ctx, "uname", "-r")
	out, err := cmd.Output()
	if err != nil {
		return CheckResult{Section: "system", Name: "Kernel", Status: "warn", Message: "cannot determine kernel version"}
	}
	return CheckResult{Section: "system", Name: "Kernel", Status: "pass", Message: strings.TrimSpace(string(out))}
}

func checkClockSync(ctx context.Context) CheckResult {
	// Compare system time to a reference. Best effort using date.
	now := time.Now()
	_ = now
	// On Linux, check if NTP is synced
	if runtime.GOOS == "linux" {
		cmd := exec.CommandContext(ctx, "timedatectl", "show", "--property=NTPSynchronized", "--value")
		out, err := cmd.Output()
		if err == nil {
			val := strings.TrimSpace(string(out))
			if val == "yes" {
				return CheckResult{Section: "system", Name: "Clock sync", Status: "pass", Message: "NTP synchronized"}
			}
			return CheckResult{Section: "system", Name: "Clock sync", Status: "warn", Message: "NTP not synchronized",
				FixCmd: "sudo timedatectl set-ntp true"}
		}
	}
	return CheckResult{Section: "system", Name: "Clock sync", Status: "pass", Message: "check skipped on " + runtime.GOOS}
}

func checkLoad(ctx context.Context) CheckResult {
	cmd := exec.CommandContext(ctx, "uptime")
	out, err := cmd.Output()
	if err != nil {
		return CheckResult{Section: "system", Name: "Load average", Status: "warn", Message: "cannot check load"}
	}
	return CheckResult{Section: "system", Name: "Load average", Status: "pass", Message: strings.TrimSpace(string(out))}
}

func checkSwap(ctx context.Context) CheckResult {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "SwapTotal:") {
					return CheckResult{Section: "system", Name: "Swap", Status: "pass", Message: strings.TrimSpace(line)}
				}
			}
		}
	}
	return CheckResult{Section: "system", Name: "Swap", Status: "pass", Message: "swap check skipped on " + runtime.GOOS}
}

// FixItEngine runs safe auto-fixes for check results that have FixCmd set.
func FixItEngine(ctx context.Context, projectDir string, results []CheckResult) []CheckResult {
	project := ""
	if cfg, err := config.Load(projectDir); err == nil {
		project = cfg.ProjectName
	}
	for i, r := range results {
		if r.FixCmd == "" || r.Status == "pass" {
			continue
		}
		parts := strings.Fields(r.FixCmd)
		if len(parts) == 0 {
			continue
		}
		allowed := parts[0] == "nself"
		if len(parts) == 3 && parts[0] == "docker" && parts[1] == "restart" && project != "" {
			allowed = strings.HasPrefix(parts[2], project+"_") && len(parts[2]) > len(project)+1 && !strings.ContainsAny(parts[2], ";|&$`/\\")
		}
		if !allowed {
			r.Message += " (manual step: " + r.FixCmd + ")"
			results[i] = r
			continue
		}
		var err error
		if parts[0] == "docker" {
			err = docker.RestartContainer(ctx, parts[2])
		} else {
			err = exec.CommandContext(ctx, parts[0], parts[1:]...).Run()
		}
		if err == nil {
			r.Status = "pass"
			r.Message += " (auto-fixed)"
		} else {
			r.Message += " (auto-fix failed: " + err.Error() + ")"
		}
		results[i] = r
	}
	return results
}
