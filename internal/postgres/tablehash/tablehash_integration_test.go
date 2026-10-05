//go:build integration

package tablehash

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Containers start on the default Docker bridge and are reached by container
// IP, so the test works from the host on Linux and from a sibling container
// that shares the Docker socket (the verification command).

func docker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// startPostgres starts image and returns the container name and its address.
func startPostgres(t *testing.T, image string) string {
	t.Helper()
	name := fmt.Sprintf("a23-it-pg-%d-%s", os.Getpid(), strings.NewReplacer(":", "-", ".", "-").Replace(image))
	_ = exec.Command("docker", "rm", "-f", name).Run()
	docker(t, "run", "-d", "--name", name, "-e", "POSTGRES_PASSWORD=it-pass", image)
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", name).Run() })
	ip := docker(t, "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if ip == "" {
		t.Fatalf("no IP for %s", name)
	}
	return ip
}

// connect waits for the server and returns a connection with extra runtime
// parameters (session defaults that the hash must be immune to).
func connect(t *testing.T, ip string, params map[string]string) *pgx.Conn {
	t.Helper()
	return connectDB(t, ip, "postgres", params)
}

// connectDB is connect to a named database.
func connectDB(t *testing.T, ip, dbname string, params map[string]string) *pgx.Conn {
	t.Helper()
	cfg, err := pgx.ParseConfig(fmt.Sprintf("postgresql://postgres:it-pass@%s:5432/%s?sslmode=disable", ip, dbname))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range params {
		cfg.RuntimeParams[k] = v
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := pgx.ConnectConfig(ctx, cfg)
		if err == nil {
			var one int
			if err = conn.QueryRow(ctx, "SELECT 1").Scan(&one); err == nil {
				cancel()
				t.Cleanup(func() { _ = conn.Close(context.Background()) })
				return conn
			}
			_ = conn.Close(ctx)
		}
		cancel()
		if time.Now().After(deadline) {
			t.Fatalf("postgres at %s not ready: %v", ip, err)
		}
		time.Sleep(time.Second)
	}
}

