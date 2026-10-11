package report

// Tests for security CR r2 (lead ruling R5): CS_N_ENV_FILE candidates, compose
// env_file inputs, the stored licence key, the widened names and the notes.
// Every fake secret is built at run time; HOME is isolated (main_test.go).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// N-M1: every value ever seen for a CS_N_ENV_FILE key is read.
func TestDoctorReportCustomEnvFileEveryCandidate(t *testing.T) {
	dir := t.TempDir()
	dev, prod := "dev"+rep("Ab1", 6), "prd"+rep("Cd2", 6)
	loc, sec := "loc"+rep("Ef3", 6), "sec"+rep("Gh4", 6)
	cas, proc := "cas"+rep("Ij5", 6), "prc"+rep("Kl6", 6)
	writeFile(t, filepath.Join(dir, "svc.dev.env"), "SVC_PASSWORD="+dev+"\n")
	writeFile(t, filepath.Join(dir, "svc.prod.env"), "SVC_PASSWORD="+prod+"\n")
	writeFile(t, filepath.Join(dir, "loc.env"), "LOC_SECRET="+loc+"\n")
	writeFile(t, filepath.Join(dir, "sec.env"), "SEC_SECRET="+sec+"\n")
	writeFile(t, filepath.Join(dir, "a.env"), "CAS_SECRET="+cas+"\n")
	writeFile(t, filepath.Join(dir, "b.env"), "PRC_SECRET="+proc+"\n")
	// (a) per-environment override: .env (dev, default) vs .env.prod.
	writeFile(t, filepath.Join(dir, ".env"), "CS_1_ENV_FILE=svc.dev.env\nCS_3_ENV_FILE=a.env\n")
	writeFile(t, filepath.Join(dir, ".env.prod"), "CS_1_ENV_FILE=svc.prod.env\n")
	// (b) .env.local wins over .env.secrets in the build, but sorts earlier.
	writeFile(t, filepath.Join(dir, ".env.local"), "CS_2_ENV_FILE=loc.env\n")
	writeFile(t, filepath.Join(dir, ".env.secrets"), "CS_2_ENV_FILE=sec.env\n")
	// (c) cascade says a.env, the process env says b.env.
	environ := func() []string { return []string{"CS_3_ENV_FILE=b.env"} }
	all := []string{dev, prod, loc, sec, cas, proc}
	log := "x " + strings.Join(all, " y ") + " z\n"
	files := collectFiles(t, Options{ProjectDir: dir, Environ: environ}, logsSource(map[string]string{"svc": log}))
	assertAbsent(t, files, all...)
	if !strings.Contains(files["logs/svc.log"], " z") {
		t.Errorf("log over-redacted: %q", files["logs/svc.log"])
	}
}

// N-M2: compose env_file entries (string, list, {path} list) are scrub inputs.
func TestDoctorReportComposeEnvFileIsAScrubInput(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	one, two, three, four := "ef1"+rep("Mn7", 6), "ef2"+rep("Op8", 6), "ef3"+rep("Qr9", 6), "ef4"+rep("St1", 6)
	out := "efo" + rep("Uv2", 6)
	writeFile(t, filepath.Join(dir, "one.env"), "ONE_SECRET="+one+"\n")
	writeFile(t, filepath.Join(dir, "two.env"), "TWO_SECRET="+two+"\n")
	writeFile(t, filepath.Join(dir, "conf", "three.env"), "THREE_SECRET="+three+"\n")
	writeFile(t, filepath.Join(outside, "o.env"), "OUT_SECRET="+out+"\n")
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), strings.Join([]string{
		"services:",
		"  a:", "    image: x", "    env_file: ./one.env",
		"  b:", "    image: x", "    env_file:", "      - two.env", "      - ./missing.env",
		"  c:", "    image: x", "    env_file:", "      - path: conf/three.env", "        required: false",
		"  d:", "    image: x", "    env_file: " + filepath.Join(outside, "o.env"), "",
	}, "\n"))
	frag := filepath.Join(dir, ".nself", "plugins", "p.yml")
	writeFile(t, frag, "services:\n  p:\n    env_file: [./p.env]\n")
	writeFile(t, filepath.Join(dir, ".nself", "plugins", "p.env"), "PLUGIN_SECRET="+four+"\n")
	// A fragment entry that only resolves from the project dir (fallback).
	five := "ef5" + rep("Yz4", 6)
	writeFile(t, filepath.Join(dir, "conf", "five.env"), "FIVE_SECRET="+five+"\n")
	frag2 := filepath.Join(dir, ".nself", "plugins", "q.yml")
	writeFile(t, frag2, "services:\n  q:\n    env_file: conf/five.env\n")
	logs := map[string]string{"p": strings.Join([]string{"a", one, "b", two, "c", three, "d", four, "e", out, "f", five, "g"}, " ") + "\n"}
	files := collectFiles(t, Options{ProjectDir: dir, ComposeFragments: []string{frag, frag2}, Environ: noEnviron}, logsSource(logs))
	assertAbsent(t, files, one, two, three, four, five)
	if !strings.Contains(files["logs/p.log"], out) {
		t.Error("an env_file outside the project dir was read as a scrub input")
	}
	man := files["manifest.json"]
	for _, want := range []string{"compose env_file", "missing.env", "o.env"} {
		if !strings.Contains(man, want) {
			t.Errorf("manifest lacks a note naming %q: %s", want, man)
		}
	}
}

