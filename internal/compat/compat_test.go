package compat

import "testing"

// TestV15EnvTable covers the ADR 0021 spellings: 1 and true (any case) enable
// v1.5; unset, empty, 0, yes and anything else do not.
func TestV15EnvTable(t *testing.T) {
	cases := []struct {
		name string
		set  bool
		val  string
		want bool
	}{
		{"unset", false, "", false},
		{"empty", true, "", false},
		{"zero", true, "0", false},
		{"one", true, "1", true},
		{"true", true, "true", true},
		{"TRUE", true, "TRUE", true},
		{"True", true, "True", true},
		{"yes", true, "yes", false},
		{"false", true, "false", false},
		{"padded", true, " 1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv(EnvVar, c.val)
			} else {
				// t.Setenv registers the restore; Unsetenv then clears it.
				t.Setenv(EnvVar, "x")
				unset(t)
			}
			if got := V15(); got != c.want {
				t.Fatalf("V15() with %q (set=%v) = %v, want %v", c.val, c.set, got, c.want)
			}
		})
	}
}

// TestV15NotCached proves the value is re-read on every call.
func TestV15NotCached(t *testing.T) {
	t.Setenv(EnvVar, "1")
	if !V15() {
		t.Fatal("want true")
	}
	t.Setenv(EnvVar, "0")
	if V15() {
		t.Fatal("want false after flip")
	}
}

// TestMode checks the mode string follows V15.
func TestMode(t *testing.T) {
	t.Setenv(EnvVar, "1")
	if Mode() != "v1.5" {
		t.Fatalf("Mode() = %q", Mode())
	}
	t.Setenv(EnvVar, "")
	if Mode() != "v1.4" {
		t.Fatalf("Mode() = %q", Mode())
	}
}
