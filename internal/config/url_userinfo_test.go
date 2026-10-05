package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestURLUserInfoRoundTrips(t *testing.T) {
	cases := []struct{ user, pw, want string }{
		{"", "", ""},
		{"postgres", "", "postgres"},
		{"postgres", "plain", "postgres:plain"},
		{"", "ab/cd=", ":ab%2Fcd="},
		{"postgres", "p@ss:w/rd?#", "postgres:p%40ss%3Aw%2Frd%3F%23"},
	}
	for _, c := range cases {
		got := URLUserInfo(c.user, c.pw)
		if got != c.want {
			t.Errorf("URLUserInfo(%q,%q) = %q, want %q", c.user, c.pw, got, c.want)
		}
		if got == "" {
			continue
		}
		u, err := url.Parse("redis://" + got + "@redis:6379")
		if err != nil {
			t.Fatalf("parse %q: %v", got, err)
		}
		if u.Host != "redis:6379" {
			t.Errorf("host = %q, want redis:6379", u.Host)
		}
		pw, _ := u.User.Password()
		if pw != c.pw || u.User.Username() != c.user {
			t.Errorf("round trip = %q/%q, want %q/%q", u.User.Username(), pw, c.user, c.pw)
		}
	}
}

// urlPasswordCases cover every URL-reserved character the pack names plus the
// ones that bite in practice (+, space, non-ASCII). Passwords are test data,
// not secrets. Failure messages print the case index, never the value.
var urlPasswordCases = []string{
	"a/b@c:d#e?f%g",
	"p@ss",
	"abc",
	"a+b c",
	"100%",
	"%40already%2Fencoded",
	"x y\tz",
	"pä$$wörd!*'()",
	"@:/?#%+ ",
	"a/b@c:d#e?f%g+h i",
}

func TestURLPasswordRoundTrips(t *testing.T) {
	for i, pw := range urlPasswordCases {
		enc := URLPassword(pw)
		if strings.ContainsAny(enc, "@/?# ") || strings.Count(enc, ":") != 0 {
			t.Errorf("case %d: encoded value still has a URL delimiter", i)
		}
		u, err := url.Parse("postgresql://user:" + enc + "@db:5432/app?sslmode=disable")
		if err != nil {
			t.Fatalf("case %d: parse: %v", i, err)
		}
		if u.Host != "db:5432" || u.Path != "/app" || u.RawQuery != "sslmode=disable" {
			t.Errorf("case %d: URL shape changed (host %q path %q query %q)", i, u.Host, u.Path, u.RawQuery)
		}
		got, _ := u.User.Password()
		if got != pw {
			t.Errorf("case %d: round trip differs from the original", i)
		}
		// Redis shape: empty user.
		r, err := url.Parse("redis://:" + enc + "@redis:6379")
		if err != nil {
			t.Fatalf("case %d: redis parse: %v", i, err)
		}
		if rp, _ := r.User.Password(); rp != pw || r.Host != "redis:6379" {
			t.Errorf("case %d: redis round trip differs", i)
		}
	}
}

func TestURLPasswordMatchesURLUserInfo(t *testing.T) {
	for i, pw := range urlPasswordCases {
		if got, want := URLPassword(pw), strings.TrimPrefix(URLUserInfo("u", pw), "u:"); got != want {
			t.Errorf("case %d: URLPassword and URLUserInfo disagree", i)
		}
		if got, want := URLPassword(pw), strings.TrimPrefix(URLUserInfo("", pw), ":"); got != want {
			t.Errorf("case %d: URLPassword and URLUserInfo (empty user) disagree", i)
		}
	}
}

func TestURLPasswordEmptyAndPlain(t *testing.T) {
	if got := URLPassword(""); got != "" {
		t.Errorf("empty password must stay empty, got a value of %d bytes", len(got))
	}
	if got := URLPassword("abc"); got != "abc" {
		t.Errorf("a plain password must be unchanged")
	}
}

// TestURLPasswordIsNotIdempotent documents why callers encode exactly once:
// feeding the output back in encodes the percent sign again.
func TestURLPasswordIsNotIdempotent(t *testing.T) {
	once := URLPassword("p@ss")
	if twice := URLPassword(once); twice == once {
		t.Fatal("double encoding must change the value; it is a bug to do it")
	}
}
