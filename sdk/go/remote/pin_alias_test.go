package remote

import (
	"reflect"
	"strings"
	"testing"
)

func TestPinAlias(t *testing.T) {
	for _, host := range []string{"host.test", "203.0.113.7", "2001:db8::1", strings.Repeat("a", 253)} {
		alias := PinAlias(HostSpec{Host: host})
		if _, err := checkPinAlias(alias); err != nil {
			t.Fatalf("pin alias %q: %v", alias, err)
		}
		if _, err := ParseHostSpec(alias); err == nil {
			t.Fatalf("pin alias %q is also a hostname", alias)
		}
		if alias != PinAlias(HostSpec{Host: strings.ToUpper(host), Port: 22}) {
			t.Fatal("case or explicit default port changed alias")
		}
	}
}

func TestLookupHostMigration(t *testing.T) {
	p, _ := pinFile(t)
	spec := HostSpec{Host: "host.test", Port: 22}
	if err := p.Add(spec.Host, keyA); err != nil {
		t.Fatal(err)
	}
	if keys, err := p.LookupHost(spec); err != nil || !reflect.DeepEqual(keys, []string{keyA}) {
		t.Fatalf("legacy migration: %q, %v", keys, err)
	}
	if keys, err := p.Lookup(PinAlias(spec)); err != nil || len(keys) != 1 {
		t.Fatalf("canonical pin missing: %q, %v", keys, err)
	}
	// Once migrated, the legacy entry can never override the canonical pin.
	if err := p.Replace(spec.Host, keyB); err != nil {
		t.Fatal(err)
	}
	if keys, err := p.LookupHost(spec); err != nil || !reflect.DeepEqual(keys, []string{keyA}) {
		t.Fatalf("legacy fallback after migration: %q, %v", keys, err)
	}
}

func TestLookupHostRejectsAmbiguousLegacy(t *testing.T) {
	for _, spec := range []HostSpec{
		{Host: "a.test", Port: 2222}, {Host: "nself-a.test-2222"},
		{Host: "2001:db8::1", Port: 2222}, {Host: "2001-db8--1", Port: 2222},
		{Host: "203.0.113.7"},
	} {
		p, _ := pinFile(t)
		for _, legacy := range []string{"nself-a.test-2222", "nself-2001-db8--1-2222", "nself-203.0.113.7-22"} {
			if err := p.Add(legacy, keyA); err != nil {
				t.Fatal(err)
			}
		}
		if keys, err := p.LookupHost(spec); err != nil || len(keys) != 0 {
			t.Fatalf("ambiguous legacy trusted for %+v: %q, %v", spec, keys, err)
		}
		if err := p.Add(PinAlias(spec), keyB); err != nil {
			t.Fatal(err)
		}
		if keys, err := p.LookupHost(spec); err != nil || !reflect.DeepEqual(keys, []string{keyB}) {
			t.Fatalf("explicit enrollment: %q, %v", keys, err)
		}
	}
}
