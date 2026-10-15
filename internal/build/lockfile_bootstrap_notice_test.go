package build

// lockfile_bootstrap_notice_test.go — D-0719: a v1.5 first build with no lock
// and no docker-compose.yml names the hasura move when the project's postgres
// volume exists (the data a move to v2.51.0 touches).

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestBootstrapNoticeNoPinSource(t *testing.T) {
	run := func(t *testing.T, volume bool, withCompose bool) string {
		t.Helper()
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, true)
		var asked string
		prevV, prevW := postgresVolumeExists, bootstrapNoticeW
		postgresVolumeExists = func(name string) bool { asked = name; return volume }
		var buf bytes.Buffer
		bootstrapNoticeW = &buf
		t.Cleanup(func() { postgresVolumeExists, bootstrapNoticeW = prevV, prevW })
		if withCompose {
			withExistingCompose(t, f, "services:\n  hasura:\n    image: "+hasura240+"\n")
		}
		lockBuild(t, f)
		if volume && !withCompose && asked != "golden_postgres_data" {
			t.Errorf("asked for volume %q, want golden_postgres_data", asked)
		}
		return buf.String()
	}

	t.Run("volume present prints one notice naming the move and the backup", func(t *testing.T) {
		out := run(t, true, false)
		if n := strings.Count(out, "notice:"); n != 1 {
			t.Fatalf("want exactly one notice, got %d: %q", n, out)
		}
		for _, want := range []string{"hasura", "nself backup create", "nself.lock.yaml"} {
			if !strings.Contains(out, want) {
				t.Errorf("notice lacks %q: %q", want, out)
			}
		}
	})
	t.Run("no volume means a new project and no notice", func(t *testing.T) {
		if out := run(t, false, false); out != "" {
			t.Fatalf("unexpected notice: %q", out)
		}
	})
	t.Run("an existing compose is a pin source and no notice", func(t *testing.T) {
		if out := run(t, true, true); out != "" {
			t.Fatalf("unexpected notice: %q", out)
		}
	})
}
