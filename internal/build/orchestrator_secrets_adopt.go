package build

// orchestrator_secrets_adopt.go — adopt the live credential before persisting a
// generated one (D-0506). A project built before credentials were persisted has
// a running database whose password lives only in the previous build's
// .nself/compose.env; persisting a fresh random value would lock the build out.
// Inputs:  workdir, the build Sink, the env key.
// Outputs: the literal value the previous build rendered, if any.
// Constraints: reads through the Sink so plan mode sees the same bytes. A
//              ${VAR} reference or an empty value is not a credential. Values
//              are never logged.

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// adoptLiveCredential returns key's literal value from the previous build's
// .nself/compose.env, else from .env.computed.
func adoptLiveCredential(sink Sink, workdir, key string) (string, bool) {
	for _, rel := range []string{composeEnvFile, ".env.computed"} {
		data, err := sink.ReadFile(filepath.Join(workdir, rel))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			k, raw, ok := strings.Cut(line, "=")
			if !ok || k != key {
				continue
			}
			if v := parseEnvValue(raw); v != "" && !strings.HasPrefix(v, "${") {
				return v, true
			}
		}
	}
	return "", false
}

// parseEnvValue is the inverse of config.QuoteComposeEnvValue and
// config.QuoteEnvValue: it strips one layer of single or double quotes and
// undoes the double-quote escapes (\\ \" $$).
func parseEnvValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return strings.NewReplacer(`\\`, `\`, `\"`, `"`, "$$", "$").Replace(raw[1 : len(raw)-1])
	}
	return config.UnquoteEnvValue(raw)
}
