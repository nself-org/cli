package commands

// Purpose: finds certbot hooks that would copy certbot's files into an ssl
// dir the ACME engine owns (D-0411): "*_hook"/"*-hook" lines in cli.ini and
// renewal/*.conf (certbot saves a `renew --deploy-hook` there and every certbot
// scheduler replays it) and the scripts certbot runs from
// renewal-hooks/{pre,deploy,post}.
// Inputs: a certbot tree and the ssl dirs to protect.
// Outputs: "file:line" for each hook line that names one of them.
// Constraints: read only; only absence is tolerated, any other read error is
// returned so the caller refuses; a path counts only where it starts and ends
// (no substring of a longer path).

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// certbotHookFiles lists where certbot reads hooks under dir: cli.ini and
// renewal/*.conf (confs), and the files it runs from renewal-hooks/{pre,deploy,post}
// (scripts: regular, executable, no trailing '~', dot files included). Names
// certbot ignores are dropped before they are opened.
func certbotHookFiles(dir string) (confs, scripts []string, err error) {
	list := func(d string, keep func(string) bool) ([]string, error) {
		es, err := os.ReadDir(d)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		var out []string
		for _, e := range es {
			if keep(e.Name()) {
				out = append(out, filepath.Join(d, e.Name()))
			}
		}
		return out, err
	}
	// regular stats p; a dangling link is nothing to run, other errors are returned
	regular := func(p string) (os.FileInfo, bool, error) {
		st, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return st, err == nil && st.Mode().IsRegular(), err
	}
	renewal, err := list(filepath.Join(dir, "renewal"), func(n string) bool { return strings.HasSuffix(n, ".conf") })
	if err != nil {
		return nil, nil, err
	}
	for _, p := range append([]string{filepath.Join(dir, "cli.ini")}, renewal...) {
		if _, ok, err := regular(p); err != nil {
			return nil, nil, err
		} else if ok {
			confs = append(confs, p)
		}
	}
	for _, sub := range []string{"pre", "deploy", "post"} {
		ps, err := list(filepath.Join(dir, "renewal-hooks", sub), func(n string) bool { return !strings.HasSuffix(n, "~") })
		if err != nil {
			return nil, nil, err
		}
		for _, p := range ps {
			st, ok, err := regular(p)
			if err != nil {
				return nil, nil, err
			}
			if ok && (runtime.GOOS == "windows" || st.Mode()&0o111 != 0) {
				scripts = append(scripts, p)
			}
		}
	}
	return confs, scripts, nil
}

// certbotUserConfigs lists the per-user cli.ini files certbot reads besides
// <dir>/cli.ini: own is $XDG_CONFIG_HOME/letsencrypt and ~/.config/letsencrypt of
// the user running nself, root is root's (a root certbot scheduler reads that
// one). Missing ones are harmless. A variable so tests do not read the real home.
var certbotUserConfigs = func() (own, root []string) {
	add := func(list *[]string, base string) {
		if p := filepath.Join(base, "letsencrypt", "cli.ini"); !slices.Contains(own, p) && !slices.Contains(root, p) {
			*list = append(*list, p)
		}
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		add(&own, x)
	}
	if h, err := os.UserHomeDir(); err == nil {
		add(&own, filepath.Join(h, ".config"))
	}
	if u, err := user.Lookup("root"); err == nil && u.HomeDir != "" {
		add(&root, filepath.Join(u.HomeDir, ".config"))
	}
	return own, root
}

// userCertbotConfs returns the per-user cli.ini files that exist. Only absence
// is tolerated, except that a non-root caller who may not enter root's home
// cannot be the one that wrote there, so that denial is skipped.
func userCertbotConfs() ([]string, error) {
	var found []string
	own, root := certbotUserConfigs()
	for _, p := range append(slices.Clone(own), root...) {
		st, err := os.Stat(p)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case errors.Is(err, os.ErrPermission) && slices.Contains(root, p) && os.Geteuid() != 0:
		case err != nil:
			return nil, err
		case st.Mode().IsRegular():
			found = append(found, p)
		}
	}
	return found, nil
}

// certbotHooks lists the certbot hooks under dir that name one of sslDirs.
// A missing tree has none. The per-user cli.ini files (certbotUserConfigs) are
// scanned too, whatever dir is (D-0595).
func certbotHooks(dir string, sslDirs ...string) ([]string, error) {
	confs, scripts, err := certbotHookFiles(dir)
	if err != nil {
		return nil, err
	}
	perUser, err := userCertbotConfs()
	if err != nil {
		return nil, err
	}
	for _, p := range perUser {
		if !slices.Contains(confs, p) {
			confs = append(confs, p)
		}
	}
	var needles []string
	for _, d := range sslDirs {
		needles = append(needles, filepath.Clean(d))
		if real, err := filepath.EvalSymlinks(d); err == nil && real != filepath.Clean(d) {
			needles = append(needles, real)
		}
	}
	var hits []string
	for _, p := range append(confs, scripts...) {
		data, err := os.ReadFile(p) //nolint:gosec // certbot's tree, read only
		if err != nil {
			return nil, err
		}
		isConf := slices.Contains(confs, p)
		lines := strings.Split(string(data), "\n")
		for i := 0; i < len(lines); i++ {
			line, start, quoted := lines[i], i, false
			if isConf {
				line, i, quoted = certbotConfigEntry(lines, i)
			}
			code, ok := hookCode(line, isConf)
			if quoted {
				code = line // '#' inside a config-quoted value is literal config text.
			}
			if ok && slices.ContainsFunc(needles, func(n string) bool { return mentionsPath(code, n) }) {
				hits = append(hits, fmt.Sprintf("%s:%d", p, start+1))
			}
		}
	}
	return hits, nil
}

