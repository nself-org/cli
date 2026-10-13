package catalog

import (
	"strings"
	"testing"
)

func TestDecodeInvalidE116(t *testing.T) {
	valid := string(sampleCatalog(1))
	cases := map[string]string{
		"empty":            ``,
		"not json":         `<html>502</html>`,
		"trailing garbage": valid + `{}`,
		"wrong version":    strings.Replace(valid, `"schema_version": 1,`, `"schema_version": 2,`, 1),
		"no plugins":       `{"schema_version": 1, "plugins": []}`,
		"empty slug":       strings.Replace(valid, `"slug": "ghost"`, `"slug": ""`, 1),
		"unknown tier":     strings.Replace(valid, `"tier": "licensed"`, `"tier": "pro"`, 1),
		"duplicate row":    strings.Replace(valid, `"tier": "licensed"`, `"tier": "free"`, 1),
		"oversized":        `{"schema_version": 1, "pad": "` + strings.Repeat("x", maxBody) + `"}`,
	}
	for name, body := range cases {
		if _, err := Decode([]byte(body)); codeOf(err) != "E116" {
			t.Errorf("%s: got %v, want E116", name, err)
		}
	}
	if c, err := Decode([]byte(valid)); err != nil || len(c.Plugins) != 4 {
		t.Fatalf("valid catalog rejected: %v", err)
	}
}

func TestLookupAndFindByCommand(t *testing.T) {
	c, err := Decode(sampleCatalog(1))
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := c.Lookup("chat"); !ok || e.Tier != TierFree {
		t.Errorf("Lookup(chat) = %+v %v, want the free row", e, ok)
	}
	if _, ok := c.Lookup("missing"); ok {
		t.Error("Lookup(missing) found a row")
	}
	if e, ok := c.FindByCommand("traefik"); !ok || e.Slug != "traefik" {
		t.Errorf("FindByCommand(traefik) = %+v %v", e, ok)
	}
	for _, cmd := range []string{"", "ghost", "routes", "chat"} {
		if e, ok := c.FindByCommand(cmd); ok {
			t.Errorf("FindByCommand(%q) = %+v, want none", cmd, e)
		}
	}
}

func TestSearch(t *testing.T) {
	c, err := Decode(sampleCatalog(1))
	if err != nil {
		t.Fatal(err)
	}
	slugs := func(es []Entry) string {
		var out []string
		for _, e := range es {
			out = append(out, e.Slug+"/"+e.Tier)
		}
		return strings.Join(out, ",")
	}
	for _, tc := range []struct{ q, tier, want string }{
		{"chat", "", "chat/free,chat/licensed"},
		{"CHAT", TierLicensed, "chat/licensed"},
		{"retention", "", "chat/licensed"},
		{"messaging", TierFree, "chat/free"},
		{"proxy", "", "traefik/free"},
		{"  ", "", "chat/free,chat/licensed,traefik/free,ghost/free"},
		{"zzz", "", ""},
	} {
		if got := slugs(c.Search(tc.q, tc.tier)); got != tc.want {
			t.Errorf("Search(%q, %q) = %q, want %q", tc.q, tc.tier, got, tc.want)
		}
	}
	// A bundle name matches even when slug, description and category do not.
	c.Plugins[2].Bundles = []string{"edge-bundle"}
	if got := slugs(c.Search("edge-bundle", "")); got != "traefik/free" {
		t.Errorf("bundle search = %q", got)
	}
}

// FuzzDecodeCatalog: Decode never panics, and anything it accepts satisfies
// the invariants the queries rely on.
func FuzzDecodeCatalog(f *testing.F) {
	f.Add(sampleCatalog(1))
	f.Add([]byte(`{"schema_version":1,"plugins":[{"slug":"a","tier":"free","commands":{"command":"a"}}]}`))
	f.Add([]byte(`{"schema_version":1,"plugins":[{"slug":"a","tier":"free","bundles":null,"commands":null}],"bundles":{"x":null}}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := Decode(data)
		if err != nil {
			if codeOf(err) != "E116" {
				t.Fatalf("non-E116 error %v", err)
			}
			return
		}
		if c.SchemaVersion != SchemaVersion || len(c.Plugins) == 0 {
			t.Fatalf("accepted catalog breaks invariants: %+v", c)
		}
		for _, p := range c.Plugins {
			if p.Slug == "" || (p.Tier != TierFree && p.Tier != TierLicensed) {
				t.Fatalf("accepted bad row %+v", p)
			}
			c.FindByCommand(p.Slug)
			c.Lookup(p.Slug)
		}
		c.Search(string(data[:min(len(data), 8)]), "")
	})
}
