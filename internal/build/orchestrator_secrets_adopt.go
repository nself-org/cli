package build

// orchestrator_secrets_adopt.go — adopt the live credential instead of
// generating a new one (D-0506). A project built before credentials were
// persisted has a running database whose password lives only in the previous
// build's .nself/compose.env; a fresh random value would lock the build out.
// Inputs:  workdir, the build Sink, the env key.
// Outputs: the literal value the previous build rendered, if any.
// Constraints: reads through the Sink so plan mode sees the same bytes. A
//              ${VAR} reference or an empty value is not a credential. Values
//              are never logged. An adopted value is used for this build's
//              render only and is never written to .env.secrets. .nself/
//              compose.env holds whichever ENV built last (it records that
//              ENV in a marker comment, and a different ENV adopts nothing); adoption only
//              happens when no env file and no process export supplies the
//              key, so it can only pick up a value an earlier build got from a
//              process export or generated itself. .env.computed is not read:
//              it never carries these keys literally.

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// adoptLiveCredential returns key's literal value from the previous build's
// .nself/compose.env.
// When the file names the ENV that wrote it (composeEnvMarker) and it differs
// from env, nothing is adopted: a prod build must not inherit a dev password
// (D-1149). A file without the marker predates it and is still adopted, so a
// project built before the marker keeps its live database password.
func adoptLiveCredential(sink Sink, workdir, env, key string) (string, bool) {
	data, err := sink.ReadFile(filepath.Join(workdir, composeEnvFile))
	if err != nil {
		return "", false
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var found string
	var have bool
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if built, ok := strings.CutPrefix(line, composeEnvMarker); ok {
			if strings.TrimSpace(built) != env {
				return "", false
			}
			continue
		}
		if line == "" || line[0] == '#' {
			continue
		}
		k, raw, ok := strings.Cut(line, "=")
		if !ok || k != key {
			continue
		}
		if v := parseEnvValue(raw); v != "" && !have && !strings.HasPrefix(v, "${") {
			found, have = v, true
		}
	}
	return found, have
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
