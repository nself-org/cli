package signature

import "testing"

func TestPrereleaseComparisonBoundaries(t *testing.T) {
	for _, pair := range [][2]string{{"1", "2"}, {"9", "10"}, {"99999999999999999999", "100000000000000000000"}, {"1", "alpha"}, {"alpha", "beta"}, {"rc", "rc.1"}, {"rc.1", "rc.2"}} {
		t.Run(pair[0]+"-"+pair[1], func(t *testing.T) {
			if !preLess(pair[0], pair[1]) {
				t.Fatal("lower identifier must compare less")
			}
			if preLess(pair[1], pair[0]) {
				t.Fatal("higher identifier must not compare less")
			}
			if preLess(pair[0], pair[0]) {
				t.Fatal("equal identifiers must not compare less")
			}
		})
	}
}
