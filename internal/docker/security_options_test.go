package docker

import "testing"

func TestRemappedOptions(t *testing.T) {
	cases := []struct {
		opts []string
		want bool
	}{
		{nil, false},
		{[]string{"name=seccomp,profile=builtin", "name=cgroupns"}, false},
		{[]string{"name=seccomp,profile=builtin", "name=rootless"}, true},
		{[]string{"name=userns"}, true},
		{[]string{"name=rootless,foo=bar"}, true},
		{[]string{"name=rootlessish", "name=usernsx"}, false},
	}
	for _, tc := range cases {
		if got := remappedOptions(tc.opts); got != tc.want {
			t.Errorf("remappedOptions(%v) = %v, want %v", tc.opts, got, tc.want)
		}
	}
}
