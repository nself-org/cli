package reconcile

// containers_test.go — compareRunning, including D-0505: a planned service
// with no container is "create" while the project has containers.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompareRunning(t *testing.T) {
	defined := map[string]struct{}{"postgres": {}, "hasura": {}, "traefik": {}, "traefik-config": {}, "redis": {}, "meilisearch-init": {}}
	stateful := map[string]bool{"postgres": true, "traefik": false}
	cases := []struct {
		name    string
		running []RunningContainer
		hashes  map[string]string
		want    []ContainerItem
		orphans []string
	}{
		{
			name:    "added services are create",
			running: []RunningContainer{{Name: "p_postgres", Service: "postgres", ConfigHash: "a"}, {Name: "p_hasura", Service: "hasura", ConfigHash: "b"}},
			hashes:  map[string]string{"postgres": "a", "hasura": "b", "traefik": "c", "traefik-config": "d"},
			want: []ContainerItem{
				{Service: "traefik", Action: ContainerCreate, AppliedBy: AppliedNextStart},
				{Service: "traefik-config", Action: ContainerCreate, AppliedBy: AppliedNextStart},
			},
		},
		{
			name:    "create keeps stateful",
			running: []RunningContainer{{Name: "p_hasura", Service: "hasura", ConfigHash: "b"}},
			hashes:  map[string]string{"postgres": "a", "hasura": "x"},
			want: []ContainerItem{
				{Service: "hasura", Action: ContainerRecreate, AppliedBy: AppliedNextStart},
				{Service: "postgres", Action: ContainerCreate, Stateful: true, AppliedBy: AppliedNextStart},
			},
		},
		{
			name:   "no containers at all: nothing is create",
			hashes: map[string]string{"postgres": "a", "hasura": "b"},
			want:   []ContainerItem{},
		},
		{
			name:    "a stopped container is present, not create",
			running: []RunningContainer{{Name: "p_postgres", Service: "postgres", State: "exited", ConfigHash: "a"}, {Name: "p_hasura", Service: "hasura", ConfigHash: "b"}},
			hashes:  map[string]string{"postgres": "a", "hasura": "b"},
			want:    []ContainerItem{},
		},
		{
			name:    "profile-off service (defined, not in hashes) is not create",
			running: []RunningContainer{{Name: "p_postgres", Service: "postgres", ConfigHash: "a"}},
			hashes:  map[string]string{"postgres": "a"},
			want:    []ContainerItem{},
		},
		{
			name:    "orphans stay orphans; unknown label stays unknown",
			running: []RunningContainer{{Name: "p_ghost", Service: "ghost", ConfigHash: "z"}, {Name: "p_postgres", Service: "postgres"}},
			hashes:  map[string]string{"postgres": "a", "redis": "r"},
			want: []ContainerItem{
				{Service: "postgres", Action: ContainerUnknown, Stateful: true, AppliedBy: AppliedNextStart},
				{Service: "redis", Action: ContainerCreate, AppliedBy: AppliedNextStart},
			},
			orphans: []string{"p_ghost"},
		},
		{
			// nself start removes init containers after every run.
			name:    "removed init container is not create",
			running: []RunningContainer{{Name: "p_postgres", Service: "postgres", ConfigHash: "a"}},
			hashes:  map[string]string{"postgres": "a", "meilisearch-init": "m"},
			want:    []ContainerItem{},
		},
		{
			// Only containers of services the plan no longer defines (a
			// rename): the next start creates every planned service.
			name:    "only orphans: planned services are create",
			running: []RunningContainer{{Name: "p_old", Service: "old", ConfigHash: "z"}},
			hashes:  map[string]string{"postgres": "a"},
			want: []ContainerItem{
				{Service: "postgres", Action: ContainerCreate, Stateful: true, AppliedBy: AppliedNextStart},
			},
			orphans: []string{"p_old"},
		},
	}
	for _, c := range cases {
		items, orphans := compareRunning(c.running, c.hashes, defined, stateful, map[string]bool{"meilisearch-init": true})
		if !reflect.DeepEqual(items, c.want) {
			t.Errorf("%s: items\n got %+v\nwant %+v", c.name, items, c.want)
		}
		var names []string
		for _, o := range orphans {
			names = append(names, o.Name)
		}
		if !reflect.DeepEqual(names, c.orphans) {
			t.Errorf("%s: orphans %q, want %q", c.name, names, c.orphans)
		}
	}
}

func TestInitContainerServices(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("docker-compose.yml", `services:
  postgres:
    image: postgres
    labels:
      nself.type: database
  meilisearch-init:
    image: busybox
    labels:
      nself.type: init-container
      nself.auto-remove: "true"
`)
	frag := write("plugin.yml", `services:
  plugin-init:
    image: busybox
    labels:
      - "nself.type=init-container"
  plugin:
    image: x
    labels: ["nself.type=plugin"]
`)
	got, err := initContainerServices([]string{base, frag, filepath.Join(dir, "missing.yml")})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{"meilisearch-init": true, "plugin-init": true}; !reflect.DeepEqual(got, want) {
		t.Errorf("initContainerServices = %v, want %v", got, want)
	}
	if _, err := initContainerServices([]string{write("bad.yml", "services: [")}); err == nil {
		t.Error("a broken compose file must be an error (container impact unknown), not an empty set")
	}
}
