package supabase

import (
	"strings"
	"testing"
)

func TestBuildCompatShimsReferencedFunctionsOnly(t *testing.T) {
	c := BuildCompat(`CREATE TABLE public.t (id uuid DEFAULT extensions.uuid_generate_v4() NOT NULL, owner uuid DEFAULT auth.uid());`)
	if strings.Join(c.Shims, ",") != "extensions.uuid_generate_v4,auth.uid" {
		t.Fatalf("shims: %v", c.Shims)
	}
	for _, want := range []string{"CREATE SCHEMA IF NOT EXISTS extensions", "extensions.uuid_generate_v4()", "hasura.user", "x-hasura-user-id"} {
		if !strings.Contains(c.SQL, want) {
			t.Fatalf("missing %q in %s", want, c.SQL)
		}
	}
	if strings.Contains(c.SQL, "extensions.gen_random_uuid") || len(c.Exemptions) != 0 {
		t.Fatal("only referenced shims belong in the file")
	}
}

func TestBuildCompatListsFunctionsWithoutShim(t *testing.T) {
	c := BuildCompat(`SELECT extensions.digest('a','sha256'), auth.role();`)
	if c.SQL != "" || len(c.Exemptions) != 2 || c.Exemptions[0].ID != "compat:auth.role" || c.Exemptions[1].ID != "compat:extensions.digest" {
		t.Fatalf("%+v", c)
	}
}

func TestBuildCompatEmptyWhenNothingReferenced(t *testing.T) {
	if c := BuildCompat(`CREATE TABLE public.t (id int);`); c.SQL != "" || len(c.Shims) != 0 {
		t.Fatalf("%+v", c)
	}
}
