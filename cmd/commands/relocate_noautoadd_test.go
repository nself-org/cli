package commands

// relocate_noautoadd_test.go — D-0910: NSELF_NO_AUTO_ADD accepts the same
// truthy values as NSELF_V15 and is also read from the project .env cascade.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelocateNoAutoAddResolution(t *testing.T) {
	cases := []struct {
		name    string
		process string            // NSELF_NO_AUTO_ADD in the process environment
		files   map[string]string // files written to the project directory
		refused bool
	}{
		{"process 1", "1", nil, true},
		{"process true", "true", nil, true},
		{"process TRUE", "TRUE", nil, true},
		{"process yes is not truthy", "yes", nil, false},
		{"process 0", "0", nil, false},
		{"unset", "", nil, false},
		{"project .env", "", map[string]string{".env": "NSELF_NO_AUTO_ADD=1\n"}, true},
		{"project .env true", "", map[string]string{".env": "NSELF_NO_AUTO_ADD=true\n"}, true},
		{"process 0 beats .env", "0", map[string]string{".env": "NSELF_NO_AUTO_ADD=1\n"}, false},
		{".env.local overrides .env", "", map[string]string{
			".env":       "NSELF_NO_AUTO_ADD=1\n",
			".env.local": "NSELF_NO_AUTO_ADD=0\n",
		}, false},
		{".env.local can switch it on", "", map[string]string{".env.local": "NSELF_NO_AUTO_ADD=1\n"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, calls, _ := relocateSetup(t, "", nil)
			wd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			for name, body := range c.files {
				if err := os.WriteFile(filepath.Join(wd, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv(envNoAutoAdd, c.process)
			autoAddPlugin = recordOver(calls, publishStub(t))
			handled, err := relocateBreakout("server", []string{"list"})
			if !handled {
				t.Fatal("the break-out must be handled")
			}
			if c.refused {
				assertBreakoutRefused(t, err, "infra")
				if len(*calls) != 0 {
					t.Fatalf("the seam ran with auto-add off: %v", *calls)
				}
				return
			}
			if len(*calls) != 1 {
				t.Fatalf("auto-add should have run once, seam calls %v (err %v)", *calls, err)
			}
		})
	}
}
