package commands

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/ui"
)

// initPreset describes a named project preset for nself init --preset.
type initPreset struct {
	Name        string
	Description string
	// SuggestedPlugins are the pro plugin bundle slugs relevant for this preset.
	SuggestedPlugins []string
	// EnabledServices are the optional service flags this preset recommends enabling.
	EnabledServices []string
	// Notes are printed after init completes.
	Notes []string
}

// initPresets is the canonical registry of built-in presets.
var initPresets = map[string]initPreset{
	"b2b-saas": {
		Name:             "b2b-saas",
		Description:      "Multi-tenant B2B SaaS backend with auth, billing, and team management",
		SuggestedPlugins: []string{"auth", "billing", "notify", "realtime"},
		EnabledServices:  []string{"redis", "functions"},
		Notes: []string{
			"Add team/org tables: nself db migrate",
			"Install billing plugin: nself plugin install billing",
			"Set up Stripe keys in .env.secrets",
		},
	},
	"mobile-backend": {
		Name:             "mobile-backend",
		Description:      "Mobile app backend with push notifications, offline sync, and real-time subscriptions",
		SuggestedPlugins: []string{"notify", "sync", "realtime", "auth"},
		EnabledServices:  []string{"redis"},
		Notes: []string{
			"Configure push provider in .env (FCM/APNS keys)",
			"Install sync plugin: nself plugin install sync",
			"Enable real-time: nself plugin install realtime",
		},
	},
	"ai-assistant": {
		Name:             "ai-assistant",
		Description:      "AI assistant backend with vector search, memory, and multi-model routing (ɳClaw-style)",
		SuggestedPlugins: []string{"ai", "claw", "mux", "browser", "voice"},
		EnabledServices:  []string{"redis", "search"},
		Notes: []string{
			"Set AI provider keys in .env.secrets (OPENAI_API_KEY / ANTHROPIC_API_KEY)",
			"Install AI bundle: nself plugin install ai mux",
			"pgvector is pre-enabled for vector search",
		},
	},
	"community-forum": {
		Name:             "community-forum",
		Description:      "Community forum backend with moderation, voting, and email digests",
		SuggestedPlugins: []string{"notify", "moderation", "cms", "auth"},
		EnabledServices:  []string{"mail", "search"},
		Notes: []string{
			"Configure mail provider in .env (SMTP settings)",
			"Install moderation plugin: nself plugin install moderation",
		},
	},
	"media-hosting": {
		Name:             "media-hosting",
		Description:      "Media hosting and streaming backend with transcoding and CDN integration",
		SuggestedPlugins: []string{"media-processing", "streaming", "file-processing", "subtitle-manager"},
		EnabledServices:  []string{"minio"},
		Notes: []string{
			"MinIO (S3-compatible storage) is enabled. Configure MINIO_ACCESS_KEY in .env",
			"Install nTV bundle: nself plugin install media-processing streaming",
		},
	},
	"dev": {
		Name:             "dev",
		Description:      "Lightweight development environment: Postgres + Hasura + Auth only, no monitoring stack",
		SuggestedPlugins: []string{},
		EnabledServices:  []string{},
		Notes: []string{
			"Monitoring stack is disabled for fast cold-start",
			"Add services later: nself service enable redis",
			"Install free plugins as needed: nself plugin install <name>",
		},
	},
	"sentry": {
		Name:             "sentry",
		Description:      "Local-first ɳSentry observability stack: uptime, status pages, incidents, alerts, errors, RUM",
		SuggestedPlugins: []string{"uptime-monitor", "status-page", "incident-mgmt", "alert-router", "slo-tracker", "synthetic-monitor", "rum", "errors", "cron-monitor", "oncall", "crash", "anomaly", "audit"},
		EnabledServices:  []string{"redis"},
		Notes: []string{
			"Install the ɳSentry bundle: nself plugin install uptime-monitor status-page incident-mgmt alert-router",
			"Bundle plugins need a license key (nself license activate) — audit is free (Security-Always-Free)",
			"Plugin migrations seed a dev tenant + dev API key: nsk_dev_local_0000000000000000 (local only, never valid in cloud)",
			"Point the CLI at the local API: export NSELF_SENTRY_API_URL=http://localhost:3848",
			"Log in with the dev key: nself sentry login --api-url http://localhost:3848 --api-key nsk_dev_local_0000000000000000",
			"Now the full cloud CLI works offline: nself sentry monitors add --url https://example.com",
		},
	},
	"nclaw-app": {
		Name:             "nclaw-app",
		Description:      "Pre-configured backend for ɳClaw: AI memory, multi-model routing, email, and OAuth",
		SuggestedPlugins: []string{"ai", "claw", "claw-web", "mux", "voice", "browser", "google", "notify", "cron", "claw-budget", "claw-news"},
		EnabledServices:  []string{"redis", "search"},
		Notes: []string{
			"Set ANTHROPIC_API_KEY in .env.secrets",
			"Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET in .env.secrets",
			"Install nClaw bundle: nself plugin install ai claw mux",
			"pgvector is pre-enabled for vector search (required by the claw plugin)",
		},
	},
}

// listInitPresets prints all available presets as a formatted table to stdout.
func listInitPresets() {
	fmt.Println()
	ui.Section("Available presets (--preset <name>)")
	fmt.Println()

	tbl := ui.NewTable("Preset", "Description", "Suggested plugins")
	for _, name := range []string{"b2b-saas", "mobile-backend", "ai-assistant", "community-forum", "media-hosting", "dev", "sentry", "nclaw-app"} {
		p := initPresets[name]
		plugins := "(none)"
		if len(p.SuggestedPlugins) > 0 {
			plugins = strings.Join(p.SuggestedPlugins, ", ")
		}
		tbl.AddRow(p.Name, p.Description, plugins)
	}
	tbl.Render()
	fmt.Println()
}

// printPresetPostInit prints preset-specific post-init notes.
func printPresetPostInit(presetName string) {
	p, ok := resolveInitPreset(presetName)
	if !ok {
		return
	}
	fmt.Println()
	ui.Section(fmt.Sprintf("Preset: %s — next steps", p.Name))
	for i, note := range p.Notes {
		fmt.Printf("  %s%d.%s %s\n", ui.Bold, i+1, ui.Reset, note)
	}
	fmt.Println()
}

// Purpose: resolve preset recommendations for the current compatibility mode.
// Inputs: registry name. Outputs: preset and presence flag.
// Constraints: leave the shared registry unchanged; no environment writes.
// SPORT: P7-PLUG-60.
func resolveInitPreset(name string) (initPreset, bool) {
	p, ok := initPresets[name]
	// compat.V15(P7-PLUG-60): media-hosting recommends MinIO -> recommends core object storage and the MinIO plugin for existing data
	if compat.V15() && name == "media-hosting" && ok {
		p.EnabledServices = []string{"object-storage"}
		p.Notes = []string{
			"Enable SeaweedFS object storage: set OBJECT_STORAGE_ENABLED=true in .env",
			"Set S3_ACCESS_KEY and S3_SECRET_KEY in .env.secrets",
			"Keep existing MinIO data: nself add minio",
		}
	}
	return p, ok
}
