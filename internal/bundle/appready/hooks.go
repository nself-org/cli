package appready

// hooks.go — the cmd-supplied functions and the members the steps check.
//
// Purpose: internal/bundle may not import reconcile, database, health or the
// start command (layers.yaml), so cmd/commands injects them as Hooks. A nil
// Hooks value (callers that are not the CLI) skips every step after the ledger.
// Inputs: Hooks, and the installed plugin directory for LoadMembers.
// Outputs: Member values read from <pluginDir>/<name>/plugin.json.
// Constraints: a missing or invalid plugin.json is an error naming the plugin.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
	"github.com/nself-org/cli/internal/plugin/readiness"
)

// ErrNoProject is what Hooks.Reconcile returns outside an nSelf project: the
// plugins are installed, but there is no stack to reconcile, start or check.
var ErrNoProject = errors.New("not in an nSelf project")

// Hooks are the project-level operations the post-install steps call.
// A nil function skips its step and marks the report unverified.
type Hooks struct {
	// Reconcile regenerates project files for the new membership.
	Reconcile func(ctx context.Context) error
	// Start boots the stack (the in-process `nself start`).
	Start func(ctx context.Context) error
	// TrackTables tracks the members' manifest tables in Hasura (P7-PLUG-78).
	// The tracking step calls it before the ExportMetadata verification; nil
	// skips it (v1.4 builds none). Return *StepError to name the plugin.
	TrackTables func(ctx context.Context, members []Member) error
	// ExportMetadata returns Hasura's export_metadata response.
	ExportMetadata func(ctx context.Context) ([]byte, error)
	// ServiceHealth returns an error listing unhealthy services.
	ServiceHealth func(ctx context.Context) error
	// HealthURL is the plugin's /health URL; default http://127.0.0.1:<port><endpoint>.
	HealthURL func(m Member) string
	// Timeout bounds each waiting step; default readiness.TimeoutFromEnv().
	Timeout time.Duration
	// Interval is the poll interval; tests set milliseconds.
	Interval time.Duration
}

// Member is one installed plugin of the bundle.
type Member struct {
	Name             string
	Dir              string
	Port             int
	HealthEndpoint   string
	Tables           []string
	BootMigrations   bool // manifest declares migrations.apply: boot
	HasMigrationsDir bool // a migrations directory exists on disk
}

// LoadMembers reads the named plugins' manifests from pluginDir.
func LoadMembers(pluginDir string, names []string) ([]Member, error) {
	out := make([]Member, 0, len(names))
	for _, name := range names {
		dir := filepath.Join(pluginDir, name)
		raw, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
		if err != nil {
			return nil, &StepError{Plugin: name, Err: fmt.Errorf("plugin %s: cannot read plugin.json: %w", name, err)}
		}
		m, err := manifestv2.ParseQuiet(raw)
		if err != nil {
			return nil, &StepError{Plugin: name, Err: fmt.Errorf("plugin %s: plugin.json invalid: %w", name, err)}
		}
		mem := Member{Name: name, Dir: dir, Port: m.Port, HealthEndpoint: m.HealthEndpoint, Tables: m.Tables}
		if m.Service != nil {
			if mem.Port == 0 && m.Service.Port != nil {
				mem.Port = *m.Service.Port
			}
			if mem.HealthEndpoint == "" && m.Service.Healthcheck != nil {
				mem.HealthEndpoint = *m.Service.Healthcheck
			}
		}
		if mem.HealthEndpoint == "" {
			mem.HealthEndpoint = "/health"
		}
		if !strings.HasPrefix(mem.HealthEndpoint, "/") {
			mem.HealthEndpoint = "/" + mem.HealthEndpoint
		}
		migDir := "migrations"
		if m.Migrations != nil {
			mem.BootMigrations = m.Migrations.Apply == "boot"
			if m.Migrations.Dir != "" {
				migDir = m.Migrations.Dir
			}
		}
		if st, serr := os.Stat(filepath.Join(dir, migDir)); serr == nil && st.IsDir() {
			mem.HasMigrationsDir = true
		}
		out = append(out, mem)
	}
	return out, nil
}

// DefaultHealthURL is where the member's container publishes /health on the host.
func DefaultHealthURL(m Member) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", m.Port, m.HealthEndpoint)
}

func (h Hooks) healthURL(m Member) string {
	if h.HealthURL != nil {
		return h.HealthURL(m)
	}
	return DefaultHealthURL(m)
}

func (h Hooks) timeout() time.Duration {
	if h.Timeout > 0 {
		return h.Timeout
	}
	d, err := readiness.TimeoutFromEnv()
	if err != nil || d <= 0 {
		return readiness.DefaultTimeout
	}
	return d
}

func (h Hooks) interval() time.Duration {
	if h.Interval > 0 {
		return h.Interval
	}
	return 2 * time.Second
}
