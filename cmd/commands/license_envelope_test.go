package commands

// Purpose: behaviour tests of the licence envelopes (account fragment,
// P7-SURF-16): one valid v1 envelope per command in v1.5 mode, v1.4 bytes
// unchanged, and the mask test: no fixture licence key, JWT or account token
// appears in any document or golden.
// Inputs: a fake HOME, a ping stub on httptest and the fixture key below.
// Constraints: no network beyond httptest; fixtures are obviously fake; the
// JWT is built at run time so no token-shaped literal sits in the source.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// fixtureLicenseKey is a synthetic key; fixtureAccessToken a synthetic token.
const (
	fixtureLicenseKey  = "nself_pro_abcdefghijklmnopqrstuvwxyz1234"
	fixtureAccessToken = "tok-surf16-access-0123456789abcdef"
)

// licenseFixture is an isolated HOME with a signing ping stub.
type licenseFixture struct {
	jwt  string
	dir  string
	priv ed25519.PrivateKey
}

// newLicenseFixture isolates HOME, the key environment and the cache, and
// serves a signed validation answer on NSELF_PING_API_URL.
func newLicenseFixture(t *testing.T) *licenseFixture {
	t.Helper()
	compattest.Set(t, true)
	isolateHome(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	for i := 1; i <= 10; i++ {
		t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
	}
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "synthetic", Public: pub}}
	t.Cleanup(func() { license.PingKeys = old })
	enc := base64.RawURLEncoding
	unsigned := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`)) + "." + enc.EncodeToString(mustJSON(t, map[string]any{
		"sub": license.HashKey(fixtureLicenseKey), "plugins": []string{"chat"}, "iss": "ping.nself.org", "aud": "nself-cli",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}))
	jwt := unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	body := mustJSON(t, map[string]any{"valid": true, "jwt": jwt, "tier": "plus", "plugins_allowed": []string{"chat"},
		"plugins": []string{"chat"}, "expires": "2027-01-02"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(priv, body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NSELF_PING_API_URL", srv.URL)
	dir := t.TempDir()
	t.Chdir(dir)
	return &licenseFixture{jwt: jwt, dir: dir, priv: priv}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// runLicenseJSON runs cmd's body in v1.5 --json mode with stdout isolated as
// the decorator does, and returns the document stream.
func runLicenseJSON(t *testing.T, cmd *cobra.Command, flags map[string]string, args ...string) (string, error) {
	t.Helper()
	cmd.SetContext(context.Background())
	flags = mergeFlags(map[string]string{"json": "true"}, flags)
	return jsonV15(t, cmd, flags, func() error {
		defer output.IsolateStdout()()
		return cmd.RunE(cmd, args)
	})
}

func mergeFlags(a, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

// noSecrets fails when out holds the fixture key, JWT or token.
func noSecrets(t *testing.T, what, out string, extra ...string) {
	t.Helper()
	for _, s := range append([]string{fixtureLicenseKey, fixtureAccessToken}, extra...) {
		if s != "" && strings.Contains(out, s) {
			t.Errorf("%s leaks %q:\n%s", what, s, out)
		}
	}
}

func TestLicenseEnvelopes(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("LICENSE_ALLOW_SIMULATION", "true")
	masked := license.MaskKey(fixtureLicenseKey)
	step := func(name string, cmd *cobra.Command, flags map[string]string, args ...string) map[string]any {
		t.Helper()
		out, err := runLicenseJSON(t, cmd, flags, args...)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		noSecrets(t, name, out, fx.jwt)
		oneEnvelope(t, out, name)
		t.Chdir(fx.dir)
		var doc struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Data
	}

	set := step("license set", licenseSetCmd, nil, fixtureLicenseKey)
	if set["key"] != masked || set["product"] == "" {
		t.Errorf("set = %v, want key %s", set, masked)
	}
	status := step("license status", licenseStatusCmd, map[string]string{"plugin": "chat"})
	keys, _ := status["keys"].([]any)
	if len(keys) != 1 || keys[0].(map[string]any)["key"] != masked || keys[0].(map[string]any)["status"] != "Active" {
		t.Errorf("status keys = %v", status["keys"])
	}
	if acc, _ := status["plugin_access"].(map[string]any); acc["granted"] != true {
		t.Errorf("plugin_access = %v, want granted", status["plugin_access"])
	}
	if list := step("license list", licenseListCmd, nil); len(list["keys"].([]any)) != 1 {
		t.Errorf("list = %v", list)
	}
	if v := step("license validate", licenseValidateCmd, nil); v["keys"].([]any)[0] != masked {
		t.Errorf("validate = %v", v)
	}
	if show := step("license show", licenseShowCmd, nil); show["configured"] != true || !strings.HasPrefix(show["key_prefix"].(string), "...") {
		t.Errorf("show = %v", show)
	}
	if rv := step("license revalidate", licenseRevalidateCmd, nil); rv["tier"] == "" {
		t.Errorf("revalidate = %v", rv)
	}
	ref := step("license refresh", licenseRefreshCmd, nil)
	if rk := ref["keys"].([]any); ref["mode"] != "keys" || len(rk) != 1 || rk[0].(map[string]any)["refreshed"] != true {
		t.Errorf("refresh = %v", ref)
	}
	if ex := step("license export", licenseExportCmd, nil); ex["signed"] != true || ex["tier"] != "plus" {
		t.Errorf("export = %v", ex)
	}
	if up := step("license upgrade", licenseUpgradeCmd, nil); up["opened"] != false || up["url"] != pricingURL {
		t.Errorf("upgrade = %v", up)
	}
	step("license health", licenseHealthCmd, nil)
	if mig := step("license migrate", licenseMigrateCmd, nil); mig["mode"] != "status" {
		t.Errorf("migrate = %v", mig)
	}
	step("license simulate-offline", licenseSimulateCmd, nil, "5")
	step("license simulate-offline", licenseSimulateCmd, map[string]string{"clear": "true"})
	if rm := step("license remove", licenseRemoveCmd, nil, "pro"); rm["removed"] != float64(1) {
		t.Errorf("remove = %v", rm)
	}
	if rs := step("license restore", licenseRestoreCmd, map[string]string{"key": fixtureLicenseKey}); rs["restored"] != true {
		t.Errorf("restore = %v", rs)
	}
	if rv := step("license revoke", licenseRevokeCmd, nil); rv["revoked"] != false {
		t.Errorf("revoke without --yes = %v, want revoked false", rv)
	}
	if rv := step("license revoke", licenseRevokeCmd, map[string]string{"yes": "true"}); rv["revoked"] != true {
		t.Errorf("revoke = %v", rv)
	}
	added := step("license add", licenseAddCmd, nil, fixtureLicenseKey)
	if a := added["added"].([]any); len(a) != 1 || a[0].(map[string]any)["key"] != masked {
		t.Errorf("add = %v", added)
	}
	step("license clear", licenseClearCmd, nil)
	if empty := step("license status", licenseStatusCmd, nil); len(empty["keys"].([]any)) != 0 {
		t.Errorf("status after clear = %v", empty)
	}
	if none := step("license show", licenseShowCmd, nil); none["configured"] != false {
		t.Errorf("show without a key = %v", none)
	}
	step("license", licenseCmd, nil)
}

// TestLicenseImportExportRoundTrip: import takes the human export and answers
// with dataResult; the file is not echoed beyond its path.
func TestLicenseImportEnvelope(t *testing.T) {
	fx := newLicenseFixture(t)
	if _, err := runLicenseJSON(t, licenseSetCmd, nil, fixtureLicenseKey); err != nil {
		t.Fatal(err)
	}
	blob, err := license.ExportCache()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "cache.json")
	if err := os.WriteFile(file, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runLicenseJSON(t, licenseImportCmd, nil, file)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "license import", out, fx.jwt)
	oneEnvelope(t, out, "license import")
}

// TestLicenseRefreshFailureIsScrubbed: a failing key is a row with an error,
// and the error text carries no key.
func TestLicenseRefreshFailureIsScrubbed(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1/"+fixtureLicenseKey)
	out, err := runLicenseJSON(t, licenseRefreshCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "license refresh", out, fx.jwt)
	doc := oneEnvelope(t, out, "license refresh")
	if !strings.Contains(string(doc["data"]), `"refreshed": false`) || !strings.Contains(string(doc["data"]), `"error"`) {
		t.Errorf("refresh failure row missing:\n%s", out)
	}
}

// reply starts a ping stub that signs body, and points NSELF_PING_API_URL at it.
func (fx *licenseFixture) reply(t *testing.T, body map[string]any) {
	t.Helper()
	raw := mustJSON(t, body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(fx.priv, raw)))
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NSELF_PING_API_URL", srv.URL)
}

// TestLicenseHostileReplyIsScrubbed: a server reason that carries another
// licence key and a JWT reaches the document masked and redacted.
func TestLicenseHostileReplyIsScrubbed(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	other := "nself_plus_zyxwvutsrqponmlkjihgfedcba9876"
	fx.reply(t, map[string]any{"valid": false, "reason": "revoked " + other + " " + fx.jwt + " " + fixtureAccessToken})
	out, err := runLicenseJSON(t, licenseRefreshCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "license refresh", out, fx.jwt, other)
	oneEnvelope(t, out, "license refresh")
	if !strings.Contains(out, license.MaskKey(other)) || !strings.Contains(out, "[REDACTED]") {
		t.Errorf("hostile reason was not masked and redacted:\n%s", out)
	}
}

// TestLicenseMigrateKeyFlagIsScrubbed: an explicit --key that is not
// configured is masked when the transport error echoes the request URL.
func TestLicenseMigrateKeyFlagIsScrubbed(t *testing.T) {
	fx := newLicenseFixture(t)
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1/"+fixtureLicenseKey)
	out, err := runLicenseJSON(t, licenseMigrateCmd, map[string]string{"key": fixtureLicenseKey})
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "license migrate", out, fx.jwt)
	oneEnvelope(t, out, "license migrate")
}

// TestLicenseHealthWithoutKey: the no-key health answer is an envelope too.
func TestLicenseHealthWithoutKey(t *testing.T) {
	newLicenseFixture(t)
	out, err := runLicenseJSON(t, licenseHealthCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc := oneEnvelope(t, out, "license health")
	if !strings.Contains(string(doc["data"]), `"no_key"`) {
		t.Errorf("health without a key: %s", out)
	}
}

// TestLicenseShowV14Bytes: v1.4 keeps the bare pre-contract document.
func TestLicenseShowV14Bytes(t *testing.T) {
	newLicenseFixture(t)
	compattest.Set(t, false)
	resetRegistryCache()
	t.Setenv("NSELF_LICENSE_KEY_1", fixtureLicenseKey)
	licenseShowCmd.SetContext(context.Background())
	if err := licenseShowCmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = licenseShowCmd.Flags().Set("json", "false") })
	out, err := captureStdout(t, func() error { return licenseShowCmd.RunE(licenseShowCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	// Exact bytes: the output is the pre-contract struct, encoded as v1.4 does.
	var legacy licenseShowJSON
	dec := json.NewDecoder(strings.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&legacy); err != nil {
		t.Fatalf("v1.4 output is not the pre-contract document: %v\n%s", err, out)
	}
	var again bytes.Buffer
	if err := json.NewEncoder(&again).Encode(legacy); err != nil || again.String() != out {
		t.Fatalf("v1.4 show --json bytes changed:\n got %q\nwant %q", out, again.String())
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("v1.4 output is not bare JSON: %v\n%s", err, out)
	}
	if _, enveloped := doc["schema_version"]; enveloped || doc["tier"] == nil || doc["key_prefix"] == nil {
		t.Errorf("v1.4 show --json = %s, want the bare document", out)
	}
	noSecrets(t, "v1.4 license show", out)
}

// TestLicenseMaskedInGoldens is the mask test: no committed golden holds the
// fixture key, token or a JWT-shaped value.
func TestLicenseMaskedInGoldens(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "json", "license*.golden.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no licence goldens found: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		noSecrets(t, f, string(b))
		if strings.Contains(string(b), "eyJ") {
			t.Errorf("%s holds a JWT-shaped value", f)
		}
	}
}
