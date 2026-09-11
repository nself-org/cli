package server

// Purpose: proves hetznerClient's request/response wire format against a
// local httptest.Server standing in for the Hetzner Cloud API — mirroring
// internal/access/hetzner_mismatch_test.go's withHetznerServer pattern. No
// test here or anywhere else in this package ever reaches api.hetzner.cloud;
// hetznerAPIBaseURL is redirected for the duration of each test.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := hetznerAPIBaseURL
	hetznerAPIBaseURL = srv.URL
	t.Cleanup(func() { hetznerAPIBaseURL = old })
}

func TestHetznerClient_CreateServer_SendsExpectedRequest(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody map[string]interface{}

	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server":{"id":42,"name":"ci-box","status":"running",
			"server_type":{"name":"cx22"},"datacenter":{"location":{"name":"fsn1"}},
			"public_net":{"ipv4":{"id":100,"ip":"203.0.113.10"},"ipv6":{"id":0,"ip":""}},
			"labels":{"managed-by":"nself-cli"}}}`))
	})

	c := NewHetznerClient("test-token")
	srv, err := c.CreateServer(context.Background(), ProvisionRequest{
		Name: "ci-box", ServerType: "cx22", Location: "fsn1", Image: "ubuntu-24.04",
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotMethod != http.MethodPost || gotPath != "/servers" {
		t.Errorf("method/path = %s %s, want POST /servers", gotMethod, gotPath)
	}
	if gotBody["name"] != "ci-box" || gotBody["server_type"] != "cx22" {
		t.Errorf("request body = %v", gotBody)
	}
	if srv.ID != 42 || srv.IPv4 != "203.0.113.10" || srv.Location != "fsn1" {
		t.Errorf("parsed server = %+v", srv)
	}
}

func TestHetznerClient_ErrorEnvelope_IsWrapped(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_input","message":"server_type is invalid"}}`))
	})

	c := NewHetznerClient("test-token")
	_, err := c.CreateServer(context.Background(), ProvisionRequest{
		Name: "x", ServerType: "bogus", Location: "fsn1", Image: "ubuntu-24.04",
	})
	if err == nil {
		t.Fatal("CreateServer: want error on non-2xx response")
	}
	if got := err.Error(); !strings.Contains(got, "invalid_input") || !strings.Contains(got, "server_type is invalid") {
		t.Errorf("error = %q, want it to surface the Hetzner error code and message", got)
	}
}

func TestHetznerClient_DeleteServer(t *testing.T) {
	var gotMethod, gotPath string
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"action":{"id":1,"status":"success"}}`))
	})

	c := NewHetznerClient("test-token")
	if err := c.DeleteServer(context.Background(), 42); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/servers/42" {
		t.Errorf("method/path = %s %s, want DELETE /servers/42", gotMethod, gotPath)
	}
}

func TestHetznerClient_SetPrimaryIPAutoDelete(t *testing.T) {
	var gotBody map[string]interface{}
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	})

	c := NewHetznerClient("test-token")
	if err := c.SetPrimaryIPAutoDelete(context.Background(), 10, false); err != nil {
		t.Fatalf("SetPrimaryIPAutoDelete: %v", err)
	}
	if gotBody["auto_delete"] != false {
		t.Errorf("request body auto_delete = %v, want false", gotBody["auto_delete"])
	}
}
