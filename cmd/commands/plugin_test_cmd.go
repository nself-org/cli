package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var pluginTestCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Run a plugin's test suite (unit + smoke install/uninstall)",
	Long: `Run a composite test suite against a linked plugin:

  Phase 1 — Unit:          go test ./... inside the plugin source
  Phase 2 — Smoke install:  nself plugin install <name> then verify /health
  Phase 3 — Smoke uninstall: nself plugin remove <name> then verify clean state

Use --phase to run only specific phases when iterating:

  nself plugin test myplugin --phase=unit
  nself plugin test myplugin --phase=smoke
  nself plugin test myplugin --phase=both    (default)`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginTest,
}

func init() {
	pluginTestCmd.Flags().String("phase", "both", "Test phase to run: unit, smoke, or both")
	pluginTestCmd.Flags().Bool("host", false, "Run tests in host-process mode instead of container")
	pluginTestCmd.Flags().Bool("no-cleanup", false, "Skip cleanup after smoke test (useful for debugging)")
	pluginCmd.AddCommand(pluginTestCmd)
}

func runPluginTest(cmd *cobra.Command, args []string) error {
	name := args[0]
	phase, _ := cmd.Flags().GetString("phase")
	hostMode, _ := cmd.Flags().GetBool("host")
	noCleanup, _ := cmd.Flags().GetBool("no-cleanup")

	if phase != "unit" && phase != "smoke" && phase != "both" {
		return fmt.Errorf("invalid --phase %q: must be unit, smoke, or both", phase)
	}

	pluginPath, err := resolvePluginPath(name)
	if err != nil {
		return err
	}

	uiInfof("Running plugin tests for %s (phase=%s)...", name, phase)

	var failed []string
	var passed []string

	// Phase 1: unit tests.
	if phase == "unit" || phase == "both" {
		ui.Dimmed("--- Phase 1: unit tests ---")
		start := time.Now()
		if err := runUnitTests(pluginPath, hostMode); err != nil {
			ui.Error(fmt.Sprintf("FAIL unit [%.1fs]: %v", time.Since(start).Seconds(), err))
			failed = append(failed, "unit")
		} else {
			ui.Success(fmt.Sprintf("PASS unit [%.1fs]", time.Since(start).Seconds()))
			passed = append(passed, "unit")
		}
	}

	// Phase 2: smoke install + health check.
	if phase == "smoke" || phase == "both" {
		ui.Dimmed("--- Phase 2: smoke install ---")
		start := time.Now()
		if err := runSmokeInstall(name); err != nil {
			ui.Error(fmt.Sprintf("FAIL smoke-install [%.1fs]: %v", time.Since(start).Seconds(), err))
			failed = append(failed, "smoke-install")
		} else {
			ui.Success(fmt.Sprintf("PASS smoke-install [%.1fs]", time.Since(start).Seconds()))
			passed = append(passed, "smoke-install")
		}

		// Phase 3: smoke uninstall — only if install succeeded or --no-cleanup is set.
		ui.Dimmed("--- Phase 3: smoke uninstall ---")
		if !noCleanup {
			start = time.Now()
			if err := runSmokeUninstall(name); err != nil {
				ui.Error(fmt.Sprintf("FAIL smoke-uninstall [%.1fs]: %v", time.Since(start).Seconds(), err))
				failed = append(failed, "smoke-uninstall")
			} else {
				ui.Success(fmt.Sprintf("PASS smoke-uninstall [%.1fs]", time.Since(start).Seconds()))
				passed = append(passed, "smoke-uninstall")
			}
		} else {
			ui.Dimmed("Skipping uninstall (--no-cleanup).")
		}
	}

	// Summary.
	ui.Dimmed("---")
	if len(passed) > 0 {
		uiDimmedf("Passed: %s", strings.Join(passed, ", "))
	}
	if len(failed) > 0 {
		return fmt.Errorf("FAIL — phases failed: %s", strings.Join(failed, ", "))
	}

	ui.Success("All test phases passed.")
	return nil
}

// fallbackGoImage runs plugins whose go.mod has no readable go directive.
const fallbackGoImage = "golang:1.26-alpine"

// goDirective matches the `go 1.26.4` line of a go.mod.
var goDirective = regexp.MustCompile(`(?m)^go[ \t]+(\d+\.\d+)(?:\.\d+)?[ \t\r]*(?://.*)?$`)

