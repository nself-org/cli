package config

// cascade.go — canonical .env cascade file order (CLI-R18).
//
// Purpose: Single source of truth for which .env* files nSelf reads and in
//          what precedence order, shared by Load() (loader.go), `nself env
//          explain` (cmd/commands/env_explain.go), and the migration shim
//          (internal/migrate/env_order.go) so the three can never disagree
//          about what actually wins.
// Inputs:  a normalized environment name (dev/staging/prod/...) and whether
//          the NSELF_LEGACY_ENV_ORDER escape hatch is active.
// Outputs: EnvCascadeOrder returns an ordered []string of filenames, lowest
//          precedence first (later entries win). EnvCascade resolves that
//          list to on-disk paths with existence checked, for display and
//          migration purposes.
// Constraints: Pure — no env var reads, no warnings, no filesystem writes.
//              The two orders inlined in EnvCascadeOrder must stay
//              byte-for-byte in sync with the historical and approved orders
//              documented in loader.go and
//              .claude/tasks/cli-review-tickets-2026-08-23.md
//              (DECISIONS — 2026-08-23, GATE B).
// SPORT:   cli/internal/config — CLI-R18 env cascade canon.

import (
	"os"
	"path/filepath"
	"regexp"
)

// LegacyEnvOrderVar is the escape-hatch environment variable that restores
// the pre-CLI-R18 cascade order for exactly one minor version after the
// reorder ships. Every use prints a warning naming the variable and the
// version it will stop being honored in — see warnLegacyEnvOrder in
// loader.go.
const LegacyEnvOrderVar = "NSELF_LEGACY_ENV_ORDER"

// LegacyOrderActive reports whether the NSELF_LEGACY_ENV_ORDER escape hatch
// is set to a truthy value in the current process environment.
func LegacyOrderActive() bool {
	return getEnvBool(LegacyEnvOrderVar, false)
}

// DeployRemoteVar is set to "true" by `nself deploy` for the build it spawns
// when the target is a remote host. The build then drops .env.local from the
// cascade (RemoteCascadeActive), exactly as the env file pushed to the host
// does: a developer's personal override never ships to a server. Internal:
// there is no reason to set it by hand.
const DeployRemoteVar = "NSELF_DEPLOY_REMOTE"

// RemoteCascadeActive reports whether the current process is building for a
// remote deploy (DeployRemoteVar is truthy).
func RemoteCascadeActive() bool {
	return getEnvBool(DeployRemoteVar, false)
}

// WithoutLocalOverride returns order minus .env.local. The remote deploy
// cascade is EnvCascadeOrder with this applied, in the build and in the env
// snapshot pushed to the host alike.
func WithoutLocalOverride(order []string) []string {
	out := make([]string, 0, len(order))
	for _, n := range order {
		if n != ".env.local" {
			out = append(out, n)
		}
	}
	return out
}

// CascadeFile describes one file consulted by the env cascade, in load order.
type CascadeFile struct {
	// Name is the filename relative to the project directory, e.g. ".env.secrets".
	Name string
	// Path is Name joined with the project directory.
	Path string
	// Exists reports whether the file is present on disk.
	Exists bool
}

// EnvCascadeOrder returns the ordered list of filenames (lowest precedence
// first; later entries win) consulted to resolve the given environment name.
//
// Canonical order (legacy=false), approved 2026-08-23 GATE B:
//
//	.env → .env.{dev|staging|prod} → .env.secrets → .env.local
//
// .env is the shared, committed base. Exactly one of .env.dev/.env.staging/
// .env.prod loads, matching envName ("local" loads .env.dev). A custom envName
// (qa, live, ...) loads .env.<envName> and never .env.dev (P7-DEPL-12). .env.secrets never ships in git.
// .env.local is the personal override and always wins. .env.ai no longer
// exists as a cascade layer — its content is folded into .env.secrets at
// init/upgrade (see internal/setup/envai.go and internal/migrate/env_order.go).
//
// Legacy order (legacy=true), restored only via NSELF_LEGACY_ENV_ORDER for
// exactly one minor version:
//
//	.env.dev → .env.{staging|prod} → .env.secrets → .env.local → .env → .env.ai
//
// .env.dev always loaded as a base layer regardless of envName (a quirk the
// canonical order removes), with .env.staging/.env.prod layered on top only
// for those two envs, and bare .env / .env.ai winning last.
func EnvCascadeOrder(envName string, legacy bool) []string {
	name := normalizeEnv(envName)

	if legacy {
		order := []string{".env.dev"}
		switch name {
		case "staging":
			order = append(order, ".env.staging")
		case "prod":
			order = append(order, ".env.prod")
		}
		return append(order, ".env.secrets", ".env.local", ".env", ".env.ai")
	}

	order := []string{".env"}
	switch name {
	case "dev", "local":
		order = append(order, ".env.dev")
	case "staging":
		order = append(order, ".env.staging")
	case "prod":
		order = append(order, ".env.prod")
	default:
		// A custom environment (qa, live, ...) layers its own file over .env and
		// never inherits .env.dev: dev values must not reach a server build. A
		// name that is not a plain file-name fragment gets no layer of its own,
		// so ENV can never point the cascade at another path.
		if customEnvNameRe.MatchString(name) {
			order = append(order, ".env."+name)
		}
	}
	return append(order, ".env.secrets", ".env.local")
}

// customEnvNameRe is the shape a custom environment name must have to name a
// cascade file (.env.<name>).
var customEnvNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// EnvCascade resolves EnvCascadeOrder to on-disk paths under projectDir, with
// existence checked. Used by `nself env explain` and the migration shim; both
// need to know not just the order but which files are actually present.
func EnvCascade(projectDir, envName string, legacy bool) []CascadeFile {
	names := EnvCascadeOrder(envName, legacy)
	files := make([]CascadeFile, 0, len(names))
	for _, n := range names {
		p := filepath.Join(projectDir, n)
		_, err := os.Stat(p)
		files = append(files, CascadeFile{Name: n, Path: p, Exists: err == nil})
	}
	return files
}

// AllCascadeFilenames is the fixed superset of every filename that appears in
// either cascade order, across every environment name. Used by the migration
// shim to read every candidate file once regardless of which order or env is
// being analyzed.
var AllCascadeFilenames = []string{
	".env",
	".env.dev",
	".env.staging",
	".env.prod",
	".env.secrets",
	".env.local",
	".env.ai",
}
