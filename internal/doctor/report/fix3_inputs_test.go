package report

// Tests for fix round 3 inputs (lead ruling R6 a, b, c, f and Opus M2, S2, S6):
// build-written env files, unquoted PEM values in .env, compose values in their
// runtime spelling, fragment env_file confinement, scrubbed entry names.
// Every fake secret is built at run time; HOME is isolated (main_test.go).

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// R6 a: compose.env, .env.computed and any .env.<name> are scrub inputs only.
func TestDoctorReportBuildWrittenEnvFilesAreScrubInputs(t *testing.T) {
	dir := t.TempDir()
	adm, comp, demo := "ad"+rep("Mn3", 8), "cm"+rep("Pq4", 8), "dm"+rep("Rs5", 8)
	writeFile(t, filepath.Join(dir, ".env"), "ENV=dev\nPROJECT_NAME=myapp\n")
	writeFile(t, filepath.Join(dir, ".nself", "compose.env"), "COMPOSEONLY_ADMIN_SECRET="+adm+"\n")
	writeFile(t, filepath.Join(dir, ".env.computed"), "COMPUTEDONLY_DB_PASSWORD="+comp+"\n")
	writeFile(t, filepath.Join(dir, ".env.demo"), "DEMOONLY_API_KEY="+demo+"\n")
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), "services:\n  h:\n    image: x\n    environment:\n      COMPOSEONLY_ADMIN_SECRET: ${COMPOSEONLY_ADMIN_SECRET}\n")
	logs := map[string]string{"h": "loaded " + adm + " and " + comp + " and " + demo + " ok\n"}
	files := collectFiles(t, Options{ProjectDir: dir, ComposeEnvFiles: []string{filepath.Join(dir, ".nself", "compose.env")}, Environ: noEnviron}, logsSource(logs))
	assertAbsent(t, files, adm, comp, demo)
	if !strings.Contains(files["logs/h.log"], " ok") {
		t.Errorf("log over-redacted: %q", files["logs/h.log"])
	}
	for _, k := range []string{"COMPOSEONLY_ADMIN_SECRET", "COMPUTEDONLY_DB_PASSWORD", "DEMOONLY_API_KEY"} {
		if strings.Contains(files["config-keys.txt"], k) {
			t.Errorf("config-keys.txt lists %s: build-written files are scrub inputs only", k)
		}
	}
	if !strings.Contains(files["config-keys.txt"], "PROJECT_NAME") {
		t.Errorf("config-keys.txt lost the cascade keys: %q", files["config-keys.txt"])
	}
}

// M2: an unquoted multi-line PEM in .env: the base64 tail line looks like
// KEY=VALUE (letter first, no + or /, == padding) but is key material.
func TestDoctorReportUnquotedPEMTailIsNeverAKey(t *testing.T) {
	for _, mode := range []string{"keyed", "bare block", "unterminated"} {
		bare, open := mode == "bare block", mode == "unterminated"
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			first := rep("MIIFAKEFAKEFAKE", 4)
			tail := "Ab" + rep("Cd9", 9) + "=="
			head := "JWT_PRIVATE_KEY=-----BEGIN " + "PRIVATE KEY-----"
			if bare {
				head = "-----BEGIN " + "PRIVATE KEY-----"
			}
			body := "PROJECT_NAME=myapp\n" + head + "\n" + first + "\n" + tail + "\n-----END " + "PRIVATE KEY-----\nAFTER_KEY=plain\n"
			if open { // godotenv parses this text without an error and reports the tail as a key
				body = "PROJECT_NAME=myapp\n" + head + "\n" + tail + "\n"
				first = tail
			}
			writeFile(t, filepath.Join(dir, ".env"), body)
			files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(map[string]string{"a": "x " + first + " y " + tail + " z\n"}))
			stem := strings.TrimSuffix(tail, "==")
			assertAbsent(t, files, stem, first)
			keys := files["config-keys.txt"]
			if !strings.Contains(keys, "PROJECT_NAME") || (!open && !strings.Contains(keys, "AFTER_KEY")) {
				t.Errorf("real keys around the PEM were lost: %q", keys)
			}
		})
	}
}

