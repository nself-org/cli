package exporter

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
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
	return nil
}

func secretFound(member string) error {
	return errs.New("E526", "Secret found in "+member+"; bundle removed")
}

// Purpose: match raw values and JSON-escaped representations without false
// negatives when a secret spans read boundaries. Inputs: resolved values.
// Outputs: pattern set. Constraints: all patterns have at least six raw bytes.
func secretPatterns(secrets []string) [][]byte {
	var p [][]byte
	for _, s := range secrets {
		if len(s) < 6 {
			continue
		}
		p = append(p, []byte(s))
		p = append(p, []byte(strings.TrimPrefix(url.UserPassword("", s).String(), ":")), []byte(url.QueryEscape(s)))
		p = append(p, []byte(strings.ReplaceAll(s, "'", "''")))
		copyEscaped := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t", "\b", "\\b", "\f", "\\f", "\v", "\\v").Replace(s)
		p = append(p, []byte(copyEscaped), []byte(hex.EncodeToString([]byte(s))))
		b, _ := json.Marshal(s)
		p = append(p, b[1:len(b)-1])
		b, _ = json.Marshal(string(b[1 : len(b)-1]))
		p = append(p, b[1:len(b)-1])
	}
	return p
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
