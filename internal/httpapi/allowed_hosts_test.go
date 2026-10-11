package httpapi

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
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
			var cliErr *errs.CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != "E430" || cliErr.Why != "an allowed host is a DNS name, an IPv4 literal or a bracketed IPv6 literal, without scheme, port or path" || cliErr.Fix != "pass --allowed-host <name>; the server adds the port itself" {
				t.Fatalf("invalid allowed host guidance: %v", err)
			}
		})
	}
}
