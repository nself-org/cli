package admin

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func TestAdminSharedDefaultPortPin(t *testing.T) {
	for _, port := range []int{0, 22} {
		compattest.Set(t, true)
		alias := remote.PinAlias(remote.HostSpec{Host: "host.test", Port: 22})
		e := newArgvEnv(t, "", &hostKey{alias: alias, pin: true})
		if err := callEntry("verify", "u", "host.test", port); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(e.argv(t), " "), "HostKeyAlias="+alias) {
			t.Fatal("admin did not use the shared pin alias")
		}
	}
}

func TestAdminOperatorTrustExcludesLegacyPins(t *testing.T) {
	compattest.Set(t, true)
	e := newArgvEnv(t, "", &hostKey{alias: "nself-a.test-2222"})
	if err := (remote.PinnedHostKeys{Path: e.pinFile()}).Add("nself-a.test-2222", "ssh-ed25519 AQID"); err != nil {
		t.Fatal(err)
	}
	if err := callEntry("verify", "u", "nself-a.test-2222", 22); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(e.argv(t), " "), `GlobalKnownHostsFile="/dev/null"`) {
		t.Fatal("operator trust still exposes colliding legacy pins")
	}
}
