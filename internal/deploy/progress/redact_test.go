package progress

import (
	"strings"
	"testing"
)

// All values are obviously fake test strings.
func TestRedactText(t *testing.T) {
	fakeJWT := "eyJ" + "fakeheader0123456789" + "." + "fakepayload0123456789" + "." + "fakesig0123456789"
	cases := []struct {
		name, in string
		gone     []string // must not survive
		keep     []string // must survive
	}{
		{"env style", "failed: POSTGRES_PASSWORD=fake-pw-1 on host", []string{"fake-pw-1"}, []string{"POSTGRES_PASSWORD=[REDACTED]"}},
		{"flag style", "run with --token=fake-tok-2 again", []string{"fake-tok-2"}, []string{"--token=[REDACTED]"}},
		{"json style", `body {"secret": "fake-sec-3"} rejected`, []string{"fake-sec-3"}, []string{`"secret": [REDACTED]`}},
		{"dsn", "dial postgres:" + "//app:fake-pw-4@db.example.test:5432/x failed", []string{"fake-pw-4"}, []string{"[REDACTED]@db"}},
		{"jwt shaped", "bearer " + fakeJWT + " refused", []string{fakeJWT}, nil},
		{"prose left alone", "invalid token: expired", nil, []string{"invalid token: expired"}},
		{"clean step text", "Restart web (sequenced rolling)", nil, []string{"Restart web (sequenced rolling)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactText(tc.in)
			for _, g := range tc.gone {
				if strings.Contains(got, g) {
					t.Errorf("%q survived in %q", g, got)
				}
			}
			for _, k := range tc.keep {
				if !strings.Contains(got, k) {
					t.Errorf("%q missing from %q", k, got)
				}
			}
			if tc.name == "clean step text" && got != tc.in {
				t.Errorf("clean text changed: %q", got)
			}
		})
	}
}
