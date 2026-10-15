package build

// Purpose: refuse proxy swaps that would change nginx's published endpoints.
// Inputs: installed or declared Traefik and resolved nginx host bindings.
// Outputs: unchanged project files and no auto-install on a refused build.
// Constraints: exercise force, check and plan before any generation writes.
import "testing"

func TestProxyRefusesChangedHostBindings(t *testing.T) {
	for _, declared := range []bool{false, true} {
		for _, tc := range []struct{ key, value, want string }{
			{"NGINX_HTTP_PORT", "8080", "NGINX_HTTP_PORT=8080"},
			{"NGINX_SSL_PORT", "8443", "NGINX_HTTPS_PORT/NGINX_SSL_PORT=8443"},
			{"NGINX_HTTPS_PORT", "9443", "NGINX_HTTPS_PORT/NGINX_SSL_PORT=9443"},
			{"NGINX_BIND_IP", "10.0.0.5", "NGINX_BIND_IP=10.0.0.5"},
			{"NGINX_BIND_IP", "127.0.0.1", "NGINX_BIND_IP=127.0.0.1"},
			{"NGINX_BIND_IP", "", "NGINX_BIND_IP=127.0.0.1"},
		} {
			name := "installed/"
			if declared {
				name = "declared/"
			}
			t.Run(name+tc.key+"="+tc.value, func(t *testing.T) {
				f := freshProxyFixture(t)
				t.Setenv("NGINX_BIND_IP", "0.0.0.0")
				t.Setenv(tc.key, tc.value)
				calls := new(int)
				if declared {
					calls = declareProxy(t, f)
				} else {
					installProxy(t, f.plugins)
				}
				proxyRefusedBuild(t, f, "E481", tc.want, "nself remove traefik")
				if *calls != 0 {
					t.Fatalf("refusal attempted %d installs", *calls)
				}
			})
		}
	}
}
