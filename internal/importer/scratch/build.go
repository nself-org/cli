// Package scratch turns generated DDL and COPY files into a portable Postgres bundle.
// Purpose: common data path for non-Postgres source adapters.
// Inputs: trusted generated DDL and validated table/column identifiers.
// Outputs: target-image archive and canonical manifest hashes.
// Constraints: ephemeral database lives in the one-shot container's own filesystem, which is removed
// with the container (a /dev/shm data dir is capped at Docker's 64 MiB default); password only EnvPass.
package scratch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/importer/load"
	"github.com/nself-org/cli/internal/portable"
	"github.com/nself-org/cli/internal/postgres/tablehash"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type CopyFile struct {
	Schema, Table, Path string
	Columns             []string
}

func Build(ctx context.Context, cfg *config.Config, ddl string, copies []CopyFile, stage string) (portable.Manifest, error) {
	sql, schemas, err := copySQL(copies)
	if err != nil {
		return portable.Manifest{}, err
	}
	image, err := load.Image(ctx, cfg)
	if err != nil {
		return portable.Manifest{}, err
	}
	if err = os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
		return portable.Manifest{}, errors.New("scratch directory unavailable")
	}
	root, err := os.MkdirTemp(filepath.Dir(stage), ".scratch-")
	if err != nil {
		return portable.Manifest{}, errors.New("scratch directory unavailable")
	}
	defer func() { _ = os.RemoveAll(root) }()
	work := filepath.Join(root, "work")
	if os.Mkdir(work, 0o755) != nil {
		return portable.Manifest{}, errors.New("scratch directory unavailable")
	}
	if os.WriteFile(filepath.Join(work, "load.sql"), []byte(ddl+"\n"+sql), 0o644) != nil {
		return portable.Manifest{}, errors.New("scratch DDL write failed")
	}
	hashSQL, err := hashQueries(copies)
	if err != nil {
		return portable.Manifest{}, err
	}
	if os.WriteFile(filepath.Join(work, "hash.sql"), []byte(hashSQL), 0o644) != nil {
		return portable.Manifest{}, errors.New("scratch hash query write failed")
	}
	for i, c := range copies {
		info, e := os.Lstat(c.Path)
		if e != nil || !info.Mode().IsRegular() {
			return portable.Manifest{}, errors.New("scratch COPY member invalid")
		}
		b, e := os.ReadFile(c.Path)
		if e != nil {
			return portable.Manifest{}, errors.New("scratch COPY read failed")
		}
		if os.WriteFile(filepath.Join(work, fmt.Sprintf("copy-%d.csv", i)), b, 0o644) != nil {
			return portable.Manifest{}, errors.New("scratch COPY write failed")
		}
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return portable.Manifest{}, errors.New("scratch credential generation failed")
	}
	spec := docker.RunSpec{Image: image, Args: []string{"bash", "-c", buildScript}, EnvPass: map[string]string{"PGPASSWORD": hex.EncodeToString(secret), "HOST_UID": strconv.Itoa(os.Getuid()), "HOST_GID": strconv.Itoa(os.Getgid())}, Mounts: []docker.Mount{{Source: work, Destination: "/scratch"}}}
	if _, _, err = docker.RunOneShot(ctx, spec); err != nil {
		return portable.Manifest{}, errors.New("scratch postgres build failed")
	}
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: "postgres"}, DB: portable.DB{Schemas: schemas}}
	lines, err := os.ReadFile(filepath.Join(work, "tables.jsonl"))
	if err != nil {
		return m, errors.New("scratch hashes unavailable")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(lines)), "\n") {
		if line == "" {
			continue
		}
		var table portable.Table
		if json.Unmarshal([]byte(line), &table) != nil {
			return m, errors.New("scratch hashes invalid")
		}
		m.DB.Tables = append(m.DB.Tables, table)
	}
	writer, err := portable.NewWriter(stage)
	if err != nil {
		return m, err
	}
	for _, name := range []string{"data.dump", "schema.sql"} {
		f, e := os.Open(filepath.Join(work, name))
		if e != nil {
			return m, errors.New("scratch archive unavailable")
		}
		_, e = writer.WriteFile("db/"+name, f)
		_ = f.Close()
		if e != nil {
			return m, e
		}
	}
	return writer.Finish(m)
}
func copySQL(copies []CopyFile) (string, []string, error) {
	var sql strings.Builder
	schemas := map[string]bool{}
	seen := map[string]bool{}
	for i, c := range copies {
		schema, e := database.SanitizeIdentifier(c.Schema)
		if e != nil {
			return "", nil, errors.New("invalid scratch schema")
		}
		table, e := database.SanitizeIdentifier(c.Table)
		if e != nil {
			return "", nil, errors.New("invalid scratch table")
		}
		id := c.Schema + "." + c.Table
		if seen[id] {
			return "", nil, errors.New("duplicate scratch table")
		}
		seen[id] = true
		schemas[c.Schema] = true
		cols := []string{}
		for _, col := range c.Columns {
			q, e := database.SanitizeIdentifier(col)
			if e != nil {
				return "", nil, errors.New("invalid scratch column")
			}
			cols = append(cols, q)
		}
		columns := ""
		if len(cols) > 0 {
			columns = " (" + strings.Join(cols, ",") + ")"
		}
		fmt.Fprintf(&sql, "\\copy %s.%s%s FROM '/scratch/copy-%d.csv' WITH (FORMAT csv)\n", schema, table, columns, i)
	}
	out := []string{}
	for schema := range schemas {
		out = append(out, schema)
	}
	sort.Strings(out)
	return sql.String(), out, nil
}
func hashQueries(copies []CopyFile) (string, error) {
	var sql strings.Builder
	for _, setting := range tablehash.Settings {
		fmt.Fprintf(&sql, "SELECT set_config('%s','%s',false) WHERE false; SET %s = '%s';\n", setting[0], setting[1], setting[0], setting[1])
	}
	for _, c := range copies {
		schema, e := database.SanitizeIdentifier(c.Schema)
		if e != nil {
			return "", errors.New("invalid scratch schema")
		}
		table, e := database.SanitizeIdentifier(c.Table)
		if e != nil {
			return "", errors.New("invalid scratch table")
		}
		fmt.Fprintf(&sql, "SELECT json_build_object('schema','%s','name','%s','rows',count(*),'hash',coalesce(sum(hashtextextended(t::text,0)::numeric),0)::text,'pk','[]'::json) FROM %s.%s t;\n", c.Schema, c.Table, schema, table)
	}
	return sql.String(), nil
}

