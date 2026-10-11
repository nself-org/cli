package report

// Tests for the scrub inputs (M1, M3, M4) and the manifest notes (S4, S6, S7).
// Every fake secret is built at run time.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// collectFiles runs Collect, writes the tarball and returns its entries.
func collectFiles(t *testing.T, opt Options, src Sources) map[string]string {
	t.Helper()
	b, err := Collect(context.Background(), opt, src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "r.tar.gz")
	if _, err := WriteTarball(b, out, fixedNow); err != nil {
		t.Fatal(err)
	}
	return extract(t, out)
}

func logsSource(logs map[string]string) Sources {
	return Sources{
		Logs: func(context.Context, int) (map[string]string, error) { return logs, nil },
		Now:  func() time.Time { return fixedNow },
	}
}

func assertAbsent(t *testing.T, files map[string]string, secrets ...string) {
	t.Helper()
	for name, body := range files {
		for _, s := range secrets {
			if strings.Contains(body, s) {
				t.Errorf("%s still holds %q", name, s)
			}
		}
	}
}

func TestDoctorReportEnvPEMBodyIsNotAKey(t *testing.T) {
	dir := t.TempDir()
	b1, b2 := rep("MIIFAKEFAKEFAKE", 4), rep("ZmFrZWZha2V", 4)+"=="
	pem := "-----BEGIN " + "PRIVATE KEY-----\n" + b1 + "\n" + b2 + "\n-----END " + "PRIVATE KEY-----"
	writeFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=x\nTLS_PRIVATE_KEY=\""+pem+"\"\nexport OTHER=1\n")
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(map[string]string{"web": "body " + b1 + "\n" + b2 + "\n"}))
	if got := files["config-keys.txt"]; got != "OTHER\nPROJECT_NAME\nTLS_PRIVATE_KEY\n" {
		t.Errorf("config-keys.txt = %q", got)
	}
	assertAbsent(t, files, b1, b2, strings.TrimSuffix(b2, "=="))
}

func TestScanDotenvKeys(t *testing.T) {
	raw := "A=1\n# c=2\nB='open\nC=notakey\nD'\nbad key=3\nE=\"x\\\"y\"\nF=ok\n9X=1\n"
	var keys []string
	for _, p := range scanDotenv(raw) {
		keys = append(keys, p.Key)
	}
	if got := strings.Join(keys, ","); got != "A,B,E,F" {
		t.Errorf("keys = %s, want A,B,E,F", got)
	}
}

func TestDoctorReportWidenedNamesInCompose(t *testing.T) {
	dir := t.TempDir()
	salt, seed, auth := "salt"+rep("Aa8", 4), "seed"+rep("Bb8", 4), "adm:"+rep("Cc8", 4)
	lic, sess, cook := "lic"+rep("Dd8", 4), "sess"+rep("Ee8", 4), "cook"+rep("Ff8", 4)
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), strings.Join([]string{
		"services:",
		"  api:",
		"    environment:",
		"      JWT_SALT: " + salt, "      APP_SEED: " + seed, "      BASIC_AUTH: " + auth,
		"      NSELF_LICENSE: " + lic, "      SESSION_STORE: " + sess, "      COOKIE_KEYS: " + cook,
		"      AUTH_URL: https://auth.example.org",
		`    command: ["sh","-c","curl -u ` + auth + ` http://h --salt ` + salt + `"]`,
		"    entrypoint: /bin/run --auth=" + seed + " --session=" + sess,
		"    labels:",
		"      - traefik.http.middlewares.x.basicauth.users=" + auth,
		"      - site.cookie=" + cook,
		"    healthcheck:",
		"      test: [\"CMD\", \"curl\", \"-H\", \"X-Api: " + lic + "\", \"http://localhost\"]",
		"",
	}, "\n"))
	logs := map[string]string{"api": "salt " + salt + " used\nlic " + lic + " sess " + sess + " cookie " + cook + " seed " + seed + "\n"}
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(logs))
	assertAbsent(t, files, salt, seed, auth, lic, sess, cook)
	if !strings.Contains(files["docker-compose.yml"], "AUTH_URL: https://auth.example.org") {
		t.Error("AUTH_URL was redacted")
	}
}

