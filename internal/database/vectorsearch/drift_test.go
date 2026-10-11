package vectorsearch

import "testing"

func TestJSONDrift(t *testing.T) {
	base := `{"version":3,"sources":[{"name":"default","tables":[]}]}`
	cases := []struct {
		name       string
		live, disk string
		want       bool
	}{
		{"equal", base, base, false},
		{"format and key order", base, "{\n \"sources\": [{\"tables\": [], \"name\": \"default\"}],\n \"version\": 3\n}", false},
		{"only resource_version differs", `{"resource_version":7,"version":3,"sources":[{"name":"default","tables":[]}]}`, base, false},
		{"resource_version on both sides", `{"resource_version":7,"version":3,"sources":[{"name":"default","tables":[]}]}`, `{"resource_version":2,"version":3,"sources":[{"name":"default","tables":[]}]}`, false},
		{"real difference", `{"resource_version":7,"version":3,"sources":[]}`, base, true},
		{"invalid disk", base, `{`, true},
	}
	for _, tc := range cases {
		if got := len(jsonDrift([]byte(tc.live), []byte(tc.disk))) > 0; got != tc.want {
			t.Errorf("%s: drift = %v, want %v", tc.name, got, tc.want)
		}
	}
}
