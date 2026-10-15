package docker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUserNamespaceRemapped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake Docker uses a POSIX shell")
	}
	for _, tc := range []struct {
		name, output  string
		want, wantErr bool
	}{
		{"rootful", `["name=seccomp,profile=builtin","name=cgroupns"]`, false, false},
		{"rootless", `["name=seccomp,profile=builtin","name=rootless"]`, true, false},
		{"userns", `["name=userns"]`, true, false},
		{"empty", `[]`, false, false},
		{"invalid", `not-json`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			script := "#!/bin/sh\n[ \"$*\" = 'info --format {{json .SecurityOptions}}' ] || exit 2\nprintf '%s\\n' '" + tc.output + "'\n"
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			got, err := UserNamespaceRemapped(context.Background())
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("got (%v, %v), want (%v, error=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
