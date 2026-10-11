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
	out, err := compose.captureBounded(ctx, "docker compose config", workdir, "config", "--format", "json")
	if err != nil {
		return nil, err
	}
	return ParseServiceSpecs(out)
}

// MaxHealthTimeout caps ServiceSpec.HealthTimeout: a healthcheck declared with
// a huge retries or interval must not stall a deploy for hours.
const MaxHealthTimeout = 10 * time.Minute

// ParseServiceSpecs decodes a `config --format json` document into one
// ServiceSpec per service, sorted by name. internal/deploy reads the same
// document fetched over ssh and uses this, so the local and remote health
// gates see the same specs.
func ParseServiceSpecs(raw []byte) ([]ServiceSpec, error) {
	var doc struct {
		Services map[string]struct {
			Restart     string `json:"restart"`
			Healthcheck *struct {
				Test        json.RawMessage `json:"test"`
				Disable     bool            `json:"disable"`
				Interval    json.RawMessage `json:"interval"`
				Timeout     json.RawMessage `json:"timeout"`
				StartPeriod json.RawMessage `json:"start_period"`
				Retries     json.RawMessage `json:"retries"`
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

// healthDuration reads a compose healthcheck duration: a Go duration string
// ("1m30s") or a JSON number (nanoseconds, compose-go's time.Duration). Absent
// or null is def; zero or negative is def too, as Docker reads it. A value
// above MaxHealthTimeout is clamped to it.
func healthDuration(name, field string, raw json.RawMessage, def time.Duration) (time.Duration, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return def, nil
	}
	var d time.Duration
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		if d, err = time.ParseDuration(str); err != nil {
			return 0, fmt.Errorf("service %s: healthcheck %s %q: %w", name, field, str, err)
		}
	} else {
		var ns int64
		if err := json.Unmarshal(raw, &ns); err != nil {
			return 0, fmt.Errorf("service %s: healthcheck %s %s is neither a duration string nor a whole number of nanoseconds", name, field, raw)
		}
		d = time.Duration(ns)
	}
	if d <= 0 {
		return def, nil
	}
	return min(d, MaxHealthTimeout), nil
}

// healthTimeout is start_period + retries*(interval+timeout), capped at
// MaxHealthTimeout. Docker reads an absent or zero interval, timeout or retries
// as its default (30s, 30s, 3), a non-positive start_period as 0. A bad value
// is an error naming the service and field.
func healthTimeout(name string, interval, timeout, startPeriod, retries json.RawMessage) (time.Duration, error) {
	iv, err := healthDuration(name, "interval", interval, 30*time.Second)
	if err != nil {
		return 0, err
	}
	to, err := healthDuration(name, "timeout", timeout, 30*time.Second)
	if err != nil {
		return 0, err
	}
	sp, err := healthDuration(name, "start_period", startPeriod, 0)
	if err != nil {
		return 0, err
	}
	n := int64(3)
	if len(retries) > 0 && string(retries) != "null" {
		if err := json.Unmarshal(retries, &n); err != nil {
			return 0, fmt.Errorf("service %s: healthcheck retries %s is not a whole number that fits: %w", name, retries, err)
		}
		if n <= 0 {
			n = 3
		}
	}
	// Each term is at most MaxHealthTimeout, so the sums cannot overflow; the
	// multiply is checked against the cap first.
	per := iv + to
	if n > int64((MaxHealthTimeout-sp)/per) {
		return MaxHealthTimeout, nil
	}
	return sp + time.Duration(n)*per, nil
}

// healthcheckNone: the test is ["NONE"], i.e. the healthcheck is disabled.
func healthcheckNone(raw json.RawMessage) bool {
	var t []string
	return json.Unmarshal(raw, &t) == nil && len(t) > 0 && t[0] == "NONE"
}