// R6 c / Opus M3 and S6: compose values in the spelling the container sees.
func TestDoctorReportComposeRuntimeSpelling(t *testing.T) {
	dir := t.TempDir()
	esc, def, dash, alt := "e"+rep("Aq1", 6), "d"+rep("Bw2", 6), "h"+rep("Ce3", 6), "a"+rep("Dr4", 6)
	nest, list, plain := "n"+rep("Ft5", 6), "l"+rep("Gy6", 6), "p"+rep("Hu7", 6)
	hex, big := "0x1F2E3D4C5B6A7988", "1234567890123456789012345678901234567890"
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), strings.Join([]string{
		"services:",
		"  a:",
		"    image: x",
		"    environment:",
		`      APP_SECRET: "pa$$` + esc + `"`,
		"      APP_TOKEN: ${APP_TOKEN:-" + def + "}",
		"      APP_KEY: ${NOPE_KEY-" + dash + "}",
		"      APP_PASS: ${SET_PASS:+" + alt + "}",
		"      APP_SEED: ${A_SEED:-${B_SEED:-" + nest + "}}",
		"      API_SECRET: " + hex,
		"      API_TOKEN: " + big,
		"      PLAIN_SECRET: " + plain,
		"  b:",
		"    image: x",
		"    environment:",
		"      - APP_API_KEY=${Z_KEY:-" + list + "}",
		"",
	}, "\n"))
	runtime := []string{"pa$" + esc, def, dash, alt, nest, list, hex, big}
	logs := "connecting with " + strings.Join(runtime, " | ") + " | " + plain + " ok\n"
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(map[string]string{"a": logs}))
	assertAbsent(t, files, runtime...)
	assertAbsent(t, files, plain)
	if !strings.Contains(files["logs/a.log"], " ok") {
		t.Errorf("log over-redacted: %q", files["logs/a.log"])
	}
}

func TestRuntimeFormsExpansion(t *testing.T) {
	var e runtimeExpander
	if got := e.expand("a$$b${X:-d1}c${Y-d2}${Z:+d3}${W:-${V:-in}}$Q${R}"); got != "a$bd1cd2d3in$Q${R}" {
		t.Errorf("expand = %q", got)
	}
	var lits []string
	for _, l := range e.lits {
		lits = append(lits, l.text)
	}
	if strings.Join(lits, ",") != "d1,d2,d3,in,in" {
		t.Errorf("literals = %v", lits)
	}
}

// R6 b: a plugin fragment lives outside the project; its env_file may resolve
// inside the directory of that fragment and nowhere else.
func TestDoctorReportFragmentEnvFileConfinedToFragmentDir(t *testing.T) {
	proj, fragDir, other := t.TempDir(), t.TempDir(), t.TempDir()
	in, rel, abs := "fr"+rep("Jk8", 8), "up"+rep("Lm9", 8), "ot"+rep("Np1", 8)
	writeFile(t, filepath.Join(fragDir, "plugin.env"), "FRAG_SECRET="+in+"\n")
	writeFile(t, filepath.Join(other, "o.env"), "OTHER_SECRET="+abs+"\n")
	writeFile(t, filepath.Join(filepath.Dir(fragDir), "up.env"), "UP_SECRET="+rel+"\n")
	t.Cleanup(func() { _ = os.Remove(filepath.Join(filepath.Dir(fragDir), "up.env")) })
	frag := filepath.Join(fragDir, "docker-compose.plugin.yml")
	writeFile(t, frag, "services:\n  p:\n    image: x\n    env_file:\n      - ./plugin.env\n      - ../up.env\n      - "+filepath.Join(other, "o.env")+"\n")
	logs := "got " + in + " and " + rel + " and " + abs + "\n"
	files := collectFiles(t, Options{ProjectDir: proj, ComposeFragments: []string{frag}, Environ: noEnviron}, logsSource(map[string]string{"p": logs}))
	if strings.Contains(files["logs/p.log"], in) {
		t.Error("the fragment-relative env_file was not read")
	}
	for _, s := range []string{rel, abs} {
		if !strings.Contains(files["logs/p.log"], s) {
			t.Errorf("an env_file outside the fragment dir was read (%q)", s[:3])
		}
	}
	if !strings.Contains(files["manifest.json"], "skipped as a scrub input") {
		t.Error("the manifest has no note about the skipped env_file entries")
	}
}

