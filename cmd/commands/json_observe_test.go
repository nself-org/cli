package commands

// Tests for the observe fragment envelopes (P7-SURF-11): status urls, status
// health (and check, service, endpoint, history, config), doctor heal, doctor
// images, help topics, version, plus the doctor backup-hint wiring.
//
// Purpose: prove each converted command writes one v1 envelope that validates
// against its generated schema in v1.5 mode, that the pre-contract bare shapes
// stay byte-identical in v1.4 mode, and that doctor heal never puts the key in
// a document.
// Constraints: nothing writes outside t.TempDir; commands run in-process
// through the pilot output seam.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/spf13/cobra"
)

func TestEnvelopeCoverageObserve(t *testing.T) { assertFragmentEnvelopeCoverage(t, "observe") }

// TestEnvelopeCoverageObserveWatchNeedsReason: `status health watch` is a
// stream; stripping its reason comment must fail the coverage check.
func TestEnvelopeCoverageObserveWatchNeedsReason(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("../../internal/canon/domains", "observe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]cmdregistry.Command{}
	for _, row := range reg.Commands {
		rows[row.Path] = row
	}
	if got := envelopeCoverageFindings(b, rows); len(got) != 0 {
		t.Fatalf("committed observe.yaml has findings: %v", got)
	}
	lines := strings.Split(string(b), "\n")
	stripped := 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "status health watch:") {
			lines[i] = line[:strings.Index(line, "} #")+1]
			stripped++
		}
	}
	if stripped != 1 {
		t.Fatalf("watch row not found exactly once (%d)", stripped)
	}
	if got := envelopeCoverageFindings([]byte(strings.Join(lines, "\n")), rows); len(got) != 1 || !strings.Contains(got[0], "status health watch") {
		t.Fatalf("watch without a reason was accepted: %v", got)
	}
}

// TestObserveRegistryClasses: the registry classes of the converted rows in
// both modes, and the doctor heal consent declaration.
func TestObserveRegistryClasses(t *testing.T) {
	v15, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	v14, err := buildRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	row := func(r *cmdregistry.Registry, path string) cmdregistry.Command {
		c, ok := r.Lookup(path)
		if !ok {
			t.Fatalf("registry has no %q", path)
		}
		return *c
	}
	for _, p := range []string{"status urls", "status health", "status health check", "status health service",
		"status health endpoint", "status health history", "status health config", "doctor heal", "doctor images", "help topics", "version"} {
		c := row(v15, "nself "+p)
		if c.JSON != canon.JSONEnvelope || c.DataSchema == nil {
			t.Errorf("v1.5 %s: json=%s data_schema=%v, want envelope with a schema", p, c.JSON, c.DataSchema)
		}
	}
	if c := row(v15, "nself status health watch"); c.Output != canon.OutputStream || c.JSON != canon.JSONLegacy {
		t.Errorf("status health watch: output=%s json=%s, want stream/legacy", c.Output, c.JSON)
	}
	// v1.4: the pre-contract rows stay legacy (their bytes are pinned by
	// json-conversion-check.sh); doctor images is additive in both modes.
	for _, p := range []string{"urls", "health", "health check", "health service", "health endpoint", "health history", "health config", "version"} {
		if got := row(v14, "nself "+p).JSON; got != canon.JSONLegacy {
			t.Errorf("v1.4 %s: json=%s, want legacy", p, got)
		}
	}
	if got := row(v14, "nself doctor images").JSON; got != canon.JSONEnvelope {
		t.Errorf("v1.4 doctor images: json=%s, want envelope", got)
	}
	heal := row(v15, "nself doctor heal")
	if heal.Confirm == nil || len(heal.Confirm.Flags) != 0 || heal.Confirm.Plan != nil {
		t.Fatalf("doctor heal confirm = %+v, want {flags: []}", heal.Confirm)
	}
	if got := cmdregistry.EffectiveSideEffect(&heal, map[string]bool{"jwt": true}); got != canon.SideEffectWrite {
		t.Errorf("doctor heal --jwt side effect = %s, want write", got)
	}
	if got := cmdregistry.EffectiveSideEffect(&heal, nil); got != canon.SideEffectRead {
		t.Errorf("doctor heal side effect = %s, want read", got)
	}
}

