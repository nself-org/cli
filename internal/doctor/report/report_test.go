package report

// Tests for Collect + WriteTarball: planted secrets, no stack, name safety,
// no-overwrite, and the real-source wiring with docker and the child stubbed.
// Every fake secret is built at run time.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// extract reads a tar.gz into name -> content.
func extract(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = string(b)
	}
}

func names(m map[string]string) []string {
	var n []string
	for k := range m {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

func TestDoctorReportPlantedSecrets(t *testing.T) {
	dir := t.TempDir()
	admin := "adm" + rep("Qx7", 8)
	jwtKey := `jw"t\k` + rep("e9", 6) // quote and backslash: needs JSON escaping
	pgPass := "pa ss/w&rd+" + rep("7", 6)
	composeOnly := "cmp" + rep("Lm4", 7)
	stray := "stray" + rep("Vv2", 6)
	c := newCorpus()
	dbPw, jwtInner := "db"+rep("Nn8", 6), "in"+rep("Kk6", 12)
	writeFile(t, filepath.Join(dir, ".env"), "ENV=dev\nPROJECT_NAME=myapp\nPOSTGRES_DB=myapp\nHASURA_GRAPHQL_ADMIN_SECRET="+admin+"\nDATABASE_URL=postgres://nself:"+dbPw+"@postgres:5432/myapp\nHASURA_GRAPHQL_JWT_SECRET={\"type\":\"HS256\",\"key\":\""+jwtInner+"\"}\n")
	writeFile(t, filepath.Join(dir, ".env.secrets"), "JWT_SECRET='"+jwtKey+"'\nPOSTGRES_PASSWORD=\""+pgPass+"\"\n")
	writeFile(t, filepath.Join(dir, ".env.local"), "SENTRY_DSN=https://k:"+stray+"@sentry.example.org/1\n")
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), "services:\n  hasura:\n    image: hasura/graphql-engine\n    environment:\n      HASURA_GRAPHQL_ADMIN_SECRET: "+admin+"\n      COMPOSE_ONLY_TOKEN: "+composeOnly+"\n      POSTGRES_DB: myapp\n  api:\n    environment:\n      - POSTGRES_PASSWORD="+pgPass+"\n      - PORT=5432\n    command: [\"serve\", \"--admin-secret\", \""+admin+"\"]\n")
	writeFile(t, filepath.Join(dir, "nginx", "sites", "app.conf"), "server {\n  proxy_set_header X-Hasura-Admin-Secret "+admin+";\n  ssl_certificate_key /etc/nginx/ssl/app.key;\n  server_name app.example.org;\n}\n")
	writeFile(t, filepath.Join(dir, "nginx", "ssl", "app.conf"), "ssl material "+c.pem)
	writeFile(t, filepath.Join(dir, "nginx", "ssl", "app.key"), c.pem)
	writeFile(t, filepath.Join(dir, "nginx", "keys.conf"), "pem inside\n"+c.pem+"\n")
	if err := os.Symlink(filepath.Join(dir, ".env.secrets"), filepath.Join(dir, "nginx", "link.conf")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	src := Sources{
		Versions: func(context.Context) map[string]string {
			return map[string]string{"nself": "1.4.x", "note": "built with " + admin}
		},
		Doctor: func(context.Context) (string, error) {
			return "PASS config\nWARN admin secret " + admin + " is weak\npassword=" + stray + "\n", nil
		},
		Logs: func(_ context.Context, tail int) (map[string]string, error) {
			if tail != 7 {
				t.Errorf("tail = %d, want 7", tail)
			}
			return map[string]string{
				"hasura":   "starting\nbad request, admin secret " + admin + " given\nconnect postgres://nself:" + url.QueryEscape(pgPass) + "@postgres:5432/myapp\n",
				"postgres": "ready\nrole password " + dbPw + " and hs key " + jwtInner + "\nsent " + url.PathEscape(pgPass) + " and " + jsonEsc(jwtKey) + "\n",
				"../evil":  "x\n",
			}, nil
		},
		Now: func() time.Time { return fixedNow },
	}
	b, err := Collect(context.Background(), Options{ProjectDir: dir, NginxDir: filepath.Join(dir, "nginx"), LogLines: 7}, src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, DefaultName(fixedNow))
	if _, err := WriteTarball(b, out, fixedNow); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(out); st.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", st.Mode().Perm())
		}
	}
	files := extract(t, out)
	want := []string{"config-keys.txt", "docker-compose.yml", "doctor.txt", "logs/__evil.log", "logs/hasura.log", "logs/postgres.log", "manifest.json", "nginx/sites/app.conf", "versions.json"}
	if got := names(files); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("file set\n got %v\nwant %v", got, want)
	}
	planted := []string{admin, jwtKey, pgPass, composeOnly, stray, dbPw, jwtInner, c.pem, "MIIFAKE"}
	for _, v := range append([]string{}, planted[:7]...) {
		planted = append(planted, url.QueryEscape(v), url.PathEscape(v), jsonEsc(v))
	}
	for name, body := range files {
		for _, v := range planted {
			if strings.Contains(body, v) {
				t.Errorf("%s still holds planted value %q", name, v)
			}
		}
	}
	for _, k := range []string{"HASURA_GRAPHQL_ADMIN_SECRET", "JWT_SECRET", "POSTGRES_DB", "SENTRY_DSN"} {
		if !strings.Contains(files["config-keys.txt"], k) {
			t.Errorf("config-keys.txt lacks key name %s", k)
		}
	}
	if !strings.Contains(files["docker-compose.yml"], "POSTGRES_DB: myapp") || !strings.Contains(files["nginx/sites/app.conf"], "server_name app.example.org;") {
		t.Error("non-secret neighbours were over-redacted")
	}
	var man struct {
		Files      []struct{ Name string }
		Redactions map[string]int
		Total      int `json:"redactions_total"`
	}
	if err := json.Unmarshal([]byte(files["manifest.json"]), &man); err != nil {
		t.Fatal(err)
	}
	if man.Total == 0 || man.Redactions[ClassValue] == 0 || man.Total != b.Total || len(man.Files) != len(want) {
		t.Errorf("manifest = %+v", man)
	}
}

