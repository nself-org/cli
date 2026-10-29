package config

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestAuthPassthroughDoesNotWarn(t *testing.T) {
	var out bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	defer slog.SetDefault(previous)
	warnUnknownEnvVars(map[string]string{"AUTH_JWT_CUSTOM_CLAIMS": "{}", "AUTH_NEW_OPTION": "true", "HASURA_AUTH_NEW_OPTION": "true"}, knownEnvVars)
	if out.Len() != 0 {
		t.Fatalf("accepted auth passthrough warned: %s", &out)
	}
}
