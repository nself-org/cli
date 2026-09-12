package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDestroy_NoBackupFlag_Refused(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}

	_, err := Destroy(context.Background(), fc, DestroyRequest{ServerID: 1})
	if !errors.Is(err, ErrNoBackup) {
		t.Fatalf("Destroy error = %v, want ErrNoBackup", err)
	}
	if _, ok := fc.servers[1]; !ok {
		t.Fatal("server was deleted despite no --snapshot or --force-no-backup — this is the design-1 gate")
	}
}

func TestDestroy_ForceNoBackup_Proceeds(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}

	result, err := Destroy(context.Background(), fc, DestroyRequest{ServerID: 1, ForceNoBackup: true})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.SnapshotID != 0 {
		t.Errorf("SnapshotID = %d, want 0 (no snapshot requested)", result.SnapshotID)
	}
	if _, ok := fc.servers[1]; ok {
		t.Fatal("server still exists after Destroy with --force-no-backup")
	}
}

func TestDestroy_Snapshot_WaitsForAvailableBeforeDeleting(t *testing.T) {
	oldInterval := snapshotPollInterval
	snapshotPollInterval = time.Millisecond
	t.Cleanup(func() { snapshotPollInterval = oldInterval })

	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}
	pc := &pollCountingClient{fakeClient: fc, flipAfter: 1}

	result, err := Destroy(context.Background(), pc, DestroyRequest{
		ServerID: 1, TakeSnapshot: true, SnapshotWait: time.Second,
	})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.SnapshotID == 0 {
		t.Error("SnapshotID = 0, want the created snapshot's image ID")
	}
	if _, ok := fc.servers[1]; ok {
		t.Fatal("server still exists after a verified snapshot + destroy")
	}
}

func TestDestroy_SnapshotNeverAvailable_ServerNeverDeleted(t *testing.T) {
	oldInterval := snapshotPollInterval
	snapshotPollInterval = time.Millisecond
	t.Cleanup(func() { snapshotPollInterval = oldInterval })

	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}
	// flipAfter huge: the image never reaches "available" inside the wait budget.
	pc := &pollCountingClient{fakeClient: fc, flipAfter: 1_000_000}

	_, err := Destroy(context.Background(), pc, DestroyRequest{
		ServerID: 1, TakeSnapshot: true, SnapshotWait: 20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("Destroy: want error when the snapshot never becomes available")
	}
	if _, ok := fc.servers[1]; !ok {
		t.Fatal("server was deleted despite the snapshot never reaching status=available")
	}
}

func TestDestroy_DefaultRetainsPrimaryIP(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}
	fc.primaryIPs[10] = &PrimaryIP{ID: 10, IP: "203.0.113.10", AssigneeID: 1, AutoDelete: true}

	result, err := Destroy(context.Background(), fc, DestroyRequest{ServerID: 1, ForceNoBackup: true})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if len(result.RetainedIPs) != 1 {
		t.Fatalf("RetainedIPs = %v, want the one primary IP", result.RetainedIPs)
	}
	if fc.primaryIPs[10].AutoDelete {
		t.Error("primary IP auto_delete must be false by the time the server is destroyed")
	}
}

func TestDestroy_ReleaseIP_ReleasesInstead(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}
	fc.primaryIPs[10] = &PrimaryIP{ID: 10, IP: "203.0.113.10", AssigneeID: 1, AutoDelete: false}

	result, err := Destroy(context.Background(), fc, DestroyRequest{ServerID: 1, ForceNoBackup: true, ReleaseIP: true})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if len(result.ReleasedIPs) != 1 {
		t.Fatalf("ReleasedIPs = %v, want the one primary IP", result.ReleasedIPs)
	}
}

func TestDestroy_MissingServerID(t *testing.T) {
	fc := newFakeClient()
	if _, err := Destroy(context.Background(), fc, DestroyRequest{ForceNoBackup: true}); err == nil {
		t.Fatal("Destroy: want error when ServerID is 0")
	}
}
