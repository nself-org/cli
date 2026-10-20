package httpapi

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFuzzServerHistoryBounded(t *testing.T) {
	for _, name := range []string{"first", "second", "third"} {
		t.Run(name, func(t *testing.T) {
			s, inv := fuzzServer(t)
			if inv.count() != 0 {
				t.Fatal("previous input's invocations retained")
			}
			if s.o.Log != io.Discard {
				t.Fatal("fuzz server retains logs")
			}
			r := httptest.NewRequest("POST", "http://localhost:3857/v1/commands/config/get", strings.NewReader("{}"))
			r.Header.Set("Authorization", "Bearer "+advToken)
			r.Header.Set("Content-Type", "application/json")
			s.Handler().ServeHTTP(httptest.NewRecorder(), r)
			if inv.count() != 1 {
				t.Fatal("request not recorded")
			}
		})
	}
}