func jsonEsc(s string) string {
	v, _ := jsonEscape(s, true)
	return v
}

func TestDoctorReportNoStack(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "ENV=dev\n")
	for name, logs := range map[string]func(context.Context, int) (map[string]string, error){
		"docker error":  func(context.Context, int) (map[string]string, error) { return nil, errors.New("docker: not installed") },
		"no containers": func(context.Context, int) (map[string]string, error) { return map[string]string{}, nil },
		"real docker absent": func() func(context.Context, int) (map[string]string, error) {
			s := RealSources(DockerSetup{ProjectDir: dir, DockerPath: filepath.Join(dir, "no-such-docker")})
			return s.Logs
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			src := Sources{Logs: logs, Doctor: func(context.Context) (string, error) { return "ok\n", nil }, Now: func() time.Time { return fixedNow }}
			b, err := Collect(context.Background(), Options{ProjectDir: dir, LogLines: 50}, src)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "r.tar.gz")
			if _, err := WriteTarball(b, out, fixedNow); err != nil {
				t.Fatal(err)
			}
			files := extract(t, out)
			for n := range files {
				if strings.HasPrefix(n, "logs/") {
					t.Errorf("unexpected log file %s", n)
				}
			}
			var man struct{ Notes []string }
			if err := json.Unmarshal([]byte(files["manifest.json"]), &man); err != nil {
				t.Fatal(err)
			}
			if len(man.Notes) == 0 || !strings.Contains(strings.Join(man.Notes, " "), "no running stack") {
				t.Errorf("manifest notes = %v", man.Notes)
			}
		})
	}
}

func TestDoctorReportNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "r.tar.gz")
	writeFile(t, out, "precious")
	b, _ := Collect(context.Background(), Options{ProjectDir: dir, LogLines: 5}, Sources{Now: func() time.Time { return fixedNow }})
	if _, err := WriteTarball(b, out, fixedNow); err == nil {
		t.Fatal("overwrote an existing file")
	}
	if got, _ := os.ReadFile(out); string(got) != "precious" {
		t.Errorf("existing file changed: %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover temp files: %v", entries)
	}
}

func TestDoctorReportLogNames(t *testing.T) {
	for in, want := range map[string]string{
		"hasura": "logs/hasura.log", "../../etc/passwd": "logs/____etc_passwd.log", "a/b": "logs/a_b.log",
		"": "logs/unnamed.log", "..": "logs/_.log", "web 1": "logs/web_1.log", "../evil": "logs/__evil.log",
	} {
		got := LogName(in)
		if got != want || strings.Contains(got[5:], "/") || strings.Contains(got, "..") {
			t.Errorf("LogName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDoctorReportChildRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/echo")
	}
	out, err := RunDoctorChild(context.Background(), t.TempDir(), func() (string, error) { return "/bin/echo", nil })
	if err != nil || strings.TrimSpace(out) != "doctor" {
		t.Errorf("out=%q err=%v", out, err)
	}
	if _, err := RunDoctorChild(context.Background(), t.TempDir(), func() (string, error) { return "", errors.New("no exe") }); err == nil {
		t.Error("missing executable not reported")
	}
}
