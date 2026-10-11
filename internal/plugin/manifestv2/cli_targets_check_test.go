package manifestv2_test

import (
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// TestCheckCLITargets pins the D-0259 refusals: names outside NamePattern and
// binaries outside BinaryPattern are E106, core verbs E113, a command declared
// twice E106; the multi-command plugins that ship today stay valid.
func TestCheckCLITargets(t *testing.T) {
	ok := []struct {
		name, typ, bin string
		cmds           []string
	}{
		{"sentry-cli", "cli", "nself-sentry", []string{"sentry", "sentry-server"}},
		{"tenant", "cli", "nself-tenant", []string{"tenant", "billing"}},
		{"ai-cli", "cli", "nself-ai", nil},
		{"k8s", "cli", "", nil},
		{"svc", "service", "nself-x", []string{"init"}},
	}
	for _, c := range ok {
		got, err := manifestv2.CheckedCLITargets(c.name, c.typ, c.bin, c.cmds)
		if err != nil {
			t.Errorf("%s: unexpected refusal: %v", c.name, err)
		}
		if want := manifestv2.CLITargets(c.name, c.typ, c.bin, c.cmds); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", c.name, got, want)
		}
	}

	bad := []struct {
		label, bin string
		cmds       []string
		code       string
		mentions   []string
	}{
		{"space", "", []string{"server", "roms list"}, "E106", []string{"roms list", manifestv2.NamePattern}},
		{"upper case", "", []string{"Feeds"}, "E106", []string{"Feeds"}},
		{"leading digit", "", []string{"9lives"}, "E106", []string{"9lives"}},
		{"binary without prefix", "mytool", nil, "E106", []string{"mytool", manifestv2.BinaryPattern}},
		{"binary upper case", "nself-Tool", nil, "E106", []string{"Tool"}},
		{"core verb", "", []string{"search", "init"}, "E113", []string{"init", "p"}},
		{"core verb via binaryName", "nself-backup", nil, "E113", []string{"backup"}},
		{"declared twice", "", []string{"feeds", "feeds"}, "E106", []string{"feeds", "twice"}},
	}
	for _, c := range bad {
		got, err := manifestv2.CheckedCLITargets("p", "", c.bin, c.cmds)
		if got != nil {
			t.Errorf("%s: refused targets still returned: %v", c.label, got)
		}
		t.Run(c.label, func(t *testing.T) { wantCode(t, err, c.code, c.mentions...) })
	}
}

// TestCheckCLITargets_EveryCoreVerb proves the whole canon list is refused,
// not only the verbs the plugins happened to use.
func TestCheckCLITargets_EveryCoreVerb(t *testing.T) {
	for _, v := range manifestv2.CoreVerbs {
		wantCode(t, manifestv2.CheckCLITargets("p", []manifestv2.CLITarget{{Binary: "nself-" + v, Command: v}}), "E113", v)
	}
}

// TestCheckCLITargets_FreePluginManifests is the positive control on real
// data: the cliCommands of the five free plugins D-0259 names (plugins
// origin/main fdb1be58) and Bundles realtime are all refused now.
func TestCheckCLITargets_FreePluginManifests(t *testing.T) {
	for name, cmds := range map[string][]string{
		"content-acquisition": {"init", "subscribe", "feeds", "calendar", "queue", "server"},
		"retro-gaming":        {"server", "roms list", "roms stats", "cores list", "cores seed", "sessions recent", "stats"},
		"subtitle-manager":    {"init", "search", "server", "sync", "qc", "normalize"},
		"torrent-manager":     {"init", "search", "best-match", "add", "list", "stats", "server"},
		"vpn":                 {"init", "providers", "connect", "disconnect", "status", "servers"},
		"realtime":            {"init", "server", "status", "rooms", "connections", "presence"},
	} {
		if _, err := manifestv2.CheckedCLITargets(name, "", "", cmds); err == nil {
			t.Errorf("%s: cliCommands %v accepted", name, cmds)
		}
	}
}
