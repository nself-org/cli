package plugin

import (
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
)

// TestRegistryCacheRoundTripKeepsImplementationFields pins the fix for the bug
// that made every CLI-R11 extraction install into a dead command.
//
// The registry cache is written by Registry.MarshalJSON, which rebuilds each
// entry field by field. Any field missing from that copy is silently dropped,
// and pluginType, binaryName, language and runtime were all missing. So the
// first request parsed the registry correctly, the cache write threw those
// fields away, and the very next read — which is a cache read — returned a
// plugin that claimed to provide no command. linkCLIBinary then did nothing,
// the install reported success, and the command did not exist.
//
// The failure needed two runs to show up, which is why it survived unit tests:
// everything passes on a cold cache.
func TestRegistryCacheRoundTripKeepsImplementationFields(t *testing.T) {
	original := &Registry{Plugins: []PluginManifest{{
		Name:       "infra",
		Version:    "1.0.0",
		Tier:       "free",
		Language:   "go",
		Runtime:    "go",
		PluginType: "cli",
		BinaryName: "nself-infra",
	}}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	reloaded, err := parseRegistryJSON(data)
	if err != nil {
		t.Fatalf("parse cached registry: %v", err)
	}

	got, ok := findPlugin(reloaded, "infra")
	if !ok {
		t.Fatal("plugin vanished from the cache round-trip")
	}

	for _, f := range []struct{ name, want, have string }{
		{"PluginType", "cli", got.PluginType},
		{"BinaryName", "nself-infra", got.BinaryName},
		{"Language", "go", got.Language},
		{"Runtime", "go", got.Runtime},
	} {
		if f.have != f.want {
			t.Errorf("%s lost in cache round-trip: got %q, want %q", f.name, f.have, f.want)
		}
	}

	// The consequence, stated directly: this is the call whose empty answer
	// meant the command was never published.
	if bin := cliBinaryName("infra", got); bin != "nself-infra" {
		t.Errorf("cliBinaryName after cache round-trip = %q, want nself-infra — "+
			"an empty result here is what silently produced a dead command", bin)
	}
}

// TestRegistryCacheRoundTripIsDrivenByTheManifest guards the class rather than
// the four fields above. Registry.MarshalJSON is an allowlist, so a field added
// to PluginManifest is dropped from the cache unless someone remembers to add
// it there too. Nobody remembered for four of them.
func TestRegistryCacheRoundTripIsDrivenByTheManifest(t *testing.T) {
	full := &Registry{Plugins: []PluginManifest{{
		Name:            "example",
		Version:         "2.3.4",
		Description:     "d",
		Category:        "c",
		License:         "MIT",
		Tier:            "free",
		Repository:      "https://example.invalid/repo",
		Checksum:        "abc",
		Tags:            []string{"t"},
		Tables:          []string{"np_example"},
		Port:            1234,
		Language:        "go",
		Runtime:         "go",
		PluginType:      "cli",
		BinaryName:      "nself-example",
		RequiresLicense: true,
		UpdatedAt:       "2026-08-25T00:00:00Z",
	}}}

	data, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	reloaded, err := parseRegistryJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, ok := findPlugin(reloaded, "example")
	if !ok {
		t.Fatal("example not found after round-trip")
	}

	want := full.Plugins[0]
	checks := map[string][2]any{
		"Name":            {want.Name, got.Name},
		"Version":         {want.Version, got.Version},
		"Description":     {want.Description, got.Description},
		"Category":        {want.Category, got.Category},
		"License":         {want.License, got.License},
		"Tier":            {want.Tier, got.Tier},
		"Repository":      {want.Repository, got.Repository},
		"Checksum":        {want.Checksum, got.Checksum},
		"Port":            {want.Port, got.Port},
		"Language":        {want.Language, got.Language},
		"Runtime":         {want.Runtime, got.Runtime},
		"PluginType":      {want.PluginType, got.PluginType},
		"BinaryName":      {want.BinaryName, got.BinaryName},
		"RequiresLicense": {want.RequiresLicense, got.RequiresLicense},
		"UpdatedAt":       {want.UpdatedAt, got.UpdatedAt},
	}
	for name, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s not preserved by the cache: wrote %v, read back %v", name, pair[0], pair[1])
		}
	}
	if len(got.Tags) != 1 || got.Tags[0] != "t" {
		t.Errorf("Tags not preserved: %v", got.Tags)
	}
	if len(got.Tables) != 1 || got.Tables[0] != "np_example" {
		t.Errorf("Tables not preserved: %v", got.Tables)
	}
}

// TestCacheKeepsSignatureMaterial pins the most consequential instance of the
// dropped-field bug.
//
// Registry.MarshalJSON did not copy AuthorPublicKey, Signature or
// PublishStatus. The registry cache is read on every run after the first, so
// from the second install onward verifyPluginSignature received three empty
// strings. That takes the "no signature supplied" branch. Whether that
// branch refuses depends on NSELF_PLUGIN_REQUIRE_CHECKSUM (FIX-CLI-6,
// default off pending full registry checksum/signature coverage) — this test
// asserts the material itself (AuthorPublicKey/Signature/PublishStatus)
// survives the round-trip, and that hard mode refuses when opted in.
func TestCacheKeepsSignatureMaterial(t *testing.T) {
	original := &Registry{Plugins: []PluginManifest{{
		Name:             "signed-plugin",
		Version:          "1.0.0",
		PublishStatus:    "stable",
		AuthorPublicKey:  "aa" + "bb" + "cc",
		Signature:        "dd" + "ee" + "ff",
		ReleaseSignature: &signature.Release{KeyID: "plugins-0123456789abcdef", Alg: signature.Alg, Sig: "c2ln"},
	}}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	reloaded, err := parseRegistryJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, ok := findPlugin(reloaded, "signed-plugin")
	if !ok {
		t.Fatal("plugin vanished from the cache round-trip")
	}

	if got.AuthorPublicKey == "" || got.Signature == "" {
		t.Errorf("signature material lost: key=%q sig=%q — verification would be skipped",
			got.AuthorPublicKey, got.Signature)
	}
	if got.PublishStatus != "stable" {
		t.Errorf("publishStatus = %q, want \"stable\" — an empty value turns a "+
			"refused unsigned install into an allowed one", got.PublishStatus)
	}

	// The behaviour that matters, asserted directly: under hard mode
	// (NSELF_PLUGIN_REQUIRE_CHECKSUM=1), a stable plugin arriving with no
	// signature must be refused — this is the path the cache used to send
	// every stable plugin down, silently, regardless of the switch.
	if got.ReleaseSignature == nil || *got.ReleaseSignature != *original.Plugins[0].ReleaseSignature {
		t.Errorf("release_signature lost or changed in the cache round trip: %+v", got.ReleaseSignature)
	}
	t.Setenv(pluginRequireChecksumEnv, "1")
	stripped := *got
	stripped.ReleaseSignature = nil
	if err := verifyReleaseSignature(&stripped, false, false, ArtifactKindSource, false); err == nil {
		t.Error("a stable plugin with no signature was accepted even in hard mode")
	}
}
