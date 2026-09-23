package docker

import (
	"reflect"
	"testing"
)

// Docker collapses consecutive published ports into one range entry; the
// project's own MinIO ("9000-9001") must count as owned, or nself start
// refuses over its own container (nself-web prod, 2026-09-23).
func TestExtractComposeHostPortsRanges(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"127.0.0.1:5432->5432/tcp", []int{5432}},
		{"0.0.0.0:5432->5432/tcp", []int{5432}},
		{":::5432->5432/tcp", []int{5432}},
		{"127.0.0.1:9000-9001->9000-9001/tcp", []int{9000, 9001}},
		{"[::]:9000-9002->9000-9002/tcp", []int{9000, 9001, 9002}},
		{"9001-9000->9001-9000/tcp", nil},
		{"1-5000->1-5000/tcp", nil},
		{"5432/tcp", nil},
		{"", nil},
	}
	for _, c := range cases {
		if got := extractComposeHostPorts(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("extractComposeHostPorts(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := extractComposeHostPort("127.0.0.1:9000-9001->9000-9001/tcp"); got != 0 {
		t.Errorf("extractComposeHostPort(range) = %d, want 0 (ranges go through extractComposeHostPorts)", got)
	}
}
