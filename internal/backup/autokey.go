package backup

// autokey.go: zero-config backup encryption identity (P7-PROD-08).
//
// Purpose: an encrypted backup with no recipient configured creates an age
// identity once, at ~/.config/nself/<project>-age.key (the init-key path), and
// uses its recipient. The identity is never overwritten, never printed, never
// logged and never placed in argv.
// Inputs: the project name; the NSELF_BACKUP_NO_AUTO_KEY switch; the compat mode.
// Outputs: the recipient (public) and identity path; a plain notice on stderr
// when an identity is created; E222 / E224 errors.
// Constraints: file 0600, directory 0700; creation is temp file + fsync + link,
// so two concurrent first runs end with exactly one key and the same recipient.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// NoAutoKeyEnv set to 1 keeps the fail-closed refusal when no recipient is configured.
const NoAutoKeyEnv = "NSELF_BACKUP_NO_AUTO_KEY"

// BackedUpMarkerSuffix names the empty file the owner creates next to the
// identity once it is copied somewhere other than this host. The doctor hint
// stays quiet only when it exists.
const BackedUpMarkerSuffix = ".backed-up"

// autoKeyNotice receives the creation notice; tests replace it.
var autoKeyNotice io.Writer = os.Stderr

// Identity describes an age identity file. The secret is never held here.
type Identity struct {
	Path      string
	Recipient string // public key, safe to print
	Created   bool   // true when this call created the file
}

// AutoKeyDisabled reports whether NSELF_BACKUP_NO_AUTO_KEY=1.
func AutoKeyDisabled() bool { return os.Getenv(NoAutoKeyEnv) == "1" }

// IdentityPath returns ~/.config/nself/<project>-age.key.
func IdentityPath(project string) (string, error) {
	if project == "" || project != filepath.Base(project) || project == "." || project == ".." || strings.HasPrefix(project, ".") {
		return "", errs.Newf("E222", "project name %q cannot name an identity file", project)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errs.Wrap("E222", "cannot find the home directory for the backup identity", err)
	}
	return filepath.Join(home, ".config", "nself", project+"-age.key"), nil
}

// resolveAutoRecipients applies the auto identity to a stream or create that
// has no recipient. It returns recipients unchanged unless v1.5 mode applies.
// dryRun never creates a file: an existing identity is used, otherwise a
// placeholder stands in because the real run will create one.
func resolveAutoRecipients(project string, recipients []string, optOut, dryRun bool) ([]string, error) {
	if len(recipients) > 0 || optOut {
		return recipients, nil
	}
	// compat.V15(P7-PROD-08): refuse -> auto identity
	if !compat.V15() {
		return recipients, nil
	}
	if AutoKeyDisabled() {
		return nil, errs.New("E224", "no backup recipient is configured and automatic key creation is disabled ("+NoAutoKeyEnv+"=1)")
	}
	if dryRun {
		if p, err := IdentityPath(project); err == nil {
			if id, ok, _ := readIdentity(p); ok {
				return []string{id.Recipient}, nil
			}
		}
		return []string{"<identity created on the first real run>"}, nil
	}
	id, err := EnsureIdentity(project)
	if err != nil {
		return nil, err
	}
	if id.Created {
		writeCreationNotice(autoKeyNotice, id)
	}
	return []string{id.Recipient}, nil
}

// EnsureIdentity returns the project's identity, creating it when absent.
// An existing identity is never replaced; wider-than-0600 modes are tightened.
func EnsureIdentity(project string) (Identity, error) {
	path, err := IdentityPath(project)
	if err != nil {
		return Identity{}, err
	}
	if err := ensureKeyDir(filepath.Dir(path)); err != nil {
		return Identity{}, err
	}
	if id, ok, err := readIdentity(path); err != nil {
		return Identity{}, err
	} else if ok {
		return id, nil
	}
	secret, pub, err := generateAgeKey()
	if err != nil {
		return Identity{}, err
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	created, err := publishKey(path, secret)
	if err != nil {
		return Identity{}, err
	}
	if !created { // another process won the race: use its key, drop ours
		id, ok, err := readIdentity(path)
		if err != nil || !ok {
			return Identity{}, errs.Wrap("E222", "the backup identity appeared but cannot be read: "+path, err)
		}
		return id, nil
	}
	return Identity{Path: path, Recipient: pub, Created: true}, nil
}

func ensureKeyDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errs.Wrap("E222", "cannot create the key directory "+dir, err)
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return errs.Wrap("E222", "the key directory is not a directory: "+dir, err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		slog.Warn("tightening the key directory to 0700", "dir", dir, "was", fmt.Sprintf("%04o", fi.Mode().Perm()))
		if err := os.Chmod(dir, 0o700); err != nil {
			return errs.Wrap("E222", "cannot tighten the key directory to 0700: "+dir, err)
		}
	}
	return nil
}

