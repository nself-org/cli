package imagetags

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// imageInspectRemote executes only a shell function double, never Docker.
type imageInspectRemote struct {
	answer string
}

func (r imageInspectRemote) Run(ctx context.Context, command string) ([]byte, error) {
	script := "docker() { " + r.answer + "; }; " + strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(command, "sh -c '"), "'"), `'\''`, `'`)
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
		{"empty missing suffix", "printf '%s\\n' 'Error: No such image: ' >&2; return 1", true, 0},
		{"missing plus diagnostic", "printf '%s\\n' 'Error: No such image: web:1' 'permission denied' >&2; return 1", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := imageInspectRemote{tc.answer}
			saved, err := Save(context.Background(), r, []string{"web:1"})
			if (err != nil) != tc.wantErr || len(saved) != tc.count {
				t.Fatalf("saved=%v err=%v", saved, err)
			}
		})
	}
}

func TestSaveImageTagsNormalizedMissingReference(t *testing.T) {
	for _, tc := range []struct{ ref, missing string }{
		{"web", "web:latest"},
		{"docker.io/library/web:1", "web:1"},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			r := imageInspectRemote{"printf '%s\\n' " + q("Error response from daemon: No such image: "+tc.missing) + " >&2; return 1"}
			saved, err := Save(context.Background(), r, []string{tc.ref})
			if err != nil || len(saved) != 0 {
				t.Fatalf("missing normalized reference: saved=%v err=%v", saved, err)
			}
		})
	}
}

func (imageInspectRemote) Label() string { return "fixture" }
func fakeImageID(digit string) string    { return "sha256:" + strings.Repeat(digit, 64) }
func q(s string) string                  { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func TestRestoreSurvivesCancellationAndQuotesReferences(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ref := "registry.example/web'quoted:1"
	saved := []Tag{{ID: fakeImageID("a"), Ref: ref}}
	r := imageInspectRemote{`[ "$1" = tag ] && [ "$2" = ` + q(saved[0].ID) + ` ] && [ "$3" = ` + q(ref) + ` ]`}
	if err := Restore(ctx, r, saved); err != nil {
		t.Fatal(err)
	}
	if err := Restore(ctx, imageInspectRemote{"return 1"}, saved); err == nil {
		t.Fatal("failed Docker tag must fail restoration")
	}
	if err := Restore(ctx, r, nil); err != nil {
		t.Fatal(err)
	}
	if saved, err := Save(ctx, r, nil); err != nil || len(saved) != 0 {
		t.Fatalf("empty refs: %v %v", saved, err)
	}
}
