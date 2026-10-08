package canon

// Mode views of the raw canon (contract:cli.canon-fragments v1, EPIC D3).
//
// Purpose:     turn the merged raw data (canonical paths plus move rows) into the
//              registry the running compat mode exposes.
// Inputs:      a validated raw *File.
// Outputs:     a *File whose Commands are the registry entries for the mode and
//              whose rows are empty; or a *ValidationError.
// Constraints: pure (no I/O, no globals); the raw File is never modified.
//
// v1.5: entries at canonical paths (mode v1.4 entries excluded), hubs, builtins
// as canon builtin, and one generated deprecated-shim entry (target = to) per
// moves/shims/retired_hubs `from`. Break-outs and removed rows add no entry:
// their old spellings are authored as mode v1.4 entries and vanish in v1.5.
// v1.4: every moved entry mapped back to its `from` path (canon pending at
// depth 1, subcommand deeper), mode v1.4 entries included, no hubs, builtins
// unchanged.

import (
	"fmt"
	"sort"
	"strings"
)

// moveRow is one row that renames a path in the v1.4 view.
type moveRow struct{ from, to string }

// View returns the registry view for a compat mode. A rule violation (a
// generated shim colliding with a command, a `to` that is missing or a shim,
// two entries mapping onto one v1.4 path) is returned as a *ValidationError.
func (f *File) View(v15 bool) (*File, error) {
	out := &File{SchemaVersion: f.SchemaVersion, Verbs: f.Verbs, Commands: make(map[string]Entry, len(f.Commands))}
	var p []string
	if v15 {
		p = f.viewV15(out.Commands)
	} else {
		p = f.viewV14(out.Commands)
	}
	if err := NewValidationError(p); err != nil {
		return nil, err
	}
	return out, nil
}

// forwards lists every row whose `from` becomes a deprecated-shim stub in v1.5.
func (f *File) forwards() []Row {
	var rows []Row
	rows = append(rows, f.Moves...)
	rows = append(rows, f.Shims...)
	rows = append(rows, f.RetiredHubs...)
	return rows
}

func (f *File) viewV15(out map[string]Entry) []string {
	var p []string
	for k, e := range f.Commands {
		if e.Mode == ModeV14 {
			continue
		}
		e.Mode = ""
		out[k] = e
	}
	verbs := map[string]bool{}
	for _, v := range f.Verbs {
		verbs[v] = true
	}
	for _, h := range f.Hubs {
		if _, dup := out[h.Path]; dup {
			p = append(p, fmt.Sprintf("%shub %q is also a command at that path", f.where("hub", h.Path), h.Path))
			continue
		}
		e := Entry{}
		if Depth(h.Path) == 1 {
			e.Canon = CanonPending
			if verbs[h.Path] {
				e.Canon = CanonCore
			}
		}
		out[h.Path] = e
	}
	for _, b := range f.Builtins {
		e, ok := out[b]
		if !ok {
			e = Entry{SideEffect: SideEffectRead}
		}
		e.Canon = CanonBuiltin
		out[b] = e
	}
	rows := f.forwards()
	for _, r := range rows {
		if _, dup := out[r.From]; dup {
			p = append(p, fmt.Sprintf("%sfrom %q is also a command at its canonical path; author the old spelling with mode: v1.4", f.where("from", r.From), r.From))
			continue
		}
		out[r.From] = Entry{Canon: CanonShim, Target: r.To}
	}
	for _, r := range rows {
		e, ok := out[r.From]
		if !ok || e.Canon != CanonShim || e.Target != r.To {
			continue
		}
		t, ok := out[r.To]
		switch {
		case !ok:
			p = append(p, fmt.Sprintf("%sfrom %q: to %q does not resolve to a command in the v1.5 view", f.where("from", r.From), r.From, r.To))
		case t.Canon == CanonShim:
			p = append(p, fmt.Sprintf("%sfrom %q: to %q is itself a shim", f.where("from", r.From), r.From, r.To))
		default:
			e.SideEffect = t.SideEffect
			if e.SideEffect == "" { // a non-runnable target: classify the stub as the riskiest class
				e.SideEffect = SideEffectDestructive
			}
			out[r.From] = e
		}
	}
	return p
}

func (f *File) viewV14(out map[string]Entry) []string {
	var p []string
	moves := make([]moveRow, 0, len(f.Moves))
	for _, m := range f.Moves {
		moves = append(moves, moveRow{m.From, m.To})
	}
	// longest `to` first, so the most specific move wins
	sort.Slice(moves, func(i, j int) bool { return len(moves[i].to) > len(moves[j].to) })
	// back maps a canonical path to its v1.4 path; root reports that the path is
	// exactly a moved node (not a descendant of one).
	back := func(key string) (nk string, moved, root bool) {
		for _, m := range moves {
			if key == m.to || strings.HasPrefix(key, m.to+" ") {
				return m.from + key[len(m.to):], true, key == m.to
			}
		}
		return key, false, false
	}
	keys := make([]string, 0, len(f.Commands))
	for k := range f.Commands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := f.Commands[k]
		nk := k
		if e.Mode != ModeV14 {
			var root bool
			nk, _, root = back(k)
			if root && e.Canon != CanonShim {
				// a moved node leaves the canon: top-level pending, deeper subcommand
				e.Canon = ""
				if Depth(nk) == 1 {
					e.Canon = CanonPending
				}
			}
			if e.Canon == CanonShim && e.Target != "" {
				e.Target, _, _ = back(e.Target)
			}
			// The admin builtin becomes a plugin in v1.5 only. Its v1.4
			// canon entries retain the presplit pending/subcommand values.
			if e.Canon == CanonPlugin && (k == "admin" || strings.HasPrefix(k, "admin ")) {
				e.Canon = ""
				if k == "admin" {
					e.Canon = CanonPending
				}
			}
		}
		e.Mode = ""
		if _, dup := out[nk]; dup {
			p = append(p, fmt.Sprintf("%scommands[%q] maps onto v1.4 path %q, already used by another entry", f.where("commands", k), k, nk))
			continue
		}
		out[nk] = e
	}
	return p
}
