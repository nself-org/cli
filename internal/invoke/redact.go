package invoke

// Purpose: find and replace the secret values of a machine request.
// Inputs: a registry command and a Request. Outputs: RedactRequest and
// SecretValues.
// Constraints: redaction fails closed. A positional past the declared args and
// a flag the command does not declare are redacted too: such a request is
// refused, but the refusal still hashes into the request id, and a secret sent
// in the wrong slot must not reach it.

import (
	"fmt"
	"sort"

	"github.com/nself-org/cli/internal/cmdregistry"
)

// secretArg reports whether positional i is secret (a variadic secret covers
// the rest; an index past the declared args is treated as secret).
func secretArg(cmd *cmdregistry.Command, i int) bool {
	if cmd == nil {
		return true
	}
	for j, a := range cmd.Args {
		if j == i || (a.Variadic && i >= j) {
			return a.Secret
		}
	}
	return true
}

// secretFlag reports whether flag name is secret (a flag the command does not
// declare is treated as secret).
func secretFlag(cmd *cmdregistry.Command, name string) bool {
	if cmd == nil {
		return true
	}
	for _, f := range cmd.Flags {
		if f.Name == name {
			return f.Secret
		}
	}
	return true
}

// RedactRequest returns a copy of r with every registry-secret arg and flag
// value replaced by Redacted. A nil cmd (unknown path) redacts all of them.
func RedactRequest(cmd *cmdregistry.Command, r Request) Request {
	out := Request{Confirm: r.Confirm, Argv: append([]string(nil), r.Argv...)}
	for i, a := range r.Args {
		if secretArg(cmd, i) {
			a = Redacted
		}
		out.Args = append(out.Args, a)
	}
	if r.Flags != nil {
		out.Flags = make(map[string]any, len(r.Flags))
		for name, v := range r.Flags {
			if secretFlag(cmd, name) {
				v = Redacted
			}
			out.Flags[name] = v
		}
	}
	return out
}

// SecretValues lists every secret value the request carries, as the strings a
// child could echo (longest first), for scrubbing stderr tails.
func SecretValues(cmd *cmdregistry.Command, r Request) []string {
	var vals []string
	add := func(s string) {
		if s != "" && s != Redacted {
			vals = append(vals, s)
		}
	}
	for i, a := range r.Args {
		if secretArg(cmd, i) {
			add(a)
		}
	}
	for name, v := range r.Flags {
		if !secretFlag(cmd, name) {
			continue
		}
		switch x := v.(type) {
		case []string:
			for _, s := range x {
				add(s)
			}
		case []any:
			for _, e := range x {
				add(fmt.Sprint(e))
			}
		default:
			add(fmt.Sprint(x))
		}
	}
	sort.SliceStable(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) })
	return vals
}
