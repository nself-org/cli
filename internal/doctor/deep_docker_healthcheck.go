package doctor

// deep_docker_healthcheck.go — G-014 (second defect): a container whose
// healthcheck command cannot even execute inside its own image reports
// "unhealthy" forever, regardless of whether the service actually works.
// That status then carries zero information — worse than no healthcheck at
// all, because it looks like a real, actionable signal.
//
// WHY this lives in doctor, not build: `nself build` is a pure generator —
// it never touches the Docker daemon (see internal/build; nothing in that
// package imports internal/docker). Detecting a command that is not on the
// image's PATH requires a live container to exec into, which only exists
// once something is running. `nself doctor --deep` already inspects live
// container health in DockerDeepChecks (deep_docker.go) and already
// suggests `docker restart` for every "unhealthy" container — exactly the
// place the false signal was measured live: curl was never installed in
// nself-mux/nself-cron's image, so their healthcheck always errored, and
// "restart" was always the wrong suggestion (they were fine; a DB DNS
// failure elsewhere was the real, unrelated problem for two of them).
//
// Purpose: given a container already known to report "unhealthy", decide
// whether that status is trustworthy or the healthcheck command itself is
// unusable, and produce the right CheckResult either way.
// Inputs:  ctx, the container name (from DockerDeepChecks' `docker ps` scan).
// Outputs: a CheckResult — the existing generic "unhealthy"/"docker restart"
//          message when the healthcheck is fine or undeterminable, or a
//          distinct "invalid healthcheck" message/fix when the command is
//          confirmed missing from the image.
// Constraints: read-only (docker inspect + docker exec `command -v`, never
//              a mutating command); best-effort — any ambiguity falls back
//              to the pre-existing generic message rather than guessing.

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// healthcheckConfig mirrors the subset of `docker inspect --format
// '{{json .Config.Healthcheck}}'` this check needs.
type healthcheckConfig struct {
	Test []string `json:"Test"`
}

// inspectHealthcheckTest returns the container's healthcheck Test slice —
// e.g. ["CMD-SHELL", "curl -f http://localhost:8080/health"] — or (nil, nil)
// when the image defines no healthcheck at all ("Healthcheck": null).
func inspectHealthcheckTest(ctx context.Context, containerName string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .Config.Healthcheck}}", containerName)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect healthcheck for %s: %w", containerName, err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var hc healthcheckConfig
	if err := json.Unmarshal([]byte(trimmed), &hc); err != nil {
		return nil, fmt.Errorf("parsing healthcheck config for %s: %w", containerName, err)
	}
	return hc.Test, nil
}

// extractHealthcheckBinary parses a Docker healthcheck Test slice and
// returns its mode ("CMD-SHELL", "CMD", "NONE", or "" if unrecognized) and
// the first binary the test would need to invoke. Pure — no docker access —
// so it is exercised directly in tests against canned Test slices.
func extractHealthcheckBinary(test []string) (mode, binary string) {
	if len(test) == 0 {
		return "", ""
	}
	mode = test[0]
	switch mode {
	case "NONE":
		return mode, ""
	case "CMD-SHELL":
		if len(test) < 2 {
			return mode, ""
		}
		fields := strings.Fields(test[1])
		if len(fields) == 0 {
			return mode, ""
		}
		return mode, fields[0]
	case "CMD":
		if len(test) < 2 {
			return mode, ""
		}
		return mode, test[1]
	default:
		// Older/uncommon shape without a mode marker — best-effort: the
		// first element itself is the binary.
		return "", test[0]
	}
}

// classifyBinaryProbe interprets the result of a `command -v <binary>`
// probe run inside a container. ranToCompletion means the shell actually
// executed (exit 0 or a normal non-zero exit) rather than failing to start
// at all (no shell in the image, container not running, daemon
// unreachable) — only then is "not found" a conclusive signal. Pure —
// tested directly with canned stdout/ranToCompletion pairs.
func classifyBinaryProbe(stdout string, ranToCompletion bool) (found, verifiable bool) {
	if !ranToCompletion {
		return false, false
	}
	return strings.TrimSpace(stdout) != "", true
}

// probeBinaryInContainer runs `docker exec <container> sh -c "command -v
// <binary>"` and reports whether the binary was found, whether the probe
// was conclusive at all, and a hard error only when docker itself could not
// run the exec (container not running, daemon unreachable, etc.).
func probeBinaryInContainer(ctx context.Context, containerName, binary string) (found, verifiable bool, err error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "sh", "-c", "command -v "+binary)
	out, runErr := cmd.Output()
	if runErr == nil {
		found, verifiable = classifyBinaryProbe(string(out), true)
		return found, verifiable, nil
	}
	if _, ok := runErr.(*exec.ExitError); ok {
		// The shell ran and command -v exited non-zero: conclusively absent.
		found, verifiable = classifyBinaryProbe(string(out), true)
		return found, verifiable, nil
	}
	return false, false, fmt.Errorf("docker exec %s: %w", containerName, runErr)
}

// diagnoseUnhealthyContainer decides whether an already-"unhealthy"
// container's status is trustworthy. genericResult is what the caller would
// otherwise report (the pre-existing "unhealthy" / "docker restart"
// message) — returned unchanged whenever the healthcheck can't be
// conclusively shown broken, so this never downgrades a real failure.
func diagnoseUnhealthyContainer(ctx context.Context, cName string, genericResult CheckResult) CheckResult {
	test, err := inspectHealthcheckTest(ctx, cName)
	if err != nil || len(test) == 0 {
		return genericResult
	}
	mode, binary := extractHealthcheckBinary(test)
	if mode == "NONE" || binary == "" {
		return genericResult
	}

	found, verifiable, probeErr := probeBinaryInContainer(ctx, cName, binary)
	if probeErr != nil || !verifiable || found {
		return genericResult
	}

	return CheckResult{
		Section: "docker",
		Name:    fmt.Sprintf("Container: %s (invalid healthcheck)", cName),
		Status:  "fail",
		Message: fmt.Sprintf(
			"healthcheck command %q is not installed in this image — status is permanently \"unhealthy\" regardless of whether the service works, so it carries no information",
			binary),
		FixCmd: fmt.Sprintf(
			"Install %q in the image, or change the healthcheck test to a command the image has — then: nself build --force && nself start",
			binary),
	}
}
