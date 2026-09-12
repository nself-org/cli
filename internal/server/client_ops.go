package server

// Purpose: hetznerClient's Client interface methods — the actual HTTP calls
// against api.hetzner.cloud/v1. Each method is a thin encode-request /
// decode-response / map-to-our-types wrapper; the safety/orchestration logic
// (snapshot-before-destroy, disk-shrink detection, IP protection) lives in
// provision.go/resize.go/destroy.go/primaryip.go, never here.
// Inputs: a *hetznerClient (holds the API token) and typed request values.
// Outputs: this package's Server/ServerType/Image/Action/PrimaryIP types.
// Constraints: every method must go through do() (client.go) so tests can
// redirect hetznerAPIBaseURL to an httptest.Server — never call
// hetznerHTTPClient directly from here.

import (
	"context"
	"fmt"
)

func (c *hetznerClient) CreateServer(ctx context.Context, req ProvisionRequest) (*Server, error) {
	body := map[string]interface{}{
		"name":        req.Name,
		"server_type": req.ServerType,
		"image":       req.Image,
		"location":    req.Location,
	}
	if len(req.SSHKeys) > 0 {
		body["ssh_keys"] = req.SSHKeys
	}
	if len(req.Labels) > 0 {
		body["labels"] = req.Labels
	}
	if req.UserData != "" {
		body["user_data"] = req.UserData
	}

	var out struct {
		Server wireServer `json:"server"`
	}
	if err := c.do(ctx, "POST", "/servers", body, &out); err != nil {
		return nil, fmt.Errorf("create server %q: %w", req.Name, err)
	}
	s := out.Server.toServer()
	return &s, nil
}

func (c *hetznerClient) ListServers(ctx context.Context, opts ListOptions) ([]Server, error) {
	path := "/servers"
	if opts.LabelSelector != "" {
		path += "?label_selector=" + opts.LabelSelector
	}
	var out struct {
		Servers []wireServer `json:"servers"`
	}
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	servers := make([]Server, 0, len(out.Servers))
	for _, w := range out.Servers {
		servers = append(servers, w.toServer())
	}
	return servers, nil
}

func (c *hetznerClient) GetServer(ctx context.Context, id int64) (*Server, error) {
	var out struct {
		Server wireServer `json:"server"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/servers/%d", id), nil, &out); err != nil {
		return nil, fmt.Errorf("get server %d: %w", id, err)
	}
	s := out.Server.toServer()
	return &s, nil
}

func (c *hetznerClient) DeleteServer(ctx context.Context, id int64) error {
	if err := c.do(ctx, "DELETE", fmt.Sprintf("/servers/%d", id), nil, nil); err != nil {
		return fmt.Errorf("delete server %d: %w", id, err)
	}
	return nil
}

func (c *hetznerClient) ListServerTypes(ctx context.Context) ([]ServerType, error) {
	var out struct {
		ServerTypes []wireServerType `json:"server_types"`
	}
	if err := c.do(ctx, "GET", "/server_types", nil, &out); err != nil {
		return nil, fmt.Errorf("list server types: %w", err)
	}
	types := make([]ServerType, 0, len(out.ServerTypes))
	for _, w := range out.ServerTypes {
		types = append(types, w.toServerType())
	}
	return types, nil
}

func (c *hetznerClient) ChangeServerType(ctx context.Context, serverID int64, targetType string, upgradeDisk bool) (*Action, error) {
	body := map[string]interface{}{
		"server_type":  targetType,
		"upgrade_disk": upgradeDisk,
	}
	var out struct {
		Action wireAction `json:"action"`
	}
	if err := c.do(ctx, "POST", fmt.Sprintf("/servers/%d/actions/change_type", serverID), body, &out); err != nil {
		return nil, fmt.Errorf("change server %d to type %q: %w", serverID, targetType, err)
	}
	a := out.Action.toAction()
	return &a, nil
}

func (c *hetznerClient) CreateSnapshot(ctx context.Context, serverID int64, description string) (*Image, *Action, error) {
	body := map[string]interface{}{"type": "snapshot"}
	if description != "" {
		body["description"] = description
	}
	var out struct {
		Image  wireImage  `json:"image"`
		Action wireAction `json:"action"`
	}
	if err := c.do(ctx, "POST", fmt.Sprintf("/servers/%d/actions/create_image", serverID), body, &out); err != nil {
		return nil, nil, fmt.Errorf("snapshot server %d: %w", serverID, err)
	}
	img := out.Image.toImage()
	act := out.Action.toAction()
	return &img, &act, nil
}

func (c *hetznerClient) GetImage(ctx context.Context, id int64) (*Image, error) {
	var out struct {
		Image wireImage `json:"image"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/images/%d", id), nil, &out); err != nil {
		return nil, fmt.Errorf("get image %d: %w", id, err)
	}
	img := out.Image.toImage()
	return &img, nil
}

func (c *hetznerClient) GetAction(ctx context.Context, id int64) (*Action, error) {
	var out struct {
		Action wireAction `json:"action"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/actions/%d", id), nil, &out); err != nil {
		return nil, fmt.Errorf("get action %d: %w", id, err)
	}
	a := out.Action.toAction()
	return &a, nil
}

func (c *hetznerClient) ListPrimaryIPs(ctx context.Context, serverID int64) ([]PrimaryIP, error) {
	srv, err := c.GetServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("resolve primary IPs for server %d: %w", serverID, err)
	}
	var ips []PrimaryIP
	for _, id := range []int64{srv.IPv4ID, srv.IPv6ID} {
		if id == 0 {
			continue
		}
		var out struct {
			PrimaryIP wirePrimaryIP `json:"primary_ip"`
		}
		if err := c.do(ctx, "GET", fmt.Sprintf("/primary_ips/%d", id), nil, &out); err != nil {
			return nil, fmt.Errorf("get primary IP %d: %w", id, err)
		}
		ips = append(ips, out.PrimaryIP.toPrimaryIP())
	}
	return ips, nil
}

func (c *hetznerClient) SetPrimaryIPAutoDelete(ctx context.Context, ipID int64, autoDelete bool) error {
	body := map[string]interface{}{"auto_delete": autoDelete}
	if err := c.do(ctx, "PUT", fmt.Sprintf("/primary_ips/%d", ipID), body, nil); err != nil {
		return fmt.Errorf("set auto_delete=%v on primary IP %d: %w", autoDelete, ipID, err)
	}
	return nil
}
