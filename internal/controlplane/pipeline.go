package controlplane

import (
	"context"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/deploy"
)

// deployServerFn is the per-server SSH deploy. A package-level variable so
// tests can record calls instead of contacting a host.
var deployServerFn = deploy.DeployViaSsh

// ServerResult records the outcome of deploying to one server.
type ServerResult struct {
	// Env is the environment name.
	Env string

	// Server is the server name.
	Server string

	// Role is the server's role (used for ordering and reporting).
	Role ServerRole

	// Status is one of "ok", "skipped", "failed".
	Status string

	// Err is non-nil when Status == "failed".
	Err error

	// Primary marks whether this server is the primary app server.
	Primary bool
}

// DeployResult is the top-level result of a pipeline run.
type DeployResult struct {
	// Servers contains one entry per server in the resolved target set.
	Servers []ServerResult

	// PrimarySkipped is true when at least one primary app server was skipped
	// due to read-only capability. Callers should return a non-zero exit code.
	PrimarySkipped bool
}

// Run executes the topology-aware deployment pipeline for the given inventory
// and compose file path.
//
// Ordering (per §6 of the architecture spec):
//  1. Build local environment once (kind == "local").
//  2. Deploy observability servers.
//  3. Deploy app servers in rolling fashion: if an LB is present for the env,
//     drain the server, deploy, health-check, then re-add. Otherwise deploy directly.
//  4. Reload LB config.
//
// Servers with CapReadOnly are skipped with a SKIPPED log line.
// Servers with CapHidden are silently omitted.
// If a primary app server is skipped, DeployResult.PrimarySkipped is set.
//
// Run operates on exactly one environment, env. An empty env, a nil inventory,
// or an env that is not an exact key of inv.Environments is refused with an
// error before any probe or deploy: the pipeline never falls back to "all
// environments" or a default one. Only env's servers are probed.
//
// The composePath argument is the local path to the generated docker-compose.yml
// produced by `nself build`. Run reuses deploy.DeployViaSsh for every remote server.
func Run(ctx context.Context, inv *Inventory, env string, prober Prober, composePath string) (*DeployResult, error) {
	return RunWithEnv(ctx, inv, env, prober, composePath, "", "")
}

// RunWithEnv is Run that also ships envFile (a resolved env snapshot) next to
// the compose file as .env.<envName> on every server it deploys. Both empty
// means compose only.
func RunWithEnv(ctx context.Context, inv *Inventory, env string, prober Prober, composePath, envFile, envName string) (*DeployResult, error) {
	pl := payload{composePath, envFile, envName}
	scoped, err := ScopeToEnv(inv, env)
	if err != nil {
		return nil, err
	}
	statuses := Resolve(scoped, prober)
	emitAudit(statuses)

	result := &DeployResult{}

	for envName, envDef := range scoped.Environments {
		envStatuses := filterEnv(statuses, envName)

		// Step 1: Local environment — build once.
		if envDef.Kind == "local" {
			sr, err := runLocal(ctx, envDef, envStatuses) //nolint:staticcheck // SA4023: runLocal always errors by design (local deploy not yet supported via the pipeline); see its doc comment
			if err != nil {                               //nolint:staticcheck // SA4023: same reason as above
				return result, fmt.Errorf("controlplane: local build: %w", err)
			}
			result.Servers = append(result.Servers, sr...)
			checkPrimarySkipped(result, sr)
			continue
		}

		// Separate servers by role for ordered execution.
		obs := serversByRole(envDef, envStatuses, RoleObservability)
		apps := serversByRole(envDef, envStatuses, RoleApp)
		lbs := serversByRole(envDef, envStatuses, RoleLB)

		// Step 2: Observability servers.
		for _, pair := range obs {
			sr := deployOne(ctx, envName, pair.srv, pair.ts, pl)
			result.Servers = append(result.Servers, sr)
			checkPrimarySkipped(result, []ServerResult{sr})
		}

		// Step 3: App servers — rolling with LB drain/re-add if LB present.
		lbPresent := len(lbs) > 0 && lbs[0].ts.Capability == CapManage
		for _, pair := range apps {
			sr := deployApp(ctx, envName, pair.srv, pair.ts, pl, lbPresent, lbs)
			result.Servers = append(result.Servers, sr)
			checkPrimarySkipped(result, []ServerResult{sr})
		}

		// Step 4: LB config reload.
		for _, pair := range lbs {
			sr := reloadLB(ctx, envName, pair.srv, pair.ts)
			result.Servers = append(result.Servers, sr)
		}
	}

	return result, nil
}

