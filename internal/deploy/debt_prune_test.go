package deploy

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestPruneReportsEarlierRemovalFailure(t *testing.T) {
	for _, failed := range []string{"02", "01"} {
		t.Run(failed, func(t *testing.T) {
			f := newFakeRemote(t)
			rp := t.TempDir()
			seedTars(t, rp, "01", "02", "03")
			bin := t.TempDir()
			writeFile(t, filepath.Join(bin, "rm"), "#!/bin/sh\ncase \"$*\" in *"+tarName(failed)+"*) exit 1;; esac\nexit 0\n", 0755)
			t.Setenv("PATH", bin+":"+strings.Join([]string{"/usr/bin", "/bin"}, ":"))
			if _, err := PrunePreDeployTars(context.Background(), f, rp, 1); err == nil {
				t.Fatal("earlier rm failure was hidden")
			}
		})
	}
}