// R6 f: entry names pass the scrubber and a path-safe filter; the manifest
// lists the same names.
func TestDoctorReportEntryNamesAreScrubbed(t *testing.T) {
	dir := t.TempDir()
	sec := "sv" + rep("Qr2", 8)
	writeFile(t, filepath.Join(dir, ".env"), "API_SECRET="+sec+"\n")
	writeFile(t, filepath.Join(dir, "nginx", "sites", sec+".conf"), "server { listen 80; }\n")
	logs := map[string]string{"svc-" + sec: "hello\n", "../evil": "x\n"}
	files := collectFiles(t, Options{ProjectDir: dir, NginxDir: filepath.Join(dir, "nginx"), Environ: noEnviron}, logsSource(logs))
	var nginx, logf int
	for _, n := range names(files) {
		if strings.Contains(n, sec) {
			t.Errorf("entry name %q holds the secret", n)
		}
		if strings.HasPrefix(n, "nginx/") {
			nginx++
		}
		if strings.HasPrefix(n, "logs/") && strings.HasSuffix(n, ".log") {
			logf++
			if strings.Contains(strings.TrimPrefix(strings.TrimSuffix(n, ".log"), "logs/"), "/") {
				t.Errorf("log entry %q has a path separator", n)
			}
		}
	}
	if nginx != 1 || logf != 2 {
		t.Errorf("nginx=%d logs=%d entries, want 1 and 2: %v", nginx, logf, names(files))
	}
	assertAbsent(t, files, sec)
	for _, n := range names(files) {
		if n != "manifest.json" && n != "logs/" && !strings.Contains(files["manifest.json"], `"`+n+`"`) {
			t.Errorf("manifest does not list %q", n)
		}
	}
}

// S1: the cap must not cut a secret in half before it is scrubbed.
func TestDoctorReportLogCapDoesNotSplitASecret(t *testing.T) {
	dir := t.TempDir()
	sec := "cp" + rep("Wx6", 10)
	writeFile(t, filepath.Join(dir, ".env"), "CAP_SECRET="+sec+"\n")
	line := rep("a", 40) + sec + rep("b", 40) // one line, longer than the cap
	files := collectFiles(t, Options{ProjectDir: dir, MaxLogBytes: 64, Environ: noEnviron}, logsSource(map[string]string{"a": line}))
	assertAbsent(t, files, sec[len(sec)-22:])
	if !strings.Contains(files["manifest.json"], "truncated to its last 64 bytes") {
		t.Error("manifest has no truncation note")
	}
}

// S6: a YAML scalar is learned as written and as the decoder prints it (the
// card-like number pattern would hide a missing decimal form in a bundle test,
// so this looks at the learned literals directly).
func TestLearnComposeYAMLScalarForms(t *testing.T) {
	in := newInputSet(t.TempDir())
	doc := "services:\n  a:\n    environment:\n      API_SECRET: 0x1F2E3D4C5B6A7988\n"
	if !learnComposeYAML([]byte(doc), in.projectDir, in) {
		t.Fatal("document refused")
	}
	have := map[string]bool{}
	for _, v := range in.values {
		for _, s := range v.Variants {
			have[s] = true
		}
	}
	for _, want := range []string{"0x1F2E3D4C5B6A7988", strconv.FormatInt(0x1F2E3D4C5B6A7988, 10)} {
		if !have[want] {
			t.Errorf("spelling %q was not learned", want)
		}
	}
}

// R7: a default equal to a service, container or image name is public
// structure, not a literal; a non-structure default is still redacted.
func TestDoctorReportStructureNamesAreNotLiterals(t *testing.T) {
	proj, fragDir := t.TempDir(), t.TempDir()
	other := "o" + rep("Kx9", 4) // 13 chars, not a structure name
	frag := filepath.Join(fragDir, "docker-compose.plugin.yml")
	writeFile(t, frag, strings.Join([]string{
		"services:",
		"  postgres:",
		"    image: postgres:16-alpine",
		"    container_name: Mon-DB",
		"    environment:",
		"      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-postgres}",
		"      MON_API_TOKEN: ${MON_API_TOKEN:-mon-db}",
		"  app:",
		"    image: ghcr.io/acme/webapp:1.2",
		"    environment:",
		"      APP_TOKEN: ${APP_TOKEN:-" + other + "}",
		"      WEB_KEY: ${WEB_KEY:-webapp}",
		"",
	}, "\n"))
	logs := map[string]string{"postgres": "postgres: ready; image postgres:16-alpine; mon-db up; webapp ok\n", "app": "token " + other + " end\n"}
	files := collectFiles(t, Options{ProjectDir: proj, ComposeFragments: []string{frag}, Environ: noEnviron}, logsSource(logs))
	for _, keep := range []string{"postgres: ready; image postgres:16-alpine; mon-db up; webapp ok"} {
		if !strings.Contains(files["logs/postgres.log"], keep) {
			t.Errorf("structure names were redacted: %q", files["logs/postgres.log"])
		}
	}
	assertAbsent(t, files, other)
	if !strings.Contains(files["manifest.json"], "learned values equal service/image names and were not used as literals") {
		t.Errorf("manifest has no structure note: %s", files["manifest.json"])
	}
}
