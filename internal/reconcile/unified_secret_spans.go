package reconcile

// unified_secret_spans.go — the in-line span rules of the non-env diff
// redaction (D-0321): --flag values and KEY=value / KEY: value pairs anywhere
// in a line. Companion of unified_secrets.go; same constraints (config.IsSecretKey
// is the only name rule, over-masking is the safe side).

import (
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

var (
	// flagName matches a --name flag at the start of a token.
	flagName = regexp.MustCompile(`(^|[\s"',\[])(--[A-Za-z][A-Za-z0-9_-]*)`)
	// flagToken matches the start of the next flag: whitespace, then - and a
	// letter or another dash.
	flagToken = regexp.MustCompile(`\s-[A-Za-z-]`)
	// flagListSep matches the gap between a flag and its value in a list of
	// quoted strings: quote, comma, quote.
	flagListSep = regexp.MustCompile(`^["']\s*,\s*["']`)
	// bareFlag matches a flag that ends the line with no value.
	bareFlag = regexp.MustCompile(`(--[A-Za-z][A-Za-z0-9_-]*)["']?[\s,]*$`)
	// pairKey matches "KEY=" or "KEY:" (optionally quoted) anywhere in a line.
	pairKey = regexp.MustCompile(`(["']?)([A-Za-z_][A-Za-z0-9_.\-]*)(["']?)([ \t]*[:=][ \t]*)`)
	// scalarQuote matches the quote that opens a YAML scalar value: "key: '",
	// "- \"" or a bare quote at the start of the line.
	scalarQuote = regexp.MustCompile(`^\s*(?:-\s+)?(?:(?:"[^"]*"|'[^']*'|[^\s'"\[{][^:]*?)\s*:\s+)?(["'])`)
)

// closeAt returns the offset of the first closing q at or after from, or
// len(line) and q when the quote stays open. In double quotes a backslash
// escapes the next byte; in single quotes a doubled ” is an escaped quote
// (YAML), so neither closes.
func closeAt(line string, from int, q byte) (int, byte) {
	for i := from; i < len(line); i++ {
		switch c := line[i]; {
		case q == '"' && c == '\\':
			i++
		case c == q && q == '\'' && i+1 < len(line) && line[i+1] == '\'':
			i++
		case c == q:
			return i, 0
		}
	}
	return len(line), q
}

// scalarSpan finds the quoted YAML scalar the line's value opens. It returns
// the offset of its opening quote (-1 when the line has none), the offset of
// the closing quote (len(line) when open) and the open quote (0 when closed).
func scalarSpan(line string) (start, end int, open byte) {
	loc := scalarQuote.FindStringSubmatchIndex(line)
	if loc == nil {
		return -1, len(line), 0
	}
	end, open = closeAt(line, loc[3], line[loc[2]])
	return loc[2], end, open
}

// limitAt returns where a value found at offset p must stop: the closing quote
// of the quoted scalar that holds p, else the end of the line.
func limitAt(line string, p int) int {
	if start, end, _ := scalarSpan(line); start >= 0 && p > start && p < end {
		return end
	}
	return len(line)
}

// flags masks the value of every --name flag whose name is a secret name. An
// unquoted value runs to the next flag token (whitespace, then - and a letter
// or dash), the end of the enclosing quoted scalar or the end of the line,
// because a password may hold spaces, quotes and $. A quoted list element
// ("--requirepass", "v") loses only its content. It returns the new text and
// the quote left open by a scalar that continues on the next line.
func (sc *secretScan) flags(line string) (string, byte) {
	var sb strings.Builder
	pos := 0
	_, _, open := scalarSpan(line)
	for _, loc := range flagName.FindAllStringSubmatchIndex(line, -1) {
		ns, ne := loc[4], loc[5]
		if ns < pos || !config.IsSecretKey(line[ns:ne]) {
			continue
		}
		vs, ve := flagValueSpan(line, ne)
		if ve <= vs {
			continue
		}
		sb.WriteString(line[pos:vs])
		sb.WriteString(redactedValue)
		sc.note(line[vs:ve])
		pos = ve
	}
	sb.WriteString(line[pos:])
	return sb.String(), open
}

// flagValueSpan returns the span of the value of the flag that ends at ne, or
// an empty span when the flag has none on this line.
func flagValueSpan(line string, ne int) (int, int) {
	rest := line[ne:]
	limit := limitAt(line, ne)
	var vs int
	switch {
	case strings.HasPrefix(rest, "="):
		vs = ne + 1
	case flagListSep.MatchString(rest):
		sep := flagListSep.FindString(rest)
		vs = ne + len(sep)
		end, _ := closeAt(line, vs, sep[len(sep)-1])
		return vs, end
	default:
		vs = ne + len(rest) - len(strings.TrimLeft(rest, " \t"))
		if vs == ne {
			return ne, ne
		}
	}
	if vs >= limit {
		return vs, vs
	}
	end := limit
	if loc := flagToken.FindStringIndex(line[vs:limit]); loc != nil {
		end = vs + loc[0]
	}
	return vs, vs + len(strings.TrimRight(line[vs:end], " \t"))
}

// endsInSecretFlag reports whether line ends in a secret --flag with no value
// (a YAML list puts the flag and its value on two lines).
func endsInSecretFlag(line string) bool {
	m := bareFlag.FindStringSubmatch(line)
	return m != nil && config.IsSecretKey(m[1])
}

// pairs masks the value of every KEY=value or KEY: value pair whose KEY is a
// secret name, anywhere in the line: assignments, flow maps, JSON, quoted
// headers ("x-hasura-admin-secret: v"), shell exports and `password = "v"`.
// The value ends at its closing quote when quoted; at the closing quote of an
// enclosing quote (- "KEY=value"); at , } ] inside a flow list or map; else at
// the end of the enclosing quoted scalar or the line (a plain value may hold
// spaces). It returns the new text and the quote left open on this line.
func (sc *secretScan) pairs(line string) (string, byte) {
	var sb strings.Builder
	pos, open := 0, byte(0)
	for _, loc := range pairKey.FindAllStringSubmatchIndex(line, -1) {
		s, ks, ke, vs := loc[0], loc[4], loc[5], loc[1]
		if s < pos || !config.IsSecretKey(line[ks:ke]) {
			continue
		}
		if (s > 0 && line[s-1] == '-') || strings.HasPrefix(line[vs:], "//") {
			continue // part of a --flag name, or a URL scheme
		}
		start, end, q := pairValueSpan(line, loc)
		if end <= start {
			continue
		}
		sb.WriteString(line[pos:start])
		sb.WriteString(redactedValue)
		sc.note(line[start:end])
		pos = end
		if q != 0 {
			open = q
			break
		}
	}
	sb.WriteString(line[pos:])
	return sb.String(), open
}

// pairValueSpan returns the span of the value of the pair that matched at loc
// (pairKey submatch offsets), and the quote left open (0 if none).
func pairValueSpan(line string, loc []int) (start, end int, open byte) {
	s, vs := loc[0], loc[1]
	lead, sep := line[loc[2]:loc[3]], strings.TrimSpace(line[loc[8]:loc[9]])
	if lead != "" && line[loc[6]:loc[7]] == "" { // "KEY=value" held in quotes
		end, open = closeAt(line, vs, lead[0])
		return vs, end, open
	}
	if vs >= len(line) {
		return vs, vs, 0
	}
	if c := line[vs]; c == '"' || c == '\'' {
		end, open = closeAt(line, vs+1, c)
		return vs + 1, end, open
	}
	limit := max(limitAt(line, s), vs)
	switch strings.TrimSpace(line[:s]) {
	case "", "-", "export":
		if sep == "=" {
			return vs, vs + len(strings.TrimRight(line[vs:limit], " \t")), 0
		}
	}
	end = limit
	if strings.ContainsAny(line[:s], "[{") {
		if i := strings.IndexAny(line[vs:limit], ",}]"); i >= 0 {
			end = vs + i
		}
	}
	return vs, vs + len(strings.TrimRight(line[vs:end], " \t")), 0
}

// directiveToken matches one whitespace-separated token.
var directiveToken = regexp.MustCompile(`\S+`)

// directive masks the arguments of a directive line (nginx and similar: tokens
// ending in ";", optionally followed by a # comment). The first token is the
// directive name and is never tested. After the first later token that names a
// secret (config.IsSecretKey, quotes and a leading $ stripped),
// every argument up to the ";" becomes one [REDACTED] and each argument is
// collected for repeat masking. Other lines come back unchanged.
func (sc *secretScan) directive(line string) string {
	semi := strings.LastIndex(line, ";")
	if semi < 0 {
		return line
	}
	if rest := strings.TrimSpace(line[semi+1:]); rest != "" && rest[0] != '#' {
		return line
	}
	locs := directiveToken.FindAllStringIndex(line[:semi], -1)
	for i, loc := range locs {
		tok := strings.TrimLeft(strings.Trim(line[loc[0]:loc[1]], `"'`), "$")
		if i == 0 || i == len(locs)-1 || !config.IsSecretKey(tok) {
			continue // the directive name (first token) is never tested
		}
		for _, arg := range locs[i+1:] {
			sc.note(line[arg[0]:arg[1]])
		}
		return line[:loc[1]] + " " + redactedValue + line[semi:]
	}
	return line
}
