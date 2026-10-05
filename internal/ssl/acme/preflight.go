package acme

// preflight.go: what the CLI resolves before it writes anything (also under
// --dry-run): served dirs, the nginx container, the whole-dir ssl mount (a
// per-certificate mount cannot follow generation symlinks), `age`, the age key
// (path passed in: this package never imports internal/secrets) and the
// contact. A failure is an *Error (What + Fix) that callers map to E151.

import (
	"context"
	"errors"
	"fmt"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/nginxtopo"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Error is a refusal with its remediation.
type Error struct{ What, Fix string }

func (e *Error) Error() string { return e.What }

// Refuse builds an *Error from a remediation and a formatted message.
func Refuse(fix, format string, a ...any) error {
	return &Error{What: fmt.Sprintf(format, a...), Fix: fix}
}

// Input is what Resolve needs; the func fields default to the docker funnel and exec.LookPath.
type Input struct {
	ProjectDir, FrontedBy, ProjectName, NginxContainer, Contact, AgeKeyPath string
	// NoSecrets is set for runs that handle no DNS credential (HTTP-01): `age` and the age key are not required.
	NoSecrets bool
	FindNginx func(context.Context, docker.ServiceMatch) (string, error)
	Mounts    func(context.Context, string) ([]docker.Mount, error)
	LookPath  func(string) (string, error)
}

// Resolution is the printed answer to "where does this write, who reloads".
type Resolution struct{ Root, SSLDir, NginxDir, Container, Contact, AgeBin, AgeKey string }

// Resolve runs the preflight list in order and returns the first failure.
func Resolve(ctx context.Context, in Input) (*Resolution, error) {
	if in.FindNginx == nil {
		in.FindNginx = docker.FindServiceContainer
	}
	if in.Mounts == nil {
		in.Mounts = docker.ContainerMounts
	}
	if in.LookPath == nil {
		in.LookPath = exec.LookPath
	}
	r := &Resolution{Contact: strings.TrimSpace(in.Contact), AgeKey: in.AgeKeyPath, Container: in.NginxContainer}
	var err error
	if r.Root, err = nginxtopo.ServedRoot(in.ProjectDir, in.FrontedBy); err != nil {
		return nil, Refuse("lay the project out under the fronting stack's directory or unset NGINX_FRONTED_BY", "%v", err)
	}
	r.SSLDir, r.NginxDir = filepath.Join(r.Root, "ssl"), filepath.Join(r.Root, "nginx")
	if st, e := os.Stat(r.SSLDir); e != nil || !st.IsDir() {
		return nil, Refuse("run `nself build` first", "served ssl dir %s does not exist", r.SSLDir)
	}
	if r.Container == "" {
		r.Container, err = in.FindNginx(ctx, docker.ServiceMatch{Service: "nginx", WorkingDirs: []string{r.Root, in.ProjectDir}, Project: in.ProjectName})
		if errors.Is(err, docker.ErrServiceContainerNotFound) || errors.Is(err, docker.ErrServiceContainerAmbiguous) {
			return nil, Refuse("start the stack, or name the container with --nginx-container", "%v", err)
		} else if err != nil {
			return nil, Refuse("check that docker is running", "looking for the nginx container: %v", err)
		}
	}
	mounts, err := in.Mounts(ctx, r.Container)
	if err != nil {
		return nil, Refuse("check that docker is running", "reading mounts of %s: %v", r.Container, err)
	}
	if bad := mountProblem(mounts, r.SSLDir); bad != "" {
		return nil, Refuse("mount the whole ssl directory at "+nginxtopo.NginxSSLContainerPath, "nginx container %s: %s", r.Container, bad)
	}
	if in.NoSecrets {
		r.AgeKey = ""
	} else {
		if r.AgeBin, err = in.LookPath("age"); err != nil {
			return nil, Refuse("install age (https://age-encryption.org); it protects the DNS credentials", "`age` is not on PATH")
		}
		f, oerr := os.Open(in.AgeKeyPath)
		if oerr != nil {
			return nil, Refuse("run `nself secrets init` or set SECRETS_AGE_KEY_PATH", "age key %s is not readable", in.AgeKeyPath)
		}
		_ = f.Close()
	}
	if r.Contact == "" {
		return nil, Refuse("pass --email or set ACME_EMAIL or ADMIN_EMAIL", "no ACME contact email")
	}
	return r, nil
}

// mountProblem returns "" when sslDir is mounted whole at the nginx ssl path
// and nothing is mounted beneath it (a nested mount shadows the whole-dir one
// and pins the old inode), else a description naming the mounts in the way.
func mountProblem(mounts []docker.Mount, sslDir string) string {
	const dst = nginxtopo.NginxSSLContainerPath
	var seen []string
	whole := false
	for _, m := range mounts {
		d := path.Clean(m.Destination)
		ra, ea := filepath.EvalSymlinks(m.Source)
		rb, eb := filepath.EvalSymlinks(sslDir)
		if d == dst && (filepath.Clean(m.Source) == filepath.Clean(sslDir) || ea == nil && eb == nil && ra == rb) {
			whole = true
		} else if d == dst || strings.HasPrefix(d, dst+"/") {
			seen = append(seen, m.Source+" -> "+d)
		}
	}
	switch {
	case whole && len(seen) == 0:
		return ""
	case len(seen) == 0:
		return "has no mount at " + dst
	}
	return fmt.Sprintf("mounts %s in the way of the whole %s at %s", strings.Join(seen, ", "), sslDir, dst)
}

// Print writes the resolution, one fact per line.
func (r *Resolution) Print(w io.Writer) {
	_, _ = fmt.Fprintf(w, "served root:       %s\nserved ssl dir:    %s\nserved nginx dir:  %s\n", r.Root, r.SSLDir, r.NginxDir)
	_, _ = fmt.Fprintf(w, "nginx container:   %s\nnginx ssl mount:   %s -> %s (whole dir)\nacme contact:      %s\n",
		r.Container, r.SSLDir, nginxtopo.NginxSSLContainerPath, r.Contact)
	if r.AgeBin != "" {
		_, _ = fmt.Fprintf(w, "age:               %s (key %s)\n", r.AgeBin, r.AgeKey)
	}
}
