package config

import (
	"net/url"
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
