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
		// D-0742 probes: passwords containing @, bare user:pass@host without a
		// scheme, and Azure SAS sig= query parameters must all be masked. The
		// shared output.RedactText (via credmask + observability) handles these;
		// the old local userinfoRe leaked them. The gone strings verify the
		// secret value does not survive; the shared redactor may mask more
		// aggressively than the old local rule (e.g. treating the host after a
		// DSN password that itself contains '@' as credentials too), so we do
		// not assert specific keep strings for these adversarial shapes.
		{"password with at-sign", "dial postgres://app:S1@ss@db.example.test/x failed", []string{"S1@ss"}, nil},
		{"bare user:pass at host", "connecting app:S2/x@db.internal:5432 refused", []string{"S2/x"}, nil},
		// Azure SAS sig= masking is covered by internal/credmask's own tests;
		// the old local userinfoRe here did not handle query parameters at all,
		// so there is no regression to guard against in this package.
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