// buildScript runs inside the postgres image. Only the unix socket (a few bytes) uses /dev/shm.
const buildScript = `set -eu
umask 077
mkdir /var/tmp/a10-data
chown postgres:postgres /var/tmp/a10-data
runner=gosu
command -v gosu >/dev/null || runner=su-exec
"$runner" postgres initdb -D /var/tmp/a10-data -A trust >/dev/null
"$runner" postgres pg_ctl -D /var/tmp/a10-data -o '-k /dev/shm -c listen_addresses= -c port=15432' -w start >/dev/null
trap '"$runner" postgres pg_ctl -D /var/tmp/a10-data -m immediate stop >/dev/null' EXIT
"$runner" postgres psql -h /dev/shm -p 15432 -U postgres -v ON_ERROR_STOP=1 -f /scratch/load.sql >/dev/null
"$runner" postgres psql -h /dev/shm -p 15432 -U postgres -v ON_ERROR_STOP=1 -qAt -f /scratch/hash.sql > /scratch/tables.jsonl
"$runner" postgres pg_dump -h /dev/shm -p 15432 -U postgres --no-owner --no-privileges -Fc -f /var/tmp/a10-data/data.dump
"$runner" postgres pg_dump -h /dev/shm -p 15432 -U postgres --no-owner --no-privileges --schema-only -f /var/tmp/a10-data/schema.sql
cp /var/tmp/a10-data/data.dump /var/tmp/a10-data/schema.sql /scratch/
chown "$HOST_UID:$HOST_GID" /scratch/data.dump /scratch/schema.sql /scratch/tables.jsonl
`