// readIdentity reads an existing identity. ok is false when none exists. A
// symlink or non-regular file is refused; a wider mode is tightened to 0600.
func readIdentity(path string) (Identity, bool, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, errs.Wrap("E222", "cannot inspect the backup identity "+path, err)
	}
	if !fi.Mode().IsRegular() {
		return Identity{}, false, errs.Newf("E222", "the backup identity path is not a regular file (symlinks are refused): %s", path)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		slog.Warn("tightening the backup identity to 0600", "path", path, "was", fmt.Sprintf("%04o", fi.Mode().Perm()))
		if err := os.Chmod(path, 0o600); err != nil {
			return Identity{}, false, errs.Wrap("E222", "cannot tighten the backup identity to 0600: "+path, err)
		}
	}
	// age-keygen -y prints only the public key; the secret never reaches us.
	out, err := exec.Command("age-keygen", "-y", path).Output()
	pub := strings.TrimSpace(string(out))
	if err != nil || !strings.HasPrefix(pub, "age1") {
		return Identity{}, false, errs.Newf("E222", "the existing backup identity %s is not a usable age identity; it was left untouched", path)
	}
	return Identity{Path: path, Recipient: pub}, true, nil
}

// generateAgeKey runs age-keygen with its secret captured in memory only.
func generateAgeKey() (secret []byte, pub string, err error) {
	bin, err := exec.LookPath("age-keygen")
	if err != nil {
		return nil, "", errs.Wrap("E222", "age-keygen is not installed, so no backup identity can be created (install age, or pass --recipient)", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, "", errs.Wrap("E222", "age-keygen failed: "+strings.TrimSpace(firstLine(stderr.String())), err)
	}
	secret = stdout.Bytes()
	for _, l := range strings.Split(string(secret), "\n") {
		if strings.HasPrefix(l, "# public key:") {
			pub = strings.TrimSpace(strings.TrimPrefix(l, "# public key:"))
		}
	}
	if !strings.HasPrefix(pub, "age1") || !bytes.Contains(secret, []byte("AGE-SECRET-KEY-")) {
		return nil, "", errs.New("E222", "age-keygen produced no usable key")
	}
	return secret, pub, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// publishKey writes secret to a 0600 temp file in the key directory, fsyncs
// it, and links it to path. Link fails when path exists, so an existing key is
// never replaced. created is false when path already existed.
func publishKey(path string, secret []byte) (created bool, err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, errs.Wrap("E222", "cannot create a temporary identity file in "+dir, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return false, errs.Wrap("E222", "cannot set the identity file mode to 0600", err)
	}
	if _, err := tmp.Write(secret); err != nil {
		_ = tmp.Close()
		return false, errs.Wrap("E222", "cannot write the backup identity", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, errs.Wrap("E222", "cannot flush the backup identity to disk", err)
	}
	if err := tmp.Close(); err != nil {
		return false, errs.Wrap("E222", "cannot close the backup identity file", err)
	}
	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) || errors.Is(err, syscall.EEXIST) {
			return false, nil
		}
		return false, errs.Wrap("E222", "cannot place the backup identity at "+path, err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return true, nil
}

// writeCreationNotice tells the owner plainly what was created and the cost of
// losing it. It never contains secret material.
func writeCreationNotice(w io.Writer, id Identity) {
	_, _ = fmt.Fprintf(w, `nSelf created a backup encryption identity because none was configured.
  Identity file: %[1]s (mode 0600)
  Recipient:     %[2]s (public, safe to share)
IMPORTANT: this file is the only way to decrypt these backups. If it is lost,
every backup encrypted to it is unrecoverable and nSelf cannot recover it.
Back it up now, off this machine (a password manager or an offline drive).
After copying it, run: touch %[1]s%[3]s   (silences the nself doctor reminder)
Restore with: nself backup restore-remote --from <backup> --key %[1]s
Anyone who can read this file can read the backups. To harden: move the
identity off this host and keep only the recipient here (BACKUP_AGE_RECIPIENTS).
`, id.Path, id.Recipient, BackedUpMarkerSuffix)
}

// NewIdentityMissing is the E223 error for a decrypt that has no identity.
func NewIdentityMissing(path string) error {
	return errs.Newf("E223", "the backup identity is missing: %s (pass --key <file>, or restore the file from your off-host copy)", path)
}
