package composeenv

// Purpose: read .nself/compose.env, the interpolation file `nself build`
// writes, for the credential a later build may adopt (D-0506, D-1149).
// Inputs: the file bytes, the ENV the reading build renders for, a key.
// Outputs: the literal value the previous build rendered for the key.
// Constraints: compose.env holds whichever ENV built last. The writer records
//              that ENV in a comment line (Marker); a different ENV adopts
//              nothing, so a prod build never inherits a dev password. A file
//              without the marker predates it and is still read. A ${VAR}
//              reference or an empty value is not a credential. The first
//              literal wins. Values are never logged.

import (
	"bufio"
	"bytes"
	"strings"
)

// Marker prefixes the comment line naming the ENV that wrote the file. It is a
// comment, so docker compose ignores it.
const Marker = "# nself-build-env: "

// EnvName is the ENV a build renders for; an unset env is dev.
func EnvName(env string) string {
	if env == "" {
		return "dev"
	}
	return env
}

// Literal returns key's literal value from compose.env data written by env.
// parse turns the raw text after `=` into the value (it undoes the quoting the
// writer applied).
func Literal(data []byte, env, key string, parse func(string) string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var found string
	var have bool
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if built, ok := strings.CutPrefix(line, Marker); ok {
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
		if v := parse(raw); v != "" && !have && !strings.HasPrefix(v, "${") {
			found, have = v, true
		}
	}
	return found, have
}
