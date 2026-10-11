package httpapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestAllowedHostValidation(t *testing.T) {
	for _, host := range []string{"localhost", "api.example.test", "API.example.test", "api.example.test.", "127.0.0.1", "192.0.2.1", "[::1]", "[2001:db8::1]"} {
		t.Run("valid/"+host, func(t *testing.T) {
			if _, err := New(Options{Registry: &cmdregistry.Registry{}, Auth: BearerAuth("fake-token-0000"), AllowedHosts: []string{host}}); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, host := range []string{"", "myhost:3857", "http://myhost", "a/b", "my host", " myhost", "myhost\t", "::1", "[::1]:3857", "[127.0.0.1]", "[bad]", "a..b", "-myhost", "myhost-", "my_host", strings.Repeat("a", 64)} {
		t.Run("invalid/"+host, func(t *testing.T) {
			_, err := New(Options{Registry: &cmdregistry.Registry{}, Auth: BearerAuth("fake-token-0000"), AllowedHosts: []string{host}})
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", host)) {
				t.Fatalf("invalid host %q: %v", host, err)
			}
		})
	}
}
