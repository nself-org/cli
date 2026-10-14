package doctor

// deep_docker_socket_test.go — P7-DEPL-21: proxy-docker-socket verdicts from a
// fake inspector and a synthetic build.SocketReport. The exception table is
// the build guard's own.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
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

// sockCtr is a live container; state defaults to running.
func sockCtr(name, service string, mounts []docker.Mount, env map[string]string, labels map[string]string) docker.ContainerInfo {
	return docker.ContainerInfo{Name: name, Service: service, State: "running", Mounts: mounts, Env: env, Labels: labels}
}

var (
	bindSock = []docker.Mount{{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock", Type: "bind"}}
	unixHost = map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"}
	volumeOf = func(name string) []docker.Mount {
		return []docker.Mount{{Source: "/var/lib/docker/volumes/" + name + "/_data", Destination: "/v", Type: "volume"}}
	}
	anonymous = strings.Repeat("ab12", 16)
)

func socketReport() *build.SocketReport {
	return &build.SocketReport{
		Owners: map[string][]string{
			"forgejo": {"forgejo"}, "monitoring": {"monitoring"}, "sockshort": {"sockshort"}, "evil": {"evil"},
			"nginx": {"forgejo"}, "dupsvc": {"forgejo", "monitoring"}, "sockdata": {"sockdata"},
		},
		Core:    map[string]bool{"nself-admin": true, "ping-api": true, "postgres": true, "nginx": true},
		Volumes: map[string]string{"sockvol": "is backed by /var/run", "plainvol": ""},
	}
}

func TestDockerSocketDoctor(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	t.Setenv("DOCKER_HOST", "")
	dir := t.TempDir()
	bind := func(src string) []docker.Mount {
		return []docker.Mount{{Source: src, Destination: "/data", Type: "bind"}}
	}
	link := func(name, target string) string {
		p := filepath.Join(dir, name)
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bindData := bind(dir)
	toRun, toSafe := link("run", "/var/run"), link("safe", dir)
	dangling, loopA := link("dangling", filepath.Join(dir, "gone")), link("loopa", filepath.Join(dir, "loopb"))
	link("loopb", loopA)

	cases := []struct {
		name       string
		ctrs       []docker.ContainerInfo
		err        error
		version    string
		wantStatus string
		wantIn     []string
	}{
		{"no containers", nil, nil, "", "pass", nil},
		{"clean proxy and plugin", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bindData, nil, nil), sockCtr("p_s", "sockshort", nil, map[string]string{"DOCKER_HOST": "tcp://x:2375"}, nil)}, nil, "", "pass", nil},
		{"proxy with the socket fails", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bindSock, nil, nil)}, nil, "", "fail", []string{"proxy service nginx", "p_nginx"}},
		{"traefik with unix DOCKER_HOST fails", []docker.ContainerInfo{sockCtr("p_traefik", "traefik", nil, unixHost, nil)}, nil, "", "fail", []string{"traefik", "DOCKER_HOST"}},
		{"parent dir bind fails", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", []docker.Mount{{Source: "/var/run", Destination: "/r", Type: "bind"}}, nil, nil)}, nil, "", "fail", []string{"nginx"}},
		{"/private alias bind fails", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", []docker.Mount{{Source: "/private/var/run", Destination: "/r", Type: "bind"}}, nil, nil)}, nil, "", "fail", []string{"nginx"}},
		{"socket at the container path fails", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", []docker.Mount{{Source: "/x", Destination: "/run/docker.sock", Type: "bind"}}, nil, nil)}, nil, "", "fail", []string{"nginx"}},
		{"bind through a symlink to /var/run fails", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind(toRun), nil, nil)}, nil, "", "fail", []string{"nginx", toRun}},
		{"bind below a symlink to /var/run fails", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind(toRun+"/docker.sock"), nil, nil)}, nil, "", "fail", []string{"nginx"}},
		{"bind through a symlink to a safe dir passes", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind(toSafe), nil, nil)}, nil, "", "pass", nil},
		{"dangling symlink bind is unknown", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind(dangling), nil, nil)}, nil, "", "unknown", []string{dangling}},
		{"looping symlink bind is unknown", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind(loopA), nil, nil)}, nil, "", "unknown", []string{loopA}},
		{"relative bind source is unknown", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind("data/x"), nil, nil)}, nil, "", "unknown", []string{"data/x"}},
		{"bind source this host cannot see is unknown", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind("/nonexistent-sockguard/x"), nil, nil)}, nil, "", "unknown", []string{"/nonexistent-sockguard/x"}},
		{"plugin without an exception fails naming plugin and service", []docker.ContainerInfo{sockCtr("p_s", "sockshort", bindSock, nil, nil)}, nil, "", "fail", []string{"plugin sockshort", "service sockshort"}},
		{"plugin with unix DOCKER_HOST fails", []docker.ContainerInfo{sockCtr("p_e", "evil", nil, unixHost, nil)}, nil, "", "fail", []string{"plugin evil"}},
		{"excepted plugin warns", []docker.ContainerInfo{sockCtr("p_f", "forgejo", bindSock, nil, nil)}, nil, "", "warn", []string{"plugin forgejo", "P7-TRUST-21", "1.5.0"}},
		{"expired exception fails", []docker.ContainerInfo{sockCtr("p_m", "monitoring", bindSock, nil, nil)}, nil, "1.5.0", "fail", []string{"plugin monitoring"}},
		{"unknown version fails closed", []docker.ContainerInfo{sockCtr("p_m", "monitoring", bindSock, nil, nil)}, nil, "dev", "fail", []string{"plugin monitoring"}},
		{"a fragment declaring nginx grants the proxy no exception", []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bindSock, nil, nil)}, nil, "", "fail", []string{"proxy service nginx"}},
		{"ambiguous ownership grants no exception", []docker.ContainerInfo{sockCtr("p_d", "dupsvc", bindSock, nil, nil)}, nil, "", "fail", []string{"forgejo, monitoring"}},
		{"a label cannot claim another plugin's exception", []docker.ContainerInfo{sockCtr("p_e", "evil", bindSock, nil, map[string]string{"nself.plugin": "forgejo"})}, nil, "", "fail", []string{"plugin forgejo"}},
		{"deleted fragment: a service the core stack lacks is still a plugin", []docker.ContainerInfo{sockCtr("p_g", "ghost", bindSock, nil, nil)}, nil, "", "fail", []string{"no fragment names it", "ghost"}},
		{"label alone cannot earn the exception", []docker.ContainerInfo{sockCtr("p_l", "labeled", bindSock, nil, map[string]string{"nself.plugin": "forgejo"})}, nil, "", "fail", []string{"plugin forgejo"}},
		{"restarting container is inspected", []docker.ContainerInfo{{Name: "p_n", Service: "nginx", State: "restarting", Mounts: bindSock}}, nil, "", "fail", []string{"nginx"}},
		{"paused container is inspected", []docker.ContainerInfo{{Name: "p_s", Service: "sockshort", State: "paused", Mounts: bindSock}}, nil, "", "fail", []string{"sockshort"}},
		{"core admin and custom services are not judged", []docker.ContainerInfo{sockCtr("p_a", "nself-admin", bindSock, unixHost, nil), sockCtr("p_p", "ping-api", bindSock, nil, nil)}, nil, "", "pass", nil},
		{"service falls back to the compose label", []docker.ContainerInfo{{Name: "p_n", State: "running", Mounts: bindSock, Labels: map[string]string{"com.docker.compose.service": "nginx"}}}, nil, "", "fail", []string{"proxy service nginx"}},
		{"volume backed by the socket fails", []docker.ContainerInfo{sockCtr("p_s", "sockdata", volumeOf("proj_sockvol"), nil, nil)}, nil, "", "fail", []string{"proj_sockvol", "/var/run"}},
		{"ordinary named volume is unknown, a Compose declaration proves nothing", []docker.ContainerInfo{sockCtr("p_s", "sockdata", volumeOf("proj_plainvol"), nil, nil)}, nil, "", "unknown", []string{"proj_plainvol", "cannot be verified"}},
		{"stale backing: declared plain, live volume unprovable", []docker.ContainerInfo{sockCtr("p_s", "sockdata", volumeOf("plainvol"), nil, nil)}, nil, "", "unknown", []string{"plainvol"}},
		{"anonymous volume is unknown", []docker.ContainerInfo{sockCtr("p_s", "sockdata", volumeOf(anonymous), nil, nil)}, nil, "", "unknown", []string{anonymous}},
		{"undeclared volume is unknown", []docker.ContainerInfo{sockCtr("p_s", "sockdata", volumeOf("proj_mystery"), nil, nil)}, nil, "", "unknown", []string{"proj_mystery"}},
		{"volume whose path cannot be identified is unknown", []docker.ContainerInfo{sockCtr("p_s", "sockdata", []docker.Mount{{Source: "/mnt/odd", Destination: "/v", Type: "volume"}}, nil, nil)}, nil, "", "unknown", []string{"/mnt/odd"}},
		{"mount of unknown type is unknown", []docker.ContainerInfo{sockCtr("p_s", "sockdata", []docker.Mount{{Source: "/x", Destination: "/v"}}, nil, nil)}, nil, "", "unknown", []string{"unknown type"}},
		{"container without a state is unknown, not pass", []docker.ContainerInfo{{Name: "p_n", Service: "nginx"}}, nil, "", "unknown", []string{"p_n"}},
		{"container without a service is unknown, not pass", []docker.ContainerInfo{{Name: "p_x", State: "running"}}, nil, "", "unknown", []string{"p_x"}},
		{"a failure outranks an unknown", []docker.ContainerInfo{sockCtr("p_n", "nginx", bindSock, nil, nil), {Name: "p_x", State: "running"}}, nil, "", "fail", []string{"proxy service nginx"}},
		{"inspector error is unknown, never pass", nil, errors.New("daemon down"), "", "unknown", []string{"daemon down"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version.Version = "1.4.14"
			if tc.version != "" {
				version.Version = tc.version
			}
			got := CheckProxyDockerSocket(context.Background(), fakeSocketInspector{tc.ctrs, tc.err}, socketReport())
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

	t.Run("a remote DOCKER_HOST makes every bind source unverifiable", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "tcp://remote.invalid:2375")
		got := CheckProxyDockerSocket(context.Background(), fakeSocketInspector{containers: []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind(dir), nil, nil)}}, socketReport())
		if got.Status != "unknown" {
			t.Fatalf("got %+v", got)
		}
		t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(dir, "d.sock"))
		got = CheckProxyDockerSocket(context.Background(), fakeSocketInspector{containers: []docker.ContainerInfo{sockCtr("p_nginx", "nginx", bind(dir), nil, nil)}}, socketReport())
		if got.Status != "pass" {
			t.Fatalf("a local unix DOCKER_HOST must not block verification: %+v", got)
		}
	})

	t.Run("no compose file skips, never passes", func(t *testing.T) {
		got := DockerSocketChecks(context.Background(), t.TempDir())
		if len(got) != 1 || got[0].Status != "skip" || got[0].Name != "proxy-docker-socket" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("unreadable compose inputs are unknown, never pass", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: [unclosed"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := DockerSocketChecks(context.Background(), dir)
		if len(got) != 1 || got[0].Status != "unknown" {
			t.Fatalf("got %+v", got)
		}
	})
}
