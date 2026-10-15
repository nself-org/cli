package commands

// ssl_acme_traefik_spec.go: what the traefik-config one-shot runs with and what
// it can read (P7-DEPL-25). The run spec comes from the project's compose model
// and is validated before docker sees it (the model is operator-writable and the
// ACME timer runs as root); the ACME generation directories are made readable
// for the renderer and, when the renewal runs as root, handed to the uid that
// traefik runs as.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ssl/acme"
)

// traefikImageRE is a plain image reference: no leading dash (never a docker flag), no spaces.
var traefikImageRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*(:[A-Za-z0-9._-]+)?(@sha256:[0-9a-f]{64})?$`)

// traefikWritable are the only container paths the one-shot may write.
var traefikWritable = map[string]bool{"/dynamic": true, "/static": true}

// traefikConfigSpec turns the compose model of traefik-config into a one-shot
// run that keeps the service's own hardening (read-only root, no capabilities,
// no-new-privileges) with no network (the renderer needs none) and never pulls:
// a missing image fails instead of running whatever the registry holds under
// that name. Mount sources must lie under roots (the project and the served
// stack), and only /dynamic and /static are writable.
func traefikConfigSpec(raw []byte, roots []string) (docker.RunSpec, error) {
	var doc struct {
		Services map[string]struct {
			Image       string         `json:"image"`
			Command     []string       `json:"command"`
			Entrypoint  []string       `json:"entrypoint"`
			Environment map[string]any `json:"environment"`
			Tmpfs       []string       `json:"tmpfs"`
			User        string         `json:"user"`
			ReadOnly    bool           `json:"read_only"`
			CapDrop     []string       `json:"cap_drop"`
			SecurityOpt []string       `json:"security_opt"`
			Volumes     []struct {
				Type     string `json:"type"`
				Source   string `json:"source"`
				Target   string `json:"target"`
				ReadOnly bool   `json:"read_only"`
			} `json:"volumes"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return docker.RunSpec{}, fmt.Errorf("parsing the compose model of %s: %w", traefikConfigService, err)
	}
	s, ok := doc.Services[traefikConfigService]
	if !ok || s.Image == "" || len(s.Command) == 0 {
		return docker.RunSpec{}, fmt.Errorf("the compose model has no usable %s service (is the traefik plugin installed and built?)", traefikConfigService)
	}
	if !traefikImageRE.MatchString(s.Image) {
		return docker.RunSpec{}, fmt.Errorf("%s image %q is not a plain image reference", traefikConfigService, s.Image)
	}
	if len(s.Entrypoint) > 0 || len(s.Environment) > 0 || len(s.Tmpfs) > 0 {
		return docker.RunSpec{}, fmt.Errorf("%s sets an entrypoint, environment or tmpfs, which a one-shot run does not carry over; refusing rather than rendering differently", traefikConfigService)
	}
	spec := docker.RunSpec{Image: s.Image, Args: s.Command, User: s.User, Network: "none", PullNever: true,
		ReadOnly: s.ReadOnly, CapDrop: s.CapDrop, SecurityOpt: s.SecurityOpt}
	for _, v := range s.Volumes {
		switch {
		case v.Type != "bind":
			return docker.RunSpec{}, fmt.Errorf("%s mounts %s as a %s volume; only bind mounts are supported", traefikConfigService, v.Target, v.Type)
		case !filepath.IsAbs(v.Source) || !filepath.IsAbs(v.Target) || filepath.Clean(v.Target) != v.Target || !under(v.Source, roots):
			return docker.RunSpec{}, fmt.Errorf("%s mount %s -> %s is outside the project and the served stack", traefikConfigService, v.Source, v.Target)
		case !v.ReadOnly && !traefikWritable[v.Target]:
			return docker.RunSpec{}, fmt.Errorf("%s mounts %s writable; only /dynamic and /static may be", traefikConfigService, v.Target)
		}
		spec.Mounts = append(spec.Mounts, docker.Mount{Source: v.Source, Destination: v.Target, Type: "bind", ReadOnly: v.ReadOnly})
	}
	return spec, nil
}

// under reports whether p is inside one of roots (a root itself does not count).
func under(p string, roots []string) bool {
	real := func(x string) string { // a symlink inside a root must not lead out of it
		if r, err := filepath.EvalSymlinks(x); err == nil {
			return r
		}
		return filepath.Clean(x)
	}
	p = real(p)
	for _, r := range roots {
		if rel, err := filepath.Rel(real(r), p); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// traefikEuid is the ownership-policy seam for root timer renewals.
var traefikEuid = os.Geteuid

// installAccess lets the renderer traverse generations and the proxy own keys.
// A non-root renewal keeps its own uid; root uses the ids exported by build.
func (r *acmeRun) installAccess() *acme.InstallAccess {
	if r.provider != build.ProviderTraefik {
		return nil
	}
	uid, gid := -1, -1
	if traefikEuid() == 0 {
		uid, gid = hostOwner(r.workdir)
	}
	return &acme.InstallAccess{DirMode: 0o711, ParentMode: 0o751, UID: uid, GID: gid}
}

// hostOwner reads the ids traefik runs as from .nself/compose.env (-1, -1 when absent).
func hostOwner(workdir string) (uid, gid int) {
	env, err := config.ReadEnvFile(filepath.Join(workdir, ".nself", "compose.env"))
	if err != nil {
		return -1, -1
	}
	u, e1 := strconv.Atoi(env["NSELF_HOST_UID"])
	g, e2 := strconv.Atoi(env["NSELF_HOST_GID"])
	if e1 != nil || e2 != nil || u < 0 || g < 0 {
		return -1, -1
	}
	return u, g
}