// compareBare asserts bare equals the reference marshalled the pre-contract way.
func compareBare(t *testing.T, got string, reference any) {
	t.Helper()
	want, err := json.MarshalIndent(reference, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want)+"\n" {
		t.Fatalf("bare output changed\n--- got ---\n%s\n--- want ---\n%s\n", got, want)
	}
}

func TestVersionJSON(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		out, errOut := pilotBuffers(t)
		if err := versionCmd.Flags().Set("json", "true"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = versionCmd.Flags().Set("json", "false") })
		if err := versionCmd.RunE(versionCmd, nil); err != nil {
			t.Fatal(err)
		}
		if compat.V15() {
			data := validateEnvelope(t, out.String(), "version")
			if data["platform"] == "" || data["capabilities"] == nil {
				t.Fatalf("version data incomplete: %v", data)
			}
			return
		}
		var doc map[string]any
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		compareBare(t, out.String(), doc) // the former map form: sorted keys, 2-space indent
		if _, has := doc["schema_version"]; has || errOut.Len() != 0 {
			t.Fatalf("v1.4 must print the bare map and nothing on stderr: %s / %s", out, errOut)
		}
	})
}

func TestHealthConfigJSON(t *testing.T) {
	oldTimeout, oldInterval, oldRetries, oldEnv, oldJSON, oldQuiet := healthTimeout, healthInterval, healthRetries, healthEnv, healthJSON, healthQuiet
	t.Cleanup(func() {
		healthTimeout, healthInterval, healthRetries, healthEnv, healthJSON, healthQuiet = oldTimeout, oldInterval, oldRetries, oldEnv, oldJSON, oldQuiet
	})
	healthTimeout, healthInterval, healthRetries, healthEnv, healthJSON, healthQuiet = 30, 10, 3, "staging", true, false
	compattest.Both(t, func(t *testing.T) {
		out, _ := pilotBuffers(t)
		if err := healthConfigCmd.RunE(healthConfigCmd, nil); err != nil {
			t.Fatal(err)
		}
		if compat.V15() {
			if data := validateEnvelope(t, out.String(), "status health config"); data["env"] != "staging" {
				t.Fatalf("data = %v", data)
			}
			return
		}
		// The former shape: a map, so keys sorted.
		compareBare(t, out.String(), map[string]interface{}{"timeout_seconds": 30, "interval_seconds": 10, "retries": 3,
			"env": "staging", "json_output": true, "quiet": false})
	})
}

func TestHealthReportJSON(t *testing.T) {
	report := &health.HealthReport{Timestamp: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), Healthy: 1, Total: 1,
		Results: []health.HealthResult{{Service: "postgres", Status: "healthy", Duration: 12 * time.Millisecond, Details: "ok"}}}
	for _, command := range []string{"status health", "status health check"} {
		compattest.Both(t, func(t *testing.T) {
			out, _ := pilotBuffers(t)
			if err := emitHealthJSON(command, report); err != nil {
				t.Fatal(err)
			}
			if compat.V15() {
				if data := validateEnvelope(t, out.String(), command); data["total"] != float64(1) {
					t.Fatalf("data = %v", data)
				}
				return
			}
			compareBare(t, out.String(), report)
		})
	}
}

