package server

import (
	"context"
	"errors"
	"testing"
)

func TestResize_DiskShrink_RefusedWithExplanation(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1", ServerType: "cx41"}
	fc.serverTypes = []ServerType{
		{Name: "cx41", Cores: 4, Disk: 160},
		{Name: "cx22", Cores: 2, Disk: 40},
	}

	_, err := Resize(context.Background(), fc, ResizeRequest{ServerID: 1, TargetType: "cx22"})
	if err == nil {
		t.Fatal("Resize: want error for disk shrink, got nil")
	}
	if !errors.Is(err, ErrDiskShrink) {
		t.Errorf("Resize error = %v, want it to wrap ErrDiskShrink", err)
	}
	if fc.servers[1].ServerType != "cx41" {
		t.Errorf("server type changed to %q despite refused resize", fc.servers[1].ServerType)
	}
}

func TestResize_Grow_Succeeds(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1", ServerType: "cx22"}
	fc.serverTypes = []ServerType{
		{Name: "cx22", Cores: 2, Disk: 40},
		{Name: "cx41", Cores: 4, Disk: 160},
	}

	act, err := Resize(context.Background(), fc, ResizeRequest{ServerID: 1, TargetType: "cx41"})
	if err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if act.Status != "success" {
		t.Errorf("action status = %q, want success", act.Status)
	}
	if fc.servers[1].ServerType != "cx41" {
		t.Errorf("server type = %q, want cx41", fc.servers[1].ServerType)
	}
}

func TestResize_UnknownTargetType(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1", ServerType: "cx22"}
	fc.serverTypes = []ServerType{{Name: "cx22", Disk: 40}}

	if _, err := Resize(context.Background(), fc, ResizeRequest{ServerID: 1, TargetType: "cx999"}); err == nil {
		t.Fatal("Resize: want error for unknown target type, got nil")
	}
}

func TestResize_MissingFields(t *testing.T) {
	fc := newFakeClient()
	if _, err := Resize(context.Background(), fc, ResizeRequest{TargetType: "cx41"}); err == nil {
		t.Error("Resize: want error when ServerID is 0")
	}
	if _, err := Resize(context.Background(), fc, ResizeRequest{ServerID: 1}); err == nil {
		t.Error("Resize: want error when TargetType is empty")
	}
}
