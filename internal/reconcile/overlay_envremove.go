package reconcile

// Purpose: plan and apply dotenv removals with the same statement boundaries.
// Inputs: dotenv bytes and a key. Outputs: bytes without that key or a safe error.
// Constraints: quoted multiline values and unrelated statements stay intact.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// EnvFileWithoutLine removes every statement defining key, including export and multiline forms.
func EnvFileWithoutLine(old []byte, key string) ([]byte, error) {
	before, err := config.ParseEnvBytes("the env file", old)
	if err != nil {
		return nil, err
	}
	if _, present := before[key]; !present {
		return old, nil
	}
	stmts, err := splitEnvStatements(old)
	if err != nil {
		return nil, err
	}
	if err := proveStatements(stmts, key); err != nil {
		return nil, err
	}
	var lines []string
	for _, st := range stmts {
		if st.key != key {
			lines = append(lines, st.lines...)
		}
	}
	out := []byte(strings.Join(lines, "\n"))
	if len(lines) > 0 {
		out = append(out, '\n')
	}
	after, err := config.ParseEnvBytes("the pending env file", out)
	if err != nil {
		return nil, err
	}
	delete(before, key)
	if _, present := after[key]; present || len(after) != len(before) {
		return nil, fmt.Errorf("cannot remove %s without breaking the env file", key)
	}
	for other := range before {
		if _, present := after[other]; !present {
			return nil, fmt.Errorf("cannot remove %s without breaking the env file", key)
		}
	}
	return out, nil
}
