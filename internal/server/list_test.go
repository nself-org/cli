package server

import (
	"context"
	"testing"
)

func TestList_ReturnsAllServers(t *testing.T) {
	fc := newFakeClient()
	fc.servers[1] = &Server{ID: 1, Name: "web-1"}
	fc.servers[2] = &Server{ID: 2, Name: "web-2"}

	servers, err := List(context.Background(), fc, ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(servers))
	}
}

func TestList_Empty(t *testing.T) {
	fc := newFakeClient()
	servers, err := List(context.Background(), fc, ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("got %d servers, want 0", len(servers))
	}
}
