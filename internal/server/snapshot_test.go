package server

import (
	"context"
	"testing"
	"time"
)

// pollCountingClient wraps fakeClient and flips its one image to
// status=available after flipAfter GetImage calls, so the wait loop is
// exercised across more than one poll tick without any goroutine/race.
type pollCountingClient struct {
	*fakeClient
	calls     int
	flipAfter int
}

func (p *pollCountingClient) GetImage(ctx context.Context, id int64) (*Image, error) {
	p.calls++
	if p.calls >= p.flipAfter {
		p.images[id].Status = "available"
	}
	return p.fakeClient.GetImage(ctx, id)
}

func TestTakeVerifiedSnapshot_BecomesAvailable(t *testing.T) {
	oldInterval := snapshotPollInterval
	snapshotPollInterval = time.Millisecond
	t.Cleanup(func() { snapshotPollInterval = oldInterval })

	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}
	pc := &pollCountingClient{fakeClient: fc, flipAfter: 3}

	img, act, err := pc.CreateSnapshot(context.Background(), 1, "test")
	if err != nil {
		t.Fatalf("seed CreateSnapshot: %v", err)
	}

	got, err := waitForSnapshotAvailable(context.Background(), pc, img.ID, act, time.Now().Add(time.Second), time.Second)
	if err != nil {
		t.Fatalf("waitForSnapshotAvailable: %v", err)
	}
	if got.Status != "available" {
		t.Errorf("status = %q, want available", got.Status)
	}
	if pc.calls < 3 {
		t.Errorf("GetImage called %d times, want the wait loop to actually poll", pc.calls)
	}
}

func TestTakeVerifiedSnapshot_ActionError_Aborts(t *testing.T) {
	fc := newFakeClient()
	act := &Action{ID: 99, Status: "error", Error: &ActionError{Code: "resource_limit_exceeded", Message: "no capacity"}}
	fc.actions[99] = act
	fc.images[1] = &Image{ID: 1, Status: "creating"}

	_, err := waitForSnapshotAvailable(context.Background(), fc, 1, act, time.Now().Add(time.Second), time.Second)
	if err == nil {
		t.Fatal("want error when the action itself failed")
	}
}

func TestTakeVerifiedSnapshot_TimesOut(t *testing.T) {
	oldInterval := snapshotPollInterval
	snapshotPollInterval = time.Millisecond
	t.Cleanup(func() { snapshotPollInterval = oldInterval })

	fc := newFakeClient()
	fc.images[1] = &Image{ID: 1, Status: "creating"} // never flips to available
	act := &Action{ID: 1, Status: "running"}
	fc.actions[1] = act

	_, err := waitForSnapshotAvailable(context.Background(), fc, 1, act, time.Now().Add(10*time.Millisecond), 10*time.Millisecond)
	if err == nil {
		t.Fatal("want timeout error when snapshot never becomes available")
	}
}
