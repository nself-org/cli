package report

// inputs_interp.go: compose values interpolated against the env inputs (S-3).
//
// Purpose: `ADMIN_SECRET: "<head>${SUF}"` and `ADMIN_SECRET: ${PLAIN_VALUE}`
// give the container a string the compose file does not hold; the value of
// PLAIN_VALUE sits in .env under a name that is not secret-like, so nothing
// else learns it. For a compose key that IS secret-named, the value is
// interpolated against every env input and each referenced variable's value
// is learned too.
// Inputs: the compose key and raw value, the env values recorded by
//         recordEnv (cascade layers, compose.env, .env.*, CS env files,
//         compose env_file files, the process environment).
// Outputs: literals added under the compose key.
// Constraints: every candidate value of a variable (one per layer) is tried
//         once; at most maxInterp expansions per value. A literal piece of
//         MinSecretLen or more between references is learned as well.

import (
	"regexp"
	"strings"
)

const maxInterp = 8

var refSplitRe = regexp.MustCompile(`\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)

// recordEnv remembers a variable's value for later interpolation.
func (in *inputSet) recordEnv(k, v string) {
	if !IsKeyName(k) {
		return
	}
	if in.env == nil {
		in.env = map[string][]string{}
	}
	in.env[k] = appendUnique(in.env[k], TrimQuotes(v))
}

// learnInterpolated learns the interpolated forms of a reference-bearing value
// under a secret-named compose key.
func (in *inputSet) learnInterpolated(k, v string) {
	if !IsSecretName(k) || !refSplitRe.MatchString(strings.ReplaceAll(v, "$$", "")) {
		return
	}
	for idx := -1; idx < maxInterp; idx++ { // -1: every variable unset
		pick := idx
		e := runtimeExpander{env: func(name string) (string, bool) {
			c := in.env[name]
			if pick < 0 || len(c) == 0 {
				return "", false
			}
			return c[min(pick, len(c)-1)], true
		}}
		if full := e.expand(v); full != v && !strings.Contains(full, "${") {
			in.learn(k, full)
		}
		if idx >= 0 && idx+1 >= longestCandidates(in.env, v) {
			break
		}
	}
	for _, m := range refSplitRe.FindAllString(v, -1) {
		for _, val := range in.env[refName(m)] {
			in.learn(k, val)
		}
	}
	for _, piece := range refSplitRe.Split(v, -1) {
		if len(piece) >= MinSecretLen && !strings.Contains(piece, "$") {
			in.learn(k, piece)
		}
	}
}

// refName is the variable name in ${NAME...} or $NAME.
func refName(ref string) string {
	ref = strings.TrimPrefix(strings.TrimPrefix(ref, "$"), "{")
	n := 0
	for n < len(ref) && isNameByte(ref[n], false) {
		n++
	}
	return ref[:n]
}

// longestCandidates is the largest number of candidate values among the
// variables v references (at least 1).
func longestCandidates(env map[string][]string, v string) int {
	most := 1
	for _, m := range refSplitRe.FindAllString(v, -1) {
		if n := len(env[refName(m)]); n > most {
			most = n
		}
	}
	return most
}
