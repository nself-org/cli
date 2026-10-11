package report

// Tests for the per-container log deadline (N3) and the scrub-before-cap
// helper behaviour. No docker is started: the fetch is a seam.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReadContainerLogDeadlineBecomesANote(t *testing.T) {
	var notes []string
	note := func(s string) { notes = append(notes, s) }
	hung := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "partial line\n", ctx.Err()
	}
	got := readContainerLog(context.Background(), 20*time.Millisecond, "proj-web-1", note, hung)
	if !strings.Contains(got, "partial line") || !strings.Contains(got, "timed out") {
		t.Errorf("text = %q", got)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "proj-web-1") {
		t.Errorf("notes = %v", notes)
	}
}

func TestReadContainerLogOtherErrorsAndSuccess(t *testing.T) {
	var notes []string
	note := func(s string) { notes = append(notes, s) }
	ok := func(context.Context) (string, error) { return "fine\n", nil }
	bad := func(context.Context) (string, error) { return "", errors.New("no such container") }
	if got := readContainerLog(context.Background(), time.Second, "c", note, ok); got != "fine\n" {
		t.Errorf("ok text = %q", got)
	}
	if got := readContainerLog(context.Background(), time.Second, "c", note, bad); !strings.Contains(got, "log read failed: no such container") {
		t.Errorf("error text = %q", got)
	}
	// A cancelled parent (Ctrl-C) is not a per-container timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = readContainerLog(ctx, time.Second, "c", note, func(c context.Context) (string, error) { return "", c.Err() })
	if len(notes) != 0 {
		t.Errorf("unexpected notes %v", notes)
	}
}
