package deploy

// Purpose: the fronting stack's nginx container for a project fronted by another
//          stack (NGINX_FRONTED_BY, D-0425 apply side). The apply finds that
//          container on the host by compose labels, the rule of internal/docker
//          FindServiceContainer run remotely: exactly one RUNNING container with
//          com.docker.compose.service=nginx whose
//          com.docker.compose.project.working_dir is the front root as given or
//          its real path. Its ID (never its name) is kept, and the apply acts on
//          it only through `docker exec <id> nginx -t` and `nginx -s reload`.
// Inputs:  a Remote, the front root (as given and real).
// Outputs: a frontContainer; E485 errors that name every running nginx-service
//          container with its working_dir label.
// Constraints: no guessed container name, no compose project name, no compose
//              command and nothing is restarted or recreated. The ID is checked
//              against containerIDRe before use. The container must bind-mount
//              <front>/nginx or <front>/nginx/sites (given or real path). The
//              caller re-checks that the container still runs before nginx -t,
//              before the reload and before the rollback reload. Host output is
//              never trusted for display: control characters are replaced.

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/nself-org/cli/internal/errs"
)

const (
	composeServiceLabel    = "com.docker.compose.service"
	composeWorkingDirLabel = "com.docker.compose.project.working_dir"
)

// containerIDRe is what a container ID or name must match before it is used.
var containerIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// frontContainer is the resolved fronting nginx.
type frontContainer struct {
	ID, Name, WorkingDir string
}

// psRow is one running nginx-service container as `docker ps` lists it.
type psRow struct{ id, name, workingDir string }

func (p psRow) String() string {
	return fmt.Sprintf("%s (%s=%s)", safeText(p.name), composeWorkingDirLabel, safeText(p.workingDir))
}

// safeText replaces control characters of host-provided text.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}

func frontFix(front string) string {
	return fmt.Sprintf("start the fronting stack's nginx from %s (docker compose up -d nginx in that directory) so exactly one running nginx container carries that project directory, or set remote_path to a project directly under the fronting stack's directory; nothing was sent", front)
}

// resolveFrontContainer finds the one running fronting nginx for front (the root
// as given) or realFront (its real path, "" when unknown).
func resolveFrontContainer(ctx context.Context, r Remote, front, realFront string) (frontContainer, error) {
	out, err := r.Run(ctx, shScript(fmt.Sprintf(`docker ps --no-trunc --filter %s --filter status=running --format %s`,
		q("label="+composeServiceLabel+"=nginx"), q(`{{.ID}}	{{.Names}}	{{.Label "`+composeWorkingDirLabel+`"}}`))))
	if err != nil {
		return frontContainer{}, fmt.Errorf("listing the nginx containers on %s: %w", r.Label(), err)
	}
	var all, match []psRow
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(parts) != 3 || parts[0] == "" {
			continue
		}
		row := psRow{id: parts[0], name: parts[1], workingDir: parts[2]}
		all = append(all, row)
		if wd := path.Clean(row.workingDir); row.workingDir != "" && (wd == path.Clean(front) || (realFront != "" && wd == path.Clean(realFront))) {
			match = append(match, row)
		}
	}
	if len(match) != 1 {
		return frontContainer{}, errs.New("E485", fmt.Sprintf("the fronting nginx for %s must be exactly one running container with %s=nginx and %s=%s, found %d; running nginx-service containers on %s: %s",
			safeText(front), composeServiceLabel, composeWorkingDirLabel, safeText(front), len(match), r.Label(), describeRows(all))).WithFix(frontFix(safeText(front)))
	}
	c := frontContainer{ID: match[0].id, Name: match[0].name, WorkingDir: match[0].workingDir}
	if !containerIDRe.MatchString(c.ID) {
		return frontContainer{}, errs.New("E485", fmt.Sprintf("the fronting nginx container id %q is not a plain container id; nothing was sent", safeText(c.ID))).WithFix(frontFix(front))
	}
	return c, nil
}

func describeRows(rows []psRow) string {
	if len(rows) == 0 {
		return "none"
	}
	parts := make([]string, len(rows))
	for i, p := range rows {
		parts[i] = p.String()
	}
	return strings.Join(parts, ", ")
}

