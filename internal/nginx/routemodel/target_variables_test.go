package routemodel

import "testing"

func TestTargetVariablesRejected(t *testing.T) {
	for _, tc := range []struct {
		target string
		reject bool
	}{
		{"http://h/$uri", true},
		{"http://h/x?q=$args", true},
		{"http://$host/x", true},
		{"http://h/%24uri?q=%24args", false},
		{"http://h/x?q=literal", false},
	} {
		t.Run(tc.target, func(t *testing.T) {
			if err := validateTarget(tc.target); (err != nil) != tc.reject {
				t.Fatalf("validateTarget = %v, reject=%v", err, tc.reject)
			}
		})
	}
}
