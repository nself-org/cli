package commands

import "testing"

func TestRelocateNoAutoAddTruthy(t *testing.T) {
	for _, tc := range []struct {
		value  string
		refuse bool
	}{
		{"1", true}, {"true", true}, {"TRUE", true}, {"TrUe", true},
		{"", false}, {"0", false}, {"false", false}, {"yes", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			_, calls, _ := relocateSetup(t, "", publishStub(t))
			t.Setenv(envNoAutoAdd, tc.value)
			handled, err := relocateBreakout("ci", nil)
			if !handled {
				t.Fatal("ci not handled")
			}
			if tc.refuse {
				if aiCode(err) != "E404" || len(*calls) != 0 {
					t.Fatalf("want refusal without install: code=%s calls=%v", aiCode(err), *calls)
				}
			} else if err != nil || len(*calls) != 1 {
				t.Fatalf("want install: err=%v calls=%v", err, *calls)
			}
		})
	}
}
