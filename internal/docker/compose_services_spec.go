package docker

// compose_services_spec.go — the resolved per-service facts: which services
// exist, whether each has a compose-declared healthcheck, and how long Docker
// may take to declare it failed.
//
// Purpose: turn `docker compose config --format json` into ServiceSpec values
// for the health gate (compose_services.go).
// Constraints: every docker call goes through Compose.capture; a malformed
// healthcheck duration is an error, never a silent default.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ComposeServiceSpecs reads the resolved config JSON over the compose's
// manifest and env files: one ServiceSpec per service, sorted by name.
func ComposeServiceSpecs(ctx context.Context, compose *Compose, workdir string) ([]ServiceSpec, error) {
	out, err := compose.capture(ctx, workdir, "config", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseServiceSpecs(out)
}

// parseServiceSpecs decodes the `config --format json` document.
func parseServiceSpecs(raw []byte) ([]ServiceSpec, error) {
	var doc struct {
		Services map[string]struct {
			Restart     string `json:"restart"`
			Healthcheck *struct {
				Test        json.RawMessage `json:"test"`
				Disable     bool            `json:"disable"`
				Interval    string          `json:"interval"`
				Timeout     string          `json:"timeout"`
				StartPeriod string          `json:"start_period"`
				Retries     *int            `json:"retries"`
			} `json:"healthcheck"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing docker compose config JSON: %w", err)
	}
	specs := make([]ServiceSpec, 0, len(doc.Services))
	for name, s := range doc.Services {
		hc := s.Healthcheck
		has := hc != nil && !hc.Disable && !healthcheckNone(hc.Test)
		spec := ServiceSpec{Name: name, HasHealthcheck: has, Restart: s.Restart}
		if has {
			var err error
			if spec.HealthTimeout, err = healthTimeout(name, hc.Interval, hc.Timeout, hc.StartPeriod, hc.Retries); err != nil {
				return nil, err
			}
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs, nil
}

// healthTimeout is start_period + retries*(interval+timeout) from the compose
// duration strings. Docker reads an absent or zero value as its default
// (interval 30s, timeout 30s, retries 3), a negative start_period as 0. A bad
// duration is an error.
func healthTimeout(name, interval, timeout, startPeriod string, retries *int) (time.Duration, error) {
	var d [3]time.Duration
	for i, f := range []struct {
		field, v string
		def      time.Duration
	}{
		{"interval", interval, 30 * time.Second}, {"timeout", timeout, 30 * time.Second}, {"start_period", startPeriod, 0},
	} {
		d[i] = f.def
		if f.v == "" {
			continue
		}
		var err error
		if d[i], err = time.ParseDuration(f.v); err != nil {
			return 0, fmt.Errorf("service %s: healthcheck %s %q: %w", name, f.field, f.v, err)
		}
		if d[i] <= 0 {
			d[i] = f.def // 0 means "use the default" (start_period's is 0 too)
		}
	}
	n := 3
	if retries != nil && *retries > 0 {
		n = *retries
	}
	return d[2] + time.Duration(n)*(d[0]+d[1]), nil
}

// healthcheckNone: the test is ["NONE"], i.e. the healthcheck is disabled.
func healthcheckNone(raw json.RawMessage) bool {
	var t []string
	return json.Unmarshal(raw, &t) == nil && len(t) > 0 && t[0] == "NONE"
}
