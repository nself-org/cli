package deploy

import "testing"

func TestSameNamesRequiresOwnershipEvidence(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{name: "both nil"},
		{name: "both empty", a: []string{}, b: []string{}},
		{name: "nil and empty", b: []string{}},
		{name: "empty and nil", a: []string{}},
		{name: "empty recorded set", a: []string{"app.example.com"}},
		{name: "empty current set", b: []string{"app.example.com"}},
		{name: "matching", a: []string{"app.example.com"}, b: []string{"app.example.com"}, want: true},
		{name: "matching multiple", a: []string{"a.example.com", "b.example.com"}, b: []string{"a.example.com", "b.example.com"}, want: true},
		{name: "different", a: []string{"app.example.com"}, b: []string{"other.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameNames(tt.a, tt.b); got != tt.want {
				t.Errorf("sameNames(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
