package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestFailureRegisteredSummary(t *testing.T) {
	s := &Server{routes: map[string]string{"/v1/commands/config/get": "config get"}}
	for _, code := range []string{"E420", "E425", "E426", "E428", "E429", "E430", "E431", "E432"} {
		t.Run(code, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://refused.example.test:3857/v1/commands/config/get?value=fake-private-value", nil)
			r.Header.Set("Origin", "http://refused-origin.example.test")
			r.Header.Set("Authorization", "Bearer fake-token-0000")
			w := httptest.NewRecorder()
			s.failure(w, r, code, 400)
			var envelope struct {
				Error struct{ Code, Message string }
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Error.Code != code || envelope.Error.Message != errs.Registry[code].Summary {
				t.Fatalf("error = %+v", envelope.Error)
			}
			for _, value := range []string{"refused.example.test", "refused-origin.example.test", "fake-private-value", "fake-token-0000"} {
				if strings.Contains(w.Body.String(), value) {
					t.Fatalf("response echoed request value %q", value)
				}
			}
			noCORS(t, w.Header())
		})
	}
}
