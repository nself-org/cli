package commands

// Plugin doctors in `nself doctor --deep` (P7-CI-53, Epic D27).
//
// Purpose: the composition-root provider for internal/doctor's RegisterExternal
// hook. Every mounted plugin that declares a `doctor` subcommand is run as
// `<binary> doctor --json`; each check of its v1 envelope becomes one deep
// doctor row under the section "plugin:<slug>".
//
// Inputs: the plugins directory (resolvePluginDir) through mount.Discover.
//
// Outputs: []doctor.CheckResult. A plugin that times out, cannot start, exits
// with a code other than 0/10/12, or prints something that is not a doctor
// envelope yields exactly one warn row naming it. A plugin with no `doctor`
// subcommand contributes nothing.
//
// Constraints: deep mode only (the hook runs from PluginHealthChecks); every
// call has a deadline; the exec goes through plugin.CaptureOutput; plugin text
// is untrusted and is escaped with safeMountText before it reaches a row.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/doctor"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/mount"
)

// externalDoctorTimeout bounds one plugin's `doctor --json`. A var so a test
// can shrink it.
var externalDoctorTimeout = 10 * time.Second

func init() {
	doctor.RegisterExternal("plugin-doctors", externalPluginDoctors)
}

// pluginDoctorEnvelope is the part of a v1 envelope the fold reads.
type pluginDoctorEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Data          *struct {
		Checks []pluginDoctorCheck `json:"checks"`
	} `json:"data"`
}

type pluginDoctorCheck struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
	Subject     string `json:"subject"`
}

// externalPluginDoctors runs the doctors of all mounted plugins concurrently
// (each under its own deadline) and returns their rows in slug order.
func externalPluginDoctors(ctx context.Context, _ string) []doctor.CheckResult {
	specs, _ := mount.Discover(resolvePluginDir(), canonTable.Verbs)
	var withDoctor []mount.Spec
	for _, s := range specs {
		if hasDoctorSub(s) {
			withDoctor = append(withDoctor, s)
		}
	}
	rows := make([][]doctor.CheckResult, len(withDoctor))
	var wg sync.WaitGroup
	for i, s := range withDoctor {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows[i] = runPluginDoctor(ctx, s)
		}()
	}
	wg.Wait()
	var out []doctor.CheckResult
	for _, r := range rows {
		out = append(out, r...)
	}
	return out
}

// hasDoctorSub reports whether the plugin declares a top-level `doctor`
// subcommand.
func hasDoctorSub(s mount.Spec) bool {
	return slices.ContainsFunc(s.Subcommands, func(sub mount.Sub) bool {
		return len(sub.Path) == 1 && sub.Path[0] == "doctor"
	})
}

// runPluginDoctor runs one plugin's doctor and maps the result.
func runPluginDoctor(ctx context.Context, s mount.Spec) []doctor.CheckResult {
	section := "plugin:" + safeMountText(s.Slug)
	warn := func(msg string) []doctor.CheckResult {
		return []doctor.CheckResult{{Section: section, Name: "DOCTOR", Status: "warn",
			Message: fmt.Sprintf("plugin %s: %s", safeMountText(s.Slug), msg)}}
	}
	cctx, cancel := context.WithTimeout(ctx, externalDoctorTimeout)
	defer cancel()
	stdout, exit, err := plugin.CaptureOutput(cctx, s.BinaryPath, []string{"doctor", "--json"})
	switch {
	case cctx.Err() != nil:
		return warn(fmt.Sprintf("doctor did not finish within %s", externalDoctorTimeout))
	case err != nil:
		return warn("doctor could not run: " + safeMountText(err.Error()))
	}
	var env pluginDoctorEnvelope
	if jerr := json.Unmarshal(stdout, &env); jerr != nil || env.SchemaVersion == "" || env.Data == nil {
		return warn(fmt.Sprintf("doctor printed no v1 envelope (exit %d)", exit))
	}
	if exit != 0 && exit != 10 && exit != 12 {
		return warn(fmt.Sprintf("doctor exited %d", exit))
	}
	out := make([]doctor.CheckResult, 0, len(env.Data.Checks))
	for _, c := range env.Data.Checks {
		out = append(out, mapPluginCheck(section, c))
	}
	return out
}

// mapPluginCheck converts one plugin check to a deep doctor row. An unknown
// status becomes warn.
func mapPluginCheck(section string, c pluginDoctorCheck) doctor.CheckResult {
	name := c.ID
	if name == "" {
		name = c.Name
	}
	status := c.Status
	if status != "pass" && status != "warn" && status != "fail" {
		status = "warn"
	}
	msg := c.Message
	if c.Subject != "" {
		msg += " (" + c.Subject + ")"
	}
	return doctor.CheckResult{
		Section: section,
		Name:    safeMountText(name),
		Status:  status,
		Message: safeMountText(msg),
		FixCmd:  safeMountText(c.Remediation),
	}
}
