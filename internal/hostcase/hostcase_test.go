package hostcase

import "testing"

func TestLowerASCII(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", ""},
		{"localhost:3857", "localhost:3857"},
		{"FAKE.EXAMPLE:3857", "fake.example:3857"},
		{"fa\u212Ae.example:3857", "fa\u212Ae.example:3857"},
		{"FA\u212AE.EXAMPLE:3857", "fa\u212Ae.example:3857"},
		{"\u212A\u0130\u017F.AZ", "\u212A\u0130\u017F.az"},
		{" [::ABCD]:03857.\x00\xff", " [::abcd]:03857.\x00\xff"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := LowerASCII(tc.input); got != tc.want {
				t.Fatalf("LowerASCII(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
