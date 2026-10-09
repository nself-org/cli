package controlplane

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"gopkg.in/yaml.v3"
)

// serverNameRe is the validation pattern for Server.Name values that are
// used in shell-adjacent contexts (e.g. passed as SSH remote-command args).
// Only alphanumerics, hyphens, and underscores are permitted; this prevents
// injection via shell metacharacters when the name appears in exec argv.
var serverNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateServerName returns an error when name contains characters that
// would be dangerous in a remote SSH argument position.
func ValidateServerName(name string) error {
	if name == "" {
		return fmt.Errorf("controlplane: server name must not be empty")
	}
	if !serverNameRe.MatchString(name) {
		return fmt.Errorf("controlplane: server name %q contains invalid characters (allowed: [a-zA-Z0-9_-])", name)
	}
	return nil
}

const (
	// inventoryFileName is the canonical location of the control-plane
	// inventory relative to the project root.
	inventoryFileName = ".nself/control-plane.yaml"

	// currentSchemaVersion is the schema version this code produces.
	currentSchemaVersion = 2

	// inventoryFileMode is the file permission for the inventory and its parent
	// directory. Secrets must never be readable by group/other.
	inventoryFileMode = 0o600

	// envVarPrefix is the prefix of legacy single-server deployment env vars.
	// e.g. NSELF_DEPLOY_HOST_STAGING=ubuntu@staging.example.com
	envVarPrefix = "NSELF_DEPLOY_HOST_"
)

// Load reads the control-plane inventory for the given project root.
//
// If .nself/control-plane.yaml exists it is unmarshalled and Migrate is
// applied to bring it to the current schema version. If the file is absent,
// Load synthesizes a single-server inventory from NSELF_DEPLOY_HOST_<TARGET>
// environment variables so that legacy configurations continue to work
// without modification (back-compat guarantee).
func Load(projectRoot string) (*Inventory, error) {
	path := filepath.Join(projectRoot, inventoryFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := rejectLegacyEnvCaseCollisions(); err != nil {
				return nil, err
			}
			synthesized := synthesize()
			// T31: synthesize() builds Server.RemotePath from
			// NSELF_REMOTE_PATH_<TARGET> environment variables, another
			// external-input path into the same field the YAML-load path
			// below validates. Apply the identical guard so an operator
			// (or a compromised env) cannot smuggle shell metacharacters in
			// through this route either.
			if err := validateInventoryNames(synthesized); err != nil {
				return nil, err
			}
			return synthesized, nil
		}
		return nil, fmt.Errorf("controlplane: read inventory: %w", err)
	}

	var inv Inventory
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("controlplane: parse inventory: %w", err)
	}

	if err := Migrate(&inv); err != nil {
		return nil, fmt.Errorf("controlplane: migrate inventory: %w", err)
	}

	// Validate all server names before returning. Names flow into SSH argv
	// (lb.go runLBHelper) and must contain no shell metacharacters.
	if err := validateInventoryNames(&inv); err != nil {
		return nil, err
	}

	return &inv, nil
}

// validateInventoryNames checks every Server.Name against serverNameRe and
// every Server.RemotePath against deploy.ValidateRemotePath.
//
// This is the defense point for the file-load path: .nself/control-plane.yaml
// is a hand-editable / import-able YAML file, so a RemotePath smuggled in via
// a manually edited or migrated inventory file (rather than through
// `env target add --remote-path`, which validates at the flag layer) is
// caught here before the inventory is ever used to build an SSH deploy
// command (T31 — RemotePath is interpolated into a remote shell string in
// internal/deploy/ssh.go's DeployViaSsh).
func validateInventoryNames(inv *Inventory) error {
	if err := rejectEnvCaseCollisions(inv); err != nil {
		return err
	}
	if err := validateInventoryV2(inv); err != nil {
		return err
	}
	for envName, env := range inv.Environments {
		for _, srv := range env.Servers {
			if err := ValidateServerName(srv.Name); err != nil {
				return errs.New("E485", fmt.Sprintf("environments.%s.servers.name: %v", envName, err))
			}
			if err := deploy.ValidateRemotePath(srv.RemotePath); err != nil {
				return errs.New("E485", fmt.Sprintf("environments.%s.servers.%s.remote_path: %v", envName, srv.Name, err))
			}
			if srv.Host != "" {
				if _, err := remote.ParseHostSpec(srv.Host); err != nil {
					return errs.New("E484", fmt.Sprintf("environments.%s.servers.%s.host: %v", envName, srv.Name, err))
				}
			}
		}
	}
	return nil
}

// rejectEnvCaseCollisions refuses an inventory with two environment keys that
// differ only by case (qa and QA): a deploy target named in either case would
// be ambiguous. The error is E483 and names both keys, in sorted order.
func rejectEnvCaseCollisions(inv *Inventory) error {
	seen := make(map[string]string, len(inv.Environments))
	keys := make([]string, 0, len(inv.Environments))
	for k := range inv.Environments {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		folded := strings.ToLower(k)
		if first, dup := seen[folded]; dup {
			return errs.New("E483", fmt.Sprintf("inventory environments %q and %q differ only by case; rename one so every environment name is unambiguous", first, k))
		}
		seen[folded] = k
	}
	return nil
}

