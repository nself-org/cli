package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// ConnectOpts holds all parameters for an admin remote connection.
type ConnectOpts struct {
	Host       string
	User       string
	SSHPort    int
	LocalPort  int
	RemotePort int
	// AllProjects opens the switcher with every registered project.
	AllProjects bool
	// AsUser overrides the authenticated identity (for ACL testing).
	AsUser string
}

// VerifySSHKey checks that key-based SSH auth works for the given host.
// Returns nil on success, an error describing the failure otherwise.
func VerifySSHKey(ctx context.Context, user, host string, port int) error {
	sshTail, err := adminSSHArgs(ctx, user, host, port)
	if err != nil {
		return err
	}
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=30",
	}
	args = append(args, sshTail...)
	args = append(args, "true")
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SSH key auth failed for %s@%s:%d: %w", user, host, port, err)
	}
	return nil
}

// EnsureRemoteAdmin starts nself-admin on the remote host if it is not
// already running, via systemctl --user.
func EnsureRemoteAdmin(ctx context.Context, user, host string, port int) error {
	sshTail, err := adminSSHArgs(ctx, user, host, port)
	if err != nil {
		return err
	}
	args := append([]string{"-o", "ServerAliveInterval=30"}, sshTail...)
	args = append(args, "systemctl --user start nself-admin || true")
	sshCmd := exec.CommandContext(ctx, "ssh", args...)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	return sshCmd.Run()
}

// NewSessionToken generates a cryptographically random session token.
func NewSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// OpenTunnel starts an SSH tunnel: -L localPort:127.0.0.1:remotePort.
// It returns the started exec.Cmd so the caller can wait on it or kill it.
func OpenTunnel(ctx context.Context, opts ConnectOpts) (*exec.Cmd, error) {
	sshTail, err := adminSSHArgs(ctx, opts.User, opts.Host, opts.SSHPort)
	if err != nil {
		return nil, err
	}
	forward := fmt.Sprintf("%d:127.0.0.1:%d", opts.LocalPort, opts.RemotePort)
	args := []string{
		"-N",
		"-o", "ServerAliveInterval=30",
		"-o", "ExitOnForwardFailure=yes",
		"-L", forward,
	}
	args = append(args, sshTail...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ssh tunnel: %w", err)
	}
	return cmd, nil
}

func adminSSHArgs(ctx context.Context, user, host string, port int) ([]string, error) {
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return nil, err
	}
	if spec.User != "" && spec.User != user {
		return nil, fmt.Errorf("admin SSH user conflicts with host specification")
	}
	if spec.Port != 0 && port != 0 && spec.Port != port {
		return nil, fmt.Errorf("admin SSH port conflicts with host specification")
	}
	if user != "" {
		spec.User = user
	}
	if port != 0 {
		spec.Port = port
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	var args []string
	// compat.V15(P7-DEPL-14): unpinned admin SSH -> pinned host-key policy.
	if compat.V15() {
		policy, err := adminHostKeyOptions(ctx, spec)
		if err != nil {
			return nil, err
		}
		args = append(args, policy...)
	}
	return append(args, spec.SSHArgs()...), nil
}

// adminHostKeyOptions validates the live key against operator or nSelf pins.
func adminHostKeyOptions(ctx context.Context, spec remote.HostSpec) ([]string, error) {
	// Port 22 is the default: OpenSSH and the nSelf pin file store it under the
	// bare host name, never [host]:22, so key lookups treat it like no port.
	if spec.Port == 22 {
		spec.Port = 0
	}
	port := spec.Port
	if port == 0 {
		port = 22
	}
	keys, err := remote.ScanHostKeys(ctx, spec.Host, port)
	if err != nil {
		return nil, errs.New("E487", fmt.Sprintf("host key scan for %s failed: %v", spec.Host, err))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	operatorAlias := spec.Host
	pinAlias := spec.Host
	if spec.Port != 0 {
		operatorAlias = "[" + spec.Host + "]:" + strconv.Itoa(spec.Port)
		pinAlias = "nself-" + strings.ReplaceAll(spec.Host, ":", "-") + "-" + strconv.Itoa(spec.Port)
	}
	pinFile := filepath.Join(home, ".config", "nself", "deploy_known_hosts")
	operator := adminTrustedFingerprints(ctx, operatorAlias, filepath.Join(home, ".ssh", "known_hosts"))
	pinned := adminTrustedFingerprints(ctx, pinAlias, pinFile)
	for _, key := range keys {
		if operator[key.Fingerprint] || pinned[key.Fingerprint] {
			opts := []string{"-o", "StrictHostKeyChecking=yes", "-o", "GlobalKnownHostsFile=" + pinFile}
			if spec.Port != 0 && !operator[key.Fingerprint] {
				opts = append(opts, "-o", "HostKeyAlias="+pinAlias)
			}
			return opts, nil
		}
	}
	fingerprint := keys[0].Fingerprint
	return nil, errs.New("E487", fmt.Sprintf("unknown host key %s for %s; verify it, then run nself env target add <env> <server> --trust-host-key %s", fingerprint, spec.String(), fingerprint))
}

func adminTrustedFingerprints(ctx context.Context, alias, path string) map[string]bool {
	seen := map[string]bool{}
	out, err := exec.CommandContext(ctx, "ssh-keygen", "-F", alias, "-f", path).Output()
	if err != nil {
		return seen
	}
	for _, line := range strings.Split(string(out), "\n") {
		if fingerprint, err := remote.Fingerprint(line); err == nil {
			seen[fingerprint] = true
		}
	}
	return seen
}

// OpenBrowser opens the admin URL in the user's default browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// AdminURL builds the admin URL without embedding the session token.
// The token is delivered via BootstrapSession (POST /auth/bootstrap) before the
// browser is opened, so the URL itself never carries a credential.
func AdminURL(localPort int, project string) string {
	u := fmt.Sprintf("http://localhost:%d", localPort)
	if project != "" {
		u += "?project=" + project
	}
	return u
}

// bootstrapPayload is the JSON body sent to /auth/bootstrap.
type bootstrapPayload struct {
	Token string `json:"token"`
}

// BootstrapSession delivers the session token to the admin server via a
// localhost-only POST to /auth/bootstrap. The server stores the token in memory
// and responds with an HttpOnly session cookie. Call this BEFORE OpenBrowser so
// the browser already has the cookie when it loads the admin UI.
func BootstrapSession(localPort int, token string) error {
	payload, err := json.Marshal(bootstrapPayload{Token: token})
	if err != nil {
		return fmt.Errorf("bootstrap session: marshal payload: %w", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/api/auth/bootstrap", localPort)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("bootstrap session: POST %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bootstrap session: server returned %d", resp.StatusCode)
	}
	return nil
}
