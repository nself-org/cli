package config

import (
	"slices"
	"testing"
)

func TestNoAutoAddKnown(t *testing.T) {
	if !slices.Contains(knownEnvVars, "NSELF_NO_AUTO_ADD") {
		t.Fatal("NSELF_NO_AUTO_ADD must be known")
	}
}
