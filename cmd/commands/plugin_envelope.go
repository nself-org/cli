package commands

// Purpose: the v1 envelope for the `config plugins` family (`plugin ...` is
// the v1.4 spelling; P7-SURF-13): typed data for the read commands (list,
// inventory, status, updates, info, compat-check) and the wiring that makes the
// write commands (update, refresh, start, stop, enable, disable) answer with
// the shared dataResult. audit-tables, count, outdated, search, info and list
// emit from their own bodies (their pre-contract JSON stays in v1.4 mode).
// Inputs: the same plugin directory, registry and runtime state the human
// commands read; nothing new is fetched.
// Outputs: one envelope on the real stdout in v1.5 JSON mode only.
// Constraints: no new behaviour; `config plugins` itself proxies unknown
// subcommands to a plugin binary that owns stdout, so it is interactive and
// has no envelope; `config plugins logs` is a stream of raw container lines.
// SPORT: P7-SURF-13, contract:cli.json-envelope

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/version"
	"github.com/spf13/cobra"
)

// pluginListRow is one plugin of `config plugins list --json`. The listing
// fills name, version, category and the state members; `--available` fills
// the tier-pair members instead. Members are alphabetical so the bare form
// (NSELF_JSON_LEGACY=1) of `--available` keeps its pre-contract key order.
type pluginListRow struct {
	// Category is the plugin category.
	Category string `json:"category"`
	// Installed and Running are the local state (listing only).
	Installed *bool `json:"installed,omitempty"`
	// IsDefault marks the tier an install resolves to (--available only).
	IsDefault *bool `json:"is_default,omitempty"`
	// License is the licence word, free or licensed (--available only).
	License string `json:"license,omitempty"`
	// Name is the plugin slug.
	Name string `json:"name"`
	// PublishStatus is the lifecycle status from the registry (listing only).
	PublishStatus string `json:"publish_status,omitempty"`
	// Running is true while the plugin process runs (listing only).
	Running *bool `json:"running,omitempty"`
	// TierPair is true when the slug is served as a free and a licensed pair (--available only).
	TierPair *bool `json:"tier_pair,omitempty"`
	// UpdatedAt is the entry's own freshness stamp, when it carries one.
	UpdatedAt string `json:"updated_at,omitempty"`
	// Version is the plugin version.
	Version string `json:"version"`
}

// pluginListData is the data of `config plugins list --json`.
type pluginListData []pluginListRow

// pluginInventoryRow is one installed plugin of `config plugins inventory`.
type pluginInventoryRow struct {
	// Name is the plugin slug.
	Name string `json:"name"`
	// Version is the installed version.
	Version string `json:"version"`
	// License is the licence label, as the table prints it.
	License string `json:"license"`
	// Status is the plugin status.
	Status string `json:"status"`
	// Description is the manifest description.
	Description string `json:"description"`
}

// pluginInventoryData is the data of `config plugins inventory`.
type pluginInventoryData struct {
	// Plugins are the installed plugins; empty when none is installed.
	Plugins []pluginInventoryRow `json:"plugins"`
}

// pluginStatusRow is the runtime state of one plugin.
type pluginStatusRow struct {
	// Name is the plugin slug.
	Name string `json:"name"`
	// State is starting, running, stopping, stopped, failed, or unknown.
	State string `json:"state"`
	// PID is the process id, 0 when none.
	PID int `json:"pid"`
}

// pluginStatusData is the data of `config plugins status`.
type pluginStatusData struct {
	// Plugins is the named plugin, or every installed plugin.
	Plugins []pluginStatusRow `json:"plugins"`
}

// pluginUpdateRow is one installed plugin whose registry version differs.
type pluginUpdateRow struct {
	// Name is the plugin slug.
	Name string `json:"name"`
	// Installed is the installed version.
	Installed string `json:"installed"`
	// Latest is the registry version.
	Latest string `json:"latest"`
}

// pluginUpdatesData is the data of `config plugins updates`.
type pluginUpdatesData struct {
	// Updates are the plugins with a newer registry version; empty when all are current.
	Updates []pluginUpdateRow `json:"updates"`
}

// compatRow is one plugin of `config plugins compat-check --json`.
type compatRow struct {
	// Name is the plugin slug.
	Name string `json:"name"`
	// Version is the installed version.
	Version string `json:"version"`
	// Status is the plugin status.
	Status string `json:"status"`
	// Compatible is false when the plugin caps the CLI version below the running one.
	Compatible bool `json:"compatible"`
	// Reason says why it is incompatible, "" when compatible.
	Reason string `json:"reason"`
	// Hint is the upgrade command for an incompatible plugin.
	Hint string `json:"hint"`
}

// compatCheckData is the data of `config plugins compat-check --json`.
type compatCheckData struct {
	// CLIVersion is the running CLI version.
	CLIVersion string `json:"cliVersion"`
	// Plugins are the installed plugins checked; empty when none is installed.
	Plugins []compatRow `json:"plugins"`
}

// pluginInfoData is the data of `config plugins info --json`: the registry
// fields plus the reported tier and why.
type pluginInfoData struct {
	// Name is the plugin slug.
	Name string `json:"name"`
	// Version, Description, Category, License and Port come from the registry entry.
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	License     string `json:"license,omitempty"`
	// Tier is free or licensed, TierReason says why and TierError the cause when unresolved.
	Tier       string `json:"tier"`
	TierReason string `json:"tier_reason"`
	TierError  string `json:"tier_error,omitempty"`
	// Port is the plugin service port, 0 when none.
	Port int `json:"port,omitempty"`
}

