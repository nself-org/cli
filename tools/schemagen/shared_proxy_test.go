package main

import (
	"encoding/json"
	"regexp"
	"testing"
)

// TestSharedProxyFilePatternAcceptsBothPathKinds: the published pattern for a
// registered file must accept a site conf and a certificate path (the dot in
// .pem is a literal dot, not a literal backslash) and reject near misses.
func TestSharedProxyFilePatternAcceptsBothPathKinds(t *testing.T) {
	files, err := generate(defaultSet)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := files["shared-proxy.v1.schema.json"]
	if !ok {
		t.Fatal("shared-proxy.v1.schema.json not generated")
	}
	var sch struct {
		Properties struct {
			Files struct {
				Items struct {
					Pattern string `json:"pattern"`
				} `json:"items"`
			} `json:"files"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &sch); err != nil {
		t.Fatal(err)
	}
	re, err := regexp.Compile(sch.Properties.Files.Items.Pattern)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"nginx/sites/b.hasura.conf",
		"ssl/certificates/b-test/fullchain.pem",
		"ssl/certificates/b-test/privkey.pem",
	} {
		if !re.MatchString(p) {
			t.Errorf("pattern %q rejects valid path %q", re, p)
		}
	}
	for _, p := range []string{
		"ssl/certificates/b-test/fullchainXpem",
		`ssl/certificates/b-test/fullchain\.pem`,
		"ssl/certificates/b-test/cert.pem",
		"nginx/sites/a/b.conf",
	} {
		if re.MatchString(p) {
			t.Errorf("pattern %q accepts invalid path %q", re, p)
		}
	}
}