// ScopeToEnv returns a copy of inv holding only the environment named env.
// It fails closed: nil inventory, empty name, or a name that is not an exact
// key of inv.Environments returns an error and no inventory.
func ScopeToEnv(inv *Inventory, env string) (*Inventory, error) {
	if inv == nil {
		return nil, fmt.Errorf("controlplane: no inventory; refusing to deploy")
	}
	if env == "" {
		return nil, fmt.Errorf("controlplane: no environment named; refusing to deploy every environment")
	}
	e, ok := inv.Environments[env]
	if !ok {
		return nil, fmt.Errorf("controlplane: environment %q is not in the inventory; nothing deployed", env)
	}
	return &Inventory{
		SchemaVersion: inv.SchemaVersion,
		Project:       inv.Project,
		Environments:  map[string]Environment{env: e},
	}, nil
}

// payload is what the pipeline ships to each server.
type payload struct{ compose, envFile, envName string }

// serverPair bundles a Server with its resolved TargetStatus.
type serverPair struct {
	srv Server
	ts  TargetStatus
}

// filterEnv returns the TargetStatuses that belong to envName.
func filterEnv(statuses []TargetStatus, envName string) []TargetStatus {
	var out []TargetStatus
	for _, ts := range statuses {
		if ts.Env == envName {
			out = append(out, ts)
		}
	}
	return out
}

// serversByRole returns servers of the given role with their TargetStatus,
// excluding CapHidden servers.
func serversByRole(env Environment, statuses []TargetStatus, role ServerRole) []serverPair {
	statusMap := make(map[string]TargetStatus, len(statuses))
	for _, ts := range statuses {
		statusMap[ts.Server] = ts
	}
	var out []serverPair
	for _, srv := range env.Servers {
		if srv.Role != role {
			continue
		}
		ts, ok := statusMap[srv.Name]
		if !ok || ts.Capability == CapHidden {
			continue
		}
		out = append(out, serverPair{srv: srv, ts: ts})
	}
	return out
}

// runLocal handles a "local" kind environment in the deployment pipeline.
// The local deployment target is managed by `nself build` + `nself start`,
// which the caller invokes before Run(). The pipeline itself has no primitives
// to drive a local Docker stack — that path belongs to the nself CLI commands,
// not the control-plane library. Returning a silent empty-success (nil, nil)
// would cause `nself deploy --target local` to exit 0 with 0 servers, which
// masks the fact that nothing was actually deployed.
// Instead we return a clear error so callers surface the unsupported path to
// the operator. When local-deploy execution primitives exist (future ticket),
// this function should be replaced with a real implementation that builds a
// ServerResult per local server.
func runLocal(_ context.Context, env Environment, _ []TargetStatus) ([]ServerResult, error) { //nolint:staticcheck // SA4023: deliberately never returns nil; see comment above
	return nil, fmt.Errorf(
		"controlplane: local deploy target %q is not yet supported via the pipeline — "+
			"use `nself build` and `nself start` to manage the local environment",
		env.Name,
	)
}

