package commands

import (
	"fmt"
	"strings"

	clonetemplate "github.com/nself-org/cli/internal/templates/clone"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new nSelf project",
	Long: `Interactive setup wizard that creates a pristine .env configuration
for a new nSelf project.

Entry points:
  nself init                 Minimal setup with smart defaults
  nself init --wizard        Full 10-step interactive wizard
  nself init --demo          All services enabled, pre-configured
  nself init --full          Creates .env.dev, .env.staging, .env.prod, .env.secrets
  nself init --fast          Skip advanced options, use smart defaults
  nself init --non-interactive  Use all defaults without prompts

After init, run:
  Edit .env      Customize your project settings
  nself start    Boot your backend stack`,
	RunE: runInit,
}

func init() {
	f := initCmd.Flags()
	f.Bool("fast", false, "Skip advanced options, use smart defaults")
	f.Bool("interactive", false, "Explicitly enable interactive wizard")
	f.Bool("non-interactive", false, "Use all defaults without prompts")
	f.String("template", "", "Use a built-in, clone, or marketplace template (e.g. airbnb-clone, go, rust)")
	f.Bool("no-seed", false, "Skip seed data when scaffolding a clone template")
	f.Bool("dry-run", false, "Print files that would be written without writing them (clone templates only)")
	f.Bool("skip-validation", false, "Skip configuration validation")
	f.Bool("wizard", false, "Run the full 10-step interactive wizard")
	f.Bool("demo", false, "Auto-configure with all services enabled")
	f.Bool("full", false, "Create all environment files (.env.dev, .env.staging, .env.prod, .env.secrets)")
	f.Bool("force", false, "Overwrite existing configuration")
	f.Bool("quiet", false, "Suppress output messages")
	f.String("name", "", "Project name (sets PROJECT_NAME in generated .env)")
	f.String("domain", "", "Base domain (default: local.nself.org; init never prompts for it, e.g. myapp.dev)")
	f.String("profile", "", "Resource profile: 'tiny' for small VPS (Postgres+nginx only)")
	f.Bool("no-pgvector", false, "Skip pgvector extension and RAG scaffold tables (sets PGVECTOR_ENABLED=false)")
	f.String("preset", "", "Use a project-type preset: b2b-saas, mobile-backend, ai-assistant, community-forum, media-hosting, dev, sentry, nclaw-app")
	f.Bool("list-presets", false, "List all available project presets and exit")
	f.String("cs-template", "", "Scaffold a custom service at init time: specify language (go, node, python, rust, other)")

	RootCmd.AddCommand(initCmd)
}

// classifyInitError maps an init error to a categorical string safe for telemetry.
// Enumerated values: timeout, permission-denied, docker-not-found, other.
// Must never include the error message text (may contain file paths).
func classifyInitError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "timeout", "deadline exceeded", "context deadline"):
		return "timeout"
	case containsAny(msg, "permission denied", "access denied", "EACCES"):
		return "permission-denied"
	case containsAny(msg, "docker", "Docker", "cannot connect to the Docker"):
		return "docker-not-found"
	default:
		return "other"
	}
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// domainOption represents a single selectable domain pattern in the wizard.
type domainOption struct {
	label   string // display label shown in the menu
	value   string // BASE_DOMAIN value written to .env
	comment string // comment written above BASE_DOMAIN in .env
	custom  bool   // when true, prompt user for a custom value
}

// domainOptions lists the preset domain patterns presented to the user.
var domainOptions = []domainOption{
	{
		label:   "*.local.nself.org    — Works with local.nself.org wildcard DNS",
		value:   "local.nself.org",
		comment: "# Domain: *.local.nself.org (wildcard — requires DNS resolution)",
	},
	{
		label:   "*.localhost           — Browser-only, no TLS in some browsers",
		value:   "localhost",
		comment: "# Domain: *.localhost (browser-only local dev)",
	},
	{
		label:   "*.127.0.0.1.nip.io   — Works on any machine, uses wildcard DNS",
		value:   "127.0.0.1.nip.io",
		comment: "# Domain: *.127.0.0.1.nip.io (wildcard DNS via nip.io)",
	},
	{
		label:  "Custom domain         — Enter your own (e.g. myapp.dev)",
		custom: true,
	},
}

// resolveInitDomain picks BASE_DOMAIN and its .env comment without ever
// reading stdin: init asks no questions (owner canon "init && start, zero
// prompts", P7-CANON-16). An explicit --domain wins and is validated; otherwise
// the first preset (the former menu default) applies when useDefault is true,
// and "" lets setup apply its own defaults.
func resolveInitDomain(domainFlag string, useDefault bool) (domain, comment string, err error) {
	if domainFlag != "" {
		if err := validateDomain(domainFlag); err != nil {
			return "", "", err
		}
		return domainFlag, "", nil
	}
	if useDefault {
		return domainOptions[0].value, domainOptions[0].comment, nil
	}
	return "", "", nil
}

func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain must not be empty")
	}
	if strings.ContainsAny(domain, " \t\n\r") {
		return fmt.Errorf("domain must not contain whitespace: %q", domain)
	}
	return nil
}

// builtinTemplates are the scaffolding language templates bundled with the CLI.
var builtinTemplates = []string{"postgres", "express", "fastapi", "go", "rust"}

func validateTemplate(name string) error {
	for _, v := range builtinTemplates {
		if v == name {
			return nil
		}
	}
	// Not a built-in; treat as a marketplace slug — validation deferred to registry lookup.
	return nil
}

// isBuiltinTemplate reports whether name is one of the bundled language or clone templates.
func isBuiltinTemplate(name string) bool {
	for _, v := range builtinTemplates {
		if v == name {
			return true
		}
	}
	return clonetemplate.IsCloneTemplate(name)
}
