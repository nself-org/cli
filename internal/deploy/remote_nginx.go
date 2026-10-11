package deploy

// Purpose: validate the shipped nginx config before anything reloads or
//          starts (D24 step 6): `nginx -t` in a throwaway container made from
//          the image of the resolved compose `nginx` service, with that
//          service's bind mounts under the project mounted read-only at the
//          same container paths.
// Inputs:  a Remote, the remote project path, the resolved ComposeModel.
// Outputs: nil when the config is valid or there is no nginx service (one
//          line says so); an error carrying nginx's own words otherwise.
// Constraints: `docker run --rm` only; nothing is started in the stack and
//              nothing is reloaded. The compose network is joined when it
//              exists so upstream names resolve; on a first deploy it does not.

import (
	"context"
	"fmt"
	"io"
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
	script := fmt.Sprintf(`net=%s; netarg=''; if [ -n "$net" ] && docker network inspect "$net" >/dev/null 2>&1; then netarg="--network $net"; fi
docker run --rm $netarg %s %s nginx -t 2>&1`, q(net), strings.Join(mounts, " "), q(svc.Image))
	if o, err := r.Run(ctx, shScript(script)); err != nil {
		return fmt.Errorf("nginx -t failed on %s, nothing was reloaded or started: %w\n%s", r.Label(), err, strings.TrimSpace(string(o)))
	}
	return nil
}