// R5 d: the stored licence key (license.GetKey) is a scrub input; the key has
// no known prefix, so only the input can remove it.
func TestDoctorReportStoredLicenseKeyIsAScrubInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	stored, numbered := "lk-"+rep("Zy4", 8), "lk2-"+rep("Xw5", 8)
	writeFile(t, filepath.Join(home, ".nself", "license", "key"), stored+"\n")
	environ := func() []string { return []string{"NSELF_LICENSE_KEY_3=" + numbered} }
	dir := t.TempDir()
	log := "bad key " + stored + " (HTTP 401) and " + numbered + " end\n"
	files := collectFiles(t, Options{ProjectDir: dir, Environ: environ}, logsSource(map[string]string{"api": log}))
	assertAbsent(t, files, stored, numbered)
	if !strings.Contains(files["logs/api.log"], " end") {
		t.Errorf("log over-redacted: %q", files["logs/api.log"])
	}
}

// R5 c: PWD, PAT (whole word), WEBHOOK, HMAC are secret names; PATH and PATTERN
// are not; ENCRYPT alone is not.
func TestSecretNameWidenedR5(t *testing.T) {
	for _, n := range []string{"DB_PWD", "PWD", "GITHUB_PAT", "PAT_TOKEN", "PAT", "my-pat", "x.PAT.y", "SLACK_WEBHOOK_URL", "WEBHOOK", "SIG_HMAC", "HMAC_SECRET"} {
		if !IsSecretName(n) {
			t.Errorf("%s should be secret-like", n)
		}
	}
	for _, n := range []string{"PATH", "PATTERN", "XPATH", "XPAT", "PATIENT", "PATCH_LEVEL", "ENCRYPT_MODE", "COMPAT"} {
		if IsSecretName(n) {
			t.Errorf("%s should not be secret-like", n)
		}
	}
}

// R5 c: the names also drive the pattern pass (unknown values) and the env
// cascade inputs (known values) without touching PATH-like neighbours.
func TestDoctorReportR5NamesInBundle(t *testing.T) {
	dir := t.TempDir()
	pwd, pat, hook, hm := "pw"+rep("Aa1", 6), "pt"+rep("Bb2", 6), "wh"+rep("Cc3", 6), "hm"+rep("Dd4", 6)
	writeFile(t, filepath.Join(dir, ".env"), "DB_PWD="+pwd+"\nGITHUB_PAT="+pat+"\nNOTIFY_WEBHOOK_URL=https://hooks.example.org/"+hook+"\nPATH_PREFIX=/usr/local/binaries\nPATTERN_NAME=alphabetical\n")
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), "services:\n  a:\n    environment:\n      DB_PWD: "+pwd+"\n      GITHUB_PAT: "+pat+"\n      PATH: /usr/local/bin:/usr/bin\n      PATTERN: alphabetical\n")
	logs := map[string]string{"a": "pwd " + pwd + " pat " + pat + " hook https://hooks.example.org/" + hook + " hmac HMAC_KEY=" + hm + "\nPATH=/usr/local/bin:/usr/bin\nPATTERN=alphabetical\n"}
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(logs))
	assertAbsent(t, files, pwd, pat, hook, hm)
	for _, keep := range []string{"PATH: /usr/local/bin:/usr/bin", "PATTERN: alphabetical"} {
		if !strings.Contains(files["docker-compose.yml"], keep) {
			t.Errorf("docker-compose.yml lost %q: %s", keep, files["docker-compose.yml"])
		}
	}
	for _, keep := range []string{"PATH=/usr/local/bin:/usr/bin", "PATTERN=alphabetical"} {
		if !strings.Contains(files["logs/a.log"], keep) {
			t.Errorf("log lost %q: %s", keep, files["logs/a.log"])
		}
	}
}

// The shell's PWD is a directory, not a secret: it must not become a literal.
func TestDoctorReportProcessPWDIsNotAScrubInput(t *testing.T) {
	cwd := "/work/" + rep("project", 3)
	environ := func() []string { return []string{"PWD=" + cwd, "OLDPWD=" + cwd + "/old"} }
	files := collectFiles(t, Options{ProjectDir: t.TempDir(), Environ: environ}, logsSource(map[string]string{"a": "cd " + cwd + " ok\n"}))
	if !strings.Contains(files["logs/a.log"], cwd) {
		t.Errorf("working directory was redacted: %q", files["logs/a.log"])
	}
}

// S-5: an unreadable nginx conf or directory becomes a note.
func TestReadNginxUnreadableIsANote(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ok.conf"), "listen 80;\n")
	writeFile(t, filepath.Join(root, "locked.conf"), "listen 81;\n")
	writeFile(t, filepath.Join(root, "sites", "inner.conf"), "listen 82;\n")
	if err := os.Chmod(filepath.Join(root, "locked.conf"), 0); err != nil {
		t.Skip("chmod unavailable")
	}
	if err := os.Chmod(filepath.Join(root, "sites"), 0); err != nil {
		t.Skip("chmod unavailable")
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "sites"), 0o755) })
	files, notes := readNginx(root)
	if len(files) != 1 || files[0].Name != "nginx/ok.conf" {
		t.Errorf("files = %v", files)
	}
	joined := strings.Join(notes, "|")
	for _, want := range []string{"nginx/locked.conf skipped: unreadable", "nginx/sites skipped: unreadable"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes %q lack %q", joined, want)
		}
	}
}
