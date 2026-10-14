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

// certbotHooks lists the certbot hooks under dir that name one of sslDirs.
// A missing tree has none.
func certbotHooks(dir string, sslDirs ...string) ([]string, error) {
	var needles []string
	for _, d := range sslDirs {
		needles = append(needles, filepath.Clean(d))
		if real, err := filepath.EvalSymlinks(d); err == nil && real != filepath.Clean(d) {
			needles = append(needles, real)
		}
	}
	confs, scripts, err := certbotHookFiles(dir)
	if err != nil {
		return nil, err
	}
	var hits []string
	for _, p := range append(confs, scripts...) {
		data, err := os.ReadFile(p) //nolint:gosec // certbot's tree, read only
		if err != nil {
			return nil, err
		}
		isConf := slices.Contains(confs, p)
		for i, line := range strings.Split(string(data), "\n") {
			code, ok := hookCode(line, isConf)
			if ok && slices.ContainsFunc(needles, func(n string) bool { return mentionsPath(code, n) }) {
				hits = append(hits, fmt.Sprintf("%s:%d", p, i+1))
			}
		}
	}
	return hits, nil
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
