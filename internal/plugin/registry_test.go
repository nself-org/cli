package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Bug #64 — nself plugin install fails with registry format error
//
// Root cause (v0.x): install command failed when the live registry returned
// plugins as a JSON object map ({"plugin_name": {...}}) instead of an array
// ([{...}]). Only the array format was handled.
//
// Go rewrite fix: parseRegistryJSON detects the first non-whitespace byte of
// the plugins field. '[' → array format (pro registry), '{' → object format
// (free / live registry). Both formats are parsed and normalised into
// []PluginManifest.
//
// These tests verify that both formats parse correctly and that unknown formats
// return a descriptive error — not a silent empty list.
// ---------------------------------------------------------------------------

// TestParseRegistryJSON_ArrayFormat verifies that the pro-registry array format
// is parsed correctly and all plugin fields are preserved.
func TestParseRegistryJSON_ArrayFormat(t *testing.T) {
	input := `{
		"version": "1.0.0",
		"plugins": [
			{
				"name": "notify",
				"version": "1.0.0",
				"description": "Push notifications",
				"category": "messaging",
				"tier": "basic",
				"license": "source-available",
				"repository": "https://github.com/nself-org/bundles",
				"checksum": "abc123"
			}
		]
	}`

	reg, err := parseRegistryJSON([]byte(input))
	if err != nil {
		t.Fatalf("parseRegistryJSON (array format): %v", err)
	}
	if len(reg.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(reg.Plugins))
	}
	if reg.Plugins[0].Name != "notify" {
		t.Errorf("expected plugin name %q, got %q", "notify", reg.Plugins[0].Name)
	}
	if reg.Plugins[0].Tier != "basic" {
		t.Errorf("expected tier %q, got %q", "basic", reg.Plugins[0].Tier)
	}
}

// TestParseRegistryJSON_ObjectFormat verifies that the free/live-registry object
// format is parsed correctly. This is the format returned by plugins.nself.org
// (Cloudflare Worker). A missing "name" field in the object value must be
// backfilled from the map key.
func TestParseRegistryJSON_ObjectFormat(t *testing.T) {
	input := `{
		"version": "1.0.0",
		"plugins": {
			"redis": {
				"version": "1.0.0",
				"description": "Redis caching",
				"category": "caching",
				"tier": "free",
				"license": "MIT",
				"repository": "https://github.com/nself-org/plugins",
				"checksum": "def456"
			},
			"search": {
				"name": "search",
				"version": "1.1.0",
				"description": "MeiliSearch full-text search",
				"category": "search",
				"tier": "free",
				"license": "MIT",
				"repository": "https://github.com/nself-org/plugins",
				"checksum": "ghi789"
			}
		}
	}`

	reg, err := parseRegistryJSON([]byte(input))
	if err != nil {
		t.Fatalf("Bug #64 regression: parseRegistryJSON (object format) failed: %v", err)
	}
	if len(reg.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(reg.Plugins))
	}

	// Build a name → plugin map for deterministic lookup (map iteration order varies).
	byName := make(map[string]PluginManifest, len(reg.Plugins))
	for _, p := range reg.Plugins {
		byName[p.Name] = p
	}

	redisPlugin, ok := byName["redis"]
	if !ok {
		t.Fatalf("expected plugin %q (backfilled from key), not found in: %v", "redis", byName)
	}
	if redisPlugin.Tier != "free" {
		t.Errorf("expected tier %q for redis, got %q", "free", redisPlugin.Tier)
	}

	searchPlugin, ok := byName["search"]
	if !ok {
		t.Fatalf("expected plugin %q not found", "search")
	}
	if searchPlugin.Version != "1.1.0" {
		t.Errorf("expected version %q for search, got %q", "1.1.0", searchPlugin.Version)
	}
}

// TestParseRegistryJSON_ObjectFormat_NameBackfill verifies that when the plugin
// value object does not include a "name" field, it is backfilled from the map key.
func TestParseRegistryJSON_ObjectFormat_NameBackfill(t *testing.T) {
	input := `{
		"plugins": {
			"myplugin": {
				"version": "2.0.0",
				"description": "A plugin without name in body",
				"category": "utility",
				"tier": "free",
				"license": "MIT"
			}
		}
	}`

	reg, err := parseRegistryJSON([]byte(input))
	if err != nil {
		t.Fatalf("parseRegistryJSON: %v", err)
	}
	if len(reg.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(reg.Plugins))
	}
	if reg.Plugins[0].Name != "myplugin" {
		t.Errorf("name not backfilled from key: got %q, want %q", reg.Plugins[0].Name, "myplugin")
	}
}