func TestHealthHistoryJSON(t *testing.T) {
	dir := t.TempDir()
	// The command reads the history under the working directory; schema files
	// are read relative to the package directory, so chdir only around the run.
	runHistory := func(t *testing.T) {
		t.Helper()
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(wd) }()
		if err := healthHistoryCmd.RunE(healthHistoryCmd, nil); err != nil {
			t.Fatal(err)
		}
	}
	compattest.Both(t, func(t *testing.T) {
		out, _ := pilotBuffers(t)
		healthJSON = true
		t.Cleanup(func() { healthJSON = false })
		runHistory(t)
		if compat.V15() {
			data := validateEnvelope(t, out.String(), "status health history")
			if entries, ok := data["entries"].([]any); !ok || len(entries) != 0 {
				t.Fatalf("empty history must be an empty list, got %v", data["entries"])
			}
		} else if out.String() != "No health check history found.\n" {
			t.Fatalf("v1.4 with no history printed %q, want the text line", out.String())
		}
		// With one saved report: bare array in v1.4, entries in v1.5.
		rep := &health.HealthReport{Timestamp: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), Healthy: 1, Total: 1,
			Results: []health.HealthResult{{Service: "postgres", Status: "healthy", Duration: 12 * time.Millisecond}}}
		hdir := filepath.Join(dir, ".nself", "health")
		if err := os.MkdirAll(hdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := health.SaveHistory(rep, hdir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(dir, ".nself")) })
		out.Reset()
		runHistory(t)
		if compat.V15() {
			data := validateEnvelope(t, out.String(), "status health history")
			if entries, _ := data["entries"].([]any); len(entries) != 1 {
				t.Fatalf("entries = %v", data["entries"])
			}
			return
		}
		got, err := health.GetHistory(hdir, 20)
		if err != nil || len(got) != 1 {
			t.Fatalf("history = %v, %v", got, err)
		}
		compareBare(t, out.String(), got)
	})
}

func TestUrlsJSON(t *testing.T) {
	outA := urlsOutput{BaseDomain: "a.test", Env: "dev", RequiredServices: []serviceURL{{Name: "hasura", URL: "https://api.a.test", Group: "Required"}}, TotalRoutes: 1}
	outB := urlsOutput{BaseDomain: "b.test", Env: "prod", RequiredServices: []serviceURL{{Name: "hasura", URL: "https://api.b.test", Group: "Required"}}, TotalRoutes: 1}
	compattest.Both(t, func(t *testing.T) {
		out, _ := pilotBuffers(t)
		if err := emitStateJSON("status urls", outA, urlsData{urlsOutput: outA}); err != nil {
			t.Fatal(err)
		}
		if compat.V15() {
			if data := validateEnvelope(t, out.String(), "status urls"); data["base_domain"] != "a.test" || data["compared"] != nil {
				t.Fatalf("data = %v", data)
			}
		} else {
			compareBare(t, out.String(), outA)
		}
		out.Reset()
		diff := map[string]urlsOutput{"dev": outA, "prod": outB}
		if err := emitStateJSON("status urls", diff, urlsData{urlsOutput: outA, Compared: &urlsCompared{Env: "prod", Listing: outB}}); err != nil {
			t.Fatal(err)
		}
		if compat.V15() {
			data := validateEnvelope(t, out.String(), "status urls")
			if c, _ := data["compared"].(map[string]any); c["env"] != "prod" {
				t.Fatalf("compared = %v", data["compared"])
			}
		} else {
			compareBare(t, out.String(), diff)
		}
	})
}

func TestHelpTopicsJSON(t *testing.T) {
	compattest.Set(t, true)
	out, _ := pilotBuffers(t)
	if err := emitHelpTopics(nil); err != nil {
		t.Fatal(err)
	}
	data := validateEnvelope(t, out.String(), "help topics")
	topics, _ := data["topics"].([]any)
	if len(topics) != len(helpTopicOrder) {
		t.Fatalf("index lists %d topics, want %d", len(topics), len(helpTopicOrder))
	}
	for _, tp := range topics {
		if _, has := tp.(map[string]any)["body"]; has {
			t.Fatalf("the index must not carry bodies: %v", tp)
		}
	}
	out.Reset()
	if err := emitHelpTopics([]string{" Doctor "}); err != nil {
		t.Fatal(err)
	}
	one, _ := validateEnvelope(t, out.String(), "help topics")["topics"].([]any)
	if len(one) != 1 || one[0].(map[string]any)["key"] != "doctor" || !strings.Contains(one[0].(map[string]any)["body"].(string), "nself doctor") {
		t.Fatalf("single topic = %v", one)
	}
	out.Reset()
	if err := emitHelpTopics([]string{"nope"}); err == nil || out.Len() != 0 {
		t.Fatalf("unknown topic: err=%v stdout=%q, want an error and no document", err, out)
	}
}

