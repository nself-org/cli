package imageswitch

// preflight.go — checks before anything is stopped or written.
//
// Purpose: refuse early, with E509 and a named reason, when the switch could
// not finish safely: postgres not running, no pgvector image of the same
// major, a cluster feature the restore cannot carry, or too little disk.
// Inputs: the running postgres container and its databases. Outputs: a
// preflight result; the caller turns a failed check into E509.
// Constraints: read only. The only side effects are a one-shot that measures
// the docker data root (its scratch volume is removed) and the image pull that
// one-shot triggers.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// preflightResult is everything preflight learned.
type preflightResult struct {
	Checks    []Check
	Info      ContainerInfo
	Cluster   *clusterDoc
	Target    string
	DataBytes int64
	// Already is true when the running postgres is a pgvector image.
	Already bool
}

// failed returns the E509 error naming every failed check, nil when all passed.
func (r *preflightResult) failed() error {
	var bad []string
	for _, c := range r.Checks {
		if !c.OK {
			bad = append(bad, c.Name+": "+c.Detail)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return coded("E509", "image switch preflight failed", strings.Join(bad, "; "), "")
}

func (r *preflightResult) add(name string, ok bool, detail string) {
	r.Checks = append(r.Checks, Check{Name: name, OK: ok, Detail: detail})
}

// preflight runs the checks. A check that cannot run because an earlier one
// failed is not added.
func (s *Switch) preflight(ctx context.Context) (*preflightResult, error) {
	r := &preflightResult{}
	info, err := s.d.Runtime.Inspect(ctx, s.cfg.Container)
	if err != nil || !info.Running {
		why := "the container is not running"
		if err != nil {
			why = err.Error()
		}
		r.add("postgres running", false, fmt.Sprintf("%s: %s; start the stack with `nself start`", s.cfg.Container, why))
		return r, nil
	}
	r.Info = info
	r.add("postgres running", true, s.cfg.Container+" runs "+info.Image)
	if pgvectorImage(info.Image) {
		r.Already = true
		return r, nil
	}
	if info.DataVolume == "" {
		r.add("data volume", false, "the postgres data directory is not on a named volume, so there is no old volume to keep")
		return r, nil
	}
	cl, err := s.readCluster(ctx)
	if err != nil {
		return nil, err
	}
	r.Cluster = cl
	s.checkCluster(r)
	target, terr := s.cfg.TargetFor(cl.Major)
	if terr != nil || target == "" {
		r.add("pgvector image", false, fmt.Sprintf("no pgvector image for Postgres %d: %v", cl.Major, terr))
		return r, nil
	}
	r.Target = target
	r.add("pgvector image", true, target+" (same major, "+strconv.Itoa(cl.Major)+")")
	if err := s.checkReadOnlyAndSize(ctx, r); err != nil {
		return nil, err
	}
	s.checkTarget(ctx, r)
	return r, nil
}

// readCluster runs capcluster.sql in the old container.
func (s *Switch) readCluster(ctx context.Context) (*clusterDoc, error) {
	q, err := Asset("capcluster.sql")
	if err != nil {
		return nil, err
	}
	out, err := s.d.Runtime.ExecSQL(ctx, s.cfg.Container, s.cfg.User, "template1", q)
	if err != nil {
		return nil, fmt.Errorf("reading the cluster: %s", redactTail(err.Error(), 3))
	}
	return parseCluster(out)
}

// checkCluster adds the checks that read the cluster document.
func (s *Switch) checkCluster(r *preflightResult) {
	cl := r.Cluster
	if len(cl.Tablespaces) == 0 {
		r.add("tablespaces", true, "only the default tablespaces")
	} else {
		r.add("tablespaces", false, strings.Join(cl.Tablespaces, ", ")+" (the restore cannot recreate custom tablespaces)")
	}
	var bad []string
	for _, d := range cl.Databases {
		if !d.AllowConnections {
			bad = append(bad, d.Name+" does not allow connections")
		}
		if d.Provider != "c" {
			bad = append(bad, d.Name+" uses locale provider "+d.Provider+" (only libc is moved)")
		}
		if strings.IndexFunc(d.Name, unicode.IsControl) >= 0 {
			bad = append(bad, "a database name holds a control character")
		}
	}
	detail := fmt.Sprintf("%d databases", len(cl.Databases))
	if len(bad) > 0 {
		detail = strings.Join(bad, "; ")
	}
	r.add("databases", len(bad) == 0 && len(cl.Databases) > 0, detail)
}

// checkReadOnlyAndSize adds the read-only and data-size checks.
func (s *Switch) checkReadOnlyAndSize(ctx context.Context, r *preflightResult) error {
	out, err := s.d.Runtime.ExecSQL(ctx, s.cfg.Container, s.cfg.User, "template1",
		`SELECT (SELECT count(*) FROM pg_db_role_setting s, unnest(s.setconfig) c WHERE c ~ '^default_transaction_read_only='), (SELECT coalesce(sum(pg_database_size(datname)), 0) FROM pg_database)`)
	if err != nil {
		return fmt.Errorf("measuring the cluster: %s", redactTail(err.Error(), 3))
	}
	parts := strings.Split(strings.TrimSpace(out), "|")
	if len(parts) != 2 {
		return fmt.Errorf("measuring the cluster: unexpected answer %q", strings.TrimSpace(out))
	}
	ro, _ := strconv.Atoi(parts[0])
	r.DataBytes, _ = strconv.ParseInt(parts[1], 10, 64)
	if ro == 0 {
		r.add("not read-only", true, "no database sets default_transaction_read_only")
	} else {
		r.add("not read-only", false, "a database or role already sets default_transaction_read_only, so the switch cannot tell its own quiesce from yours")
	}
	return nil
}

// checkTarget runs the target image once (docker pulls it if it is missing) to
// measure the docker data root, list the extensions and locales it ships, and
// checks the stage directory with statfs.
func (s *Switch) checkTarget(ctx context.Context, r *preflightResult) {
	p, err := s.probeTarget(ctx, r.Target)
	if err != nil {
		r.add("target image", false, "could not run it: "+redactTail(err.Error(), 2))
	} else {
		need := r.DataBytes * 2
		r.add("docker data root", p.freeKB*1024 >= need, fmt.Sprintf("%s free, %s needed (2x the data)", humanBytes(p.freeKB*1024), humanBytes(need)))
		s.checkShips(ctx, r, p)
	}
	free, ferr := s.d.FreeBytes(nearestExisting(dir(s.cfg.ProjectDir)))
	need := r.DataBytes + r.DataBytes/5
	switch {
	case ferr != nil:
		r.add("stage directory", false, "could not measure free space: "+ferr.Error())
	default:
		r.add("stage directory", free >= uint64(need), fmt.Sprintf("%s free under .nself, %s needed (1.2x the data)", humanBytes(int64(free)), humanBytes(need))) //nolint:gosec // free space fits int64
	}
}

// checkShips checks the target image ships the extensions the databases use and
// the locales they were created with.
func (s *Switch) checkShips(ctx context.Context, r *preflightResult, p *targetProbe) {
	var missing []string
	for _, d := range r.Cluster.Databases {
		out, err := s.d.Runtime.ExecSQL(ctx, s.cfg.Container, s.cfg.User, d.Name, "SELECT coalesce(string_agg(extname, ','), '') FROM pg_extension")
		if err != nil {
			r.add("extensions", false, "could not list the extensions of "+d.Name+": "+redactTail(err.Error(), 2))
			return
		}
		for _, e := range strings.Split(strings.TrimSpace(out), ",") {
			if e != "" && !p.extensions[e] {
				missing = append(missing, d.Name+": "+e)
			}
		}
	}
	if len(missing) == 0 {
		r.add("extensions", true, "every installed extension is in the pgvector image")
	} else {
		r.add("extensions", false, "the pgvector image lacks "+strings.Join(missing, ", ")+"; a database that uses an extension the image does not ship cannot move")
	}
	missing = nil
	for _, d := range r.Cluster.Databases {
		for _, l := range []string{d.Collate, d.Ctype} {
			if !p.hasLocale(l) {
				missing = append(missing, d.Name+": "+l)
			}
		}
	}
	if len(missing) == 0 {
		r.add("locales", true, "every database locale is in the pgvector image")
	} else {
		r.add("locales", false, "the pgvector image does not have the locale "+strings.Join(missing, ", "))
	}
}

// nearestExisting returns p or its closest existing parent.
func nearestExisting(p string) string {
	for p != "/" && p != "." {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		p = filepath.Dir(p)
	}
	return p
}

// humanBytes renders a byte count with a binary unit.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// estimateSeconds is the downtime window: services stop before the dump and
// the stack is healthy after the switch. The data is read about four times
// (dump, restore, verify, recheck).
func estimateSeconds(dataBytes int64) int {
	const perSecond = 20 << 20
	return 60 + int(dataBytes/perSecond)
}
