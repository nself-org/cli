package server

// Purpose: an in-memory fake Client implementing the same interface as
// hetznerClient, shared by every *_test.go in this package. No test in this
// file (or any file using fakeClient) makes a network call — this is the
// hard constraint the task requires ("must NOT call a real provider API or
// create/destroy real infrastructure").
// Inputs: constructed empty; tests populate .servers/.serverTypes/.images/
// .actions/.primaryIPs directly before calling the function under test.
// Outputs: satisfies the Client interface exactly.

import (
	"context"
	"fmt"
)

type fakeClient struct {
	servers     map[int64]*Server
	serverTypes []ServerType
	images      map[int64]*Image
	actions     map[int64]*Action
	primaryIPs  map[int64]*PrimaryIP // keyed by IP ID
	nextID      int64

	deletedServerIDs []int64
	createErr        error
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		servers:    map[int64]*Server{},
		images:     map[int64]*Image{},
		actions:    map[int64]*Action{},
		primaryIPs: map[int64]*PrimaryIP{},
		nextID:     1,
	}
}

func (f *fakeClient) newID() int64 {
	id := f.nextID
	f.nextID++
	return id
}

func (f *fakeClient) CreateServer(_ context.Context, req ProvisionRequest) (*Server, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	id := f.newID()
	s := &Server{
		ID: id, Name: req.Name, Status: "running", ServerType: req.ServerType,
		Location: req.Location, Labels: req.Labels,
		IPv4: "203.0.113.10", IPv4ID: f.newID(),
	}
	f.servers[id] = s
	return s, nil
}

func (f *fakeClient) ListServers(_ context.Context, _ ListOptions) ([]Server, error) {
	out := make([]Server, 0, len(f.servers))
	for _, s := range f.servers {
		out = append(out, *s)
	}
	return out, nil
}

func (f *fakeClient) GetServer(_ context.Context, id int64) (*Server, error) {
	s, ok := f.servers[id]
	if !ok {
		return nil, fmt.Errorf("server %d not found", id)
	}
	cp := *s
	return &cp, nil
}

func (f *fakeClient) DeleteServer(_ context.Context, id int64) error {
	if _, ok := f.servers[id]; !ok {
		return fmt.Errorf("server %d not found", id)
	}
	delete(f.servers, id)
	f.deletedServerIDs = append(f.deletedServerIDs, id)
	return nil
}

func (f *fakeClient) ListServerTypes(_ context.Context) ([]ServerType, error) {
	return f.serverTypes, nil
}

func (f *fakeClient) ChangeServerType(_ context.Context, serverID int64, targetType string, _ bool) (*Action, error) {
	s, ok := f.servers[serverID]
	if !ok {
		return nil, fmt.Errorf("server %d not found", serverID)
	}
	s.ServerType = targetType
	return &Action{ID: f.newID(), Status: "success", Command: "change_server_type"}, nil
}

func (f *fakeClient) CreateSnapshot(_ context.Context, serverID int64, description string) (*Image, *Action, error) {
	if _, ok := f.servers[serverID]; !ok {
		return nil, nil, fmt.Errorf("server %d not found", serverID)
	}
	imgID := f.newID()
	img := &Image{ID: imgID, Type: "snapshot", Status: "creating", Description: description}
	f.images[imgID] = img
	act := &Action{ID: f.newID(), Status: "running", Command: "create_image"}
	f.actions[act.ID] = act
	return img, act, nil
}

func (f *fakeClient) GetImage(_ context.Context, id int64) (*Image, error) {
	img, ok := f.images[id]
	if !ok {
		return nil, fmt.Errorf("image %d not found", id)
	}
	cp := *img
	return &cp, nil
}

func (f *fakeClient) GetAction(_ context.Context, id int64) (*Action, error) {
	act, ok := f.actions[id]
	if !ok {
		return nil, fmt.Errorf("action %d not found", id)
	}
	cp := *act
	return &cp, nil
}

func (f *fakeClient) ListPrimaryIPs(_ context.Context, serverID int64) ([]PrimaryIP, error) {
	var out []PrimaryIP
	for _, ip := range f.primaryIPs {
		if ip.AssigneeID == serverID {
			out = append(out, *ip)
		}
	}
	return out, nil
}

func (f *fakeClient) SetPrimaryIPAutoDelete(_ context.Context, ipID int64, autoDelete bool) error {
	ip, ok := f.primaryIPs[ipID]
	if !ok {
		return fmt.Errorf("primary IP %d not found", ipID)
	}
	ip.AutoDelete = autoDelete
	return nil
}
