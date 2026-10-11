package deploy

// Purpose: validate the shipped nginx config before anything reloads or
//          starts (D24 step 6): `nginx -t` in a throwaway container made from
//          the image of the resolved compose `nginx` service, with that
//          service's bind mounts under the project mounted read-only at the
//          same container paths.
// Inputs:  a Remote, the remote project path, the resolved ComposeModel.
// Outputs: nil when the config is valid or there is no nginx service (one
//          line says so); an error carrying nginx's own words otherwise.
// Constraints: the throwaway container is not on the stack network, so every
//              service name, container_name and network alias of the resolved
//              config is added as a host entry (127.0.0.1): `proxy_pass
//              http://hasura:8080` must pass nginx -t as it does inside the stack.
//              `docker run --rm` only; nothing is started in the stack and
//              nothing is reloaded. The compose network is joined when it
//              exists so upstream names resolve; on a first deploy it does not.

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// NginxServiceName is the compose service the generated stack names nginx.
const NginxServiceName = "nginx"

// CheckNginx runs `nginx -t` for the shipped config. out receives the one
// skip line when the stack has no nginx service.
func CheckNginx(ctx context.Context, r Remote, remotePath string, model ComposeModel, out io.Writer) error {
	svc, ok := model.Services[NginxServiceName]
	if !ok || svc.Image == "" {
		_, _ = fmt.Fprintln(out, "  [skip] nginx -t: this stack has no nginx service")
		return nil
	}
	root := strings.TrimSuffix(remotePath, "/") + "/"
	var mounts []string
	for _, m := range svc.Mounts {
		if m.Type == "bind" && strings.HasPrefix(m.Source, root) {
			mounts = append(mounts, "-v", q(m.Source+":"+m.Target+":ro"))
		}
	}
	net := ""
	if model.Project != "" {
		net = model.Project + "_default"
	}
	var hosts []string
	for _, h := range knownHostNames(model) {
		hosts = append(hosts, "--add-host", q(h+":127.0.0.1"))
	}
	script := fmt.Sprintf(`net=%s; netarg=''; if [ -n "$net" ] && docker network inspect "$net" >/dev/null 2>&1; then netarg="--network $net"; fi
docker run --rm $netarg %s %s %s nginx -t 2>&1`, q(net), strings.Join(hosts, " "), strings.Join(mounts, " "), q(svc.Image))
	if o, err := r.Run(ctx, shScript(script)); err != nil {
		return fmt.Errorf("nginx -t failed on %s, nothing was reloaded or started: %w\n%s", r.Label(), err, strings.TrimSpace(string(o)))
	}
	return nil
}

var hostNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// knownHostNames are the names the stack resolves to its services, validated.
func knownHostNames(model ComposeModel) []string {
	set := map[string]bool{}
	for name, s := range model.Services {
		for _, n := range append([]string{name, s.ContainerName}, s.Aliases...) {
			if hostNameRe.MatchString(n) {
				set[n] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
