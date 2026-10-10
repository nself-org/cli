package config

// explain.go — which source sets one configuration key, and which wins.
//
// Purpose: the one cascade walk behind `nself config explain KEY` and
//          `nself config env explain VAR`. Both commands render the same
//          Explanation, so they can never disagree about the winner.
// Inputs:  the project directory, the active environment name (see
//          ResolveEnv) and the key. The escape hatch NSELF_LEGACY_ENV_ORDER is
//          read here exactly as Load() reads it.
// Outputs: an Explanation: the cascade, every existing file that sets the
//          key (lowest precedence first), the winning file, the process
//          environment value and the documented default. Values are returned
//          in clear; the caller decides what to print (redaction is a
//          rendering rule, not a lookup rule).
// Constraints: Read only. Precedence is the loader's, not a new one: Load()
//          applies every cascade file with godotenv.Overload, so a file that
//          sets the key replaces a process environment value, and the process
//          environment is the source only when no file sets the key.
// SPORT:   cli/internal/config — CLI-R18 env cascade canon.

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Setter is one existing cascade file that sets the key.
type Setter struct {
	// File is the cascade file name relative to the project directory.
	File string
	// Value is the value the file sets, in clear.
	Value string
}

// Explanation is the answer to "where does this key come from".
type Explanation struct {
	// Key is the variable name asked about.
	Key string
	// Known reports whether the key is in KnownEnvVars().
	Known bool
	// Default is DefaultFor(Key); empty when the key has no static default.
	Default string
	// Env is the active environment name the cascade was built for.
	Env string
	// Legacy reports that NSELF_LEGACY_ENV_ORDER selected the legacy order.
	Legacy bool
	// Cascade is every file the loader consults, lowest precedence first.
	Cascade []CascadeFile
	// Setters are the existing cascade files that set the key, in cascade order.
	Setters []Setter
	// ProcessSet and ProcessValue describe the process environment.
	ProcessSet   bool
	ProcessValue string
}

// Winner returns the winning cascade file and its value; ok is false when no
// file sets the key. The cascade is lowest precedence first, so the last
// setter wins.
func (e Explanation) Winner() (file, value string, ok bool) {
	if len(e.Setters) == 0 {
		return "", "", false
	}
	w := e.Setters[len(e.Setters)-1]
	return w.File, w.Value, true
}

// Effective names the source the loader ends up using and its value:
// a cascade file name, "process environment", "default" or "unset".
func (e Explanation) Effective() (source, value string) {
	if f, v, ok := e.Winner(); ok {
		return f, v
	}
	if e.ProcessSet {
		return "process environment", e.ProcessValue
	}
	if e.Default != "" {
		return "default", e.Default
	}
	return "unset", ""
}

// ExplainKey walks the env cascade of projectDir for the active environment
// env and reports every source of key. A file that exists but cannot be read
// is an error.
func ExplainKey(projectDir, env, key string) (Explanation, error) {
	legacy := LegacyOrderActive()
	exp := Explanation{
		Key:     key,
		Default: DefaultFor(key),
		Env:     env,
		Legacy:  legacy,
		Cascade: EnvCascade(projectDir, env, legacy),
	}
	for _, k := range KnownEnvVars() {
		if k == key {
			exp.Known = true
			break
		}
	}
	for _, f := range exp.Cascade {
		if !f.Exists {
			continue
		}
		vars, err := godotenv.Read(f.Path)
		if err != nil {
			return Explanation{}, fmt.Errorf("reading %s: %w", f.Path, err)
		}
		if v, ok := vars[key]; ok {
			exp.Setters = append(exp.Setters, Setter{File: f.Name, Value: v})
		}
	}
	exp.ProcessValue, exp.ProcessSet = os.LookupEnv(key)
	return exp, nil
}
