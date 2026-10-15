package deploy

// Purpose: the decisions about host files in the fronting stack's nginx/sites
//          (D-0425), made from the files' content, never from a path alone.
//          Generated site names carry no project prefix, so a path in this
//          host's record can have been replaced by another project's file.
// Inputs:  a Remote, the front root, frontView (stage dir, shipped paths, the
//          host's record: shipped paths and the server_name set of each).
// Outputs: E485 before any change when a shipped path would replace a file that
//          is not this project's; the recorded paths that are still this
//          project's and may be pruned, and a [kept] line for each other one.
// Constraints: only nginx/sites/<name>.conf paths are read. "This project's"
//              means the strict build marker and a server_name set that equals
//              what the record says this project shipped there (or what it ships
//              now). A path without a recorded set (an old record) is treated
//              like an unknown path: its set must equal the shipped file's, and
//              it is never pruned. An empty set never establishes ownership.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// frontView is what the host-file checks need to know.
type frontView struct {
	Stage string
	Files []string            // shipped now
	Prev  []string            // shipped by this host's previous deploy (record)
	Names map[string][]string // that record's server_name set per path; a missing key is "unknown"
}

func sameNames(a, b []string) bool {
	return len(a) > 0 && len(b) > 0 && strings.Join(a, " ") == strings.Join(b, " ")
}

// checkFrontForeign refuses (E485, nothing sent) to replace a host file that is
// not this project's.
func checkFrontForeign(ctx context.Context, r Remote, front string, v frontView) error {
	host, err := readFrontConfs(ctx, r, front, v.Files)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, p := range v.Prev {
		known[p] = true
	}
	for _, p := range v.Files {
		conf, exists := host[p]
		if !exists {
			continue
		}
		if !hasBuildMarker(conf) {
			return errs.New("E485", fmt.Sprintf("unmarked file would be overwritten: %q in the fronting stack", p)).
				WithFix("move or delete that hand-written file on the host (or add the nself generated marker to it), then deploy again; nothing was sent")
		}
		ours, err := localServerNames(v.Stage, p)
		if err != nil {
			return err
		}
		theirs := serverNameSet(conf)
		if rec, have := v.Names[p]; known[p] && have {
			if sameNames(theirs, rec) || sameNames(theirs, ours) {
				continue
			}
		} else if len(theirs) > 0 && len(ours) > 0 && sameNames(theirs, ours) {
			continue
		}
		return errs.New("E485", fmt.Sprintf("%q in the fronting stack is a generated file of another project: its server_name set is [%s], this deploy ships [%s]",
			p, safeText(strings.Join(theirs, " ")), safeText(strings.Join(ours, " ")))).
			WithFix("rename or remove the other project's file, or deploy that project instead; this project did not ship that server_name set there; nothing was sent")
	}
	return nil
}

// frontPrunable splits the recorded paths the new set dropped into the ones
// still this project's (strict marker, server_name set as recorded) and a
// [kept] line for each of the others. Missing files need no action.
func frontPrunable(ctx context.Context, r Remote, front string, v frontView) (prune, kept []string, err error) {
	keep := map[string]bool{}
	for _, f := range v.Files {
		keep[f] = true
	}
	var cand []string
	for _, p := range v.Prev {
		if !keep[p] {
			cand = append(cand, p)
		}
	}
	host, err := readFrontConfs(ctx, r, front, cand)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range cand {
		conf, exists := host[p]
		if !exists {
			continue
		}
		rec, have := v.Names[p]
		switch {
		case !hasBuildMarker(conf):
			kept = append(kept, fmt.Sprintf("[kept] %s/%s (no generated marker; not pruned)", front, p))
		case !have:
			kept = append(kept, fmt.Sprintf("[kept] %s/%s (no server_name set recorded for it; not pruned)", front, p))
		case !sameNames(serverNameSet(conf), rec):
			kept = append(kept, fmt.Sprintf("[kept] %s/%s (server_name changed since this project shipped it; not pruned)", front, p))
		default:
			prune = append(prune, p)
		}
	}
	return prune, kept, nil
}
