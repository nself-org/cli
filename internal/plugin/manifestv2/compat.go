package manifestv2

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// minRange pulls the lower bound out of a semver range such as ">=1.4.0" or
// ">=1.4.0 <2.0.0"; a bare version is its own lower bound.
var minRange = regexp.MustCompile(`^\s*(?:>=\s*)?([^\s,<>=!~^|]+)`)

// Projection derives the compatibility keys of m (Epic D1). Derived keys are
// recomputed from the v2 fields: pluginType and binaryName from commands,
// minNselfVersion from requires.nself, status from maturity and
// deprecation.state, isCommercial, licenseType, requires_license and tier from
// license, and the description of the canonical command's cliCommands entry
// from commands.summary. EntryPoint, Runtime, CLI and the other cliCommands
// entries are carried from m unchanged, and so are a licensed plugin's tier and
// licenseType (pro when it has none).
func Projection(m *Manifest) Compat {
	p := Compat{
		EntryPoint:  m.EntryPoint,
		Runtime:     m.Runtime,
		CLI:         m.CLI,
		CLICommands: append([]CLICommand(nil), m.CLICommands...),
	}
	if m.Commands != nil {
		p.PluginType = "cli"
		p.BinaryName = m.Commands.Binary
		for i := range p.CLICommands {
			if p.CLICommands[i].Name == m.Commands.Command {
				p.CLICommands[i].Description = m.Commands.Summary
			}
		}
	}
	if m.Requires != nil {
		if g := minRange.FindStringSubmatch(m.Requires.Nself); g != nil {
			p.MinNselfVersion = g[1]
		}
	}
	p.Status = statusOf(m.Maturity, m.Deprecation)
	if m.License == LicenseLicensed {
		p.IsCommercial, p.RequiresLicense = true, true
		p.LicenseType, p.Tier = licensedValue(m.LicenseType), licensedValue(m.Tier)
	} else {
		p.LicenseType, p.Tier = "free", "free"
	}
	return p
}

// licensedValue keeps a licensed plugin's own tier or licenseType (max, cloud,
// internal and so on) so released CLIs read the values they read today; a
// licensed manifest that states none, or states free, gets pro.
func licensedValue(carried string) string {
	if carried == "" || carried == "free" {
		return "pro"
	}
	return carried
}

// statusOf is the maturity to v1 status table (Epic D1, v2 -> compat status).
func statusOf(maturity string, d *Deprecation) string {
	switch maturity {
	case MaturityImplemented:
		return "stable"
	case MaturityExperimental, MaturityScaffolded:
		return "experimental"
	case MaturityPlanned:
		return "planned"
	case MaturityDeferred:
		if d != nil && d.State == StateEOL {
			return "eol"
		}
		return "deprecated"
	}
	return ""
}

// CheckCompat proves the compatibility keys of m equal Projection(m) (E112).
// Only the derived keys are compared: the carried ones have no v2 source.
func CheckCompat(m *Manifest) error {
	p := Projection(m)
	var drift []string
	cmp := func(key string, got, want any) {
		if !reflect.DeepEqual(got, want) {
			drift = append(drift, key)
		}
	}
	cmp("pluginType", m.PluginType, p.PluginType)
	cmp("binaryName", m.BinaryName, p.BinaryName)
	cmp("minNselfVersion", m.MinNselfVersion, p.MinNselfVersion)
	cmp("status", m.Status, p.Status)
	cmp("isCommercial", m.IsCommercial, p.IsCommercial)
	cmp("licenseType", m.LicenseType, p.LicenseType)
	cmp("requires_license", m.RequiresLicense, p.RequiresLicense)
	cmp("tier", m.Tier, p.Tier)
	if m.Commands != nil {
		for i, c := range m.CLICommands {
			if c.Name == m.Commands.Command && c.Description != m.Commands.Summary {
				drift = append(drift, "cliCommands["+strconv.Itoa(i)+"].description")
			}
		}
	}
	if len(drift) == 0 {
		return nil
	}
	return errs.Newf("E112", "compatibility key%s out of date: %s (v2 fields changed without regenerating them)", plural(len(drift)), strings.Join(drift, ", "))
}

// ApplyProjection writes Projection(m) into m's compatibility keys.
func ApplyProjection(m *Manifest) { m.Compat = Projection(m) }