func boolPtr(b bool) *bool { return &b }

// newPluginListData converts the listing the human path prints, with its
// filters (EOL hidden unless showEOL, category match).
func newPluginListData(plugins []plugin.PluginInfo, showEOL bool, category string) pluginListData {
	out := pluginListData{}
	for _, p := range plugins {
		if p.PublishStatus == "eol" && !showEOL {
			continue
		}
		if category != "" && !strings.EqualFold(p.Category, category) {
			continue
		}
		out = append(out, pluginListRow{Category: p.Category, Installed: boolPtr(p.Installed), Running: boolPtr(p.Running),
			Name: p.Name, PublishStatus: p.PublishStatus, UpdatedAt: p.UpdatedAt, Version: p.Version})
	}
	return out
}

// newAvailableListData converts the `--available` rows. The licence word is
// the v1.5 form (tier pro -> licensed, P7-PLUG-11).
func newAvailableListData(rows []plugin.TieredPluginInfo) pluginListData {
	out := pluginListData{}
	for _, r := range rows {
		out = append(out, pluginListRow{Category: r.Category, IsDefault: boolPtr(r.IsDefault), License: plugin.LicenseValue(r.Tier),
			Name: r.Name, TierPair: boolPtr(r.TierPair), Version: r.Version})
	}
	return out
}

func pluginInventoryFacts(_ *cobra.Command, _ []string) (any, error) {
	plugins, err := plugin.ListInstalled(resolvePluginDir())
	if err != nil {
		return nil, fmt.Errorf("listing installed plugins: %w", err)
	}
	d := pluginInventoryData{Plugins: []pluginInventoryRow{}}
	for _, p := range plugins {
		d.Plugins = append(d.Plugins, pluginInventoryRow{Name: p.Name, Version: p.Version, License: plugin.Label(p.Tier), Status: p.Status, Description: p.Description})
	}
	return d, nil
}

func pluginStatusFacts(_ *cobra.Command, args []string) (any, error) {
	d := pluginStatusData{Plugins: []pluginStatusRow{}}
	if len(args) == 1 {
		st, err := plugin.Status(args[0])
		if err != nil {
			return nil, fmt.Errorf("getting status for %q: %w", args[0], err)
		}
		d.Plugins = append(d.Plugins, pluginStatusRow{Name: st.Name, State: st.State, PID: st.PID})
		return d, nil
	}
	plugins, err := plugin.List(resolvePluginDir(), true)
	if err != nil {
		return nil, fmt.Errorf("listing installed plugins: %w", err)
	}
	for _, p := range plugins {
		row := pluginStatusRow{Name: p.Name, State: "unknown"}
		if st, err := plugin.Status(p.Name); err == nil {
			row.State, row.PID = st.State, st.PID
		}
		d.Plugins = append(d.Plugins, row)
	}
	return d, nil
}

func pluginUpdatesFacts(cmd *cobra.Command, _ []string) (any, error) {
	if err := plugin.ValidateNetworkAccess(cmd.Context(), os.Getenv("NSELF_PLUGIN_REGISTRY")); err != nil {
		return nil, err
	}
	pluginDir := resolvePluginDir()
	installed, err := plugin.List(pluginDir, true)
	if err != nil {
		return nil, fmt.Errorf("listing installed plugins: %w", err)
	}
	d := pluginUpdatesData{Updates: []pluginUpdateRow{}}
	if len(installed) == 0 {
		return d, nil
	}
	registry, err := plugin.List(pluginDir, false)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	latest := make(map[string]string, len(registry))
	for _, p := range registry {
		latest[p.Name] = p.Version
	}
	for _, p := range installed {
		if v, ok := latest[p.Name]; ok && v != p.Version {
			d.Updates = append(d.Updates, pluginUpdateRow{Name: p.Name, Installed: p.Version, Latest: v})
		}
	}
	return d, nil
}

// newCompatCheckData converts the compat-check findings.
func newCompatCheckData(results []compatResult) compatCheckData {
	d := compatCheckData{CLIVersion: version.GetVersion(), Plugins: []compatRow{}}
	for _, r := range results {
		d.Plugins = append(d.Plugins, compatRow{Name: r.Name, Version: r.Installed, Status: r.Status, Compatible: r.Compatible, Reason: r.Reason, Hint: r.Hint})
	}
	return d
}

// newPluginInfoData builds the info document; m is nil when the tier is unresolved.
func newPluginInfoData(name string, m *plugin.PluginManifest, tier, reason string, cause error) pluginInfoData {
	out := pluginInfoData{Name: name, Tier: tier, TierReason: reason}
	if m != nil {
		out.Name, out.Version, out.Description, out.Category, out.License, out.Port = m.Name, m.Version, m.Description, m.Category, m.License, m.Port
	}
	if cause != nil {
		out.TierError = cause.Error()
	}
	return out
}

func init() {
	for _, c := range []*cobra.Command{pluginUpdateCmd, pluginRefreshCmd, pluginStartCmd, pluginStopCmd, pluginDisableCmd, pluginEnableCmd} {
		c.RunE = dataEnv(c.RunE)
	}
	pluginInventoryCmd.RunE = readBody(pluginInventoryFacts, pluginInventoryCmd.RunE)
	pluginStatusCmd.RunE = readBody(pluginStatusFacts, pluginStatusCmd.RunE)
	pluginUpdatesCmd.RunE = readBody(pluginUpdatesFacts, pluginUpdatesCmd.RunE)
}
