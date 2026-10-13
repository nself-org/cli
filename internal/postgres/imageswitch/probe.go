package imageswitch

// probe.go — one run of the target image to learn what the restore will find.
//
// Purpose: before anything is stopped, learn the free space of the docker data
// root (a scratch volume's filesystem), the extensions and the locales the target
// image ships. A database that uses an extension or a locale the image lacks
// would fail the restore after the stack was already down.
// Inputs: the target image. Outputs: a targetProbe. Constraints: no network, no
// capabilities; the scratch volume is removed.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// probeScript prints three sections: df of the scratch volume, extension control files, locales.
const probeScript = `df -Pk /probe | tail -1; echo ---EXT; ls "$(pg_config --sharedir)/extension" | sed -n 's/\.control$//p'; echo ---LOC; locale -a`

// targetProbe is what the probe learned.
type targetProbe struct {
	freeKB     int64
	extensions map[string]bool
	locales    map[string]bool
}

// hasLocale reports whether a database locale name is available. C and POSIX
// always are; the spelling of the codeset (utf8, UTF-8) does not matter.
func (p *targetProbe) hasLocale(name string) bool {
	n := normLocale(name)
	return n == "c" || n == "posix" || p.locales[n]
}

// normLocale lowercases a locale name and removes "-" so en_US.UTF-8 equals en_US.utf8.
func normLocale(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "")
}

// probeTarget runs probeScript in the target image on a scratch volume.
func (s *Switch) probeTarget(ctx context.Context, image string) (*targetProbe, error) {
	scratch := s.oneShot("probe") + "-vol"
	spec := docker.RunSpec{
		Image: image, Name: s.oneShot("probe"), Args: []string{"sh", "-c", probeScript},
		Mounts: []docker.Mount{{Source: scratch, Destination: "/probe"}}, Network: "none",
		CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
	}
	out, _, err := s.d.Runtime.RunOneShot(ctx, spec)
	_ = s.d.Runtime.VolumeRemove(ctx, scratch)
	if err != nil {
		return nil, err
	}
	return parseProbe(out)
}

// parseProbe reads the probe output.
func parseProbe(out string) (*targetProbe, error) {
	p := &targetProbe{extensions: map[string]bool{}, locales: map[string]bool{}}
	section, df := "df", ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "---EXT":
			section = "ext"
		case line == "---LOC":
			section = "loc"
		case line == "":
		case section == "df":
			df = line
		case section == "ext":
			p.extensions[line] = true
		default:
			p.locales[normLocale(line)] = true
		}
	}
	f := strings.Fields(df)
	if len(f) < 4 {
		return nil, fmt.Errorf("unreadable df output %q", df)
	}
	kb, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("unreadable df output %q", df)
	}
	p.freeKB = kb
	return p, nil
}