// TestParseRegistryJSON_EmptyPlugins verifies that an empty plugins field
// (null or absent) returns an empty Registry without error.
func TestParseRegistryJSON_EmptyPlugins(t *testing.T) {
	inputs := []string{
		`{"version":"1.0.0","plugins":[]}`,
		`{"version":"1.0.0","plugins":null}`,
		`{"version":"1.0.0"}`,
	}
	for _, input := range inputs {
		reg, err := parseRegistryJSON([]byte(input))
		if err != nil {
			t.Errorf("parseRegistryJSON(%q): unexpected error: %v", input, err)
			continue
		}
		if reg == nil {
			t.Errorf("parseRegistryJSON(%q): returned nil registry", input)
		}
	}
}

// TestParseRegistryJSON_UnknownFormat verifies that an unrecognised plugins
// field format (e.g. a bare number or string) returns a descriptive error
// rather than silently returning an empty registry or panicking.
func TestParseRegistryJSON_UnknownFormat(t *testing.T) {
	input := `{"plugins": 42}`
	_, err := parseRegistryJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown registry format, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected registry plugins format") {
		t.Errorf("expected 'unexpected registry plugins format' in error, got: %v", err)
	}
}

// TestParseAPIEndpoints_StringArray verifies that the legacy string-array
// endpoint format is parsed without error.
func TestParseAPIEndpoints_StringArray(t *testing.T) {
	raw := mustMarshalJSON([]string{"/api/v1/foo", "/api/v1/bar"})
	endpoints := parseAPIEndpoints(raw)
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d: %v", len(endpoints), endpoints)
	}
	if endpoints[0] != "/api/v1/foo" {
		t.Errorf("endpoint[0]: got %q, want %q", endpoints[0], "/api/v1/foo")
	}
}

// TestParseAPIEndpoints_ObjectArray verifies that the live-registry
// object-array endpoint format is normalised to []string of paths.
func TestParseAPIEndpoints_ObjectArray(t *testing.T) {
	raw, _ := json.Marshal([]pluginEndpointEntry{
		{Method: "GET", Path: "/v1/health", Description: "health check"},
		{Method: "POST", Path: "/v1/send", Description: "send notification"},
	})
	endpoints := parseAPIEndpoints(json.RawMessage(raw))
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d: %v", len(endpoints), endpoints)
	}
	if endpoints[0] != "/v1/health" {
		t.Errorf("endpoint[0]: got %q, want %q", endpoints[0], "/v1/health")
	}
	if endpoints[1] != "/v1/send" {
		t.Errorf("endpoint[1]: got %q, want %q", endpoints[1], "/v1/send")
	}
}

