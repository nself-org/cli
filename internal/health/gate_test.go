package health

import (
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func rep(statuses map[string]string) *HealthReport {
	r := &HealthReport{}
	for _, s := range []string{"postgres", "hasura", "auth", "nginx", "redis", "minio"} {
		st, ok := statuses[s]
		if !ok {
			continue
		}
		res := HealthResult{Service: s, Status: st}
		r.Results = append(r.Results, res)
		r.Total++
		if res.OK() {
			r.Healthy++
		} else {
			r.Unhealthy++
		}
	}
	return r
}

func TestVerdict(t *testing.T) {
	ok := map[string]string{"postgres": "healthy", "hasura": "healthy", "auth": "healthy", "nginx": "healthy", "redis": "healthy"}
	coreDown := map[string]string{"postgres": "unhealthy", "hasura": "healthy", "auth": "healthy", "nginx": "healthy", "redis": "healthy", "minio": "healthy"}
	optDown := map[string]string{"postgres": "healthy", "hasura": "healthy", "auth": "healthy", "nginx": "healthy", "redis": "healthy", "minio": "unhealthy"}
	manyDown := map[string]string{"postgres": "healthy", "hasura": "healthy", "redis": "unhealthy", "minio": "unhealthy"}
	cases := []struct {
		name string
		rep  *HealthReport
		want error
	}{
		{"all healthy", rep(ok), nil},
		{"nil report", nil, errs.ErrHealthTimeout},
		{"core down above threshold", rep(coreDown), errs.ErrServiceUnhealthy},
		{"optional down above threshold", rep(optDown), nil},
		{"below threshold", rep(manyDown), errs.ErrHealthTimeout},
	}
	for _, c := range cases {
		err := Verdict(c.rep, 80)
		if !errors.Is(err, c.want) || (c.want == nil) != (err == nil) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestRequiredPct(t *testing.T) {
	for _, c := range []struct {
		quick bool
		cfg   int
		want  int
	}{{false, 0, 80}, {true, 0, 60}, {true, 95, 60}, {false, 95, 95}} {
		if got := RequiredPct(c.quick, c.cfg); got != c.want {
			t.Errorf("RequiredPct(%v,%d)=%d want %d", c.quick, c.cfg, got, c.want)
		}
	}
}
