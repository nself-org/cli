// JSON data-type registration for the extend canon fragment (add, remove,
// `config bundles ...`, `config plugins ...`; `plugin`, `bundle` and `install`
// are the v1.4 spellings).
//
// Purpose: every runnable document command of the fragment answers with the v1
// envelope in v1.5 mode (P7-SURF-13). `add` and `config bundles install`
// report {installed, skipped, next}; `remove` reports what was removed, kept
// and planned, and the next command (install_envelope.go); command groups
// answer with their subcommands (dataHub); read commands with typed facts
// (plugin_envelope.go); the writing commands with the shared dataResult.
// Rows that printed JSON before the contract keep that output in v1.4 and say
// so in the v1.4 registry view (`json: legacy` in extend.yaml, kept by the
// cmdregistry mapping for V15-only rows). Bare `config plugins` answers with its subcommands (a plugin
// proxy with arguments is refused in JSON mode). `config plugins logs` is a
// stream and registers nothing. `plugin install`, `plugin remove` and `bundle remove`
// are shims of `add` and `remove`.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import (
	"github.com/nself-org/cli/internal/audit"
	"github.com/nself-org/cli/internal/plugin/count"
)

// extendEnvelopeTypes maps each path of the fragment to its envelope data.
var extendEnvelopeTypes = map[string]any{
	"add":                         addData{},
	"remove":                      removeData{},
	"config plugins":              dataHub{},
	"config bundles":              dataHub{},
	"config bundles info":         bundleInfoJSON{},
	"config bundles install":      addData{},
	"config bundles list":         []bundleListRow{},
	"config plugins audit-tables": audit.AuditReport{},
	"config plugins compat-check": compatCheckData{},
	"config plugins count":        count.Artifact{},
	"config plugins disable":      dataResult{},
	"config plugins enable":       dataResult{},
	"config plugins info":         pluginInfoData{},
	"config plugins inventory":    pluginInventoryData{},
	"config plugins list":         pluginListData{},
	"config plugins outdated":     []outdatedPluginRow{},
	"config plugins refresh":      dataResult{},
	"config plugins search":       searchData{},
	"config plugins start":        dataResult{},
	"config plugins status":       pluginStatusData{},
	"config plugins stop":         dataResult{},
	"config plugins update":       dataResult{},
	"config plugins updates":      pluginUpdatesData{},
}

func init() {
	for path, zero := range extendEnvelopeTypes {
		registerJSONType(path, zero)
		registerV15OnlyEnvelope(path)
	}
}
