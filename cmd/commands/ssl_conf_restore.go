package commands

// Purpose: undo a custom-domain conf that nginx rejected.
// Inputs: the served conf directory and the domain `trust ssl add` writes for.
// Outputs: a snapshot taken before the write, and its restore.
// Constraints: a conf that did not exist is removed; one that did gets its bytes back.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// confSnapshot is the state of one custom-domain conf before `ssl add` wrote it.
type confSnapshot struct {
	path    string
	data    []byte
	existed bool
}

// snapshotCustomDomainConf records custom-<domain-safe>.conf in the served conf dir.
func snapshotCustomDomainConf(workdir, frontedBy, domain string) (confSnapshot, error) {
	confDir, err := servedConfDir(workdir, frontedBy)
	if err != nil {
		return confSnapshot{}, err
	}
	s := confSnapshot{path: filepath.Join(confDir, fmt.Sprintf("custom-%s.conf", domainToFilesafe(domain)))}
	data, err := os.ReadFile(s.path) //nolint:gosec // path built from the served conf dir
	switch {
	case err == nil:
		s.data, s.existed = data, true
	case !errors.Is(err, fs.ErrNotExist):
		return confSnapshot{}, err
	}
	return s, nil
}

// outcome names what restore does, for the error text shown to the operator.
func (s confSnapshot) outcome() string {
	if s.existed {
		return "the previous conf was restored"
	}
	return "the new conf was removed"
}

// restore puts the conf back as snapshotted, so the next nginx test or restart
// does not trip over the rejected one.
func (s confSnapshot) restore() error {
	if !s.existed {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return os.WriteFile(s.path, s.data, 0o644) //nolint:gosec // nginx reads it; same mode the writer uses
}
