package invoke

import (
	"strings"
	"testing"
)

// TestChildTransportEnv proves the exact exposure transport survives re-exec
// and an inherited value cannot turn a document request into a stream request.
func TestChildTransportEnv(t *testing.T) {
	const key = "NSELF_INVOKE_TRANSPORT"
	for _, transport := range []string{TransportMCP, TransportHTTP, TransportHTTPStream} {
		env, err := childEnv([]string{key + "=attacker"}, transport, "demo sub", 0)
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				values = append(values, strings.TrimPrefix(kv, key+"="))
			}
		}
		if len(values) != 1 || values[0] != transport {
			t.Errorf("%s: transport values = %v", transport, values)
		}
	}
}