func TestDoctorImagesJSON(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	present := func(context.Context, string, bool) ([]byte, error) { return []byte("{}"), nil }
	missing := func(context.Context, string, bool) ([]byte, error) { return nil, errors.New("manifest unknown: 404") }
	compattest.Both(t, func(t *testing.T) {
		out, _ := pilotBuffers(t)
		if err := runDoctorImagesJSON(cmd, present); err != nil {
			t.Fatalf("all images present must exit clean: %v", err)
		}
		data := validateEnvelope(t, out.String(), "doctor images")
		images, _ := data["images"].([]any)
		if len(images) == 0 || data["state"] != "ok" {
			t.Fatalf("data = %v", data)
		}
		out.Reset()
		err := runDoctorImagesJSON(cmd, missing)
		var exit *plugin.ExitCodeError
		want := 1
		if compat.V15() {
			want = 10
		}
		if !errors.As(err, &exit) || exit.Code != want {
			t.Fatalf("all images missing: err=%v, want exit %d", err, want)
		}
		if data := validateEnvelope(t, out.String(), "doctor images"); data["state"] != "unhealthy" {
			t.Fatalf("document is still written when images fail: %v", data)
		}
	})
}

func TestDoctorHealJSON(t *testing.T) {
	compattest.Set(t, true)
	logPath := filepath.Join(t.TempDir(), "rotation.log")
	t.Setenv("NSELF_JWT_ROTATION_LOG", logPath)
	t.Setenv("HASURA_GRAPHQL_JWT_SECRET", "00ff")

	out, errOut := pilotBuffers(t)
	if err := runSelfHealJWTJSON(true, ""); err != nil {
		t.Fatal(err)
	}
	data := validateEnvelope(t, out.String(), "doctor heal")
	if data["rotated"] != false || data["dry_run"] != true {
		t.Fatalf("dry run data = %v", data)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("a dry run wrote the rotation log")
	}

	// A real rotation without --to-file is refused before anything rotates.
	out.Reset()
	err := runSelfHealJWTJSON(false, "")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E401" || out.Len() != 0 {
		t.Fatalf("no --to-file: err=%v stdout=%q, want E401 and no document", err, out)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("a refused rotation wrote the rotation log")
	}

	keyFile := filepath.Join(t.TempDir(), "jwt.key")
	if err := runSelfHealJWTJSON(false, keyFile); err != nil {
		t.Fatal(err)
	}
	data = validateEnvelope(t, out.String(), "doctor heal")
	if data["rotated"] != true || data["key_file"] != keyFile || data["grace_until"] == "" {
		t.Fatalf("rotation data = %v", data)
	}
	key, err := os.ReadFile(keyFile)
	if err != nil || len(bytes.TrimSpace(key)) == 0 {
		t.Fatalf("key file: %q, %v", key, err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(keyFile); fi.Mode().Perm() != 0o600 {
			t.Fatalf("key file mode = %v, want 0600", fi.Mode().Perm())
		}
	}
	for name, stream := range map[string]string{"stdout": out.String(), "stderr": errOut.String()} {
		if strings.Contains(stream, strings.TrimSpace(string(key))) {
			t.Fatalf("the new key leaked into %s", name)
		}
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("rotation log missing after a real rotation: %v", err)
	}
}

func TestDoctorBackupHintWiring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	quiet := t.TempDir()
	if got := checkBackupHints(quiet, false); len(got) != 0 {
		t.Fatalf("a project with nothing to say must add no check, got %v", got)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\npg_dump -Fc \"$DB\" > /tmp/x.dump\naws s3 cp /tmp/x.dump s3://bucket/x.dump\n"
	if err := os.WriteFile(filepath.Join(dir, "backup.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got := checkBackupHints(dir, false)
	if len(got) != 1 || got[0].Name != "BACKUP-HINT-01" || got[0].Status != "pass" || !strings.Contains(got[0].Message, "nself backup stream") {
		t.Fatalf("hand-rolled backup not listed: %+v", got)
	}
	// The doctor config check the runner calls carries it.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=hint\nENV=dev\nBASE_DOMAIN=example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range checkPasswordStrength(dir, false, false) {
		found = found || r.Name == "BACKUP-HINT-01"
	}
	if !found {
		t.Fatal("checkPasswordStrength does not list the backup hint")
	}
}
