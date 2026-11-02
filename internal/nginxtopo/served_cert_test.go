package nginxtopo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestServedCertDir(t *testing.T) {
	root := t.TempDir()
	owner := filepath.Join(root, "front")
	project := filepath.Join(owner, "backend")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, frontedBy, domain, servedRoot string
		wantErr                             bool
	}{
		{"own", "", "api.example.com:443", project, false},
		{"fronted", "front", "api.example.com:443", owner, false},
		{"unresolved", "other", "api.example.com:443", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ServedCertDir(project, tc.frontedBy, tc.domain)
			if tc.wantErr {
				if got != "" || !errors.Is(err, ErrFrontingUnresolved) {
					t.Fatalf("got %q, %v; want empty path and ErrFrontingUnresolved", got, err)
				}
				return
			}
			want := filepath.Join(tc.servedRoot, "ssl", "certificates", "api-example-com-443")
			if err != nil || got != want {
				t.Fatalf("got %q, %v; want %q", got, err, want)
			}
		})
	}
}
