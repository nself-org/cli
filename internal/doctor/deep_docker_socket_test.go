package doctor

// deep_docker_socket_test.go — P7-DEPL-21: proxy-docker-socket verdicts from a
// fake inspector. Containers are synthetic; the exception table is the build
// guard's own.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/version"
)

type fakeSocketInspector struct {
	containers []docker.ContainerInfo
	err        error
}

func (f fakeSocketInspector) RunningContainers(context.Context) ([]docker.ContainerInfo, error) {
	return f.containers, f.err
}

func sockCtr(name, service string, mounts []docker.Mount, env map[string]string) docker.ContainerInfo {
	return docker.ContainerInfo{Name: name, Service: service, State: "running", Mounts: mounts, Env: env}
}

var (
	sockMount = []docker.Mount{{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock", Type: "bind"}}
	plainMnt  = []docker.Mount{{Source: "/srv/data", Destination: "/data", Type: "bind"}}
	owners    = map[string]string{"forgejo": "forgejo", "monitoring": "monitoring", "sockshort": "sockshort", "sockhost": "sockhost"}
)

func TestDockerSocketDoctor(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "1.4.14"

	cases := []struct {
		name       string
		ins        fakeSocketInspector
		version    string
		wantStatus string
		wantIn     []string
	}{
		{"no containers", fakeSocketInspector{}, "", "pass", nil},
		{"clean proxy and plugin", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_nginx", "nginx", plainMnt, nil), sockCtr("p_sockshort", "sockshort", nil, map[string]string{"DOCKER_HOST": "tcp://x:2375"})}}, "", "pass", nil},
		{"proxy with the socket fails", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_nginx", "nginx", sockMount, nil)}}, "", "fail", []string{"proxy nginx", "p_nginx"}},
		{"traefik with unix DOCKER_HOST fails", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_traefik", "traefik", nil, map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"})}}, "", "fail", []string{"traefik", "DOCKER_HOST"}},
		{"parent dir mount fails", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_nginx", "nginx", []docker.Mount{{Source: "/var/run", Destination: "/r"}}, nil)}}, "", "fail", []string{"nginx"}},
		{"non-excepted plugin fails naming plugin and service", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_sockshort", "sockshort", sockMount, nil)}}, "", "fail", []string{"plugin sockshort", "service sockshort"}},
		{"plugin with unix DOCKER_HOST fails", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_sockhost", "sockhost", nil, map[string]string{"DOCKER_HOST": "unix:///run/docker.sock"})}}, "", "fail", []string{"plugin sockhost"}},
		{"excepted plugin warns", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_forgejo", "forgejo", sockMount, nil)}}, "", "warn", []string{"plugin forgejo", "P7-TRUST-21"}},
		{"expired exception fails", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_monitoring", "monitoring", sockMount, nil)}}, "1.5.0", "fail", []string{"plugin monitoring"}},
		{"one failure outranks an exception", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_forgejo", "forgejo", sockMount, nil), sockCtr("p_nginx", "nginx", sockMount, nil)}}, "", "fail", []string{"proxy nginx", "plugin forgejo"}},
		{"core admin and unrelated services are not checked", fakeSocketInspector{containers: []docker.ContainerInfo{
			sockCtr("p_admin", "nself-admin", sockMount, map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"}),
			sockCtr("p_ping", "ping-api", sockMount, nil)}}, "", "pass", nil},
		{"service falls back to the compose label", fakeSocketInspector{containers: []docker.ContainerInfo{
			{Name: "p_nginx", State: "running", Mounts: sockMount, Labels: map[string]string{"com.docker.compose.service": "nginx"}}}}, "", "fail", []string{"proxy nginx"}},
		{"inspector error is unknown, never pass", fakeSocketInspector{err: errors.New("daemon down")}, "", "unknown", []string{"daemon down"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version.Version = "1.4.14"
			if tc.version != "" {
				version.Version = tc.version
			}
			got := CheckProxyDockerSocket(context.Background(), tc.ins, owners)
			if got.Status != tc.wantStatus || got.Name != "proxy-docker-socket" {
				t.Fatalf("status %q name %q, want %q: %s", got.Status, got.Name, tc.wantStatus, got.Message)
			}
			for _, s := range tc.wantIn {
				if !strings.Contains(got.Message, s) {
					t.Errorf("message %q lacks %q", got.Message, s)
				}
			}
		})
	}

	t.Run("no compose file skips, never passes", func(t *testing.T) {
		got := DockerSocketChecks(context.Background(), t.TempDir())
		if len(got) != 1 || got[0].Status != "skip" || got[0].Name != "proxy-docker-socket" {
			t.Fatalf("got %+v", got)
		}
	})
}
