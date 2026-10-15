package deploy

// Purpose: ship mounted credentials as bytes without path rewriting (D-0551).
// Inputs: generated secret files. Outputs: private staged and remote directories.
// Constraints: never change permissions on unrelated host-owned directories.
// SPORT: cli/deploy secret transport.

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const secretDir = ".nself/secrets"

var secretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func isSecretFile(rel string) bool {
	return strings.HasPrefix(rel, secretDir+"/") && secretName.MatchString(strings.TrimPrefix(rel, secretDir+"/"))
}

// stageSecretFiles preserves raw bytes, including strings that happen to name
// a local path. The directory, rather than each mount, protects host access.
func stageSecretFiles(root, stage string) ([]string, error) {
	var files []string
	for _, rel := range []string{".nself", secretDir} {
		info, err := os.Lstat(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s must be a real directory", rel)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, secretDir))
	if err != nil {
		return nil, err
	}
	dst := filepath.Join(stage, secretDir)
	if err := os.MkdirAll(dst, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dst, 0700); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		rel := secretDir + "/" + entry.Name()
		if !isSecretFile(rel) || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("invalid secret file %s", rel)
		}
		data, err := readCapped(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(stage, filepath.FromSlash(rel)), data, 0444); err != nil {
			return nil, err
		}
		files = append(files, rel)
	}
	return files, nil
}

// secretDirScript refuses symlinks before creating or tightening the one
// generated directory. Call before rsync/restore can write a mounted secret.
func secretDirScript(root string) string {
	nself, secrets := path.Join(root, ".nself"), path.Join(root, secretDir)
	return fmt.Sprintf(`test ! -L %s && test ! -L %s && (umask 077; mkdir -p %s) && chmod 700 %s`, q(nself), q(secrets), q(secrets), q(secrets))
}

func prepareSecretDir(ctx context.Context, r Remote, root string, files []string) error {
	for _, rel := range files {
		if isSecretFile(rel) {
			if _, err := r.Run(ctx, shScript(secretDirScript(root))); err != nil {
				return fmt.Errorf("protecting generated secret directory: %w", err)
			}
			break
		}
	}
	return nil
}

// sshSecretFiles validates the generated files before SSH enrolment or copying.
func sshSecretFiles(composePath string) ([]string, error) {
	root := filepath.Dir(composePath)
	dir := filepath.Join(root, secretDir)
	for _, rel := range []string{".nself", secretDir} {
		info, err := os.Lstat(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s must be a real directory", rel)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !isSecretFile(secretDir+"/"+entry.Name()) || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("invalid generated secret file")
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	if len(files) > 0 {
		info, err := os.Lstat(filepath.Join(root, ".nself/compose.env"))
		if err != nil {
			return nil, fmt.Errorf("file secrets require .nself/compose.env: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return nil, fmt.Errorf("file secrets require a regular 0600 .nself/compose.env")
		}
	}
	return files, nil
}

func shipSSHSecrets(ctx context.Context, t sshTransport, root string, files []string) error {
	if len(files) == 0 {
		return nil
	}
	if err := runSSH(ctx, t, secretDirScript(root)); err != nil {
		return err
	}
	localEnv := filepath.Join(filepath.Dir(filepath.Dir(files[0])), "compose.env")
	if err := rsyncFile(ctx, t, localEnv, path.Join(root, ".nself/compose.env")); err != nil {
		return err
	}
	for _, local := range files {
		if err := rsyncFile(ctx, t, local, path.Join(root, secretDir, filepath.Base(local))); err != nil {
			return err
		}
	}
	return nil
}

// CopyFileSecrets ships generated file secrets through the caller's already
// verified transport. It preserves bytes and protects the remote directory.
func (x *Transport) CopyFileSecrets(ctx context.Context, composePath, remotePath string) (bool, error) {
	files, err := sshSecretFiles(composePath)
	if err != nil {
		return false, err
	}
	return len(files) > 0, shipSSHSecrets(ctx, x.t, remotePath, files)
}

// fileSecretComposeCommand preserves user interpolation before adding generated
// versions. With no shipped snapshot, .env remains optional as in Compose's
// default discovery; an explicit --env-file would otherwise suppress it.
func fileSecretComposeCommand(command, root, snapshot string) string {
	if snapshot != "" {
		command += " --env-file " + q(snapshot)
	} else {
		env := q(path.Join(root, ".env"))
		command = "if [ -f " + env + " ]; then set -- --env-file " + env + "; else set --; fi; " + command + ` "$@"`
	}
	return command + " --env-file " + q(path.Join(root, ".nself/compose.env"))
}
