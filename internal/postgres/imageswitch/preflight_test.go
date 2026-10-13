package imageswitch

import (
	"context"
	"strings"
	"testing"
)

func TestPreflightRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		mod  func(f *fakeEnv, s *Switch)
		want string // substring of the E509 cause
	}{
		{"postgres not running", func(f *fakeEnv, _ *Switch) { f.pgRunning = false }, "not running"},
		{"custom tablespace", func(f *fakeEnv, _ *Switch) {
			f.cluster = strings.Replace(f.cluster, `"tablespaces":[]`, `"tablespaces":["fast"]`, 1)
		}, "fast"},
		{"icu locale provider", func(f *fakeEnv, _ *Switch) {
			f.cluster = strings.Replace(f.cluster, `"provider":"c"`, `"provider":"i"`, 1)
		}, "locale provider"},
		{"database refuses connections", func(f *fakeEnv, _ *Switch) {
			f.cluster = strings.Replace(f.cluster, `"allow_connections":true`, `"allow_connections":false`, 1)
		}, "does not allow connections"},
		{"extension missing in the target image", func(f *fakeEnv, _ *Switch) { f.instExt = "plpgsql,plperl" }, "plperl"},
		{"locale missing in the target image", func(f *fakeEnv, _ *Switch) {
			f.cluster = strings.Replace(f.cluster, `"collate":"en_US.utf8"`, `"collate":"de_DE.utf8"`, 1)
		}, "de_DE.utf8"},
		{"docker data root too small", func(f *fakeEnv, _ *Switch) { f.dfKB = 1 }, "docker data root"},
		{"stage directory too small", func(_ *fakeEnv, s *Switch) { s.d.FreeBytes = func(string) (uint64, error) { return 10, nil } }, "stage directory"},
		{"no pgvector image for the major", func(f *fakeEnv, _ *Switch) {
			f.cluster = strings.Replace(f.cluster, `"major":16`, `"major":14`, 1)
		}, "no pgvector image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s := newFake(t)
			tc.mod(f, s)
			_, err := s.Run(context.Background(), RunOptions{})
			if code(t, err) != "E509" || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want E509 mentioning %q", err, tc.want)
			}
			if f.called("stack.stop") || f.called("ALTER DATABASE") || f.called("dump.sh") || len(f.envApplied) != 0 {
				t.Errorf("a refused preflight must change nothing: %v", f.calls)
			}
			if st, _ := LoadState(f.dir); st != nil {
				t.Errorf("a refused preflight wrote a state: %+v", st)
			}
		})
	}
}

func TestPreflightRefusesPreexistingReadOnly(t *testing.T) {
	f, s := newFake(t)
	s.d.Runtime = roRuntime{f}
	_, err := s.Run(context.Background(), RunOptions{})
	if code(t, err) != "E509" || !strings.Contains(err.Error(), "default_transaction_read_only") {
		t.Fatalf("err = %v", err)
	}
}

// roRuntime answers the size query as if a database already set default_transaction_read_only.
type roRuntime struct{ *fakeEnv }

func (r roRuntime) ExecSQL(ctx context.Context, c, u, db, sql string) (string, error) {
	if strings.Contains(sql, "pg_database_size") {
		return "1|1048576\n", nil
	}
	return r.fakeEnv.ExecSQL(ctx, c, u, db, sql)
}

func TestPlanReportsFailedChecks(t *testing.T) {
	f, s := newFake(t)
	f.dfKB = 1
	rep, err := s.Plan(context.Background())
	if code(t, err) != "E509" || rep == nil || len(rep.Preflight) == 0 {
		t.Fatalf("plan must return its checks with E509: %+v %v", rep, err)
	}
	var failed int
	for _, c := range rep.Preflight {
		if !c.OK {
			failed++
		}
	}
	if failed == 0 {
		t.Error("no failed check in the report")
	}
}

func TestParseProbe(t *testing.T) {
	p, err := parseProbe("/dev/vda1 100 50 4096 2% /probe\n---EXT\nplpgsql\nvector\n---LOC\nC\nen_US.utf8\n")
	if err != nil {
		t.Fatal(err)
	}
	if p.freeKB != 4096 || !p.extensions["vector"] || !p.hasLocale("en_US.UTF-8") || !p.hasLocale("C") || !p.hasLocale("posix") || p.hasLocale("de_DE.utf8") {
		t.Errorf("probe = %+v", p)
	}
	if _, err := parseProbe("garbage\n"); err == nil {
		t.Error("garbage df output must fail")
	}
}

func TestHumanBytesAndEstimate(t *testing.T) {
	if got := humanBytes(1536); got != "1.5 KiB" {
		t.Errorf("humanBytes = %q", got)
	}
	if estimateSeconds(0) != 60 || estimateSeconds(20<<20*100) != 160 {
		t.Errorf("estimate = %d / %d", estimateSeconds(0), estimateSeconds(20<<20*100))
	}
}
