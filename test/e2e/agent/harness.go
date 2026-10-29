//go:build e2eagent

package agent

// Purpose: the harness side of the replay: the two loopback sim hosts (qa tier
// local-servers, prod tier prod) and every harness action (fixture, git
// commit, service stop, docker volume and image-digest probes). Each action is
// a `role: harness` transcript record; none of them is an agent step.
// Inputs: a *testing.T, the Sandbox, the replay's harness Steps.
// Outputs: transcript records; an error when an action or a probe fails.
// Constraints: docker and git run from argv slices (no host shell); the sim
// hosts get recording docker/nself stubs, as deploy-sim.yml's tests do; the
// harness never calls an nself command on the agent's behalf.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
)

// SimRemotePath is the project path on each sim host.
const SimRemotePath = "/tmp/nself-agent-e2e/app"

// SimHarness owns the fleet and the sandbox for one replay.
type SimHarness struct {
	T     *testing.T
	Fleet *simharness.Fleet
	Box   *Sandbox
	Hosts []SimHost
	// before holds the image sets of the sim hosts captured by digest-before.
	before map[string]string
}

// StartSim starts qa-1 and prod-1, installs the stubs and returns the hosts.
func StartSim(t *testing.T) (*simharness.Fleet, []SimHost) {
	t.Helper()
	fleet := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "qa-1"}, {Name: "prod-1"}}})
	hosts := []SimHost{
		{Env: "qa", Server: "qa-1", Tier: controlplane.TierLocalServers},
		{Env: "prod", Server: "prod-1", Tier: controlplane.TierProd},
	}
	for i := range hosts {
		n := fleet.Node(t, hosts[i].Server)
		hosts[i].User, hosts[i].Port, hosts[i].RemotePath = n.User, n.Port, SimRemotePath
		fleet.Exec(t, n.Name, "sh", "-c", "command -v rsync >/dev/null || apk add --no-cache rsync >/dev/null")
		for _, name := range []string{"docker", "nself"} {
			fleet.CopyTo(t, n.Name, filepath.Join("testdata", "sim", name+".sh"), "/usr/local/bin/"+name)
			fleet.Exec(t, n.Name, "chmod", "755", "/usr/local/bin/"+name)
		}
		fleet.Exec(t, n.Name, "sh", "-c", "mkdir -p "+SimRemotePath+" /tmp/agent-e2e && chown -R "+n.User+" /tmp/nself-agent-e2e /tmp/agent-e2e && chmod 1777 /tmp")
	}
	return fleet, hosts
}

// Do runs one harness step and writes its record.
func (h *SimHarness) Do(ctx context.Context, s Step, w *Writer) error {
	note := s.Action
	if s.Note != "" {
		note += ": " + s.Note
	}
	switch s.Action {
	case "fixture-marker":
		// Gap: `nself mcp` (v1.5) will not start outside a project, so an
		// MCP-only agent cannot reach `init` without a project marker.
		if err := os.WriteFile(filepath.Join(h.Box.Project, ".env"), nil, 0o600); err != nil {
			return err
		}
	case "git-commit":
		if _, err := run(ctx, h.Box.Project, "git", "add", "-A"); err != nil {
			return err
		}
		if _, err := run(ctx, h.Box.Project, "git", "-c", "user.name=agent-e2e", "-c", "user.email=agent-e2e@localhost",
			"commit", "-q", "--no-gpg-sign", "-m", "agent e2e: "+s.ID); err != nil {
			return err
		}
	case NoteStopService:
		name, err := h.serviceContainer(ctx)
		if err != nil {
			return err
		}
		if _, err := run(ctx, "", "docker", "stop", name); err != nil {
			return err
		}
		note = NoteStopService + ": " + name
	case "volumes-before", "volumes-after":
		out, err := run(ctx, "", "docker", "volume", "ls", "--format", "{{.Name}}")
		if err != nil {
			return err
		}
		vols := strings.Fields(out)
		sort.Strings(vols)
		body, _ := json.Marshal(map[string]any{"volumes": vols})
		probe := map[string]string{"volumes-before": NoteVolumesBefore, "volumes-after": NoteVolumesAfter}[s.Action]
		_, err = w.Write(Record{Role: RoleHarness, Kind: KindProbe, Envelope: body, Note: probe})
		return err
	case "digest-before", "digest-compare":
		return h.digests(s.Action, w)
	default:
		return fmt.Errorf("unknown harness action %q", s.Action)
	}
	_, err := w.Write(Record{Role: RoleHarness, Kind: KindAction, Note: note})
	return err
}

// digests reads the image set each sim host runs (the docker stub's record of
// `compose up`) and, on compare, wants prod == qa as captured before the
// promote and qa unchanged (DEPL D16 compare).
func (h *SimHarness) digests(action string, w *Writer) error {
	now := map[string]string{}
	for _, host := range h.Hosts {
		now[host.Env] = h.Fleet.Exec(h.T, host.Server, "sh", "-c", "cat /tmp/agent-e2e/running 2>/dev/null || true")
	}
	body, _ := json.Marshal(now)
	if _, err := w.Write(Record{Role: RoleHarness, Kind: KindProbe, Envelope: body, Note: action}); err != nil {
		return err
	}
	if action == "digest-before" {
		h.before = now
		return nil
	}
	switch {
	case strings.TrimSpace(h.before["qa"]) == "":
		return fmt.Errorf("qa ran no images before the promote")
	case now["qa"] != h.before["qa"]:
		return fmt.Errorf("the promote changed the qa host")
	case now["prod"] != h.before["qa"]:
		return fmt.Errorf("prod does not run the qa images by digest:\nqa:   %s\nprod: %s", h.before["qa"], now["prod"])
	}
	return nil
}

// serviceContainer picks one running service container of the project
// (hasura when present, else the first by name).
func (h *SimHarness) serviceContainer(ctx context.Context) (string, error) {
	dir, err := filepath.EvalSymlinks(h.Box.Project)
	if err != nil {
		return "", err
	}
	out, err := run(ctx, "", "docker", "ps", "--format", "{{.Names}}",
		"--filter", "label=com.docker.compose.project.working_dir="+dir)
	if err != nil {
		return "", err
	}
	names := strings.Fields(out)
	sort.Strings(names)
	if len(names) == 0 {
		return "", fmt.Errorf("no running container of the project %s", dir)
	}
	for _, n := range names {
		if strings.Contains(n, "hasura") {
			return n, nil
		}
	}
	return names[0], nil
}

func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out)
	}
	return string(out), nil
}
