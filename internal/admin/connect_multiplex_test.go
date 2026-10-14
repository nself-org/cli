package admin

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestAdminStrictPolicyDisablesMultiplexing(t *testing.T) {
	for _, trust := range []struct {
		name, alias string
		port        int
		pin         bool
	}{
		{"operator-default", "host.test", 22, false},
		{"operator-port", "[host.test]:2222", 2222, false},
		{"pin-default", "host.test", 22, true},
		{"pin-port", "nself-host.test-2222", 2222, true},
	} {
		for _, entry := range entryNames {
			t.Run(trust.name+"/"+entry, func(t *testing.T) {
				compattest.Set(t, true)
				e := newArgvEnv(t, "", &hostKey{alias: trust.alias, pin: trust.pin})
				if err := callEntry(entry, "u", "host.test", trust.port); err != nil {
					t.Fatal(err)
				}
				args := e.argv(t)
				for _, option := range []string{"ControlMaster=no", "ControlPath=none"} {
					found := false
					for i := 0; i+1 < len(args); i++ {
						if args[i] == "-o" && args[i+1] == option {
							found = true
						}
					}
					if !found {
						t.Errorf("strict policy omitted -o %s: %q", option, args)
					}
				}
			})
		}
	}
}
