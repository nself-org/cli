package cmdlog

import (
	"strings"
	"testing"
)

func TestRedactSecretNamedFlagsAndURLs(t *testing.T) {
	resetMask(t)
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"password eq", []string{"nself", "x", "--db-password=fakeV1"}, "nself x --db-password=[REDACTED]"},
		{"auth token space", []string{"nself", "x", "--auth-token", "fakeV2", "y"}, "nself x --auth-token [REDACTED] y"},
		{"passwd", []string{"nself", "x", "--Passwd", "fakeV3"}, "nself x --Passwd [REDACTED]"},
		{"api_key", []string{"nself", "x", "--api_key=fakeV4"}, "nself x --api_key=[REDACTED]"},
		{"apikey", []string{"nself", "x", "--apikey", "fakeV5"}, "nself x --apikey [REDACTED]"},
		{"private-key", []string{"nself", "x", "--private-key=fakeV6"}, "nself x --private-key=[REDACTED]"},
		{"credential", []string{"nself", "x", "--credentials", "fakeV7"}, "nself x --credentials [REDACTED]"},
		{"plain flag kept", []string{"nself", "x", "--name=svc", "--port", "80"}, "nself x --name=svc --port 80"},
		{"url positional", []string{"nself", "x", "postgres://admin:fakePw@db:5432/app"}, "nself x postgres://****@db:5432/app"},
		{"url in flag", []string{"nself", "x", "--url=https://u:p@w@host/p"}, "nself x --url=https://****@host/p"},
		{"url with plain query kept", []string{"nself", "x", "https://host/p?page=2"}, "nself x https://host/p?page=2"},
		{"url secret query param", []string{"nself", "x", "https://host/p?access_token=fakeV8&a=1"}, "nself x https://host/p?access_token=****&a=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactArgsWith(tt.argv, nil); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	if got := redactArgsWith([]string{"nself", "--db-password=fakeV1"}, nil); strings.Contains(got, "fakeV1") {
		t.Errorf("leak: %q", got)
	}
}