// Write persists inv to .nself/control-plane.yaml inside projectRoot.
// The file and its parent directory are created with mode 0600 / 0700
// respectively if they do not already exist.
func Write(projectRoot string, inv *Inventory) error {
	if err := Migrate(inv); err != nil {
		return err
	}
	dir := filepath.Join(projectRoot, ".nself")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("controlplane: create .nself dir: %w", err)
	}

	data, err := yaml.Marshal(inv)
	if err != nil {
		return fmt.Errorf("controlplane: marshal inventory: %w", err)
	}

	path := filepath.Join(projectRoot, inventoryFileName)
	if err := os.WriteFile(path, data, inventoryFileMode); err != nil {
		return fmt.Errorf("controlplane: write inventory: %w", err)
	}

	// Enforce 0600 even if the file pre-existed with looser permissions.
	if err := os.Chmod(path, inventoryFileMode); err != nil {
		return fmt.Errorf("controlplane: chmod inventory: %w", err)
	}

	return nil
}

// Migrate upgrades inv in-place to currentSchemaVersion. It is idempotent:
// calling it multiple times on the same inventory produces the same result.
// Currently the only supported input version is 1 (same as current); future
// versions will add migration steps here.
func Migrate(inv *Inventory) error {
	switch inv.SchemaVersion {
	case 0:
		// Version 0 is the unversioned legacy format. Treat it as version 1
		// with an empty environments map and synthesize from env vars.
		inv.SchemaVersion = currentSchemaVersion
		if inv.Environments == nil {
			synthesized := synthesize()
			inv.Environments = synthesized.Environments
			if inv.Project == "" {
				inv.Project = synthesized.Project
			}
		}
		return migrateTiers(inv)
	case 1:
		inv.SchemaVersion = currentSchemaVersion
		return migrateTiers(inv)
	case currentSchemaVersion:
		return migrateTiers(inv)
	default:
		return fmt.Errorf("controlplane: unsupported schema_version %d (max supported: %d)",
			inv.SchemaVersion, currentSchemaVersion)
	}
}

// synthesize builds an Inventory from NSELF_DEPLOY_HOST_<TARGET> environment
// variables. This preserves byte-behavior-identical semantics with the legacy
// single-server deployment path: each matching env var becomes one "remote"
// environment with a single app server.
//
// The following env vars are consulted per target (TARGET = suffix after
// NSELF_DEPLOY_HOST_, case-preserved but lowercased for the env name):
//
//	NSELF_DEPLOY_HOST_<TARGET>       user@host            (required)
//	NSELF_SSH_KEY_<TARGET>           env-var name to use as SSHKeyRef (optional)
//	NSELF_REMOTE_PATH_<TARGET>       remote install path  (optional, default /opt/nself)
//
// If no matching env vars exist, an empty Inventory with only the "local"
// environment is returned.
func synthesize() *Inventory {
	inv := &Inventory{
		SchemaVersion: currentSchemaVersion,
		Project:       "nself",
		Environments: map[string]Environment{
			"local": {
				Name: "local",
				Kind: "local",
				Tier: TierLocal,
				Servers: []Server{
					{
						Name:    "local-app",
						Role:    RoleApp,
						Primary: true,
					},
				},
			},
		},
	}

	for _, env := range os.Environ() {
		key, val, ok := strings.Cut(env, "=")
		if !ok {
			continue
		}
		if !strings.HasPrefix(key, envVarPrefix) {
			continue
		}
		target := strings.TrimPrefix(key, envVarPrefix)
		if target == "" || val == "" {
			continue
		}
		envName := strings.ToLower(target)

		// Determine the SSH key env-var reference name. Prefer an explicit
		// NSELF_SSH_KEY_<TARGET> override; fall back to the conventional name.
		sshKeyRef := os.Getenv("NSELF_SSH_KEY_" + target)
		if sshKeyRef == "" {
			// Default convention: env var name that holds the key path.
			sshKeyRef = "NSELF_SSH_KEY_" + target
		}

		remotePath := os.Getenv("NSELF_REMOTE_PATH_" + target)
		if spec, err := remote.ParseHostSpec(val); err == nil && spec.LegacyPath != "" {
			remotePath = spec.LegacyPath
			val = spec.String()
			warnLegacyHost()
		}
		if remotePath == "" {
			remotePath = "/opt/nself"
		}

		inv.Environments[envName] = Environment{
			Name: envName,
			Kind: "remote",
			Tier: DeriveTier(envName, "remote"),
			Servers: []Server{
				{
					Name:       envName + "-app",
					Role:       RoleApp,
					Host:       val,
					SSHKeyRef:  sshKeyRef,
					RemotePath: remotePath,
					Primary:    true,
				},
			},
		}
	}

	return inv
}
