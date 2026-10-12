package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

// Purpose: scan every non-object member plus manifest and restored plain SQL.
// Inputs: private bundle and resolved secrets. Outputs: E526 naming a fixed
// member label. Constraints: any read/render error aborts; hashes alone exempt.
func scrubBundle(ctx context.Context, cfg *config.Config, dir string, m portable.Manifest, secrets []string) error {
	patterns := secretPatterns(secrets)
	for _, f := range append(m.Files, portable.File{Path: portable.ManifestName}) {
		if strings.HasPrefix(f.Path, "storage/objects/") || f.Path == "db/data.dump" {
			continue
		}
		file, err := os.Open(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil {
			return safeFailure("open scrub member")
		}
		var hit bool
		if f.Path == "auth/users.jsonl" {
			hit, err = scanUsers(file, patterns)
		} else {
			hit, err = scan(file, patterns)
		}
		_ = file.Close()
		if err != nil {
			return safeFailure("scan bundle")
		}
		if hit {
			return secretFound(f.Path)
		}
	}
	path := filepath.Join(dir, ".scan.sql")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return safeFailure("create SQL scan")
	}
	_ = f.Close()
	// On an error path Export removes the whole stage; the success path must
	// prove the plaintext rendering is gone before anything is published.
	defer func() { _ = os.Remove(path) }()
	if err = tool(ctx, cfg, dir, []string{"pg_restore", "-f", "/stage/.scan.sql", "/stage/db/data.dump"}); err != nil {
		return err
	}
	f, err = os.Open(path)
	if err != nil {
		return safeFailure("open SQL scan")
	}
	hit, err := scan(f, patterns)
	_ = f.Close()
	if err != nil {
		return safeFailure("scan SQL")
	}
	if hit {
		return secretFound("db/data.dump")
	}
	if err = removeScan(path); err != nil {
		return safeFailure("remove SQL scan")
	}
	return nil
}

// removeScan is a test seam for proving a failed removal fails the export.
var removeScan = os.Remove

func secretFound(member string) error {
	return errs.New("E526", "Secret found in "+member+"; bundle removed")
}

func scan(r io.Reader, patterns [][]byte) (bool, error) {
	keep := 0
	for _, p := range patterns {
		if len(p) > keep {
			keep = len(p)
		}
	}
	chunk := make([]byte, 32<<10)
	tail := []byte{}
	for {
		n, err := r.Read(chunk)
		data := append(tail, chunk[:n]...)
		for _, p := range patterns {
			if bytes.Contains(data, p) {
				return true, nil
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		start := max(0, len(data)-max(0, keep-1))
		tail = append([]byte{}, data[start:]...)
	}
}

func scanUsers(r io.Reader, patterns [][]byte) (bool, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	for {
		var u map[string]any
		err := dec.Decode(&u)
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if h, ok := u["password_hash"].(string); ok && bcryptHash.MatchString(h) {
			delete(u, "password_hash")
		}
		b, err := json.Marshal(u)
		if err != nil {
			return false, err
		}
		hit, err := scan(bytes.NewReader(b), patterns)
		if hit || err != nil {
			return hit, err
		}
	}
}
