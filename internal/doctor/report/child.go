package report

// child.go: runs `<this binary> doctor` for doctor.txt.
//
// Purpose: capture the doctor run without ever outliving its timeout.
// Inputs: project dir, the os.Executable seam, timeout and wait-delay seams.
// Outputs: combined output; a *TimeoutError when the time ran out (the output
//         read so far is still returned).
// Constraints: fixed "doctor" argument (cannot recurse into doctor report);
//         cmd.WaitDelay bounds the wait for a grandchild that keeps the pipe
//         open after the child is killed (S4). A non-zero exit (failed checks)
//         is not an error.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// DoctorTimeout bounds the doctor child process.
const DoctorTimeout = 120 * time.Second

// doctorWaitDelay bounds how long Run waits for output pipes after a kill.
const doctorWaitDelay = 5 * time.Second

// TimeoutError reports that the doctor child ran out of time.
type TimeoutError struct{ After time.Duration }

func (e *TimeoutError) Error() string { return fmt.Sprintf("doctor timed out after %s", e.After) }

// RunDoctorChild runs `<this binary> doctor` in dir and returns its combined
// output. exe is the seam for os.Executable.
func RunDoctorChild(ctx context.Context, dir string, exe func() (string, error)) (string, error) {
	return runDoctorChild(ctx, dir, exe, DoctorTimeout, doctorWaitDelay)
}

func runDoctorChild(ctx context.Context, dir string, exe func() (string, error), timeout, waitDelay time.Duration) (string, error) {
	self, err := exe()
	if err != nil {
		return "", fmt.Errorf("locating this binary: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, self, "doctor") // #nosec G204 -- own binary, fixed argument
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	cmd.WaitDelay = waitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	if cctx.Err() == context.DeadlineExceeded {
		return out.String(), &TimeoutError{After: timeout}
	}
	if _, isExit := err.(*exec.ExitError); isExit {
		return out.String(), nil
	}
	return out.String(), err
}
