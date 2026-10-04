package acme

// install.go: install a certificate as a generation and switch in one rename.
// Each target `ssl/certificates/<dir>` is a relative symlink to
// `.<dir>.gen-<n>/` (fullchain.pem + privkey.pem, 0600). A new generation is
// written and its pair checked, then one rename of a temp symlink over `<dir>`
// switches key and certificate together. The previous generation stays until
// the next successful install; every run first repairs a missing link or
// mismatched pair; the Reloader runs exactly once on the success path.

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// Reloader makes the serving process pick up installed files.
type Reloader interface {
	Reload(ctx context.Context) error
}

// NginxReloader runs `nginx -t` then `nginx -s reload` in Container (Exec
// defaults to docker.ExecCapture; no socket, no restart).
type NginxReloader struct {
	Container string
	Exec      func(ctx context.Context, container string, cmd []string) (string, string, error)
}

// Reload implements Reloader; a failing `nginx -t` means nothing was reloaded.
func (n NginxReloader) Reload(ctx context.Context) error {
	if n.Exec == nil {
		n.Exec = docker.ExecCapture
	}
	for _, cmd := range [][]string{{"nginx", "-t"}, {"nginx", "-s", "reload"}} {
		if _, stderr, err := n.Exec(ctx, n.Container, cmd); err != nil {
			return fmt.Errorf("%v in %s: %w\n%s", cmd, n.Container, err, strings.TrimSpace(stderr))
		}
	}
	return nil
}

// InstallReq is one install of one certificate into one or more targets.
// Verify (optional) runs after the reload; Fault and Exit are the test hook.
type InstallReq struct {
	SSLDir    string
	Targets   []string // dirs relative to SSLDir, e.g. certificates/api-example-org
	Cert, Key []byte
	Reloader  Reloader
	Verify    func(context.Context) error
	Fault     string
	Exit      func(int)
}

// swap records one target's switch (generation -1 = none) so it can be undone.
type swap struct {
	link      string
	prev, new int
}

func split(sslDir, target string) (link, parent, base string) {
	link = filepath.Join(sslDir, filepath.FromSlash(target))
	return link, filepath.Dir(link), filepath.Base(link)
}

func genName(base string, n int) string { return "." + base + ".gen-" + strconv.Itoa(n) }

// generations lists a target's generation numbers, ascending.
func generations(parent, base string) (ns []int) {
	ms, _ := filepath.Glob(filepath.Join(parent, "."+base+".gen-*"))
	for _, m := range ms {
		if n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(m), "."+base+".gen-")); err == nil {
			ns = append(ns, n)
		}
	}
	sort.Ints(ns)
	return ns
}

// pairOK reports whether dir holds a matching fullchain.pem and privkey.pem.
func pairOK(dir string) bool {
	c, e1 := os.ReadFile(filepath.Join(dir, "fullchain.pem"))
	k, e2 := os.ReadFile(filepath.Join(dir, "privkey.pem"))
	_, e3 := tls.X509KeyPair(c, k)
	return e1 == nil && e2 == nil && e3 == nil
}

// current returns the generation a target's symlink names, or -1.
func current(link, base string) int {
	dest, err := os.Readlink(link)
	n, nerr := strconv.Atoi(strings.TrimPrefix(dest, "."+base+".gen-"))
	if err != nil || nerr != nil || dest != genName(base, n) {
		return -1
	}
	return n
}

// point atomically makes link a relative symlink to dest.
func point(link, dest string) error {
	_ = os.Remove(link + ".tmp-link")
	if err := os.Symlink(dest, link+".tmp-link"); err != nil {
		return err
	}
	return os.Rename(link+".tmp-link", link)
}

