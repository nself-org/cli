package server

import (
	"context"
	"testing"
)

func TestProvision_HappyPath_StampsManagedByLabel(t *testing.T) {
	fc := newFakeClient()
	srv, err := Provision(context.Background(), fc, ProvisionRequest{
		Name: "ci-box", ServerType: "cx22", Location: "fsn1", Image: "ubuntu-24.04",
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if srv.Name != "ci-box" {
		t.Errorf("Name = %q, want ci-box", srv.Name)
	}
	if got := srv.Labels[ManagedByLabel]; got != ManagedByValue {
		t.Errorf("label %s = %q, want %q", ManagedByLabel, got, ManagedByValue)
	}
	if len(fc.servers) != 1 {
		t.Fatalf("expected exactly 1 server created, got %d", len(fc.servers))
	}
}

func TestProvision_PreservesExplicitManagedByLabel(t *testing.T) {
	fc := newFakeClient()
	srv, err := Provision(context.Background(), fc, ProvisionRequest{
		Name: "ci-box", ServerType: "cx22", Location: "fsn1", Image: "ubuntu-24.04",
		Labels: map[string]string{ManagedByLabel: "terraform"},
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if got := srv.Labels[ManagedByLabel]; got != "terraform" {
		t.Errorf("label = %q, want caller's explicit value preserved", got)
	}
}

func TestProvision_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		req  ProvisionRequest
	}{
		{"missing name", ProvisionRequest{ServerType: "cx22", Location: "fsn1", Image: "ubuntu-24.04"}},
		{"missing type", ProvisionRequest{Name: "x", Location: "fsn1", Image: "ubuntu-24.04"}},
		{"missing location", ProvisionRequest{Name: "x", ServerType: "cx22", Image: "ubuntu-24.04"}},
		{"missing image", ProvisionRequest{Name: "x", ServerType: "cx22", Location: "fsn1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fc := newFakeClient()
			if _, err := Provision(context.Background(), fc, c.req); err == nil {
				t.Fatalf("Provision(%+v): want error, got nil", c.req)
			}
			if len(fc.servers) != 0 {
				t.Fatalf("Provision must not create a server when validation fails")
			}
		})
	}
}
