package remote

// Purpose: fuzz ParseHostSpec. Seeds are the grammar table plus the hostile
// inputs; the invariants hold for every string the fuzzer finds.

import "testing"

// FuzzParseHostSpec: an accepted input (1) re-parses from String() to a spec
// with the same String(), (2) yields SSHArgs of the shape [-p N] -- dest with
// no whitespace, shell metacharacter or leading '-' in any element but the
// fixed "-p" and "--", (3) gives a Dest the funnel's own ValidateDest accepts,
// (4) passes Validate, and (5) never returns a HostSpec alongside an error.
func FuzzParseHostSpec(f *testing.F) {
	for _, s := range []string{
		"host", "u@host", "u@host:2222", "u@10.0.0.1", "u@[2001:db8::1]:22", "u@host:/opt/nself", "[::1]", "u@my_alias", "u@_x",
		"", "-oProxyCommand=x", "u@-h", "u@h;rm", "u@h p", "u@h:0", "u@h:65536", "u@h:022", "u@[::1", "u@h:rel/path", "u@h:/a/../b",
		"user@host@evil", "u@h:22:/x", "u@2001:db8::1", "h:", "@h", "u@", "[::1]:22", "[::1]x", "u@h\n", "h\x00", "hé", "u@h%h",
		"U@h", "h:/a b", "h:/a$(id)", "[fe80::1%eth0]", "[1.2.3.4]", "a..b", "h.", "-", "--", ":", "u@h::mod",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		h, err := ParseHostSpec(s)
		if err != nil {
			if h != (HostSpec{}) {
				t.Fatalf("%q: error with a non-zero spec %+v", s, h)
			}
			if _, ok := err.(*HostSpecError); !ok {
				t.Fatalf("%q: error %T is not *HostSpecError", s, err)
			}
			return
		}
		c := h.String()
		again, err := ParseHostSpec(c)
		if err != nil {
			t.Fatalf("%q: String() %q does not re-parse: %v", s, c, err)
		}
		if again.String() != c {
			t.Fatalf("%q: String() %q re-parses to %q", s, c, again.String())
		}
		if again.User != h.User || again.Host != h.Host || again.Port != h.Port {
			t.Fatalf("%q: re-parse changed %+v to %+v", s, h, again)
		}
		if err := h.Validate(); err != nil {
			t.Fatalf("%q: Validate: %v", s, err)
		}
		checkSafeArgv(t, s, h)
	})
}
