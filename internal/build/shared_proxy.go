package build

// shared_proxy.go — the build half of the shared host proxy (EPIC ADOPT D2,
// contract:cli.shared-proxy v1, P7-ADOPT-03).
//
// Purpose: one nginx can front several projects. The owner (NGINX_MODE=shared)
// keeps nginx/shared/; a fronted project nested under it renders container-name
// upstreams, prefixed site files and a registration, and refuses a name,
// certificate or bare upstream another project already holds. Without
// nginx/shared/ nothing here changes a byte (legacy nested fronted mode).
// Inputs: the loaded config, the route model, the plugin snippets, and the
// registrations already on disk under <owner>/nginx/shared/.
// Outputs: sharedBegin qualifies the model and validates (it writes nothing);
// siteSink routes the fronted project's site writes; sharedFinish writes the
// certificates, the registration and removes stale files.
// Constraints: sharedBegin runs inside modelRoutes (before any write, also for
// --check), sharedFinish after the last nginx file is written (Build). Split by
// responsibility: shared_proxy_check.go (E503/E504), shared_proxy_sink.go (site
// writes), shared_proxy_certs.go (certificates), shared_proxy_files.go
// (registration files).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/sharedproxy"
)

// sharedRole is what this build is on a shared proxy.
type sharedRole int

const (
	sharedNone    sharedRole = iota // legacy: byte-identical output
	sharedOwner                     // NGINX_MODE=shared
	sharedFronted                   // nested under an owner that has nginx/shared/
)

// sharedProxy is the plan of one build, made by sharedBegin.
type sharedProxy struct {
	role      sharedRole
	project   string
	workdir   string // absolute
	ownerRoot string // the owner's project dir (== workdir for the owner)
	sharedDir string // <ownerRoot>/nginx/shared
	sitesDir  string // <ownerRoot>/nginx/sites
	sslDir    string // <ownerRoot>/ssl
	set       sharedproxy.Set
	prev      *sharedproxy.Registration // this project's registration before this build
	container map[string]string         // bare compose service name -> container name
	facts     sharedFacts
	files     map[string]bool // owner-relative files written by this build (fronted)
	written   map[string]bool // server names of the confs this build wrote (fronted)
	certDirs  map[string]bool // certificate directory names the site confs reference
	// dropRoot is set when this project has no active shared role but may hold
	// a registration from an earlier build under that owner dir.
	dropRoot string
}

// active reports whether this build writes into a shared proxy as a fronted project.
func (sp *sharedProxy) active() bool { return sp != nil && sp.role == sharedFronted }

// qualify maps a bare compose service name to its container name.
func (sp *sharedProxy) qualify(host string) (string, bool) {
	name, ok := sp.container[normHost(host)]
	return name, ok
}

// sharedModeCheck raises E505 for an invalid NGINX_MODE: a bad value or
// shared+fronted warn in v1.4 and fail in v1.5, shared with a provider other
// than nginx fails in both modes (new behaviour of a key that did nothing).
func (st *buildState) sharedModeCheck(provider string) error {
	if st.cfg.Nginx.Mode == config.NginxModeShared && provider != ProviderNginx {
		return errs.New("E505", "NGINX_MODE=shared needs nginx as the proxy provider, not "+provider)
	}
	problem := st.cfg.NginxModeProblem()
	if problem == "" {
		return nil
	}
	// compat.V15(P7-ADOPT-03): warn -> E505
	if compat.V15() {
		return errs.New("E505", problem)
	}
	warnOnce(problem + " (an error from v1.5)")
	return nil
}

// sharedBegin plans the shared role, qualifies m and validates. It writes nothing.
func (st *buildState) sharedBegin(m *routemodel.Model) error {
	sp, err := st.sharedDetect()
	if err != nil {
		return err
	}
	st.shared = sp
	if sp.role == sharedNone {
		return nil
	}
	sp.container = st.sharedServices()
	routemodel.QualifyUpstreams(m, sp.qualify)
	set := &snippetSet{}
	set.plugins(st.workdir, st.cfg, m.DefaultServer.HTTP.RedirectHTTPS)
	if sp.role == sharedOwner {
		set.handManaged(st.workdir, st.cfg, m.Env)
	}
	set.parse()
	if sp.role == sharedFronted {
		if err := sp.checkProjectName(m, set); err != nil {
			return err
		}
		routemodel.PrefixSiteFiles(m, sp.project)
	}
	sp.facts = sp.collect(m, set)
	if proxyAPIDocsEnabled(st.cfg) && m.BaseDomain != "" { // api-docs.conf serves docs.<base>
		sp.facts.names = append(sp.facts.names, "docs."+strings.ToLower(m.BaseDomain))
		sort.Strings(sp.facts.names)
	}
	if err := sp.check(); err != nil {
		return err
	}
	if err := st.checkCertTargets(sp); err != nil {
		return err
	}
	if sp.role == sharedFronted && !st.planning() {
		// The registration is written last; fail now if it could not be.
		if err := st.sink.MkdirAll(sp.sharedDir, 0o755); err != nil {
			return certRefusal("cannot create %s: %v", sp.sharedDir, err)
		}
		return probeWritable(sp.sharedDir)
	}
	return nil
}

