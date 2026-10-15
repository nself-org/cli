package errs

import (
	"strings"
	"testing"
)

func TestACMEInstallGuidanceNamesProxies(t *testing.T) {
	entry := Registry["E471"]
	if strings.Contains(entry.DefaultWhy, "or nginx did not serve") {
		t.Error("install failure must describe the serving proxy")
	}
	for _, proxy := range []string{"nginx", "traefik"} {
		if !strings.Contains(entry.DefaultFix, proxy) {
			t.Errorf("install remediation omits %s", proxy)
		}
	}
}
