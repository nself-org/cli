package output

import "testing"

func TestJSONRequestedFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"none", []string{"status"}, false},
		{"empty", nil, false},
		{"bare", []string{"status", "--json"}, true},
		{"true", []string{"--json=true", "status"}, true},
		{"one", []string{"--json=1"}, true},
		{"false", []string{"--json=false", "status"}, false},
		{"zero", []string{"--json=0"}, false},
		{"after terminator", []string{"exec", "--", "--json"}, false},
		{"before terminator", []string{"--json", "--", "x"}, true},
		{"last wins on", []string{"--json=false", "--json"}, true},
		{"last wins off", []string{"--json", "--json=false"}, false},
		{"garbage value", []string{"--json=maybe"}, false},
		{"empty value", []string{"--json="}, false},
		{"prefix only", []string{"--jsonx"}, false},
		{"separate value ignored", []string{"--json", "false"}, true},
		{"other flag", []string{"--format", "json"}, false},
	}
	for _, c := range cases {
		if got := JSONRequestedFromArgs(c.args); got != c.want {
			t.Errorf("%s: JSONRequestedFromArgs(%q) = %v, want %v", c.name, c.args, got, c.want)
		}
	}
}
