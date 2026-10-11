package main

// Purpose: verify the HTTP API mux exception remains narrow and fail-closed.
// Inputs: the real server and isolated synthetic server source files.
// Outputs: authentication classification assertions.
// Constraints: no listeners, credentials or production source changes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateHTTPAPIMux(t *testing.T) {
	const safeSource = `package httpapi
import "net/http"
type Server struct { mux *http.ServeMux }
var metadata = "fixture"
func (s *Server) Handler() http.Handler { return s.logged(s.authorized(s.mux)) }
func (s *Server) Mount() { s.mux.HandleFunc("/fixture", nil) }
`
	tests := []struct {
		name   string
		source string
		file   string
		want   bool
	}{
		{name: "real internal httpapi server", file: filepath.Join("..", "..", "internal", "httpapi", "server.go"), want: true},
		{name: "authenticated private mux", source: safeSource, want: true},
		{name: "Handler returns bare mux", source: strings.Replace(safeSource, "s.logged(s.authorized(s.mux))", "s.mux", 1)},
		{name: "registration on another mux", source: strings.Replace(safeSource, "s.mux.HandleFunc", "other.mux.HandleFunc", 1)},
		{name: "registration in non Server function", source: strings.Replace(safeSource, "func (s *Server) Mount()", "func Register(s *Server)", 1)},
		{name: "server outside internal httpapi", source: safeSource, file: filepath.Join("external", "httpapi", "server.go")},
		{name: "unparsable file", source: "package ("},
		{name: "Handler has unsafe return branch", source: strings.Replace(safeSource, "return s.logged(s.authorized(s.mux))", "if false { return s.mux }; return s.authorized(s.mux)", 1)},
		{name: "Handler has no return", source: strings.Replace(safeSource, "return s.logged(s.authorized(s.mux))", "panic(\"fixture\")", 1)},
		{name: "Handler has wrong receiver", source: strings.Replace(safeSource, "func (s *Server) Handler", "func (s *Other) Handler", 1)},
		{name: "Handler has value receiver", source: strings.Replace(safeSource, "func (s *Server) Handler", "func (s Server) Handler", 1)},
		{name: "Handler calls free function", source: strings.Replace(safeSource, "s.logged(s.authorized(s.mux))", "authorized(s.mux)", 1)},
		{name: "Handler calls other receiver", source: strings.Replace(safeSource, "s.logged(s.authorized(s.mux))", "other.authorized(s.mux)", 1)},
		{name: "Handler calls unknown wrapper", source: strings.Replace(safeSource, "s.logged(s.authorized(s.mux))", "s.unknown(s.mux)", 1)},
		{name: "Handler authorizes other mux", source: strings.Replace(safeSource, "s.authorized(s.mux)", "s.authorized(other.mux)", 1)},
		{name: "Handler passes multiple arguments", source: strings.Replace(safeSource, "s.authorized(s.mux)", "s.authorized(s.mux, nil)", 1)},
		{name: "Handler authorizes identifier", source: strings.Replace(safeSource, "s.authorized(s.mux)", "s.authorized(mux)", 1)},
		{name: "Handler authorizes other field", source: strings.Replace(safeSource, "s.authorized(s.mux)", "s.authorized(s.other)", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := tt.file
			if tt.source != "" {
				if file == "" {
					file = filepath.Join("internal", "httpapi", "server.go")
				}
				file = filepath.Join(t.TempDir(), file)
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(tt.source), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			routes, err := scanFile(file)
			if tt.name == "unparsable file" {
				if err == nil {
					t.Fatal("fixture unexpectedly parsed")
				}
				routes = []RouteRegistration{{File: file, Line: 1, Path: "/fixture"}}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(routes) == 0 {
				t.Fatal("fixture contains no route registration")
			}
			for _, route := range routes {
				if got := privateHTTPAPIMux(route); got != tt.want {
					t.Errorf("privateHTTPAPIMux(%s:%d) = %v, want %v", route.File, route.Line, got, tt.want)
				}
				result := ClassifyRoute(route, true)
				if result.Passed != tt.want || result.Exempt {
					t.Errorf("ClassifyRoute(%+v) = %+v, want Passed=%v without exemption", route, result, tt.want)
				}
				if tt.want && result.MatchedMW != "httpapi.Server.authorized" {
					t.Errorf("matched middleware = %q", result.MatchedMW)
				}
			}
		})
	}
}
