package server

// Purpose: Hetzner Cloud API wire-format structs (exact JSON shapes the API
// sends/receives) and the mapping functions that convert them to this
// package's own Server/ServerType/PrimaryIP/Image types. Kept separate from
// types.go (our types) and client_ops.go (the calls) so a wire-format change
// touches exactly one file.
// Inputs: raw Hetzner API JSON, decoded by client.go's do().
// Outputs: this package's own types, decoupled from Hetzner's field naming.
// Constraints: never export these wire structs — callers outside this
// package must only ever see types.go's stable shapes.

import "time"

type wireServer struct {
	ID         int64          `json:"id"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Created    time.Time      `json:"created"`
	ServerType wireServerType `json:"server_type"`
	Datacenter struct {
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"datacenter"`
	PublicNet struct {
		IPv4 struct {
			ID int64  `json:"id"`
			IP string `json:"ip"`
		} `json:"ipv4"`
		IPv6 struct {
			ID int64  `json:"id"`
			IP string `json:"ip"`
		} `json:"ipv6"`
	} `json:"public_net"`
	Labels map[string]string `json:"labels"`
}

func (w wireServer) toServer() Server {
	return Server{
		ID:         w.ID,
		Name:       w.Name,
		Status:     w.Status,
		ServerType: w.ServerType.Name,
		Location:   w.Datacenter.Location.Name,
		Created:    w.Created,
		IPv4:       w.PublicNet.IPv4.IP,
		IPv4ID:     w.PublicNet.IPv4.ID,
		IPv6:       w.PublicNet.IPv6.IP,
		IPv6ID:     w.PublicNet.IPv6.ID,
		Labels:     w.Labels,
	}
}

type wireServerType struct {
	Name   string  `json:"name"`
	Cores  int     `json:"cores"`
	Memory float64 `json:"memory"`
	Disk   int     `json:"disk"`
}

func (w wireServerType) toServerType() ServerType {
	return ServerType(w)
}

type wirePrimaryIP struct {
	ID         int64  `json:"id"`
	IP         string `json:"ip"`
	Type       string `json:"type"`
	AssigneeID int64  `json:"assignee_id"`
	AutoDelete bool   `json:"auto_delete"`
}

func (w wirePrimaryIP) toPrimaryIP() PrimaryIP {
	return PrimaryIP(w)
}

type wireImage struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

func (w wireImage) toImage() Image {
	return Image(w)
}

type wireAction struct {
	ID       int64  `json:"id"`
	Status   string `json:"status"`
	Command  string `json:"command"`
	Progress int    `json:"progress"`
	Error    *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (w wireAction) toAction() Action {
	a := Action{ID: w.ID, Status: w.Status, Command: w.Command, Progress: w.Progress}
	if w.Error != nil {
		a.Error = &ActionError{Code: w.Error.Code, Message: w.Error.Message}
	}
	return a
}
