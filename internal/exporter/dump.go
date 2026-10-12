package exporter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/portable"
)

// runTool is injectable for fail-closed and argv tests; production uses the funnel.
var runTool = docker.RunOneShot
var inspectContainer = docker.InspectContainer

// Purpose: run project-major Postgres tools without password argv.
// Inputs: private stage, args, snapshot. Outputs: tool files. Constraints: uid:gid.
func tool(ctx context.Context, cfg *config.Config, stage string, args []string) error {
	container, err := inspectContainer(ctx, cfg.ProjectName+"_postgres")
	if err != nil {
		return safeFailure("resolve postgres image")
	}
	_, _, err = runTool(ctx, docker.RunSpec{Image: container.Image, Args: args, EnvPass: map[string]string{"PGPASSWORD": cfg.Postgres.Password}, User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), Network: cfg.ProjectName + "_network", Mounts: []docker.Mount{{Source: stage, Destination: "/stage"}}})
	if err != nil {
		return safeFailure("postgres tool")
	}
	return nil
}

// Purpose: write schema and custom archive using one snapshot.
// Inputs: validated schemas. Outputs: writer members. Constraints: excludes np_*.
func dump(ctx context.Context, cfg *config.Config, w *portable.Writer, id string, schemas []string) error {
	base := []string{"pg_dump", "--host=postgres", "--username=" + cfg.Postgres.User, "--dbname=" + cfg.Postgres.DB, "--no-owner", "--no-privileges", "--snapshot=" + id, "--exclude-table=*.np_*"}
	for _, s := range schemas {
		base = append(base, "--schema="+pgPattern(s))
	}
	// An empty schema set must not turn into an unrestricted whole-database dump.
	if len(schemas) == 0 {
		base = append(base, "--schema=__nself_export_no_schema__")
	}
	for _, item := range []struct {
		name  string
		flags []string
	}{{"db/schema.sql", []string{"--schema-only"}}, {"db/data.dump", []string{"-Fc"}}} {
		path := filepath.Join(w.Dir(), ".tool-output")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return safeFailure("create tool output")
		}
		_ = f.Close()
		args := append(append(append([]string{}, base...), item.flags...), "-f", "/stage/.tool-output")
		if err = tool(ctx, cfg, w.Dir(), args); err != nil {
			return err
		}
		if item.name == "db/data.dump" {
			if err = canonicalArchive(path); err != nil {
				return safeFailure("normalize archive")
			}
		}
		f, err = os.Open(path)
		if err != nil {
			return safeFailure("read tool output")
		}
		var r io.Reader = f
		if item.name == "db/schema.sql" {
			r = &sqlReader{scanner: bufio.NewScanner(f)}
		}
		_, err = w.WriteFile(item.name, r)
		_ = f.Close()
		if err != nil {
			return safeFailure("write database member")
		}
		if err = os.Remove(path); err != nil {
			return safeFailure("remove tool output")
		}
	}
	return nil
}

func pgPattern(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// sqlReader removes random pg_dump restrict guards for deterministic schema bytes.
// The SQL is documentation, never executed by the exporter.
type sqlReader struct {
	scanner *bufio.Scanner
	pending []byte
}

func (r *sqlReader) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		line := r.scanner.Text()
		if strings.HasPrefix(line, `\restrict `) || strings.HasPrefix(line, `\unrestrict `) {
			continue
		}
		r.pending = []byte(line + "\n")
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// Purpose: normalize only the custom-format header's wall-clock timestamp.
// Inputs: pg_dump 1.14-1.16 archive. Outputs: deterministic header.
// Constraints: reject unknown layouts rather than changing arbitrary bytes.
func canonicalArchive(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := make([]byte, 12)
	if _, err = io.ReadFull(f, h); err != nil {
		return err
	}
	if string(h[:5]) != "PGDMP" || h[5] != 1 || h[6] < 14 || h[6] > 16 || h[8] != 4 || h[10] != 1 {
		return safeFailure("archive header")
	}
	offset := int64(12)
	// Before 1.15 compression is encoded as an archive int (sign + four bytes).
	if h[6] < 15 {
		offset = 16
	}
	// Seven archive ints: seconds, minutes, hours, day, month, year, isdst.
	stamp := make([]byte, 35)
	for i, v := range []byte{0, 0, 0, 1, 0, 70, 0} {
		stamp[i*5+1] = v
	}
	_, err = f.WriteAt(stamp, offset)
	return err
}