func mustExec(t *testing.T, c *pgx.Conn, sql string) {
	t.Helper()
	if _, err := c.Exec(context.Background(), sql); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func hashOf(t *testing.T, c *pgx.Conn, schema, name string) Result {
	t.Helper()
	res, bad, err := Snapshot(context.Background(), c, []Table{{schema, name}})
	if err != nil || len(bad) != 0 || len(res) != 1 {
		t.Fatalf("Snapshot(%s.%s): res=%v bad=%v err=%v", schema, name, res, bad, err)
	}
	return res[0]
}

const wideFixture = `
CREATE TYPE mood AS ENUM ('sad','ok','happy');
CREATE TABLE fx (
  id int PRIMARY KEY, t text, n numeric(14,4), f4 real, f8 double precision, ts timestamptz, d date,
  iv interval, b bytea, j jsonb, u uuid, bl boolean, arr int[], tsa text[], ip inet, m money,
  tt time, tz timetz, bits bit(5), e mood);
INSERT INTO fx
SELECT i, 'row ' || i || E' "q" \\ ,(){} ' || md5(i::text) || ' ünï☃', (i * 1.1)::numeric(14,4),
  (i / 7.0)::real, (i / 3.0)::double precision,
  timestamptz '2020-01-01 00:00:00+00' + i * interval '37 minutes 11 seconds',
  date '2020-02-29' + i, (i || ' days ' || (i % 59) || ' seconds ' || (i % 13) || ' months')::interval,
  decode(md5(i::text), 'hex'), jsonb_build_object('b', jsonb_build_array(i, null, 'x'), 'a', 'ü' || i),
  ('00000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid, i % 2 = 0, ARRAY[i, i + 1, NULL],
  ARRAY['a', 'b c', NULL, 'd"e'], ('10.0.' || (i % 250) || '.1')::inet, (i * 12.34)::numeric::money,
  time '00:00:00' + i * interval '1 minute 7 seconds', timetz '12:00:00+05:30', (i % 32)::bit(5),
  (ARRAY['sad','ok','happy'])[1 + i % 3]::mood
FROM generate_series(1, 200) i;
INSERT INTO fx (id, f4, f8, ts, d, iv, n) VALUES
  (1001, 'NaN', 'Infinity', 'infinity', 'infinity', '-1 year', 0),
  (1002, -0.0, 1e-7, '-infinity', '-infinity', '0', -0.0001),
  (1003, 1e22, 1.7976931348623157e308, '1970-01-01 00:00:00+00', 'epoch', '00:00:00.000001', 99999999.9999);`

// TestTablehashIntegration runs the acceptance checks on postgres:16-alpine
// and the cross-major check on 14, 15, 16 and 17.
func TestTablehashIntegration(t *testing.T) {
	ip := startPostgres(t, "postgres:16-alpine")
	conn := connect(t, ip, nil)

	t.Run("order independence and sensitivity", func(t *testing.T) {
		mustExec(t, conn, `CREATE TABLE a (id int, v text, ts timestamptz)`)
		mustExec(t, conn, `CREATE TABLE b (id int, v text, ts timestamptz)`)
		mustExec(t, conn, `INSERT INTO a SELECT i, 'v' || i, timestamptz '2021-03-04 05:06:07+00' + i * interval '1 hour' FROM generate_series(1, 500) i`)
		mustExec(t, conn, `INSERT INTO b SELECT i, 'v' || i, timestamptz '2021-03-04 05:06:07+00' + i * interval '1 hour' FROM generate_series(500, 1, -1) i`)
		ra, rb := hashOf(t, conn, "public", "a"), hashOf(t, conn, "public", "b")
		if ra.Rows != 500 || ra.Rows != rb.Rows || ra.Hash != rb.Hash {
			t.Fatalf("same rows, different order: %+v vs %+v", ra, rb)
		}
		mustExec(t, conn, `UPDATE b SET v = 'changed' WHERE id = 250`)
		if r := hashOf(t, conn, "public", "b"); r.Rows != 500 || r.Hash == ra.Hash {
			t.Errorf("one changed value must change the hash: %+v", r)
		}
		mustExec(t, conn, `UPDATE b SET v = 'v250' WHERE id = 250`)
		if r := hashOf(t, conn, "public", "b"); r.Hash != ra.Hash {
			t.Fatalf("restoring the value must restore the hash: %+v", r)
		}
		mustExec(t, conn, `INSERT INTO b VALUES (501, 'extra', NULL)`)
		if r := hashOf(t, conn, "public", "b"); r.Rows != 501 || r.Hash == ra.Hash {
			t.Errorf("one extra row must change count and hash: %+v", r)
		}
		mustExec(t, conn, `DELETE FROM b WHERE id = 501`)
		// A row replaced by a copy of another row: same count, different hash.
		mustExec(t, conn, `UPDATE b SET id = 1, v = 'v1', ts = (SELECT ts FROM a WHERE id = 1) WHERE id = 2`)
		if r := hashOf(t, conn, "public", "b"); r.Rows != 500 || r.Hash == ra.Hash {
			t.Errorf("a duplicated row must change the hash: %+v", r)
		}
		mustExec(t, conn, `INSERT INTO a SELECT * FROM a WHERE id = 7`)
		if r := hashOf(t, conn, "public", "a"); r.Rows != 501 || r.Hash == ra.Hash {
			t.Errorf("a duplicate insert must change count and hash: %+v", r)
		}
	})

	t.Run("session settings do not change the hash", func(t *testing.T) {
		mustExec(t, conn, wideFixture)
		want := hashOf(t, conn, "public", "fx")
		hostile := []map[string]string{
			{"TimeZone": "Asia/Tokyo"},
			{"TimeZone": "America/Los_Angeles", "DateStyle": "German, DMY", "IntervalStyle": "sql_standard"},
			{"extra_float_digits": "0", "bytea_output": "escape", "lc_monetary": "C", "DateStyle": "SQL, MDY"},
			{"TimeZone": "Pacific/Chatham", "IntervalStyle": "iso_8601", "statement_timeout": "50"},
		}
		for _, p := range hostile {
			c := connect(t, ip, p)
			got := hashOf(t, c, "public", "fx")
			if got != want {
				t.Errorf("settings %v: %+v, want %+v", p, got, want)
			}
			// The caller's own settings are back after Snapshot.
			var tz string
			_ = c.QueryRow(context.Background(), "SHOW TimeZone").Scan(&tz)
			if p["TimeZone"] != "" && tz != p["TimeZone"] {
				t.Errorf("TimeZone not restored: %q, want %q", tz, p["TimeZone"])
			}
		}
		mustExec(t, conn, `ALTER DATABASE postgres SET TimeZone = 'Australia/Lord_Howe'`)
		if got := hashOf(t, connect(t, ip, nil), "public", "fx"); got != want {
			t.Errorf("server TimeZone default: %+v, want %+v", got, want)
		}
	})

	t.Run("empty table and unverifiable names", func(t *testing.T) {
		mustExec(t, conn, `CREATE TABLE empty_t (id int)`)
		if r := hashOf(t, conn, "public", "empty_t"); r.Rows != 0 || r.Hash != "0" {
			t.Errorf("empty table: %+v, want 0 rows and hash \"0\"", r)
		}
		mustExec(t, conn, `CREATE TABLE "bad-name" (id int)`)
		res, bad, err := Snapshot(context.Background(), conn, []Table{{"public", "bad-name"}, {"public", "empty_t"}})
		if err != nil || len(res) != 1 || len(bad) != 1 || bad[0].Table.Name != "bad-name" {
			t.Fatalf("res=%v bad=%v err=%v", res, bad, err)
		}
		if _, _, err := Snapshot(context.Background(), conn, []Table{{"public", "missing_table"}}); err == nil {
			t.Error("a missing table must be an error")
		}
	})

	t.Run("list tables", func(t *testing.T) {
		mustExec(t, conn, `CREATE TABLE parted (id int, k int) PARTITION BY RANGE (k)`)
		mustExec(t, conn, `CREATE TABLE parted_1 PARTITION OF parted FOR VALUES FROM (0) TO (100)`)
		mustExec(t, conn, `INSERT INTO parted SELECT i, i FROM generate_series(1, 50) i`)
		mustExec(t, conn, `CREATE VIEW some_view AS SELECT 1 AS x`)
		tabs, err := ListTables(context.Background(), conn, []string{"public"})
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, tb := range tabs {
			names[tb.Name] = true
		}
		if !names["parted"] || names["parted_1"] || names["some_view"] || !names["fx"] {
			t.Errorf("ListTables = %v", tabs)
		}
		if r := hashOf(t, conn, "public", "parted"); r.Rows != 50 {
			t.Errorf("a partitioned parent covers its partitions: %+v", r)
		}
	})

	t.Run("row level security fails loudly, never hashes a filtered table", func(t *testing.T) {
		mustExec(t, conn, `CREATE TABLE rls_a (id int, v text)`)
		mustExec(t, conn, `CREATE TABLE rls_b (id int, v text)`)
		mustExec(t, conn, `INSERT INTO rls_a VALUES (1, 'secret'), (2, 'x')`)
		mustExec(t, conn, `INSERT INTO rls_b VALUES (99, 'different')`)
		mustExec(t, conn, `ALTER TABLE rls_a ENABLE ROW LEVEL SECURITY`)
		mustExec(t, conn, `ALTER TABLE rls_b ENABLE ROW LEVEL SECURITY`)
		mustExec(t, conn, `CREATE ROLE rls_r NOLOGIN`)
		mustExec(t, conn, `GRANT SELECT ON rls_a, rls_b TO rls_r`)
		a, b := hashOf(t, conn, "public", "rls_a"), hashOf(t, conn, "public", "rls_b")
		if a.Rows != 2 || b.Rows != 1 || a.Hash == b.Hash {
			t.Fatalf("superuser must see every row: a=%+v b=%+v", a, b)
		}
		mustExec(t, conn, `SET ROLE rls_r`)
		defer mustExec(t, conn, `RESET ROLE`)
		for _, name := range []string{"rls_a", "rls_b"} {
			res, _, err := Snapshot(context.Background(), conn, []Table{{"public", name}})
			if err == nil || !strings.Contains(err.Error(), "row-level security") || len(res) != 0 {
				t.Errorf("%s as a role without BYPASSRLS: res=%v err=%v, want the row-level security error", name, res, err)
			}
		}
		var rs string
		if err := conn.QueryRow(context.Background(), "SHOW row_security").Scan(&rs); err != nil || rs != "on" {
			t.Errorf("row_security after a failed Snapshot = %q (%v), want it restored to on", rs, err)
		}
	})

	t.Run("reg columns print the same under any search_path", func(t *testing.T) {
		mustExec(t, conn, `CREATE SCHEMA app`)
		mustExec(t, conn, `CREATE TABLE app.ref (id int, rc regclass)`)
		mustExec(t, conn, `INSERT INTO app.ref VALUES (1, 'app.ref')`)
		mustExec(t, conn, `SET search_path = public`)
		r1 := hashOf(t, conn, "app", "ref")
		mustExec(t, conn, `SET search_path = app, public`)
		r2 := hashOf(t, conn, "app", "ref")
		mustExec(t, conn, `RESET search_path`)
		if r1 != r2 {
			t.Errorf("hash depends on search_path: %+v vs %+v", r1, r2)
		}
		var sp string
		_ = conn.QueryRow(context.Background(), "SHOW search_path").Scan(&sp)
		if strings.Contains(sp, "pg_catalog") && !strings.Contains(sp, "public") {
			t.Errorf("search_path left as %q", sp)
		}
	})

	t.Run("a non-UTF8 database is refused", func(t *testing.T) {
		mustExec(t, conn, `CREATE DATABASE lat1 ENCODING 'LATIN1' TEMPLATE template0 LC_COLLATE 'C' LC_CTYPE 'C'`)
		lc := connectDB(t, ip, "lat1", nil)
		mustExec(t, lc, `CREATE TABLE w (v text)`)
		mustExec(t, lc, `INSERT INTO w VALUES (E'caf\u00e9')`)
		_, _, err := Snapshot(context.Background(), lc, []Table{{"public", "w"}})
		if !errors.Is(err, ErrNotUTF8) {
			t.Fatalf("LATIN1 database: err = %v, want ErrNotUTF8", err)
		}
	})

	t.Run("cross-major 14 15 16 17", func(t *testing.T) {
		majors := []string{"14", "15", "16", "17"}
		ips := make([]string, len(majors))
		var wg sync.WaitGroup
		for i, m := range majors {
			wg.Add(1)
			go func(i int, m string) { // docker run only; t.Fatal stays on this goroutine below
				defer wg.Done()
				name := fmt.Sprintf("a23-it-pg-%d-xm-%s", os.Getpid(), m)
				_ = exec.Command("docker", "rm", "-f", name).Run()
				if out, err := exec.Command("docker", "run", "-d", "--name", name, "-e", "POSTGRES_PASSWORD=it-pass", "postgres:"+m+"-alpine").CombinedOutput(); err != nil {
					t.Errorf("docker run %s: %v\n%s", m, err, out)
				}
			}(i, m)
		}
		wg.Wait()
		for _, m := range majors {
			name := fmt.Sprintf("a23-it-pg-%d-xm-%s", os.Getpid(), m)
			t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", name).Run() })
		}
		if t.Failed() {
			t.FailNow()
		}
		for i, m := range majors {
			ips[i] = docker(t, "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", fmt.Sprintf("a23-it-pg-%d-xm-%s", os.Getpid(), m))
		}
		var ref Result
		for i, m := range majors {
			c := connect(t, ips[i], nil)
			var ver string
			_ = c.QueryRow(context.Background(), "SHOW server_version").Scan(&ver)
			if !strings.HasPrefix(ver, m+".") && ver != m {
				t.Fatalf("expected postgres %s, server says %s", m, ver)
			}
			mustExec(t, c, wideFixture)
			got := hashOf(t, c, "public", "fx")
			t.Logf("postgres %s: rows=%d hash=%s", ver, got.Rows, got.Hash)
			if i == 0 {
				ref = got
			} else if got != ref {
				t.Errorf("CROSS-MAJOR MISMATCH postgres %s: %+v, postgres %s gave %+v", m, got, majors[0], ref)
			}
		}
	})
}