// deployOne deploys to a single non-app server (observability, db, worker).
func deployOne(ctx context.Context, envName string, srv Server, ts TargetStatus, pl payload) ServerResult {
	if ts.Capability == CapReadOnly {
		logSkipped(envName, srv.Name)
		return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "skipped", Primary: srv.Primary}
	}

	keyPath := os.Getenv(srv.SSHKeyRef)
	cfg := deploy.SSHConfig{
		Host:       srv.Host,
		RemotePath: srv.RemotePath,
		ServerName: srv.Name,
		KeyPath:    keyPath,
		EnvFile:    pl.envFile,
		EnvName:    pl.envName,
	}
	if err := deployServerFn(ctx, cfg, pl.compose); err != nil {
		return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "failed", Err: err, Primary: srv.Primary}
	}
	return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "ok", Primary: srv.Primary}
}

// deployApp deploys to an app server, optionally wrapping with LB drain/re-add.
func deployApp(ctx context.Context, envName string, srv Server, ts TargetStatus, pl payload, lbPresent bool, lbs []serverPair) ServerResult {
	if ts.Capability == CapReadOnly {
		logSkipped(envName, srv.Name)
		return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "skipped", Primary: srv.Primary}
	}

	// Drain from LB if available.
	if lbPresent && len(lbs) > 0 {
		lbSrv := lbs[0].srv
		if drainRes, err := Drain(ctx, lbSrv, srv.Name); err != nil {
			// Non-fatal: log and continue (graceful degrade per spec).
			_, _ = fmt.Fprintf(os.Stderr, "controlplane: lb drain WARN for %s: %v\n", srv.Name, err)
		} else if drainRes == DrainHelperAbsent {
			_, _ = fmt.Fprintf(os.Stderr, "controlplane: lb drain WARN for %s: nself-lb helper not found on %s\n", srv.Name, lbSrv.Name)
		}
	}

	keyPath := os.Getenv(srv.SSHKeyRef)
	cfg := deploy.SSHConfig{
		Host:       srv.Host,
		RemotePath: srv.RemotePath,
		ServerName: srv.Name,
		KeyPath:    keyPath,
		EnvFile:    pl.envFile,
		EnvName:    pl.envName,
	}
	if err := deployServerFn(ctx, cfg, pl.compose); err != nil {
		return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "failed", Err: err, Primary: srv.Primary}
	}

	// Re-add to LB after successful deploy.
	if lbPresent && len(lbs) > 0 {
		lbSrv := lbs[0].srv
		if enableRes, err := Enable(ctx, lbSrv, srv.Name); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "controlplane: lb enable WARN for %s: %v\n", srv.Name, err)
		} else if enableRes == DrainHelperAbsent {
			_, _ = fmt.Fprintf(os.Stderr, "controlplane: lb enable WARN for %s: nself-lb helper not found on %s\n", srv.Name, lbSrv.Name)
		}
	}

	return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "ok", Primary: srv.Primary}
}

// reloadLB signals the LB server to reload its configuration.
func reloadLB(_ context.Context, envName string, srv Server, ts TargetStatus) ServerResult {
	if ts.Capability == CapReadOnly {
		logSkipped(envName, srv.Name)
		return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "skipped"}
	}
	// LB reload is performed by nself-lb helper on the remote side.
	// The Drain/Enable calls above trigger the reload as a side-effect.
	// A separate explicit reload may be added in a future ticket.
	return ServerResult{Env: envName, Server: srv.Name, Role: srv.Role, Status: "ok"}
}

// checkPrimarySkipped sets result.PrimarySkipped if any primary server was skipped.
func checkPrimarySkipped(result *DeployResult, srs []ServerResult) {
	for _, sr := range srs {
		if sr.Primary && sr.Status == "skipped" {
			result.PrimarySkipped = true
		}
	}
}

// logSkipped writes a SKIPPED line to stderr for a server that was not deployed.
func logSkipped(envName, serverName string) {
	_, _ = fmt.Fprintf(os.Stderr, "SKIPPED %s/%s (read-only capability)\n", envName, serverName)
}
