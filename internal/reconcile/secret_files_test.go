package reconcile

import (
	"strings"
	"testing"
)

func TestRawSecretFileDiffRedactsEveryByte(t *testing.T) {
	path := ".nself/secrets/POSTGRES_PASSWORD"
	got := Unified(path, []byte("old-secret=left\n#old-comment"), []byte("new-secret=right\n#new-comment\n"))
	if !IsEnvPath(path) || !strings.Contains(got, "[REDACTED]") {
		t.Fatal("raw secret path must be redacted")
	}
	for _, value := range []string{"old-secret", "new-secret", "left", "right", "old-comment", "new-comment"} {
		if strings.Contains(got, value) {
			t.Fatalf("raw secret diff leaked content")
		}
	}
}