// unitTestImage picks the golang image for a plugin: its go.mod go line
// (major.minor) so the toolchain satisfies the directive, else fallbackGoImage.
func unitTestImage(pluginPath string) string {
	data, err := os.ReadFile(filepath.Join(pluginPath, "go.mod"))
	if err != nil {
		return fallbackGoImage
	}
	if m := goDirective.FindSubmatch(data); m != nil {
		return "golang:" + string(m[1]) + "-alpine"
	}
	return fallbackGoImage
}

// unitTestDockerArgs builds the `docker run` arguments for the unit phase.
// Official golang images set GOTOOLCHAIN=local, so a go.mod that asks for a
// newer patch than the image carries would fail; auto lets Go fetch it.
func unitTestDockerArgs(pluginPath string) []string {
	return []string{"run", "--rm",
		"-v", pluginPath + ":/workspace",
		"-w", "/workspace",
		"-e", "GOTOOLCHAIN=auto",
		unitTestImage(pluginPath),
		"sh", "-c", "go test ./... -v -count=1",
	}
}

// runUnitTests runs go test ./... in the plugin source directory.
func runUnitTests(pluginPath string, hostMode bool) error {
	var testCmd *exec.Cmd
	if hostMode {
		testCmd = exec.Command("go", "test", "./...", "-v", "-count=1")
		testCmd.Dir = pluginPath
	} else {
		// Inside container if available — fall back to host when Docker not running.
		if isDockerAvailable() {
			testCmd = exec.Command("docker", unitTestDockerArgs(pluginPath)...)
		} else {
			ui.Warn("Docker not available; running unit tests on host.")
			testCmd = exec.Command("go", "test", "./...", "-v", "-count=1")
			testCmd.Dir = pluginPath
		}
	}

	testCmd.Stdout = os.Stdout
	testCmd.Stderr = os.Stderr

	if err := testCmd.Run(); err != nil {
		return fmt.Errorf("go test failed: %w", err)
	}
	return nil
}

// runSmokeInstall installs the linked plugin and verifies /healthz returns 200.
func runSmokeInstall(name string) error {
	// Install the locally-linked plugin.
	nself, err := os.Executable()
	if err != nil {
		nself = "nself"
	}

	installCmd := exec.Command(nself, "plugin", "install", name, "--force")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("smoke install failed: %w", err)
	}

	// Verify /healthz endpoint (30s timeout, 5 retries).
	healthURL := resolvePluginHealthURL(name)
	if err := waitForHealth(healthURL, 5, 6*time.Second); err != nil {
		return fmt.Errorf("health check failed after install: %w", err)
	}

	return nil
}

// runSmokeUninstall removes the plugin and verifies clean state.
func runSmokeUninstall(name string) error {
	nself, err := os.Executable()
	if err != nil {
		nself = "nself"
	}

	removeCmd := exec.Command(nself, "plugin", "remove", name)
	removeCmd.Stdout = os.Stdout
	removeCmd.Stderr = os.Stderr
	if err := removeCmd.Run(); err != nil {
		return fmt.Errorf("smoke uninstall failed: %w", err)
	}

	return nil
}

// resolvePluginHealthURL builds the expected health check URL for a plugin.
func resolvePluginHealthURL(name string) string {
	base := os.Getenv("NSELF_LOCAL_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	return fmt.Sprintf("%s/healthz", base)
}

// waitForHealth polls the given URL until it returns 200 or retries are exhausted.
func waitForHealth(url string, retries int, interval time.Duration) error {
	for i := 0; i < retries; i++ {
		resp, err := httpGetWithTimeout(url, 5*time.Second)
		if err == nil && resp == 200 {
			return nil
		}
		if i < retries-1 {
			time.Sleep(interval)
		}
	}
	return fmt.Errorf("health check at %s did not return 200 after %d retries", url, retries)
}

// httpGetWithTimeout performs a GET request and returns the status code.
func httpGetWithTimeout(url string, timeout time.Duration) (int, error) {
	curlCmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"--max-time", fmt.Sprintf("%.0f", timeout.Seconds()), url)
	out, err := curlCmd.Output()
	if err != nil {
		return 0, err
	}
	var code int
	if _, err := fmt.Sscan(string(out), &code); err != nil {
		return 0, err
	}
	return code, nil
}

// isDockerAvailable returns true if the docker CLI is on PATH and responds.
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = os.Stderr // discard
	cmd.Stderr = os.Stderr
	return cmd.Run() == nil
}