// checkMounts requires a bind mount of <front>/nginx or <front>/nginx/sites
// (given or real path) at /etc/nginx or below it in the container, so a file
// shipped there is the file the container reads.
func (c frontContainer) checkMounts(ctx context.Context, r Remote, front, realFront string, linked map[string]string) error {
	out, err := r.Run(ctx, shScript(fmt.Sprintf(`docker inspect --type container --format %s %s`, q(`{{range .Mounts}}{{.Type}}|{{.Source}}|{{.Destination}}{{"\n"}}{{end}}`), q(c.ID))))
	if err != nil {
		return fmt.Errorf("reading the mounts of the fronting nginx container on %s: %w", r.Label(), err)
	}
	ok := map[string]bool{}
	for _, root := range []string{front, realFront} {
		for _, d := range frontDirs {
			if root != "" {
				ok[path.Join(root, d)] = true
			}
		}
	}
	var seen, roots []string
	found := false
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) != 3 || parts[0] != "bind" {
			continue
		}
		src := path.Clean(parts[1])
		seen = append(seen, safeText(parts[1])+" -> "+safeText(parts[2]))
		if dst := path.Clean(parts[2]); dst != "/etc/nginx" && !strings.HasPrefix(dst, "/etc/nginx/") {
			continue // the container does not read nginx config from there
		}
		roots = append(roots, src)
		found = found || ok[src]
		for _, d := range frontDirs { // a mounted source that is itself a link reaches that link's real dir
			if real, l := linked[d]; l && (src == path.Join(front, d) || src == path.Join(realFront, d)) {
				roots = append(roots, real)
			}
		}
	}
	// A nginx or nginx/sites that is a symlink inside the front root reaches the
	// container only when the mount covers the link's real path.
	for _, d := range frontDirs {
		real, l := linked[d]
		if !l || !found {
			continue
		}
		covered := false
		for _, root := range roots {
			covered = covered || real == root || strings.HasPrefix(real, root+"/")
		}
		if !covered {
			return errs.New("E485", fmt.Sprintf("%s/%s is a symlink to %s, which the fronting nginx container %s (%s) does not mount, so files shipped there would not reach it (bind mounts seen: %s)",
				safeText(front), d, safeText(real), safeText(c.Name), safeText(c.ID), strings.Join(seen, ", "))).
				WithFix("mount the link's real directory into the fronting nginx container or replace the link with a real directory, then deploy again; nothing was sent")
		}
	}
	if found {
		return nil
	}
	return errs.New("E485", fmt.Sprintf("the fronting nginx container %s (%s) does not bind-mount %s/nginx or %s/nginx/sites, so files shipped there would not reach it (bind mounts seen: %s)",
		safeText(c.Name), safeText(c.ID), safeText(front), safeText(front), strings.Join(seen, ", "))).
		WithFix("mount " + safeText(front) + "/nginx (or its sites/ dir) into the fronting nginx container, then deploy again; nothing was sent")
}

// stillRunning re-reads the container by ID: it must exist and run, else E485.
func (c frontContainer) stillRunning(ctx context.Context, r Remote) error {
	if !containerIDRe.MatchString(c.ID) {
		return errs.New("E485", "the fronting nginx container id is not a plain container id")
	}
	out, err := r.Run(ctx, shScript(fmt.Sprintf(`docker inspect --type container --format %s %s`, q("{{.State.Running}}"), q(c.ID))))
	if err != nil {
		return errs.Wrap("E485", fmt.Sprintf("the fronting nginx container %s (%s) could not be inspected on %s: %v; nginx was not tested or reloaded", safeText(c.Name), safeText(c.ID), r.Label(), err), err).
			WithFix("check that docker answers on the host and the fronting stack's nginx still exists, then deploy again (or run 'nself deploy rollback' to put the files back)")
	}
	if strings.TrimSpace(string(out)) != "true" {
		return errs.New("E485", fmt.Sprintf("the fronting nginx container %s (%s) is no longer running on %s; nginx was not tested or reloaded", safeText(c.Name), safeText(c.ID), r.Label())).
			WithFix("start the fronting stack's nginx, then deploy again (or run 'nself deploy rollback' to put the files back)")
	}
	return nil
}

// nginxTest runs `nginx -t` inside the live fronting container: it tests what
// will actually be loaded, with the fronting stack's own mounts and upstreams.
func (c frontContainer) nginxTest(ctx context.Context, r Remote) error {
	if err := c.stillRunning(ctx, r); err != nil {
		return err
	}
	if out, err := r.Run(ctx, shScript(fmt.Sprintf(`docker exec %s nginx -t 2>&1`, q(c.ID)))); err != nil {
		return fmt.Errorf("nginx -t failed in the fronting container %s on %s, nothing was reloaded: %w\n%s", safeText(c.Name), r.Label(), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// nginxReload runs `nginx -s reload` in the fronting container.
func (c frontContainer) nginxReload(ctx context.Context, r Remote) error {
	if err := c.stillRunning(ctx, r); err != nil {
		return err
	}
	if out, err := r.Run(ctx, shScript(fmt.Sprintf(`docker exec %s nginx -s reload 2>&1`, q(c.ID)))); err != nil {
		return fmt.Errorf("nginx reload failed in the fronting container %s on %s: %w\n%s", safeText(c.Name), r.Label(), err, strings.TrimSpace(string(out)))
	}
	return nil
}
