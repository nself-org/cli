// Purpose: a pinned known_hosts file keyed by alias, written atomically.
// Inputs:  the file path (PinnedHostKeys.Path), an alias, key lines.
// Outputs: OpenSSH known_hosts lines "<alias> <type> <base64>".
// Constraints: touches only Path, its sidecar lock "<Path>.lock" (0600, never
//              truncated, left in place) and its parent directory (created
//              0700). Never ~/.ssh/known_hosts. Lookup matches the alias
//              field exactly; an IP literal is refused, never matched.
//              Writers (Add, Replace, Remove) hold an exclusive flock on the
//              lock file and Lookup a shared one, on top of the in-process
//              mutex; the pin file itself is not locked because write()
//              renames over it. Non-unix builds have the mutex only.
//              ssh honours every line, so any @marker, wildcard, negation,
//              list, [host]:port or hashed line is "foreign": every
//              operation errors naming the file and line, and nothing is
//              rewritten. A parent directory or pin file that is group or
//              world writable, or is not a directory / regular file, is
//              refused, never chmod-ed (mode checks skipped on windows).

package remote

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PinnedHostKeys is a known_hosts file holding one or more keys per alias.
type PinnedHostKeys struct{ Path string }

var pinMu sync.Mutex // orders goroutines; the flock (acquire) orders processes

// keyTypes are the host key algorithms OpenSSH writes to known_hosts.
var keyTypes = map[string]bool{
	"ssh-ed25519": true, "ssh-rsa": true, "ssh-dss": true,
	"ecdsa-sha2-nistp256": true, "ecdsa-sha2-nistp384": true, "ecdsa-sha2-nistp521": true,
	"sk-ssh-ed25519@openssh.com": true, "sk-ecdsa-sha2-nistp256@openssh.com": true,
}

// parseKeyLine accepts "<type> <base64>" or "<host> <type> <base64>" (with an
// optional trailing comment) and returns the type and base64 blob.
func parseKeyLine(line string) (typ, b64 string, err error) {
	f := strings.Fields(line)
	// if / else-if, not switch: Go's cover profile has no block for a case
	// expression, so the mutation tool could not see conditions written there.
	if len(f) >= 2 && keyTypes[f[0]] {
		typ, b64 = f[0], f[1]
	} else if len(f) >= 3 && keyTypes[f[1]] {
		typ, b64 = f[1], f[2]
	} else {
		return "", "", fmt.Errorf("not a host key line: %q", line)
	}
	if _, derr := base64.StdEncoding.DecodeString(b64); derr != nil {
		return "", "", fmt.Errorf("host key is not valid base64: %w", derr)
	}
	return typ, b64, nil
}

func checkPinAlias(alias string) error {
	if err := ValidateAlias(alias); err != nil {
		return err
	}
	if isIPLiteral(alias) {
		return fmt.Errorf("pinned host keys are keyed by alias, not IP address (got %q)", alias)
	}
	return nil
}

func (p PinnedHostKeys) read() ([]string, error) {
	if p.Path == "" {
		return nil, fmt.Errorf("PinnedHostKeys.Path is empty")
	}
	dir := filepath.Dir(p.Path)
	if err := checkDir(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := checkFile(p.Path); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if bad := foreignLines(string(raw)); len(bad) > 0 {
		return nil, fmt.Errorf("pinned host keys %s: %s", p.Path, strings.Join(bad, "; "))
	}
	var lines []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// write replaces the file atomically: temp file in the same directory
// (0600), fsync, rename. The parent directory is created 0700.
func (p PinnedHostKeys) write(lines []string) error {
	dir := filepath.Dir(p.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := checkDir(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".known_hosts-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	data := ""
	if len(lines) > 0 {
		data = strings.Join(lines, "\n") + "\n"
	}
	if _, err := tmp.WriteString(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p.Path)
}

// Add pins keyLine for alias. The same key twice is a no-op. A different key
// of a type already pinned for the alias is refused: that is a changed host
// key and needs Replace, a deliberate act.
func (p PinnedHostKeys) Add(alias, keyLine string) error {
	if err := checkPinAlias(alias); err != nil {
		return err
	}
	typ, b64, err := parseKeyLine(keyLine)
	if err != nil {
		return err
	}
	pinMu.Lock()
	defer pinMu.Unlock()
	unlock, err := p.acquire(true)
	if err != nil {
		return err
	}
	defer unlock()
	lines, err := p.read()
	if err != nil {
		return err
	}
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) < 3 || f[0] != alias || f[1] != typ {
			continue
		}
		if f[2] == b64 {
			return nil
		}
		return fmt.Errorf("a different %s key is already pinned for %s; use Replace to change it", typ, alias)
	}
	return p.write(append(lines, alias+" "+typ+" "+b64))
}

// Lookup returns the "<type> <base64>" keys pinned for alias (nil when none).
func (p PinnedHostKeys) Lookup(alias string) ([]string, error) {
	if err := checkPinAlias(alias); err != nil {
		return nil, err
	}
	pinMu.Lock()
	defer pinMu.Unlock()
	unlock, err := p.acquire(false)
	if err != nil {
		return nil, err
	}
	defer unlock()
	lines, err := p.read()
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, l := range lines {
		if f := strings.Fields(l); len(f) >= 3 && f[0] == alias {
			keys = append(keys, f[1]+" "+f[2])
		}
	}
	return keys, nil
}

// Remove deletes every key pinned for alias; a missing alias is not an error.
func (p PinnedHostKeys) Remove(alias string) error {
	return p.replace(alias, nil)
}

// Replace swaps every key pinned for alias for keyLines in one atomic write.
func (p PinnedHostKeys) Replace(alias string, keyLines ...string) error {
	if len(keyLines) == 0 {
		return fmt.Errorf("Replace needs at least one key; use Remove to delete")
	}
	return p.replace(alias, keyLines)
}

func (p PinnedHostKeys) replace(alias string, keyLines []string) error {
	if err := checkPinAlias(alias); err != nil {
		return err
	}
	var add []string
	for _, k := range keyLines {
		typ, b64, err := parseKeyLine(k)
		if err != nil {
			return err
		}
		add = append(add, alias+" "+typ+" "+b64)
	}
	if len(add) == 0 && p.Path != "" {
		if _, err := os.Lstat(p.Path); os.IsNotExist(err) {
			return nil // Remove with no pin file: create nothing, not even the lock
		}
	}
	pinMu.Lock()
	defer pinMu.Unlock()
	unlock, err := p.acquire(true)
	if err != nil {
		return err
	}
	defer unlock()
	lines, err := p.read()
	if err != nil {
		return err
	}
	out := make([]string, 0, len(lines)+len(add))
	for _, l := range lines {
		if f := strings.Fields(l); len(f) >= 1 && f[0] == alias {
			continue
		}
		out = append(out, l)
	}
	if len(add) == 0 && len(out) == len(lines) {
		return nil // nothing pinned for alias: do not create or rewrite the file
	}
	return p.write(append(out, add...))
}
