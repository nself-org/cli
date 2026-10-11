package scenarios

// Purpose: prove paired overhead, warmups, order, and failed probe handling.
// Constraints: injected probes perform no network or filesystem operations.

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestHTTPInvokePairs(t *testing.T) {
	if _, ok := Get("http-invoke"); !ok {
		t.Fatal("scenario not registered")
	}
	var order []string
	probe := func(name string, ms float64) func(context.Context) (float64, []byte, error) {
		return func(context.Context) (float64, []byte, error) {
			order = append(order, name)
			return ms, []byte(`{"data":{}}`), nil
		}
	}
	samples, err := measureHTTPPairs(context.Background(), Config{Runs: 3, Warmup: 1}, probe("http", 14), probe("cli", 10))
	if err != nil || len(samples) != 3 {
		t.Fatalf("samples=%v err=%v", samples, err)
	}
	for _, s := range samples {
		if s != (Sample{Metric: "http_invoke.overhead", Unit: "ms", Value: 4}) {
			t.Fatal(s)
		}
	}
	if !reflect.DeepEqual(order, []string{"http", "cli", "cli", "http", "http", "cli", "cli", "http"}) {
		t.Fatal(order)
	}
	fail := func(context.Context) (float64, []byte, error) { return 0, nil, errors.New("probe failed") }
	if _, err := measureHTTPPairs(context.Background(), Config{Runs: 1}, fail, probe("cli", 10)); err == nil {
		t.Fatal("failed probe accepted")
	}
	different := func(context.Context) (float64, []byte, error) { return 0, []byte(`{}`), nil }
	if _, err := measureHTTPPairs(context.Background(), Config{Runs: 1}, different, probe("cli", 10)); err == nil {
		t.Fatal("different envelopes accepted")
	}
}
