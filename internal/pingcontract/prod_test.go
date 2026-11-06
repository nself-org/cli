//go:build pingprod

package pingcontract

// Production probe. OWNER-RUN ONLY: it calls https://ping.nself.org.
// Read-only GETs plus one anonymous POST /license/validate with an invalid
// (obviously fake) key. A request-log transport refuses any other method or
// path and fails the test.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const prodBase = "https://ping.nself.org"

// allowed is the complete set of requests this variant may make.
var allowed = map[string]bool{
	"GET /health":                  true,
	"GET /version":                 true,
	"GET /license/revocation-list": true,
	"POST /license/validate":       true,
}

type loggingTransport struct {
	mu   sync.Mutex
	log  []string
	base http.RoundTripper
	t    *testing.T
}

func (l *loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	key := r.Method + " " + r.URL.Path
	l.mu.Lock()
	l.log = append(l.log, key)
	l.mu.Unlock()
	if r.URL.Host != "ping.nself.org" || !allowed[key] {
		err := fmt.Errorf("pingprod refuses %s %s", r.Method, r.URL)
		l.t.Error(err)
		return nil, err
	}
	return l.base.RoundTrip(r)
}

func (l *loggingTransport) assertLog(t *testing.T) {
	t.Helper()
	posts := 0
	for _, k := range l.log {
		if !allowed[k] {
			t.Errorf("request log holds a disallowed request: %s", k)
		}
		if strings.HasPrefix(k, "POST ") {
			posts++
		}
	}
	if posts != 1 {
		t.Errorf("want exactly one POST (validate), got %d in %v", posts, l.log)
	}
}

func TestProdPingContract(t *testing.T) {
	lt := &loggingTransport{base: http.DefaultTransport, t: t}
	client := &http.Client{Transport: lt, Timeout: 15 * time.Second}
	s := loadSpec(t)

	do := func(method, path string, body []byte) (int, []byte) {
		req, err := http.NewRequest(method, prodBase+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, b
	}

	for _, tg := range targets {
		if tg.Method != "GET" {
			continue
		}
		code, b := do(tg.Method, tg.Path, nil)
		if code != http.StatusOK {
			t.Errorf("%s %s: HTTP %d", tg.Method, tg.Path, code)
			continue
		}
		var generic map[string]any
		if err := json.Unmarshal(b, &generic); err != nil {
			t.Errorf("%s: body is not a JSON object: %v", tg.Path, err)
			continue
		}
		for _, req := range requiredOf(s, tg.Schema) {
			if _, ok := generic[req]; !ok {
				t.Errorf("%s: live response lacks required field %q", tg.Path, req)
			}
		}
		if tg.Value != nil {
			if err := json.Unmarshal(b, tg.Value()); err != nil {
				t.Errorf("%s: live response does not decode into %s: %v", tg.Path, tg.Name, err)
			}
		}
	}

	if code, _ := do("GET", "/health", nil); code != http.StatusOK {
		t.Errorf("GET /health: HTTP %d", code)
	}

	// One anonymous validate with an invalid key. 200 (valid:false) or 4xx is fine.
	body := []byte(`{"license_key":"nself_pro_00000000000000000000000000000000"}`)
	code, b := do("POST", "/license/validate", body)
	if code >= 500 {
		t.Fatalf("POST /license/validate: HTTP %d", code)
	}
	var v struct {
		Valid bool `json:"valid"`
	}
	if code == http.StatusOK {
		if err := json.Unmarshal(b, &v); err != nil || v.Valid {
			t.Errorf("invalid key answered valid=%v err=%v", v.Valid, err)
		}
	}
	lt.assertLog(t)
}

func requiredOf(s *spec, name string) []string {
	var out []string
	req, _ := s.schema(name)["required"].([]any)
	for _, r := range req {
		out = append(out, r.(string))
	}
	return out
}
