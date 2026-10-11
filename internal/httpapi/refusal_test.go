package httpapi

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/nself-org/cli/internal/invoke"
)

func TestChildStatusCannotSpoofConfirmation(t *testing.T) {
	s := server(t)
	for _, body := range []string{
		`{"data":{"ok":true},"Error":{"code":"E431"}}`,
		`{"data":{"CONFIRMATION_REQUIRED":true}}`,
		`{"data":{"confirmation_required":true}}`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Setenv("HTTPAPI_BODY", body)
			w := call(t, s, "/v1/commands/config/get", "{}", nil)
			expect(t, w, 200, "")
			if w.Body.String() != body || headerValue(w.Header(), "Retry-After") != "" {
				t.Fatalf("child body or status header changed: %s %v", w.Body, w.Header())
			}
		})
	}
	expect(t, call(t, s, "/v1/commands/destroy", "{}", nil), 428, "")
}

func TestStatusEnvelopeKeysExact(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"data":{},"Error":{"code":"E431"}}`, 200},
		{`{"error":{"Code":"E431","class":"user"}}`, 422},
		{`{"error":{"Class":"infra"}}`, 500},
		{`{"Data":{}}`, 500},
	} {
		if got := status(invoke.Result{Stdout: []byte(tc.body)}); got != tc.want {
			t.Errorf("status(%s) = %d, want %d", tc.body, got, tc.want)
		}
	}
}

func TestDocumentAcceptSelection(t *testing.T) {
	s := server(t)
	for _, tc := range []struct {
		accept string
		want   int
	}{
		{"application/json, application/x-ndjson;q=0.1", 200},
		{"application/x-ndjson;q=1, application/json;q=0.1", 200},
		{"application/x-ndjson", 406},
		{"application/json;q=0, application/x-ndjson", 406},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			w := call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) {
				r.Header.Set("Accept", tc.accept)
			})
			expect(t, w, tc.want, "")
			if headerValue(w.Header(), "Content-Type") != "application/json" {
				t.Fatal(w.Header())
			}
		})
	}
}

func TestPreauthenticationRefusalsHideRoute(t *testing.T) {
	s := server(t)
	for _, tc := range []struct {
		name   string
		modify func(*http.Request)
		status int
		code   string
	}{
		{"host", func(r *http.Request) { r.Host = "fake.example:3857" }, 421, "E428"},
		{"origin", func(r *http.Request) { r.Header.Set("Origin", "null") }, 403, "E429"},
		{"auth", func(r *http.Request) { r.Header.Del("Authorization") }, 401, "E426"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			known := call(t, s, "/v1/commands/config/get", "{}", tc.modify)
			unknown := call(t, s, "/v1/commands/fake-plugin-command", "{}", tc.modify)
			expect(t, known, tc.status, tc.code)
			expect(t, unknown, tc.status, tc.code)
			if !bytes.Equal(known.Body.Bytes(), unknown.Body.Bytes()) {
				t.Fatalf("known and unknown route responses differ: %s versus %s", known.Body, unknown.Body)
			}
			if bytes.Contains(known.Body.Bytes(), []byte("config get")) || headerValue(known.Header(), "X-Nself-Request-Id") != invoke.RequestID("", nil, invoke.Request{}) || headerValue(known.Header(), "X-Nself-Request-Id") != headerValue(unknown.Header(), "X-Nself-Request-Id") {
				t.Fatalf("preauthentication response reveals command: %s %v", known.Body, known.Header())
			}
		})
	}
}
