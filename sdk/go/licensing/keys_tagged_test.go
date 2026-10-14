//go:build nself_testkeys

package licensing

import "testing"

func TestTestKeysCompiledTagged(t *testing.T) {
	t.Setenv("NSELF_ENTITLEMENT_TEST_PUBKEY", "")
	if !TestKeysCompiled() {
		t.Fatal("test-tag build hid compiled-in test keys when env was empty")
	}
}
