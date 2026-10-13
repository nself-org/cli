package imageswitch

// verify.go — verify the restore, and re-check the old cluster (EPIC D6 steps 6 and 7).
//
// Purpose: step 6 proves the new volume equals what was dumped; step 7 proves
// the old cluster is still what was dumped, so nothing written in between is
// lost by the switch. Both abort with E510 and switch nothing.
// Inputs: the stage captures. Outputs: PhaseVerified, PhaseRechecked.
// Constraints: the recheck never reads pg_stat counters (the commands' own reads
// move them) and runs the same documents as the dump, on a fresh connection.

import (
	"context"
	"strings"
)

// maxReported bounds the differences printed in an E510.
const maxReported = 12

// mismatch builds the E510 error for a list of differences.
func mismatch(what string, diffs []string) error {
	shown := diffs
	more := ""
	if len(shown) > maxReported {
		more = " (and " + itoa(len(diffs)-maxReported) + " more)"
		shown = shown[:maxReported]
	}
	return coded("E510", what, strings.Join(shown, "; ")+more, "")
}

// verify compares the restored cluster with the dump-time capture.
func (s *Switch) verify(context.Context) error {
	diffs, err := s.compareSets("old", "new")
	if err != nil {
		return coded("E510", "the restored cluster could not be compared", err.Error(), "")
	}
	if len(diffs) > 0 {
		return mismatch("the restored cluster differs from the original", diffs)
	}
	s.st.Tables, s.st.Sequences = s.summarize()
	s.say("verified %d databases, %d tables, %d sequences", len(s.st.Databases), s.st.Tables, s.st.Sequences)
	return nil
}

// recheck captures the old cluster again and compares it with the dump-time capture.
func (s *Switch) recheck(ctx context.Context) error {
	s.clearOneShots(ctx)
	s.say("checking the original did not change since the dump")
	if _, stderr, err := s.d.Runtime.RunOneShot(ctx, s.oldClusterSpec("recheck", "recheck.sh")); err != nil {
		return coded("E510", "the original could not be re-checked", s.redact(stderr+"\n"+err.Error(), 3), "")
	}
	diffs, err := s.compareSets("old", "chk")
	if err != nil {
		return coded("E510", "the original could not be re-checked", err.Error(), "")
	}
	if len(diffs) > 0 {
		return mismatch("the original changed after it was dumped", diffs)
	}
	return nil
}
