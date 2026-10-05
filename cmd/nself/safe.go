package main

import (
	"fmt"

	"github.com/nself-org/cli/internal/compat"
)

// execute runs the command tree. In v1.5 mode a panic that escapes it is
// recovered into an ordinary error, so the process still goes through report:
// one error envelope in JSON mode, an "Error: ..." line on stderr, status 1
// (an unclassified failure; a Go crash would exit 2, which means "infra").
// The panic value is carried in the message only; report redacts it. In v1.4
// mode the panic propagates exactly as before.
func execute(run func() error) (err error) {
	// compat.V15(P7-REG-06): a panic crashes the process with the Go runtime trace and exit 2 -> the panic becomes an "internal error" failure reported with exit 1
	if !compat.V15() {
		return run()
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	return run()
}
