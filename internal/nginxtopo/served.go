package nginxtopo

// served.go — the one answer to "which directory tree does the running nginx
// actually serve for this project?".
//
// Purpose: a project with NGINX_FRONTED_BY set generates no nginx container
// of its own; the fronting stack's nginx reads nginx/ (confs) and ssl/
// (certificates) from the fronting stack's own directory. Production is this
// layout (/opt/nself-web/backend is served from /opt/nself-web, D-0045,
// D-0121). ServedRoot, ServedNginxDir and ServedSSLDir generalise the
// convention internal/build.resolveNginxSitesDir and the SEC-HARDENING-06
// doctor check already trust, so TLS code (doctor, ACME, ssl commands) shares
// one resolver instead of each inventing a path.
// Inputs: projectDir (the project's resolved nSelf root) and frontedBy
// (NGINX_FRONTED_BY; empty when the project runs its own nginx).
// Outputs: an absolute-or-relative directory in the same form as projectDir,
// or an error wrapping ErrFrontingUnresolved.
// Constraints: exactly two topologies exist: own (frontedBy empty) and
// fronted (ResolveFrontingDir confirms the layout). Nothing here guesses a
// directory when the fronted layout is unconfirmed. Standard library only (L0).

import (
	"errors"
	"fmt"
	"path/filepath"
)

// ErrFrontingUnresolved is returned (wrapped) when NGINX_FRONTED_BY is set but
// the project is not laid out directly under a directory named after the
// fronting stack, so the served stack cannot be located. Match with errors.Is.
var ErrFrontingUnresolved = errors.New("cannot resolve the served nginx stack")

// ServedRoot returns the project directory of the stack whose nginx serves
// this project: projectDir itself when frontedBy is empty, otherwise the
// fronting stack's directory as confirmed by ResolveFrontingDir. An
// unconfirmed layout returns "" and an error wrapping ErrFrontingUnresolved.
func ServedRoot(projectDir, frontedBy string) (string, error) {
	// Clean first: ResolveFrontingDir compares path elements, so a trailing
	// slash or "." segment must not make a confirmed layout look unconfirmed.
	if projectDir != "" {
		projectDir = filepath.Clean(projectDir)
	}
	if frontedBy == "" {
		return projectDir, nil
	}
	root, ok := ResolveFrontingDir(projectDir, frontedBy)
	if !ok {
		// Paths use %s, not %q: %q doubles backslashes on Windows and breaks
		// callers that look for the literal path inside the message.
		return "", fmt.Errorf("%w: NGINX_FRONTED_BY=%q but %s is not directly under a directory named %q; "+
			"lay the project out as \"backend\" under %s's own directory, or unset NGINX_FRONTED_BY",
			ErrFrontingUnresolved, frontedBy, projectDir, frontedBy, frontedBy)
	}
	return root, nil
}

// ServedNginxDir returns the served stack's nginx directory (the parent of
// sites/ and conf.d/): <ServedRoot>/nginx.
func ServedNginxDir(projectDir, frontedBy string) (string, error) {
	root, err := ServedRoot(projectDir, frontedBy)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "nginx"), nil
}

// ServedSSLDir returns the served stack's ssl directory (the one mounted at
// /etc/nginx/ssl): <ServedRoot>/ssl.
func ServedSSLDir(projectDir, frontedBy string) (string, error) {
	root, err := ServedRoot(projectDir, frontedBy)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ssl"), nil
}
