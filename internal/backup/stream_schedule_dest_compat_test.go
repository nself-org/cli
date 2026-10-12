package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// Purpose: the backup schedule half of D-0676 (see stream_dest_compat_test.go
// for the helpers and the stream half). The mode comes from compattest only.

// TestScheduleStreamBarePath: backup schedule accepts a bare --to and
// --heartbeat-to on v1.4 (the unit keeps the value as written, as v1.4.13
// did) and refuses them under v1.5, naming the flag.
func TestScheduleStreamBarePath(t *testing.T) {
	skipUnitTestOnWindows(t)
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		for _, flag := range []string{"--to", "--heartbeat-to"} {
			for _, v := range []string{"/mnt/backups", "nightly-bk"} {
				buf := captureBareNotice(t)
				var out bytes.Buffer
				opts := scheduleOpts(t, true, &out)
				if flag == "--to" {
					opts.To = v
				} else {
					opts.HeartbeatTo = v
				}
				err := ScheduleStream(minimalConfig(), opts)
				if v15 {
					if err == nil || !strings.Contains(err.Error(), flag+":") || !strings.Contains(err.Error(), "path:///") {
						t.Errorf("%s %q: err = %v; want a refusal naming the flag", flag, v, err)
					}
					if out.Len() != 0 {
						t.Errorf("%s %q: a refused schedule rendered units: %q", flag, v, out.String())
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s %q: %v", flag, v, err)
				}
				if !strings.Contains(out.String(), " "+v) {
					t.Errorf("%s %q: the unit does not carry the value as written: %q", flag, v, out.String())
				}
				if v == "nightly-bk" && !strings.Contains(buf.String(), filepath.Join(opts.ProjectDir, "nightly-bk")) {
					t.Errorf("%s: relative value not resolved against the project dir: %q", flag, buf)
				}
				if noticeLines(buf) != 1 {
					t.Errorf("%s %q: want one notice, got %q", flag, v, buf)
				}
			}
		}
	})
}