// TestParseDependencies_AllShapes verifies that the registry's various
// `dependencies` shapes are all normalised to a flat []string of plugin names.
// Regression: chain-b705583a — v1.0.9 client crashed on the live registry's
// {"required":[],"optional":[]} object form because dependencies was typed
// as []string.
func TestParseDependencies_AllShapes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", ``, nil},
		{"null", `null`, nil},
		{"empty array", `[]`, nil},
		{"string array", `["redis","auth"]`, []string{"redis", "auth"}},
		{"required+optional", `{"required":["a","b"],"optional":["c"]}`, []string{"a", "b", "c"}},
		{"required only", `{"required":["a"]}`, []string{"a"}},
		{"empty required+optional", `{"required":[],"optional":[]}`, nil},
		{"plugins key", `{"plugins":["family","photos"]}`, []string{"family", "photos"}},
		{"npm only ignored", `{"npm":["axios"]}`, nil},
		{"system only ignored", `{"system":["ffmpeg"]}`, nil},
		{"mixed npm+required", `{"required":["a"],"npm":["axios"]}`, []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDependencies(json.RawMessage(tc.in))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseDependencies(%q): got %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseRegistryJSON_LiveRegistryDependencyShapes verifies that the full
// registry parse pipeline accepts a payload mixing every dependency shape the
// live registry has shipped, without erroring.
func TestParseRegistryJSON_LiveRegistryDependencyShapes(t *testing.T) {
	payload := []byte(`{
		"version": "1.0.0",
		"plugins": [
			{"name":"a","version":"1.0.0","dependencies":[]},
			{"name":"b","version":"1.0.0","dependencies":{"required":[],"optional":[]}},
			{"name":"c","version":"1.0.0","dependencies":{"npm":["axios","pg"]}},
			{"name":"d","version":"1.0.0","dependencies":{"plugins":["family","photos"]}},
			{"name":"e","version":"1.0.0","dependencies":{"required":["redis"]}}
		]
	}`)
	reg, err := parseRegistryJSON(payload)
	if err != nil {
		t.Fatalf("parseRegistryJSON: unexpected error: %v", err)
	}
	if len(reg.Plugins) != 5 {
		t.Fatalf("got %d plugins, want 5", len(reg.Plugins))
	}
	byName := map[string][]string{}
	for _, p := range reg.Plugins {
		byName[p.Name] = p.Dependencies
	}
	if got := byName["d"]; !reflect.DeepEqual(got, []string{"family", "photos"}) {
		t.Errorf("d.Dependencies: got %v, want [family photos]", got)
	}
	if got := byName["e"]; !reflect.DeepEqual(got, []string{"redis"}) {
		t.Errorf("e.Dependencies: got %v, want [redis]", got)
	}
	if got := byName["c"]; got != nil {
		t.Errorf("c.Dependencies: got %v, want nil (npm-only)", got)
	}
}

// mustMarshalJSON is a test helper that marshals v to json.RawMessage, panicking
// on error (should never happen with well-formed test data).
func mustMarshalJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestEntryToManifestLicenseType(t *testing.T) {
	e := pluginEntry{
		Name:        "test",
		Version:     "1.0.0",
		Description: "desc",
		Category:    "cat",
		License:     "MIT",
		LicenseType: "pro",
		Tier:        "pro",
	}
	m := entryToManifest(e)
	if m.LicenseType != "pro" {
		t.Errorf("expected LicenseType=pro, got %q", m.LicenseType)
	}
}

func TestEntryToManifestPopulatesRegistryFields(t *testing.T) {
	// Only checks that the fields pluginEntry CAN populate are actually populated.
	e := pluginEntry{
		Name:            "myplugin",
		Version:         "2.0.0",
		Description:     "A plugin",
		Category:        "auth",
		Tier:            "basic",
		License:         "MIT",
		LicenseType:     "free",
		Repository:      "https://github.com/test/test",
		Checksum:        "abc123",
		RequiresLicense: true,
		Tags:            []string{"test"},
		Tables:          []string{"np_test_foo"},
		Port:            9000,
		Dependencies:    mustMarshalJSON([]string{"other"}),
		APIEndpoints:    mustMarshalJSON([]string{"/api/v1"}),
	}
	m := entryToManifest(e)

	checks := map[string]bool{
		"Name":            m.Name == e.Name,
		"Version":         m.Version == e.Version,
		"Description":     m.Description == e.Description,
		"Category":        m.Category == e.Category,
		"License":         m.License == e.License,
		"LicenseType":     m.LicenseType == e.LicenseType,
		"Repository":      m.Repository == e.Repository,
		"Checksum":        m.Checksum == e.Checksum,
		"RequiresLicense": m.RequiresLicense == e.RequiresLicense,
		"Port":            m.Port == e.Port,
	}
	for field, ok := range checks {
		if !ok {
			t.Errorf("field %s not correctly mapped", field)
		}
	}

	// Ensure reflect is used (satisfies import).
	_ = reflect.TypeOf(m)
}

// TestEntryToManifest_AllFieldsCopied verifies that entryToManifest copies all
// extended fields (Tables, Port, Dependencies, APIEndpoints) from a pluginEntry
// into the returned PluginManifest without loss.
func TestEntryToManifest_AllFieldsCopied(t *testing.T) {
	e := pluginEntry{
		Name:         "myplugin",
		Version:      "1.0.0",
		Description:  "A test plugin",
		Category:     "utility",
		Tier:         "free",
		License:      "MIT",
		Repository:   "https://github.com/example/myplugin",
		Checksum:     "abc123",
		Tables:       []string{"np_myplugin_items"},
		Port:         8080,
		Dependencies: mustMarshalJSON([]string{"redis"}),
		APIEndpoints: mustMarshalJSON([]string{"/api/v1"}),
		Tags:         []string{"test"},
	}

	m := entryToManifest(e)

	if len(m.Tables) != 1 || m.Tables[0] != "np_myplugin_items" {
		t.Errorf("Tables not copied correctly: got %v, want [np_myplugin_items]", m.Tables)
	}

	if m.Port != 8080 {
		t.Errorf("Port not copied correctly: got %d, want 8080", m.Port)
	}

	if len(m.Dependencies) != 1 || m.Dependencies[0] != "redis" {
		t.Errorf("Dependencies not copied correctly: got %v, want [redis]", m.Dependencies)
	}

	if len(m.APIEndpoints) != 1 || m.APIEndpoints[0] != "/api/v1" {
		t.Errorf("APIEndpoints not copied correctly: got %v, want [/api/v1]", m.APIEndpoints)
	}

	// Verify core fields are also present.
	if m.Name != "myplugin" {
		t.Errorf("Name not copied correctly: got %q, want %q", m.Name, "myplugin")
	}
	if m.Tier != "free" {
		t.Errorf("Tier not set correctly: got %q, want %q", m.Tier, "free")
	}
}

// TestRegistryMarshalJSON_CacheRoundTrip verifies that MarshalJSON preserves
// Category, License, LicenseType, and RequiresLicense so that a cache write
// followed by a read-back returns the same values. Previously these fields
// were omitted from the cache envelope, causing nself plugin list to show
// empty categories and incorrect tier derivation after the first cache write.
func TestRegistryMarshalJSON_CacheRoundTrip(t *testing.T) {
	original := Registry{
		Plugins: []PluginManifest{
			{
				Name:            "stripe",
				Version:         "1.0.0",
				Description:     "Stripe payments",
				Category:        "commerce",
				License:         "MIT",
				LicenseType:     "free",
				RequiresLicense: false,
				Tier:            "free",
				Repository:      "https://github.com/nself-org/plugins",
				Checksum:        "abc123",
				Tags:            []string{"payments"},
				Tables:          []string{"np_stripe_customers"},
				Port:            3100,
				Dependencies:    []string{"webhooks"},
			},
		},
	}

	// Serialize (simulates writeCache).
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Deserialize (simulates readCache via parseRegistryJSON).
	got, err := parseRegistryJSON(data)
	if err != nil {
		t.Fatalf("parseRegistryJSON after marshal: %v", err)
	}
	if len(got.Plugins) != 1 {
		t.Fatalf("expected 1 plugin after round-trip, got %d", len(got.Plugins))
	}

	p := got.Plugins[0]

	if p.Category != "commerce" {
		t.Errorf("Category lost in cache round-trip: got %q, want %q", p.Category, "commerce")
	}
	if p.License != "MIT" {
		t.Errorf("License lost in cache round-trip: got %q, want %q", p.License, "MIT")
	}
	if p.LicenseType != "free" {
		t.Errorf("LicenseType lost in cache round-trip: got %q, want %q", p.LicenseType, "free")
	}
	if p.RequiresLicense != false {
		t.Errorf("RequiresLicense lost in cache round-trip: got %v, want false", p.RequiresLicense)
	}
	if p.Tier != "free" {
		t.Errorf("Tier lost in cache round-trip: got %q, want %q", p.Tier, "free")
	}
}

// BenchmarkRegistryParse measures the time to parse a registry JSON payload
// representative of the free plugin registry (~29 entries). This benchmark
// is used by the nightly-registry-perf workflow to track parse latency.
func BenchmarkRegistryParse(b *testing.B) {
	// Build a realistic registry JSON with multiple plugins (object format).
	const registryPayload = `{
		"version": "1.0.0",
		"plugins": {
			"ai": {"name": "ai", "version": "1.2.0", "description": "AI plugin", "category": "ai", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc001"},
			"claw": {"name": "claw", "version": "1.0.0", "description": "Claw memory plugin", "category": "ai", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc002"},
			"mux": {"name": "mux", "version": "1.1.0", "description": "Email routing plugin", "category": "messaging", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc003"},
			"notify": {"name": "notify", "version": "1.0.3", "description": "Push notifications", "category": "messaging", "tier": "basic", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc004"},
			"voice": {"name": "voice", "version": "1.0.1", "description": "Voice synthesis", "category": "ai", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc005"},
			"browser": {"name": "browser", "version": "1.0.0", "description": "Browser automation", "category": "tools", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc006"},
			"google": {"name": "google", "version": "1.2.1", "description": "Google Workspace integration", "category": "integration", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc007"},
			"cron": {"name": "cron", "version": "1.0.0", "description": "Scheduled tasks", "category": "automation", "tier": "basic", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc008"},
			"chat": {"name": "chat", "version": "1.1.0", "description": "Messaging plugin", "category": "messaging", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc009"},
			"livekit": {"name": "livekit", "version": "1.0.2", "description": "Live video/audio", "category": "media", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc010"},
			"recording": {"name": "recording", "version": "1.0.0", "description": "Call recording", "category": "media", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc011"},
			"moderation": {"name": "moderation", "version": "1.0.0", "description": "Content moderation", "category": "safety", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc012"},
			"media-processing": {"name": "media-processing", "version": "1.1.0", "description": "Media transcoding", "category": "media", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc013"},
			"streaming": {"name": "streaming", "version": "1.0.0", "description": "IPTV streaming", "category": "media", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc014"},
			"epg": {"name": "epg", "version": "1.0.0", "description": "Electronic program guide", "category": "media", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc015"},
			"social": {"name": "social", "version": "1.0.0", "description": "Social feed plugin", "category": "social", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc016"},
			"photos": {"name": "photos", "version": "1.0.0", "description": "Photo sharing", "category": "social", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc017"},
			"geolocation": {"name": "geolocation", "version": "1.0.0", "description": "Location services", "category": "tools", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc018"},
			"calendar": {"name": "calendar", "version": "1.0.0", "description": "Calendar integration", "category": "productivity", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc019"},
			"realtime": {"name": "realtime", "version": "1.1.0", "description": "WebSocket realtime", "category": "infrastructure", "tier": "basic", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc020"},
			"auth": {"name": "auth", "version": "2.0.0", "description": "Authentication", "category": "security", "tier": "free", "license": "MIT", "repository": "https://github.com/nself-org/plugins", "checksum": "abc021"},
			"scan": {"name": "scan", "version": "1.0.0", "description": "Security scanning", "category": "security", "tier": "free", "license": "MIT", "repository": "https://github.com/nself-org/plugins", "checksum": "abc022"},
			"cms": {"name": "cms", "version": "1.0.0", "description": "Content management", "category": "content", "tier": "basic", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc023"},
			"stripe": {"name": "stripe", "version": "1.0.0", "description": "Stripe payments", "category": "commerce", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc024"},
			"sms": {"name": "sms", "version": "1.0.0", "description": "SMS messaging", "category": "messaging", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc025"},
			"image": {"name": "image", "version": "1.0.0", "description": "Image processing", "category": "media", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc026"},
			"pdf": {"name": "pdf", "version": "1.0.0", "description": "PDF generation", "category": "documents", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc027"},
			"knowledge-base": {"name": "knowledge-base", "version": "1.0.0", "description": "Vector knowledge base", "category": "ai", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc028"},
			"mcp": {"name": "mcp", "version": "1.0.0", "description": "Model Context Protocol", "category": "ai", "tier": "pro", "license": "source-available", "repository": "https://github.com/nself-org/bundles", "checksum": "abc029"}
		}
	}`

	data := []byte(registryPayload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseRegistryJSON(data)
		if err != nil {
			b.Fatalf("parseRegistryJSON: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// CLI-R16 — plugin freshness
//
// The live plugins.nself.org registry (inspected 2026-08-23) carries a
// top-level "fetchedAt" snapshot timestamp but no per-plugin "updated_at"
// field. These tests lock in: (1) fetchedAt is parsed and preserved through
// the cache round-trip, (2) a per-plugin updated_at IS plumbed through when a
// registry entry provides one (forward-compat), and (3) its absence produces
// an empty string, never a fabricated value.
// ---------------------------------------------------------------------------

// TestParseRegistryJSON_FetchedAt verifies the registry-wide "fetchedAt"
// field is captured on Registry, and that its absence yields "".
func TestParseRegistryJSON_FetchedAt(t *testing.T) {
	withFetchedAt := `{"version":"1.0.0","fetchedAt":"2026-08-23T22:00:19.502Z","plugins":[{"name":"audit-log","version":"1.0.0"}]}`
	reg, err := parseRegistryJSON([]byte(withFetchedAt))
	if err != nil {
		t.Fatalf("parseRegistryJSON: %v", err)
	}
	if reg.FetchedAt != "2026-08-23T22:00:19.502Z" {
		t.Errorf("FetchedAt = %q, want the live registry's snapshot timestamp", reg.FetchedAt)
	}

	withoutFetchedAt := `{"version":"1.0.0","plugins":[{"name":"audit-log","version":"1.0.0"}]}`
	reg2, err := parseRegistryJSON([]byte(withoutFetchedAt))
	if err != nil {
		t.Fatalf("parseRegistryJSON: %v", err)
	}
	if reg2.FetchedAt != "" {
		t.Errorf("FetchedAt = %q, want empty when the source registry doesn't send one", reg2.FetchedAt)
	}
}

// TestEntryToManifest_UpdatedAtCopied verifies a per-plugin "updated_at" is
// plumbed through to PluginManifest.UpdatedAt when present, and stays empty
// (never invented) when absent — matching today's live registry payload.
func TestEntryToManifest_UpdatedAtCopied(t *testing.T) {
	withUpdatedAt := entryToManifest(pluginEntry{Name: "audit-log", Version: "1.0.0", UpdatedAt: "2026-08-01T00:00:00Z"})
	if withUpdatedAt.UpdatedAt != "2026-08-01T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want the entry's updated_at value", withUpdatedAt.UpdatedAt)
	}

	withoutUpdatedAt := entryToManifest(pluginEntry{Name: "audit-log", Version: "1.0.0"})
	if withoutUpdatedAt.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty when the registry entry has no updated_at", withoutUpdatedAt.UpdatedAt)
	}
}

// TestRegistryMarshalJSON_PreservesFetchedAtAndUpdatedAt extends the existing
// cache round-trip coverage to the two CLI-R16 freshness fields.
func TestRegistryMarshalJSON_PreservesFetchedAtAndUpdatedAt(t *testing.T) {
	reg := Registry{
		FetchedAt: "2026-08-23T22:00:19.502Z",
		Plugins: []PluginManifest{
			{Name: "audit-log", Version: "1.0.0", UpdatedAt: "2026-08-01T00:00:00Z"},
		},
	}

	data, err := reg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	got, err := parseRegistryJSON(data)
	if err != nil {
		t.Fatalf("parseRegistryJSON after marshal: %v", err)
	}

	if got.FetchedAt != reg.FetchedAt {
		t.Errorf("FetchedAt lost in cache round-trip: got %q, want %q", got.FetchedAt, reg.FetchedAt)
	}
	if len(got.Plugins) != 1 || got.Plugins[0].UpdatedAt != "2026-08-01T00:00:00Z" {
		t.Errorf("UpdatedAt lost in cache round-trip: got %+v", got.Plugins)
	}
}

// TestRegistryFetchedAt_NoCache verifies RegistryFetchedAt returns "" rather
// than erroring when no registry cache file exists yet.
func TestRegistryFetchedAt_NoCache(t *testing.T) {
	setTestHome(t, t.TempDir())
	if got := RegistryFetchedAt(); got != "" {
		t.Errorf("RegistryFetchedAt() = %q, want empty with no cache file", got)
	}
}

// TestRegistryFetchedAt_ReadsCache verifies RegistryFetchedAt reads the
// top-level fetchedAt back out of the on-disk registry cache written by
// writeCache, without making a network call.
func TestRegistryFetchedAt_ReadsCache(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	cacheDir := filepath.Join(home, ".nself", "cache", "plugins")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	payload := `{"version":"1.0.0","fetchedAt":"2026-08-23T22:00:19.502Z","plugins":[]}`
	if err := os.WriteFile(filepath.Join(cacheDir, registryCacheFile), []byte(payload), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got := RegistryFetchedAt(); got != "2026-08-23T22:00:19.502Z" {
		t.Errorf("RegistryFetchedAt() = %q, want the cached fetchedAt", got)
	}
}

// setTestHome points os.UserHomeDir at dir on every platform.
//
// os.UserHomeDir reads $HOME on Unix but %USERPROFILE% on Windows, so a test
// that sets only HOME silently exercises the developer's real home directory
// on Windows — which is how these tests passed locally and failed on the
// windows-2022 runner.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
