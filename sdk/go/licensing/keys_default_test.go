//go:build !nself_testkeys

package licensing

import "testing"

func TestTestKeysCompiledDefault(t *testing.T) {
	if TestKeysCompiled() {
		t.Fatal("default build enabled test keys")
	}
}
