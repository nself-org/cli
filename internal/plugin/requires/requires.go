// Package requires checks a plugin's requires.postgres_extensions against the
// project's Postgres before the plugin is downloaded (P7-ADOPT-06).
//
// Purpose: a plugin such as claw needs the `vector` extension; on a cluster
// without it the install used to fail deep inside the plugin's boot migrations.
// Check refuses up front with E507 and the fix, installing nothing.
// Inputs: the project config, the extension names from the manifest, a Probe
// (nil selects the Docker probe). Outputs: nil, or an *errs.CLIError E507. A
// custom image whose contents are unknown yields only an E508 warning on
// Stderr. A database that is running but cannot be queried is a plain error,
// never a false "missing".
// Constraints: read-only (never creates or enables an extension); extension
// names are validated against nameRE and are never placed in SQL: the probe
// lists pg_available_extensions and the comparison happens here.
package requires

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// nameRE is the Constitution 7.3a extension-name shape.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// DefaultProbe replaces the Docker probe when Check gets a nil Probe. It is a
// test seam and stays nil in production.
var DefaultProbe Probe

// Stderr receives the E508 warning. Tests replace it.
var Stderr io.Writer = os.Stderr

// Probe reports on the live cluster.
type Probe interface {
	// Running reports whether the project's postgres container is running.
	Running(ctx context.Context) (bool, error)
	// Available returns the extension names the running server can create.
	Available(ctx context.Context) (map[string]bool, error)
}

// Check returns nil when every extension in exts is available, or an E507
// error naming each missing one. An empty exts returns nil without a probe.
func Check(ctx context.Context, cfg *config.Config, exts []string, probe Probe) error {
	want, err := normalise(exts)
	if err != nil || len(want) == 0 {
		return err
	}
	if probe == nil {
		if probe = DefaultProbe; probe == nil {
			probe = newDockerProbe(cfg)
		}
	}
	running, err := probe.Running(ctx)
	if err != nil {
		return fmt.Errorf("cannot tell whether the postgres container is running (needed to check required extensions %s): %w",
			strings.Join(want, ", "), err)
	}
	if running {
		have, err := probe.Available(ctx)
		if err != nil {
			return fmt.Errorf("postgres is running but its available extensions could not be listed (required: %s): %w; check `nself status` and retry",
				strings.Join(want, ", "), err)
		}
		return refuse(missingFrom(want, have), "the running postgres cluster")
	}
	img := compose.ResolvePostgresImage(cfg.Postgres)
	provides, known := imageProvides(img)
	if !known {
		_, _ = fmt.Fprintf(Stderr, "warning [E508]: cannot tell whether image %q ships %s; install continues. Start the stack to have the cluster checked, or use an image that ships them.\n",
			img, strings.Join(want, ", "))
		return nil
	}
	return refuse(missingFrom(want, provides), fmt.Sprintf("the configured postgres image %s (postgres is not running)", img))
}

// normalise validates, lowercases-by-rule (no change) and de-duplicates names.
func normalise(exts []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.TrimSpace(e)
		if !nameRE.MatchString(e) {
			return nil, fmt.Errorf("invalid postgres extension name %q in plugin requires.postgres_extensions (want %s)", e, nameRE)
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out, nil
}

func missingFrom(want []string, have map[string]bool) []string {
	var miss []string
	for _, e := range want {
		if !have[e] {
			miss = append(miss, e)
		}
	}
	return miss
}

// refuse builds the E507 error, or nil when nothing is missing.
func refuse(missing []string, where string) error {
	if len(missing) == 0 {
		return nil
	}
	e := errs.Newf("E507", "required Postgres extension %s not available on %s",
		strings.Join(missing, ", "), where)
	fix := "Use a Postgres image that ships " + strings.Join(missing, ", ") + " (set POSTGRES_IMAGE), then rebuild and restart."
	for _, m := range missing {
		if m == "vector" {
			fix = "Run `nself db image switch --to pgvector` to move this project to the pgvector image, then retry the install."
			break
		}
	}
	e.Fix = fix
	return e
}
