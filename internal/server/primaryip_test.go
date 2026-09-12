package server

import (
	"context"
	"testing"
)

func TestProtectOrReleaseIPs_DefaultRetainsIP(t *testing.T) {
	fc := newFakeClient()
	fc.primaryIPs[10] = &PrimaryIP{ID: 10, IP: "203.0.113.10", Type: "ipv4", AssigneeID: 1, AutoDelete: true}

	retained, released, err := ProtectOrReleaseIPs(context.Background(), fc, 1, false)
	if err != nil {
		t.Fatalf("ProtectOrReleaseIPs: %v", err)
	}
	if len(released) != 0 {
		t.Errorf("released = %v, want none", released)
	}
	if len(retained) != 1 || retained[0].IP != "203.0.113.10" {
		t.Fatalf("retained = %v, want the one IP", retained)
	}
	if fc.primaryIPs[10].AutoDelete {
		t.Error("auto_delete on the IP must be false after protecting it — this is the exact footgun G-011 closes")
	}
}

func TestProtectOrReleaseIPs_ReleaseFlag_LeavesAutoDeleteTrue(t *testing.T) {
	fc := newFakeClient()
	fc.primaryIPs[10] = &PrimaryIP{ID: 10, IP: "203.0.113.10", Type: "ipv4", AssigneeID: 1, AutoDelete: false}

	retained, released, err := ProtectOrReleaseIPs(context.Background(), fc, 1, true)
	if err != nil {
		t.Fatalf("ProtectOrReleaseIPs: %v", err)
	}
	if len(retained) != 0 {
		t.Errorf("retained = %v, want none", retained)
	}
	if len(released) != 1 {
		t.Fatalf("released = %v, want the one IP", released)
	}
	if !fc.primaryIPs[10].AutoDelete {
		t.Error("--release-ip must set auto_delete=true so the IP is actually released")
	}
}

func TestProtectOrReleaseIPs_NoIPs_NoOp(t *testing.T) {
	fc := newFakeClient()
	retained, released, err := ProtectOrReleaseIPs(context.Background(), fc, 1, false)
	if err != nil {
		t.Fatalf("ProtectOrReleaseIPs: %v", err)
	}
	if len(retained) != 0 || len(released) != 0 {
		t.Errorf("server with no primary IPs must report none retained/released, got retained=%v released=%v", retained, released)
	}
}
