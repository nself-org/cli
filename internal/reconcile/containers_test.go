package reconcile

// containers_test.go — compareRunning, including D-0505: a planned service
// with no container is "create" while the project has containers.

import (
	"reflect"
	"testing"
)

func TestCompareRunning(t *testing.T) {
	defined := map[string]struct{}{"postgres": {}, "hasura": {}, "traefik": {}, "traefik-config": {}, "redis": {}}
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
	}
	for _, c := range cases {
		items, orphans := compareRunning(c.running, c.hashes, defined, stateful)
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
