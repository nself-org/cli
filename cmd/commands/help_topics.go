package commands

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/plugin/count"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// helpTopic holds a named help article.
type helpTopic struct {
	Title   string
	Summary string
	Body    string
}

// helpTopics is the built-in topic registry for nself help <topic>.
var helpTopics = map[string]helpTopic{
	"quickstart": {
		Title:   "Quick Start",
		Summary: "Get a backend running in under 5 minutes",
		Body: `  1. nself init           # Generate .env configuration
  2. nself build          # Compose the stack
  3. nself start          # Boot Postgres + Hasura + Auth + nginx
  4. nself status         # Confirm all services healthy
  5. nself doctor         # Diagnose any issues

  Switch environments:
    nself start --env staging
    nself start --env production

  Full guide: https://nself.org/docs/getting-started`,
	},
	"plugins": {
		Title:   "Plugin System",
		Summary: "Install, manage, and build plugins",
		Body:    pluginHelpBody(),
	},
	"license": {
		Title:   "License & Bundles",
		Summary: "Manage license keys for paid plugin bundles",
		Body: `  Bundles ($0.99/mo each):
    nClaw, nChat, nTV, nFamily, ClawDE

  ɳSelf+ ($3.99/mo) — all bundles + all apps + support

  Add a key:    nself license add nself_pro_<key>
  Check status: nself license status
  Validate:     nself license validate
  Upgrade:      nself license upgrade

  Docs: https://nself.org/docs/licensing`,
	},
	"envs": {
		Title:   "Environments",
		Summary: "Managing multiple environments (dev, staging, production)",
		Body: `  nSelf uses an environment cascade (later wins):
    .env → .env.{dev|staging|prod} → .env.secrets → .env.local

  .env is the shared, committed base. Exactly one of .env.dev/.env.staging/
  .env.prod loads, matching ENV. .env.secrets never ships in git. .env.local
  is your personal override and always wins.

  Inspect the cascade:
    nself env explain             # list every file, existence, precedence
    nself env explain VAR         # which file wins for VAR (redacted by
                                   # default; add --reveal to see the value)

  Switch at runtime:
    nself start --env staging
    nself start --env production
    nself deploy --env production

  Full-environment init:
    nself init --full    # generates .env.dev, .env.staging, .env.prod, .env.secrets

  Upgrading an older project: this cascade order changed in CLI-R18 (older
  releases loaded .env.dev -> .env.{staging|prod} -> .env.secrets ->
  .env.local -> .env -> .env.ai). Run 'nself migrate' once — it detects any
  variable whose effective value would change and fixes it automatically
  wherever that's safe, or flags it for your review otherwise. The old order
  can be restored temporarily with NSELF_LEGACY_ENV_ORDER=1 (removed one
  minor version after this change; every use prints a warning).

  Docs: https://nself.org/docs/environments`,
	},
	"doctor": {
		Title:   "Diagnostics",
		Summary: "Diagnose and fix common issues",
		Body: `  nself doctor              # Full system check
  nself doctor --quick      # Fast 10-second check
  nself doctor --deep       # Include resource usage + metrics
  nself doctor --fix        # Auto-fix detected issues
  nself doctor --ai         # AI provider readiness check

  Exit codes:
    0  All checks passed
    1  Checks failed
    2  Warnings only

  Docs: https://nself.org/docs/doctor`,
	},
	"errors": {
		Title:   "Error Codes",
		Summary: "Reference for all nSelf error codes",
		Body: `  Error codes use the format E<NNN>:

  E001-E049  Docker issues (not installed, daemon down, port conflict)
  E050-E099  Config issues (missing .env, invalid values)
  E100-E149  Plugin / license issues
  E150-E199  SSL / network issues
  E200-E249  Database issues
  E250-E299  Health check issues
  E300-E349  Init / project issues
  E350-E399  Domain issues

  Full reference: https://nself.org/docs/reference/error-codes`,
	},
}

var helpTopicsCmd = &cobra.Command{
	Use:   "help-topics [topic]",
	Short: "Browse help topics (quickstart, plugins, license, envs, doctor, errors)",
	Long: `Browse built-in help topics.

Available topics:
  quickstart   Get a backend running in under 5 minutes
  plugins      Install, manage, and build plugins
  license      Manage license keys for paid plugin bundles
  envs         Managing multiple environments
  doctor       Diagnose and fix common issues
  errors       Error code reference

Examples:
  nself help-topics
  nself help-topics plugins
  nself help-topics license`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return printHelpTopicIndex()
		}
		return printHelpTopic(args[0])
	},
}

func init() {
	RootCmd.AddCommand(helpTopicsCmd)
}

func pluginHelpBody() string {
	a, err := count.Load()
	if err != nil {
		return "Plugin counts unavailable. See https://nself.org/docs/plugins"
	}
	return fmt.Sprintf(`  Free plugins (%d) — install with no license key:
    nself plugin list
    nself plugin install notify

  Licensed plugins (%d) — included in Bundles:
    nself license add nself_pro_<your-key>
    nself plugin install ai mux claw

  Search: nself plugin search <keyword>
  Info:   nself plugin info <name>
  Remove: nself plugin remove <name>

  Docs: https://nself.org/docs/plugins`, a.Free.Installable, a.Pro.Installable)
}

func printHelpTopicIndex() error {
	fmt.Println()
	ui.Section("Help Topics")
	fmt.Println()

	order := []string{"quickstart", "plugins", "license", "envs", "doctor", "errors"}
	for _, key := range order {
		t := helpTopics[key]
		fmt.Printf("  %-18s  %s\n", ui.C(ui.Bold, key), t.Summary)
	}
	fmt.Println()
	fmt.Printf("  Usage: %s\n", ui.C(ui.Cyan, "nself help-topics <topic>"))
	fmt.Println()
	return nil
}

func printHelpTopic(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	t, ok := helpTopics[name]
	if !ok {
		_ = printHelpTopicIndex()
		return fmt.Errorf("unknown topic %q — see list above", name)
	}
	fmt.Println()
	fmt.Printf("%s — %s\n", ui.C(ui.Bold, t.Title), t.Summary)
	fmt.Println()
	fmt.Println(t.Body)
	fmt.Println()
	return nil
}
