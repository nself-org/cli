package build

// Purpose: read the DOCKER_HOST a compose env_file finally assigns.
// Inputs: the env_file bytes, and whether it is a raw (non-interpolated) file.
// Outputs: the last DOCKER_HOST value and whether the file assigns it.
// Constraints: Compose lets the LAST assignment win, so only the last line
// counts (an earlier "$" line must never hide a later literal one), and only
// this key is read: the syntax of other keys is not the guard's business.

import "strings"

func lastDockerHost(data []byte, raw bool) (string, bool) {
	last, found := "", false
	for _, line := range strings.Split(string(data), "\n") {
		k, v, has := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
		if has && strings.TrimSpace(k) == "DOCKER_HOST" {
			last, found = v, true
		}
	}
	if !found || raw {
		return last, found
	}
	last = strings.TrimSpace(last)
	if len(last) >= 2 && (last[0] == '"' || last[0] == '\'') {
		if i := strings.IndexByte(last[1:], last[0]); i >= 0 {
			return last[1 : i+1], true
		}
		return "$", true // an unterminated quote cannot be read: unresolved, so the guard refuses
	}
	if i := strings.Index(last, " #"); i >= 0 {
		last = last[:i]
	}
	return strings.TrimSpace(last), true
}