// Purpose: joins config-quoted values before filtering hook keys.
// Inputs: physical config lines and an entry's opening index.
// Outputs: an unwrapped entry, its final physical index, and whether quoted.
// Constraints: all keys consume continuations; shell quotes within unquoted
// values stay untouched. Unclosed values are retained conservatively.
func certbotConfigEntry(lines []string, start int) (string, int, bool) {
	t := strings.TrimSpace(lines[start])
	if t == "" || t[0] == '#' || t[0] == ';' {
		return lines[start], start, false
	}
	k := strings.IndexFunc(t, func(c rune) bool { return strings.ContainsRune("=: \t", c) })
	if k < 0 {
		return lines[start], start, false
	}
	v := strings.TrimLeft(t[k:], "=: \t")
	if v == "" || (v[0] != '\'' && v[0] != '"') {
		return lines[start], start, false
	}
	delim := v[:1]
	if strings.HasPrefix(v, strings.Repeat(delim, 3)) {
		delim = strings.Repeat(delim, 3)
	}
	parts := []string{v[len(delim):]}
	for end := start; ; end++ {
		part := parts[len(parts)-1]
		if close := certbotConfigClose(part, delim); close >= 0 {
			// A quote followed by shell syntax belongs to an unquoted command.
			tail := strings.TrimSpace(part[close+len(delim):])
			if len(delim) == 1 && tail != "" && tail[0] != '#' {
				return lines[start], start, false
			}
			parts[len(parts)-1] = part[:close]
			return t[:k] + " = " + strings.Join(parts, " "), end, true
		}
		if end+1 == len(lines) {
			return t[:k] + " = " + strings.Join(parts, " "), end, true
		}
		parts = append(parts, lines[end+1])
	}
}

// Purpose: locates a config value's closing delimiter.
// Inputs: value text and a single or triple quote delimiter.
// Outputs: its byte offset, or -1 while the value continues.
// Constraints: an escaped quote does not end a single-quoted value.
func certbotConfigClose(value, delim string) int {
	for i := 0; i < len(value); i++ {
		if len(delim) == 1 && value[i] == '\\' {
			i++
			continue
		}
		if strings.HasPrefix(value[i:], delim) {
			return i
		}
	}
	return -1
}

// hookCode returns the part of line certbot or the shell acts on: for a conf,
// only a "*_hook"/"*-hook" key's line (configobj allows a quoted key); comment
// lines ('#', and ';' in confs) are dropped and a trailing comment is cut.
func hookCode(line string, isConf bool) (string, bool) {
	t := strings.TrimSpace(line)
	if t == "" || t[0] == '#' || (isConf && t[0] == ';') {
		return "", false
	}
	// the key ends at '=', or for cli.ini (configargparse) also at ':' or a space
	if k := strings.Trim(t[:strings.IndexFunc(t+"=", func(c rune) bool { return strings.ContainsRune("=: \t", c) })], `"'`); isConf && !strings.HasSuffix(k, "_hook") && !strings.HasSuffix(k, "-hook") {
		return "", false
	}
	return cutComment(t), true
}

// cutComment cuts a trailing comment: a '#' after a space or tab that is
// outside quotes. A '#' inside "..." or '...' (or escaped) is text the shell
// runs past, so the rest of the line is kept. A line with a command
// substitution ("$(" or a backtick) or an ANSI-C string ("$'") nests quotes
// this scan cannot follow, so it is kept whole: a false refusal names the
// line, a false pass loses the certs.
func cutComment(t string) string {
	if strings.Contains(t, "$(") || strings.Contains(t, "$'") || strings.ContainsRune(t, '`') {
		return t
	}
	var quote byte
	for i := 0; i < len(t); i++ {
		switch c := t[i]; {
		case c == '\\' && quote != '\'':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && i > 0 && (t[i-1] == ' ' || t[i-1] == '\t'):
			return t[:i-1]
		}
	}
	return t
}

// mentionsPath reports whether line names dir or a path under it: dir starts
// at the line start or after a space, quote, '=', ':', a redirect or a shell
// separator, and ends at the line end or before a separator, space, quote,
// ';', ')' or '|'.
func mentionsPath(line, dir string) bool {
	for off := 0; ; {
		i := strings.Index(line[off:], dir)
		if i < 0 {
			return false
		}
		at, end := off+i, off+i+len(dir)
		before := at == 0 || strings.ContainsRune(" \t\"'=:><(|;&,{`", rune(line[at-1]))
		after := end == len(line) || strings.ContainsRune(string(filepath.Separator)+"/ \t\"';)|&\r`", rune(line[end]))
		if before && after {
			return true
		}
		off = at + 1
	}
}