// Repair re-links each target whose link is missing, dangling or mismatched to
// its newest valid generation; a real directory (not yet converted) is left.
func Repair(sslDir string, targets []string) error {
	for _, t := range targets {
		link, parent, base := split(sslDir, t)
		if fi, err := os.Lstat(link); err == nil && (fi.Mode()&os.ModeSymlink == 0 || current(link, base) >= 0 && pairOK(link)) {
			continue
		}
		gens := generations(parent, base)
		for i := len(gens) - 1; i >= 0; i-- {
			if pairOK(filepath.Join(parent, genName(base, gens[i]))) {
				if err := point(link, genName(base, gens[i])); err != nil {
					return fmt.Errorf("repairing %s: %w", t, err)
				}
				break
			}
		}
	}
	return nil
}

// Install writes a new generation per target, switches them, reloads, verifies
// and prunes. A failure after the switch restores every previous generation.
func Install(ctx context.Context, r InstallReq) (map[string]int, error) {
	if err := Repair(r.SSLDir, r.Targets); err != nil {
		return nil, err
	}
	var swaps []swap
	for _, t := range r.Targets {
		s, err := prepare(r, t)
		if err != nil {
			return nil, err
		}
		swaps = append(swaps, s)
	}
	if r.Fault == "after-generation-write" && r.Exit != nil {
		r.Exit(137)
	}
	for i, s := range swaps {
		if err := point(s.link, genName(filepath.Base(s.link), s.new)); err != nil {
			return nil, errors.Join(fmt.Errorf("switching %s: %w", s.link, err), undo(swaps[:i]))
		}
	}
	if err := r.Reloader.Reload(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("reload failed, previous certificates restored: %w", err), undo(swaps))
	}
	if r.Verify != nil {
		if err := r.Verify(ctx); err != nil {
			uerr := undo(swaps)
			_ = r.Reloader.Reload(ctx) // best effort: serve the restored pair
			return nil, errors.Join(fmt.Errorf("served certificate check failed, previous certificates restored: %w", err), uerr)
		}
	}
	out := map[string]int{}
	for i, s := range swaps {
		_, parent, base := split(r.SSLDir, r.Targets[i])
		for _, n := range generations(parent, base) {
			if n != s.new && n != s.prev {
				_ = os.RemoveAll(filepath.Join(parent, genName(base, n)))
			}
		}
		out[r.Targets[i]] = s.new
	}
	return out, nil
}

// prepare converts a real directory on first use, writes the next generation
// and checks its pair; it does not switch.
func prepare(r InstallReq, target string) (swap, error) {
	link, parent, base := split(r.SSLDir, target)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return swap{}, err
	}
	gens, prev := generations(parent, base), current(link, base)
	next := 0
	if len(gens) > 0 {
		next = gens[len(gens)-1] + 1
	}
	fi, err := os.Lstat(link)
	switch {
	case err == nil && fi.IsDir(): // first conversion: the real directory becomes the previous generation
		prev, next = next, next+1
		if err := os.Rename(link, filepath.Join(parent, genName(base, prev))); err != nil {
			return swap{}, fmt.Errorf("converting %s: %w", target, err)
		}
		if err := point(link, genName(base, prev)); err != nil {
			return swap{}, err
		}
	case err == nil && prev < 0:
		return swap{}, fmt.Errorf("%s is neither a directory nor an nself generation link; move it aside first", link)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return swap{}, err
	}
	dir := filepath.Join(parent, genName(base, next))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return swap{}, err
	}
	for name, data := range map[string][]byte{"fullchain.pem": r.Cert, "privkey.pem": r.Key} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return swap{}, err
		}
	}
	if !pairOK(dir) {
		_ = os.RemoveAll(dir)
		return swap{}, fmt.Errorf("issued certificate and key for %s do not match; nothing installed", target)
	}
	return swap{link: link, prev: prev, new: next}, nil
}

// undo points every swapped link back at its previous generation (or removes a
// link that had none); a failure is reported loudly.
func undo(swaps []swap) error {
	var errs []error
	for _, s := range swaps {
		if s.prev < 0 {
			errs = append(errs, os.Remove(s.link))
		} else {
			errs = append(errs, point(s.link, genName(filepath.Base(s.link), s.prev)))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("ROLLBACK FAILED, run `nself trust ssl renew --acme` to repair: %w", err)
	}
	return nil
}
