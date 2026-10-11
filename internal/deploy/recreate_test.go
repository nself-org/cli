package deploy

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// modelJSON is a trimmed `docker compose config --format json` document: a
// stateful db (named volume), a plain web, a one-shot job and nginx with binds.
func modelJSON(rp string) string {
	return `{"name":"proj","services":{
	  "db":{"image":"postgres:16","restart":"unless-stopped","healthcheck":{"test":["CMD","pg_isready"]},
	        "volumes":[{"type":"volume","source":"pgdata","target":"/var/lib/postgresql/data"}]},
	  "web":{"image":"web:1","restart":"always"},
	  "job":{"image":"job:1","restart":"no","healthcheck":{"test":["NONE"]}},
	  "files":{"image":"f:1","volumes":[{"type":"bind","source":"` + rp + `/.volumes/files","target":"/data"}]},
	  "nginx":{"image":"nginx:1.27@sha256:abc","volumes":[
	        {"type":"bind","source":"` + rp + `/nginx","target":"/etc/nginx/conf","read_only":true},
	        {"type":"bind","source":"` + rp + `/ssl","target":"/etc/nginx/ssl","read_only":true},
	        {"type":"bind","source":"/var/run/other","target":"/x"}]}}}`
}

// dryRunOutput is real compose 5.6 `up -d --dry-run` output: two changed
// services recreated, one created, one-shot started, the rest running.
const dryRunOutput = ` Container proj-extra-1 Creating
 Container proj-db-1 Recreate
 Container proj-web-1 Recreate
 Container proj-extra-1 Created
 Container proj-web-1 Recreated
 Container proj-db-1 Recreated
 Container f5fa25ea79fb_proj-db-1 Starting
 Container f5fa25ea79fb_proj-db-1 Started
 Container proj-job-1 Starting
 Container proj-nginx-1 Running
 Network proj_default Creating
`

func TestRecreatePreview(t *testing.T) {
	rp := "/opt/nself"
	model, err := parseComposeModel([]byte(modelJSON(rp)))
	if err != nil {
		t.Fatal(err)
	}
	model.Services["extra"] = ModelService{}
	p := parseDryRun(dryRunOutput, model, rp)
	if strings.Join(p.Recreate, ",") != "db,web" || strings.Join(p.Create, ",") != "extra" || strings.Join(p.Stateful, ",") != "db" {
		t.Fatalf("preview %+v: want recreate db,web; create extra; stateful db", p)
	}
	if !model.Stateful("files", rp) || model.Stateful("web", rp) || model.Stateful("nginx", rp) {
		t.Error("stateful = named volume or bind under the data dir, not any bind")
	}
	if spec, _ := model.Specs("job"); !spec.OneShot() || spec.HasHealthcheck {
		t.Errorf("job spec %+v: restart no with healthcheck NONE is a one-shot", spec)
	}
	if spec, _ := model.Specs("db"); !spec.HasHealthcheck {
		t.Errorf("db spec %+v", spec)
	}
	// container_name and the older underscore names map too.
	m2 := ComposeModel{Services: map[string]ModelService{"api": {ContainerName: "my-api"}, "app-api": {}, "worker": {}}}
	for container, want := range map[string]string{"my-api": "api", "proj-app-api-2": "app-api", "proj_worker_1": "worker", "nope-1": ""} {
		if got := serviceOfContainer(container, m2); got != want {
			t.Errorf("serviceOfContainer(%q) = %q, want %q", container, got, want)
		}
	}

	// The confirm rule: only a stateful recreate on a prod-class target needs it.
	cases := []struct {
		name        string
		p           RecreatePreview
		prod, yes   bool
		confirm     func(string) bool
		wantRefusal bool
		wantNaming  string
	}{
		{"not prod-class", p, false, false, nil, false, ""},
		{"prod-class, nothing stateful", RecreatePreview{Recreate: []string{"web"}}, true, false, nil, false, ""},
		{"prod-class, stateful, no confirm", p, true, false, nil, true, "db"},
		{"prod-class, stateful, --yes", p, true, true, nil, false, ""},
		{"prod-class, stateful, TTY yes", p, true, false, func(string) bool { return true }, false, ""},
		{"prod-class, stateful, TTY no", p, true, false, func(string) bool { return false }, true, "db"},
	}
	for _, c := range cases {
		err := ConfirmStateful(c.p, c.prod, c.yes, c.confirm)
		if (err != nil) != c.wantRefusal {
			t.Errorf("%s: err = %v", c.name, err)
			continue
		}
		if err != nil {
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E403" || !strings.Contains(err.Error(), c.wantNaming) {
				t.Errorf("%s: want E403 naming %s, got %v", c.name, c.wantNaming, err)
			}
		}
	}
}
