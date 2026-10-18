package commands

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
	"github.com/nself-org/cli/internal/maintenance"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const graceKey = "synthetic-license-key"

// graceFixture is a synthetic ping, a HOME with one licensed plugin and a project with a .env.
type graceFixture struct {
	mode    string // ok, 5xx, invalid, revoked, badjwt
	priv    ed25519.PrivateKey
	other   ed25519.PrivateKey
	jwt     string // the token the last ok reply carried
	project string
}

func newGraceFixture(t *testing.T) *graceFixture {
	t.Helper()
	compattest.Set(t, true)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", graceKey)
	for i := 1; i <= 10; i++ {
		t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
	}
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	old := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "synthetic", Public: pub}}
	t.Cleanup(func() { license.PingKeys = old })
	f := &graceFixture{mode: "ok", priv: priv, other: other, project: t.TempDir()}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv("NSELF_PING_API_URL", server.URL)
	plugin := filepath.Join(home, ".nself", "plugins", "chat")
	for path, data := range map[string]string{
		filepath.Join(plugin, "plugin.json"): `{"manifest_version":2,"license":"licensed","name":"chat"}`,
		filepath.Join(f.project, ".env"):     "PROJECT_NAME=grace\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *graceFixture) token(priv ed25519.PrivateKey, exp time.Time) string {
	enc := base64.RawURLEncoding
	claims, _ := json.Marshal(map[string]any{"sub": license.HashKey(graceKey), "tier": "plus", "plugins": []string{"chat"},
		"iss": "ping.nself.org", "aud": "nself-cli", "iat": time.Now().Unix(), "exp": exp.Unix()})
	unsigned := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`)) + "." + enc.EncodeToString(claims)
	return unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
}

func (f *graceFixture) serve(w http.ResponseWriter, _ *http.Request) {
	var body []byte
	switch f.mode {
	case "5xx":
		w.WriteHeader(http.StatusBadGateway)
		return
	case "invalid":
		body, _ = json.Marshal(map[string]any{"valid": false, "reason": "unknown license key"})
	case "revoked":
		body, _ = json.Marshal(map[string]any{"valid": false, "reason": "license revoked"})
	case "badjwt":
		body, _ = json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": []string{"chat"}, "jwt": f.token(f.other, time.Now().Add(time.Hour))})
	default:
		f.jwt = f.token(f.priv, time.Now().Add(time.Hour))
		body, _ = json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": []string{"chat"}, "jwt": f.jwt})
	}
	w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(f.priv, body)))
	_, _ = w.Write(body)
}

// runRefresh runs `license refresh <flags>` from cwd and returns stdout, stderr and the error.
func runRefresh(t *testing.T, cwd string, flags map[string]string) (string, string, error) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	licenseRefreshCmd.Flags().VisitAll(func(fl *pflag.Flag) { _ = fl.Value.Set(fl.DefValue); fl.Changed = false })
	for k, v := range flags {
		if err := licenseRefreshCmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	licenseRefreshCmd.SetOut(&out)
	licenseRefreshCmd.SetErr(&errOut)
	licenseRefreshCmd.SetContext(t.Context())
	err := licenseRefreshCmd.RunE(licenseRefreshCmd, nil)
	return out.String(), errOut.String(), err
}

func TestLicenseRefreshQuietGolden(t *testing.T) {
	f := newGraceFixture(t)
	exp := regexp.MustCompile(`^entitlement refreshed exp=\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ\n$`)
	cases := []struct{ mode, stdout, stderr string }{
		{"ok", "", ""},
		{"5xx", "", "entitlement refresh failed: ping-unavailable\n"},
		{"invalid", "", "entitlement refresh failed: licence-rejected\n"},
		{"revoked", "", "entitlement refresh failed: licence-rejected\n"},
		{"badjwt", "", "entitlement refresh failed: token-verify\n"},
	}
	var secrets []string
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			f.mode = tc.mode
			stdout, stderr, err := runRefresh(t, f.project, map[string]string{"project": f.project, "quiet": "true"})
			if tc.mode == "ok" {
				data, _ := os.ReadFile(entitlement.Path(f.project))
				if err != nil || !exp.MatchString(stdout) || stderr != "" || string(data) != f.jwt {
					t.Fatalf("ok: err %v stdout %q stderr %q", err, stdout, stderr)
				}
				secrets = append(secrets, f.jwt, graceKey, license.MaskKey(graceKey), license.HashKey(graceKey))
				secrets = append(secrets, strings.Split(f.jwt, ".")...)
			} else if stdout != tc.stdout || stderr != tc.stderr || errs.ExitCodeFor(err) != 1 {
				t.Fatalf("stdout %q stderr %q exit %d", stdout, stderr, errs.ExitCodeFor(err))
			}
			for _, s := range secrets {
				if s != "" && strings.Contains(stdout+stderr, s) {
					t.Fatalf("output contains secret material %q", s[:6])
				}
			}
		})
	}
}

func TestLicenseRefreshRewritesFile(t *testing.T) {
	f := newGraceFixture(t)
	// The timer path and the interactive path both rewrite the file once the new JWT verifies.
	for name, flags := range map[string]map[string]string{"timer": {"project": f.project, "quiet": "true"}, "interactive": nil} {
		t.Run(name, func(t *testing.T) {
			f.mode = "ok"
			_ = os.Remove(entitlement.Path(f.project))
			if _, _, err := runRefresh(t, f.project, flags); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(entitlement.Path(f.project))
			info, _ := os.Stat(entitlement.Path(f.project))
			if string(data) != f.jwt || info.Mode().Perm() != 0o600 {
				t.Fatalf("file not rewritten: mode %v", info.Mode())
			}
			old := string(data)
			for _, mode := range []string{"5xx", "revoked", "badjwt"} {
				f.mode = mode
				if _, _, err := runRefresh(t, f.project, flags); err != nil && name == "interactive" {
					t.Fatal(err)
				}
				if again, _ := os.ReadFile(entitlement.Path(f.project)); string(again) != old {
					t.Fatalf("%s refresh changed the old file", mode)
				}
			}
		})
	}
}

func TestLicenseImportRewritesJWT(t *testing.T) {
	f := newGraceFixture(t)
	if _, _, err := runRefresh(t, f.project, nil); err != nil {
		t.Fatal(err)
	}
	exported, err := license.ExportCache()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "export.json")
	if err := os.WriteFile(file, exported, 0o600); err != nil {
		t.Fatal(err)
	}
	f.mode = "5xx" // air-gapped: no ping
	_ = os.Remove(entitlement.Path(f.project))
	_ = os.Remove(os.Getenv("LICENSE_CACHE_PATH"))
	if err := os.Chdir(f.project); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().String("project", "", "")
	cmd.Flags().Bool("force", false, "")
	if err := licenseImportCmd.RunE(cmd, []string{file}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(entitlement.Path(f.project)); string(data) != f.jwt {
		t.Fatalf("import did not rewrite entitlement.jwt (%d bytes)", len(data))
	}
}

func TestRemoveTimerFlag(t *testing.T) {
	f := newGraceFixture(t)
	var gotDir string
	var gotAll bool
	res := maintenance.RemoveResult{Removed: true}
	old := removeTimers
	removeTimers = func(dir string, all bool) (maintenance.RemoveResult, error) {
		gotDir, gotAll = dir, all
		return res, nil
	}
	t.Cleanup(func() { removeTimers = old })

	stdout, _, err := runRefresh(t, t.TempDir(), map[string]string{"remove-timer": "true", "project": f.project})
	if err != nil || gotDir != f.project || gotAll || !strings.Contains(stdout, "removed") {
		t.Fatalf("--project: err %v dir %q all %v out %q", err, gotDir, gotAll, stdout)
	}
	if _, _, err = runRefresh(t, t.TempDir(), map[string]string{"remove-timer": "true", "all": "true"}); err != nil || !gotAll {
		t.Fatalf("--all: err %v all %v", err, gotAll)
	}
	if _, _, err = runRefresh(t, t.TempDir(), map[string]string{"remove-timer": "true"}); err == nil {
		t.Fatal("outside a project without --project or --all must fail")
	}
	res = maintenance.RemoveResult{RootRequired: true}
	_, _, err = runRefresh(t, f.project, map[string]string{"remove-timer": "true"})
	if err == nil || !strings.Contains(err.Error(), "sudo nself license refresh --remove-timer --project "+f.project) {
		t.Fatalf("root units must print the sudo command, got %v", err)
	}
	res = maintenance.RemoveResult{}
	if out, _, err := runRefresh(t, f.project, map[string]string{"remove-timer": "true"}); err != nil || !strings.Contains(out, "no entitlement refresh timer") {
		t.Fatalf("nothing to remove: %v %q", err, out)
	}
}

func TestScheduledRefreshExitCodes(t *testing.T) {
	f := newGraceFixture(t)
	old := runScheduledRefresh
	t.Cleanup(func() { runScheduledRefresh = old })
	for name, tc := range map[string]struct {
		out    entitlement.Outcome
		err    error
		code   int
		stdout string
	}{
		"root systemd units remain": {out: entitlement.Outcome{RootRequired: true, Reason: "x"}, code: 78},
		"cron timer removed":        {out: entitlement.Outcome{Removed: true, Reason: "no licensed plugin installed"}, stdout: "entitlement refresh timer removed (no licensed plugin installed)\n"},
		"class failure":             {err: &entitlement.RefreshError{Class: "ping-unavailable"}, code: 1},
		"internal failure":          {err: errors.New("boom /secret/path"), code: 1},
	} {
		t.Run(name, func(t *testing.T) {
			runScheduledRefresh = func(_ context.Context, _ string) (entitlement.Outcome, error) { return tc.out, tc.err }
			stdout, stderr, err := runRefresh(t, f.project, map[string]string{"project": f.project, "quiet": "true"})
			if errs.ExitCodeFor(err) != tc.code || stdout != tc.stdout || strings.Contains(stderr, "secret") {
				t.Fatalf("exit %d stdout %q stderr %q", errs.ExitCodeFor(err), stdout, stderr)
			}
		})
	}
}

// TestAutorefreshDisabledParity keeps the entitlement package's opt-out rule equal to the
// maintenance one (the entitlement package may not import maintenance).
func TestAutorefreshDisabledParity(t *testing.T) {
	for _, v := range []string{"", "false", "FALSE", " false ", "0", "no", "off", "true", "1", "yes", "garbage"} {
		t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", v)
		if entitlement.AutorefreshDisabled() != maintenance.AutorefreshDisabled() {
			t.Fatalf("value %q: entitlement=%v maintenance=%v", v, entitlement.AutorefreshDisabled(), maintenance.AutorefreshDisabled())
		}
	}
	if entitlement.HostTimers.Ensure == nil || entitlement.HostTimers.Remove == nil {
		t.Fatal("cmd/commands did not wire entitlement.HostTimers")
	}
}
