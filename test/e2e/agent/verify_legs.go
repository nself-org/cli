//go:build e2eagent

package agent

// Purpose: transcript checks for the two D7 legs: failure recovery and the
// destructive gate.
// Inputs: the transcript records and their paired calls (verify.go).
// Outputs: one error per violated rule.
// Constraints: harness records are matched by their note prefix, which
// flow.go writes from the constants below.

import (
	"bytes"
	"fmt"
)

// Harness notes the leg checks look for.
const (
	NoteStopService   = "stop-service"
	NoteVolumesBefore = "volumes before"
	NoteVolumesAfter  = "volumes after"
)

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func stateOK(c call) bool { d := c.data(); return c.ok() && d != nil && d["state"] == "ok" }

// checkRecoveryLeg wants, after the harness stops a service: a status call
// that reports the unhealthy state, then a successful start, then a status
// call whose data.state is ok.
func checkRecoveryLeg(recs []Record, calls []call) []error {
	stopSeq := 0
	for _, r := range recs {
		if r.Role == RoleHarness && hasPrefix(r.Note, NoteStopService) {
			stopSeq = r.Seq
			break
		}
	}
	if stopSeq == 0 {
		return []error{fmt.Errorf("missing step %q: no harness %s record", "failure recovery", NoteStopService)}
	}
	phase := 0
	for _, c := range calls {
		if c.seq < stopSeq {
			continue
		}
		switch {
		case phase == 0 && c.tool == "nself_status":
			if stateOK(c) {
				return []error{fmt.Errorf("seq %d: status after the stopped service reports ok", c.seq)}
			}
			phase = 1
		case phase == 1 && c.tool == "nself_start" && c.ok():
			phase = 2
		case phase == 2 && c.tool == "nself_status" && stateOK(c):
			return nil
		}
	}
	return []error{fmt.Errorf("missing step %q: status, start, status ok after %s (reached phase %d)", "failure recovery", NoteStopService, phase)}
}

// checkGateLeg wants a reset without confirm answered by a confirmation (not
// an error), never confirmed later, between two equal volume probes.
func checkGateLeg(recs []Record, calls []call) []error {
	var resetSeq int
	for _, c := range calls {
		if c.tool != "nself_reset" {
			continue
		}
		if c.confirmed() {
			return []error{fmt.Errorf("seq %d: the destructive-gate leg confirmed reset", c.seq)}
		}
		if !c.confirmation() {
			return []error{fmt.Errorf("seq %d: reset without confirm did not return a confirmation request", c.seq)}
		}
		if resetSeq == 0 {
			resetSeq = c.seq
		}
	}
	if resetSeq == 0 {
		return []error{fmt.Errorf("missing step %q", "destructive gate")}
	}
	var before, after *Record
	for i := range recs {
		r := &recs[i]
		if r.Role != RoleHarness || r.Kind != KindProbe {
			continue
		}
		if r.Note == NoteVolumesBefore && r.Seq < resetSeq {
			before = r
		}
		if r.Note == NoteVolumesAfter && r.Seq > resetSeq && after == nil {
			after = r
		}
	}
	if before == nil || after == nil {
		return []error{fmt.Errorf("missing step %q: volume probes around reset", "destructive gate")}
	}
	if !bytes.Equal(before.Envelope, after.Envelope) {
		return []error{fmt.Errorf("seq %d: docker volumes changed across the unconfirmed reset", after.Seq)}
	}
	return nil
}