// sharedDetect decides the role and loads what is on disk.
func (st *buildState) sharedDetect() (*sharedProxy, error) {
	abs, err := filepath.Abs(st.workdir)
	if err != nil {
		return nil, err
	}
	sp := &sharedProxy{project: st.cfg.ProjectName, workdir: abs, files: map[string]bool{}, written: map[string]bool{}, certDirs: map[string]bool{}}
	switch {
	case st.cfg.NginxSharedOwner():
		sp.role, sp.ownerRoot = sharedOwner, abs
	case st.cfg.Nginx.FrontedBy != "":
		root, err := nginxtopo.ServedRoot(abs, st.cfg.Nginx.FrontedBy)
		if err != nil {
			return sp, nil // the sites-dir resolver refuses with its own message
		}
		if _, err := os.Stat(filepath.Join(sharedproxy.DirOf(root), sharedproxy.OwnerFile)); err == nil {
			sp.role, sp.ownerRoot = sharedFronted, root
		} else if _, err := os.Stat(sharedproxy.DirOf(root)); err == nil {
			sp.dropRoot = root // owner dropped shared mode: this project's files go
		}
	default:
		sp.dropRoot = filepath.Dir(abs) // no longer fronted: drop an old registration
	}
	if sp.role == sharedNone {
		return sp, nil
	}
	ownerNginx, _ := nginxtopo.ServedNginxDir(sp.ownerRoot, "") // cannot fail with an empty frontedBy
	sp.sharedDir = sharedproxy.DirOf(sp.ownerRoot)
	sp.sitesDir = filepath.Join(ownerNginx, "sites")
	sp.sslDir, _ = nginxtopo.ServedSSLDir(sp.ownerRoot, "")
	if err := st.lockShared(sp); err != nil {
		return nil, err
	}
	if sp.set, err = sharedproxy.Load(sp.sharedDir, sp.ownerRoot); err != nil {
		return nil, errs.New("E503", "a registration under "+sp.sharedDir+" does not validate: "+err.Error()).
			WithFix("Fix or delete that file (it is written by `nself build` in the project it names), then build again.")
	}
	sp.prev = sp.set.Find(sp.project)
	if sp.role == sharedFronted {
		if !sharedproxy.ValidProject(sp.project) {
			return nil, errs.New("E503", fmt.Sprintf("project name %q cannot be registered on a shared proxy", sp.project)).
				WithFix("Use a PROJECT_NAME of letters, digits, '-' and '_', not starting with '_' or '-'.")
		}
		if sp.set.Owner != nil && sp.set.Owner.Project == sp.project {
			return nil, errs.New("E503", fmt.Sprintf("project name %q is also the name of the proxy owner in %s", sp.project, sp.ownerRoot)).
				WithFix("Give this project its own PROJECT_NAME.")
		}
		if sp.prev != nil && !sameDir(sp.prev.ProjectDir, abs) {
			return nil, errs.New("E503", fmt.Sprintf("project %q is already registered by %s, not %s", sp.project, sp.prev.ProjectDir, abs)).
				WithFix("Give this project its own PROJECT_NAME, or run `nself uninstall` in the other directory.")
		}
	}
	return sp, nil
}

// sharedServices maps this project's bare compose service names (core,
// optional, custom and installed plugin services) to container names.
func (st *buildState) sharedServices() map[string]string {
	out := map[string]string{}
	for _, svc := range DetectServices(st.cfg) {
		out[svc] = st.cfg.ServiceContainerName(svc)
	}
	dir := resolvePluginNginxDir("", st.cfg)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Name(), pluginComposeFilename)); err == nil && e.IsDir() {
			out[e.Name()] = st.cfg.ServiceContainerName(e.Name())
		}
	}
	return out
}

// sameDir reports whether a and b name one directory.
func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// serviceNames returns the sorted bare service names of this project.
func (sp *sharedProxy) serviceNames() []string {
	names := make([]string, 0, len(sp.container))
	for n := range sp.container {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// siteSink returns the sink the nginx site writers use: st.sink itself, or for
// a fronted project in shared mode a wrapper that prefixes the file names,
// stamps the marker, qualifies plugin upstreams and records the files.
func (st *buildState) siteSink() Sink {
	if !st.shared.active() {
		return st.sink
	}
	return &sharedSink{Sink: st.sink, sp: st.shared}
}

// sharedFinish writes what only a finished build can: certificates, the
// registration or `_owner.json`, and the removal of stale files.
func (st *buildState) sharedFinish() error {
	sp := st.shared
	if sp == nil {
		return nil
	}
	switch sp.role {
	case sharedOwner:
		return st.finishOwner(sp)
	case sharedFronted:
		return st.finishFronted(sp)
	}
	return st.dropRegistration(sp)
}
