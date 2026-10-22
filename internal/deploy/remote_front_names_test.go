package deploy

import "testing"

func TestSameNamesRequiresNonemptySets(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, false},
		{"both empty", []string{}, []string{}, false},
		{"left empty", nil, []string{"app.example"}, false},
		{"right empty", []string{"app.example"}, nil, false},
		{"equal", []string{"app.example"}, []string{"app.example"}, true},
		{"different", []string{"app.example"}, []string{"other.example"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameNames(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameNames(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