func TestDoctorReportProcessEnvIsAScrubInput(t *testing.T) {
	dir := t.TempDir()
	v := "proc" + rep("Gh3", 6)
	env := func() []string { return []string{"SVC_API_TOKEN=" + v, "HOME=/home/x"} }
	files := collectFiles(t, Options{ProjectDir: dir, Environ: env}, logsSource(map[string]string{"web": "key " + v + " ok\n"}))
	assertAbsent(t, files, v)
	if !strings.Contains(files["logs/web.log"], " ok") {
		t.Errorf("log over-redacted: %q", files["logs/web.log"])
	}
}

func TestDoctorReportComposeFragmentIsAScrubInput(t *testing.T) {
	dir := t.TempDir()
	v := "frag" + rep("Jk7", 6)
	frag := filepath.Join(dir, ".nself", "plugins", "p.yml")
	writeFile(t, frag, "services:\n  p:\n    environment:\n      PLUGIN_SECRET: "+v+"\n")
	files := collectFiles(t, Options{ProjectDir: dir, ComposeFragments: []string{frag}, Environ: noEnviron}, logsSource(map[string]string{"p": "boot " + v + " done\n"}))
	assertAbsent(t, files, v)
	for n := range files {
		if strings.Contains(n, "p.yml") {
			t.Errorf("fragment %s was bundled", n)
		}
	}
}

func TestDoctorReportCustomServiceEnvFile(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	in, out, viaLink := "cs"+rep("Lm1", 6), "out"+rep("No2", 6), "lnk"+rep("Pq3", 6)
	writeFile(t, filepath.Join(dir, "svc", ".env.svc"), "SVC_PASSWORD="+in+"\n")
	writeFile(t, filepath.Join(outside, "o.env"), "OUT_SECRET="+out+"\n")
	writeFile(t, filepath.Join(outside, "l.env"), "LNK_SECRET="+viaLink+"\n")
	if err := os.Symlink(filepath.Join(outside, "l.env"), filepath.Join(dir, "link.env")); err != nil {
		t.Skip("symlinks unavailable")
	}
	writeFile(t, filepath.Join(dir, ".env"), "CS_1_ENV_FILE=svc/.env.svc\nCS_2_ENV_FILE=../"+filepath.Base(outside)+"/o.env\nCS_3_ENV_FILE="+filepath.Join(outside, "o.env")+"\nCS_4_ENV_FILE=link.env\n")
	logs := map[string]string{"svc": "in " + in + " out " + out + " lnk " + viaLink + "\n"}
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(logs))
	if strings.Contains(files["logs/svc.log"], in) {
		t.Error("CS_N_ENV_FILE value inside the project was not scrubbed")
	}
	for _, s := range []string{out, viaLink} {
		if !strings.Contains(files["logs/svc.log"], s) {
			t.Errorf("a file outside the project dir was read as a scrub input (%q)", s[:4])
		}
	}
	if !strings.Contains(files["manifest.json"], "CS_N_ENV_FILE") {
		t.Error("manifest has no note about the skipped env files")
	}
}

func TestDoctorReportShortSecretsAreCounted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "API_TOKEN=zq7\nDB_PASSWORD=kw\nEMPTY_SECRET=\nREF_KEY=${X}\nLONG_SECRET="+rep("Rs4", 4)+"\n")
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, Sources{Now: func() time.Time { return fixedNow }})
	man := files["manifest.json"]
	for _, want := range []string{"2 secret-named values under 6", "API_TOKEN", "DB_PASSWORD"} {
		if !strings.Contains(man, want) {
			t.Errorf("manifest lacks %q: %s", want, man)
		}
	}
	for _, bad := range []string{"zq7", "kw\"", "EMPTY_SECRET", "REF_KEY", "LONG_SECRET"} {
		if strings.Contains(man, bad) {
			t.Errorf("manifest mentions %q: %s", bad, man)
		}
	}
}

func TestDoctorReportLogByteCap(t *testing.T) {
	dir := t.TempDir()
	head, tail := "HEADLINE"+rep("a", 40), "TAILLINE"+rep("b", 40)
	log := head + "\n" + rep("filler line\n", 20) + tail + "\n"
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron, MaxLogBytes: 100}, logsSource(map[string]string{"web": log}))
	got := files["logs/web.log"]
	if strings.Contains(got, "HEADLINE") || !strings.Contains(got, tail) || len(got) > 100+80 {
		t.Errorf("capped log = %q", got)
	}
	if !strings.Contains(files["manifest.json"], "logs/web.log truncated") {
		t.Errorf("manifest lacks the truncation note: %s", files["manifest.json"])
	}
}
