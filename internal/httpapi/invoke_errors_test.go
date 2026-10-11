package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

type fakeInvokeError struct{}

func (fakeInvokeError) Error() string { return "fake private argv --value fake-secret" }

func TestNDJSONCanceledTrailer(t *testing.T) {
	s := server(t)
	var log bytes.Buffer
	s.o.Log = &log
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.o.Invoker = func(_ context.Context, _ *cmdregistry.Registry, _ string, _ invoke.Request, o invoke.Options) (invoke.Result, error) {
		if _, err := o.Stdout.Write([]byte(streamRecord)); err != nil {
			t.Fatal(err)
		}
		cancel()
		return invoke.Result{ExitCode: 0}, ctx.Err()
	}
	w := call(t, s, "/v1/commands/stream", "{}", func(r *http.Request) {
		r.Header.Set("Accept", "application/x-ndjson")
		*r = *r.WithContext(ctx)
	})
	expect(t, w, 200, "")
	if w.Body.String() != streamRecord || headerValue(w.Result().Trailer, "X-Nself-Exit-Code") != "-1" {
		t.Fatal(w.Body.String(), w.Result().Trailer)
	}
	if strings.Contains(log.String(), "invoke error:") {
		t.Fatal("cancellation logged as invocation failure", log.String())
	}
}

func TestNDJSONEmptyOutput(t *testing.T) {
	for _, zeroWrite := range []bool{false, true} {
		t.Run(fmt.Sprint(zeroWrite), func(t *testing.T) {
			s := server(t)
			s.o.Invoker = func(_ context.Context, _ *cmdregistry.Registry, _ string, _ invoke.Request, o invoke.Options) (invoke.Result, error) {
				if zeroWrite {
					if n, err := o.Stdout.Write(nil); n != 0 || err != nil {
						t.Fatal(n, err)
					}
				}
				return invoke.Result{}, nil
			}
			w := call(t, s, "/v1/commands/stream", "{}", func(r *http.Request) { r.Header.Set("Accept", "application/x-ndjson") })
			expect(t, w, 502, "E422")
			var envelope struct {
				SchemaVersion string `json:"schema_version"`
				Command       string `json:"command"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.SchemaVersion != "1" || envelope.Command != "stream" || headerValue(w.Header(), "Content-Type") != "application/json" || headerValue(w.Header(), "X-Nself-Exit-Code") == "0" || headerValue(w.Header(), "X-Nself-Request-Id") == "" {
				t.Fatal(envelope, w.Header())
			}
		})
	}
}

func TestInvokeErrorLogsTypeOnly(t *testing.T) {
	for _, mode := range []string{"document", "before-stream", "started-stream"} {
		t.Run(mode, func(t *testing.T) {
			s := server(t)
			var log bytes.Buffer
			s.o.Log = &log
			s.o.Invoker = func(_ context.Context, _ *cmdregistry.Registry, _ string, _ invoke.Request, o invoke.Options) (invoke.Result, error) {
				if mode == "started-stream" {
					if _, err := o.Stdout.Write([]byte(streamRecord)); err != nil {
						t.Fatal(err)
					}
				}
				return invoke.Result{}, fakeInvokeError{}
			}
			path := "/v1/commands/config/get"
			if mode != "document" {
				path = "/v1/commands/stream"
			}
			w := call(t, s, path, "{}", func(r *http.Request) {
				if mode != "document" {
					r.Header.Set("Accept", "application/x-ndjson")
				}
			})
			status := 502
			if mode == "started-stream" {
				status = 200
				if w.Body.String() != streamRecord || headerValue(w.Result().Trailer, "X-Nself-Exit-Code") != "-1" {
					t.Fatal(w.Body.String(), w.Result().Trailer)
				}
			} else {
				expect(t, w, status, "E422")
			}
			lines := strings.Split(strings.TrimSpace(log.String()), "\n")
			want := fmt.Sprintf("invoke error: %T", fakeInvokeError{})
			if len(lines) != 2 || lines[0] != want || !strings.HasPrefix(lines[1], fmt.Sprintf("\"POST\" %q %d ", path, status)) || !strings.HasSuffix(lines[1], "ms") {
				t.Fatal("unexpected log", log.String())
			}
			if strings.Contains(log.String(), fakeInvokeError{}.Error()) {
				t.Fatal("error message leaked", log.String())
			}
		})
	}
}
