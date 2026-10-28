package deploy

import (
	"context"
	"os/exec"
	"testing"
)

// imageInspectRemote executes only a shell function double, never Docker.
type imageInspectRemote struct {
	*fakeRemote
	answer string
}

func (r imageInspectRemote) Run(ctx context.Context, command string) ([]byte, error) {
	script := "docker() { " + r.answer + "; }; " + unshell(command)
	return exec.CommandContext(ctx, "sh", "-c", script).Output()
}

func TestSaveImageTagsInspectOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		wantErr      bool
		count        int
	}{
		{"present", "printf '%s\\n' " + q(fakeImageID("a")), false, 1},
		{"absent", "printf '%s\\n' 'Error response from daemon: No such image: web:1' >&2; return 1", false, 0},
		{"daemon unavailable", "printf '%s\\n' 'Cannot connect to the Docker daemon' >&2; return 1", true, 0},
		{"permission denied", "printf '%s\\n' 'permission denied' >&2; return 1", true, 0},
		{"empty success", "return 0", true, 0},
		{"malformed success", "printf '%s\\n' invalid", true, 0},
		{"successful warning", "printf '%s\\n' warning >&2; printf '%s\\n' " + q(fakeImageID("a")), true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := imageInspectRemote{newFakeRemote(t), tc.answer}
			saved, err := saveImageTags(context.Background(), r, []string{"web:1"})
			if (err != nil) != tc.wantErr || len(saved) != tc.count {
				t.Fatalf("saved=%v err=%v", saved, err)
			}
		})
	}
}
