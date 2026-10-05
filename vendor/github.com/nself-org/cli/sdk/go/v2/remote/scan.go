// Purpose: capture and fingerprint a host's public keys without trusting them.
// Inputs:  a host and port (ssh-keyscan), or a Target and node id (ssh itself,
//          so ProxyJump/ProxyCommand and ssh_config are honoured).
// Outputs: []HostKey for the operator to confirm before PinnedHostKeys.Add.
// Constraints: argv only. Scanning never pins; the caller confirms the
//              fingerprint first. ScanHostKeysSSH authenticates nothing.

package remote

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HostKey is one captured public key.
type HostKey struct {
	// Line is the known_hosts line as captured ("<host> <type> <base64>").
	Line string
	// Type is the key algorithm, e.g. ssh-ed25519.
	Type string
	// Fingerprint is "SHA256:<unpadded base64>", the OpenSSH form.
	Fingerprint string
}

// Fingerprint returns the OpenSSH SHA256 fingerprint of a key line
// ("<type> <base64>" or "<host> <type> <base64>").
func Fingerprint(keyLine string) (string, error) {
	_, b64, err := parseKeyLine(keyLine)
	if err != nil {
		return "", err
	}
	blob, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

func parseHostKeys(raw string) []HostKey {
	var keys []HostKey
	for _, l := range strings.Split(raw, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		typ, _, err := parseKeyLine(l)
		if err != nil {
			continue
		}
		fp, err := Fingerprint(l)
		if err != nil {
			continue
		}
		keys = append(keys, HostKey{Line: l, Type: typ, Fingerprint: fp})
	}
	return keys
}

// ScanHostKeys runs `ssh-keyscan -T 5 -p <port> -- <host>` and returns the
// keys the host presented. It does not follow ssh_config; use ScanHostKeysSSH
// for nodes behind a jump host.
func ScanHostKeys(ctx context.Context, host string, port int) ([]HostKey, error) {
	if err := ValidateAlias(host); err != nil {
		return nil, err
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port %d out of range", port)
	}
	cmd, err := Command(ctx, "ssh-keyscan", "-T", "5", "-p", strconv.Itoa(port), "--", host)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ssh-keyscan %s:%d: %w", host, port, err)
	}
	keys := parseHostKeys(string(out))
	if len(keys) == 0 {
		return nil, fmt.Errorf("ssh-keyscan %s:%d returned no host keys", host, port)
	}
	return keys, nil
}

// ScanHostKeysSSH captures the host key through ssh itself so ProxyJump,
// ProxyCommand and the rest of ssh_config apply. It runs the CI option set
// except StrictHostKeyChecking=accept-new and a temporary UserKnownHostsFile,
// with every authentication method off and the command `true`: key exchange
// completes, authentication fails by design (exit 255), and the key is read
// from the temporary file. Only t.Dest is used; the jump host is verified by
// the operator's own configuration.
func ScanHostKeysSSH(ctx context.Context, t Target, nodeID string) ([]HostKey, error) {
	if err := ValidateNodeID(nodeID); err != nil {
		return nil, err
	}
	if err := ValidateDest(t.Dest); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ver, _ := SSHVersion(ctx) // unknown version: omit KnownHostsCommand=none
	dir, err := os.MkdirTemp("", "nself-hostkey-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	tmp := filepath.Join(dir, "known_hosts")

	args := CISSHFlags()
	pairs := CIOptions(nodeID, tmp, ver)
	for i := 1; i < len(pairs); i += 2 {
		if pairs[i] == "StrictHostKeyChecking=yes" {
			pairs[i] = "StrictHostKeyChecking=accept-new"
		}
	}
	args = append(args, pairs...)
	for _, o := range []string{
		"PreferredAuthentications=none", "PubkeyAuthentication=no", "PasswordAuthentication=no",
		"KbdInteractiveAuthentication=no", "HashKnownHosts=no",
	} {
		args = append(args, "-o", o)
	}
	args = append(args, "--", t.Dest, "true")
	cmd, err := Command(ctx, "ssh", args...)
	if err != nil {
		return nil, err
	}
	out, runErr := cmd.CombinedOutput() // exit 255 is expected
	raw, rerr := os.ReadFile(tmp)
	if rerr != nil {
		return nil, fmt.Errorf("ssh to %s captured no host key: %v\n%s", t.Dest, runErr, strings.TrimSpace(string(out)))
	}
	keys := parseHostKeys(string(raw))
	if len(keys) == 0 {
		return nil, fmt.Errorf("ssh to %s captured no host key\n%s", t.Dest, strings.TrimSpace(string(out)))
	}
	return keys, nil
}
