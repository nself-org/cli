package routemodel

// tokenize.go — the nginx configuration tokenizer shared by the route model
// and the build conflict checks, plus the value readers the directive table
// uses (times, sizes, host:port).
//
// Purpose: turn nginx config text into a tree of directives and blocks.
// Inputs: the text of an nginx file (any nesting, compact or multi-line).
// Outputs: []*Block, one per top-level directive or block, in source order.
// Constraints: a lexical reader, not an evaluator: no variables, includes or
// maps. The tree keeps every word as read (quotes included), so nothing is
// dropped. Words are read as ngx_conf_read_token reads them: a quote opens a
// quoted span only at the start of a word, a backslash escapes the next byte,
// a `}` inside a word is literal, a `#` starts a comment only at the start of
// a token. Parse reports structural faults (unbalanced braces, a missing `;`
// at the end, an open quote, text glued to a closing quote) and still returns
// the tree.

import (
	"errors"
	"fmt"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var (
	hostRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	durRE  = regexp.MustCompile(`^([0-9]{1,9})(ms|s|m|h|d|w)?`)
	sizeRE = regexp.MustCompile(`^([0-9]{1,12})([kKmMgG]?)$`)
)

// Block is one nginx directive (Block=false) or block (Block=true).
type Block struct {
	Name     string   // first word of the header; "" for an anonymous `{`
	Args     []string // remaining header words, raw (quotes kept)
	Block    bool     // true when the header is followed by `{ ... }`
	Children []*Block // directives and blocks inside the braces, in order
}

// Render returns the block as one normalised line: single spaces between
// words, `;` after a directive, ` { child child }` after a block header.
// Two sources that differ only in whitespace render identically; this is the
// verbatim form recorded in `unmodelled`.
func (b *Block) Render() string {
	var sb strings.Builder
	b.render(&sb)
	return sb.String()
}

func (b *Block) render(sb *strings.Builder) {
	header := strings.Join(append([]string{b.Name}, b.Args...), " ")
	sb.WriteString(header)
	if !b.Block {
		sb.WriteByte(';')
		return
	}
	if header == "" {
		sb.WriteString("{")
	} else {
		sb.WriteString(" {")
	}
	for _, c := range b.Children {
		sb.WriteByte(' ')
		c.render(sb)
	}
	sb.WriteString(" }")
}

// Arg returns the i-th argument with surrounding quotes removed, or "".
func (b *Block) Arg(i int) string {
	if i < 0 || i >= len(b.Args) {
		return ""
	}
	return Unquote(b.Args[i])
}

// Unquote removes one pair of matching quotes from a quoted word and
// resolves the escapes nginx resolves: \" \' \\ and \t \r \n. Other
// backslash pairs are kept as written.
func Unquote(w string) string {
	if len(w) >= 2 && (w[0] == '"' || w[0] == '\'') && w[len(w)-1] == w[0] {
		w = w[1 : len(w)-1]
	}
	var sb strings.Builder
	for i := 0; i < len(w); i++ {
		if w[i] == '\\' && i+1 < len(w) {
			if r, ok := map[byte]byte{'"': '"', '\'': '\'', '\\': '\\', 't': '\t', 'r': '\r', 'n': '\n'}[w[i+1]]; ok {
				sb.WriteByte(r)
				i++
				continue
			}
		}
		sb.WriteByte(w[i])
	}
	return sb.String()
}

// Tokenize returns the tree of text. Structural faults are ignored; use
// Parse when they matter.
func Tokenize(text string) []*Block {
	tree, _ := Parse(text)
	return tree
}

// Parse returns the tree of text and a non-nil error when the text is not
// structurally sound. The tree is always complete: an unterminated directive
// or block is closed at the point the text ends.
func Parse(text string) ([]*Block, error) {
	root := &Block{Block: true}
	stack := []*Block{root}
	var words []string
	var errs []error
	emit := func(block bool) *Block {
		b := &Block{Block: block}
		if len(words) > 0 {
			b.Name, b.Args = words[0], append([]string(nil), words[1:]...)
		}
		words = words[:0]
		top := stack[len(stack)-1]
		top.Children = append(top.Children, b)
		return b
	}
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case strings.IndexByte(" \t\n\r\f\v", c) >= 0:
			i++
		case c == '#':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == ';':
			if len(words) > 0 {
				emit(false)
			}
			i++
		case c == '{':
			stack = append(stack, emit(true))
			i++
		case c == '}':
			if len(words) > 0 {
				emit(false)
				errs = append(errs, errors.New("directive without ';' before '}'"))
			}
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			} else {
				errs = append(errs, errors.New("unexpected '}'"))
			}
			i++
		default:
			w, n, ok := readWord(text[i:])
			if !ok {
				errs = append(errs, errors.New("bad quote"))
			}
			words = append(words, w)
			i += n
		}
	}
	if len(words) > 0 {
		emit(false)
		errs = append(errs, errors.New("directive without ';' at end of input"))
	}
	if len(stack) > 1 {
		errs = append(errs, fmt.Errorf("%d block(s) not closed", len(stack)-1))
	}
	return root.Children, errors.Join(errs...)
}

// readWord reads one word from s the way ngx_conf_read_token does. A quote
// opens a quoted span only at the start of the word; a backslash escapes the
// next byte; an unquoted word ends at whitespace, `;` or `{` (a `{` after `$`
// is part of ${name}); a `}` inside a word is literal. ok is false for an open
// quote and for text glued to a closing quote.
func readWord(s string) (word string, n int, ok bool) {
	q := byte(0)
	if s[0] == '"' || s[0] == '\'' {
		q, n = s[0], 1
	}
	for n < len(s) {
		switch c := s[n]; {
		case c == '\\':
			n = min(n+2, len(s))
		case q != 0 && c == q:
			n++
			return s[:n], n, n == len(s) || strings.IndexByte(" \t\r\n\f\v;{)", s[n]) >= 0
		case q == 0 && (strings.IndexByte(" \t\r\n\f\v;", c) >= 0 || (c == '{' && n > 0 && s[n-1] != '$')):
			return s[:n], n, true
		default:
			n++
		}
	}
	return s, len(s), q == 0
}

// unquoteAll unquotes every word of a.
func unquoteAll(a []string) []string {
	out := make([]string, len(a))
	for i, w := range a {
		out[i] = Unquote(w)
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

// splitHostPort parses host[:port] or [v6][:port]; a missing port takes the
// scheme default. It returns nil for anything that is not a plain host.
func splitHostPort(s, scheme string) *hostPort {
	host, port := s, ""
	if rest, ok := strings.CutPrefix(s, "["); ok {
		h, tail, found := strings.Cut(rest, "]")
		if !found || (tail != "" && tail[0] != ':') {
			return nil
		}
		host, port = h, strings.TrimPrefix(tail, ":")
	} else if k := strings.LastIndexByte(s, ':'); k >= 0 {
		host, port = s[:k], s[k+1:]
	}
	if net.ParseIP(host) == nil && !hostRE.MatchString(host) {
		return nil
	}
	n := map[bool]int{false: 80, true: 443}[scheme == "https"]
	if port != "" {
		v, err := strconv.Atoi(port)
		if err != nil || v < 1 || v > 65535 {
			return nil
		}
		n = v
	}
	return &hostPort{host, n}
}

// parseSeconds reads an nginx time (`60`, `30s`, `1m`, `1h30m`) as whole
// seconds. Zero, sub-second remainders and unknown units are refused.
func parseSeconds(d *Block) (int, bool) {
	in := d.Arg(0)
	units := map[string]int64{"": 1000, "ms": 1, "s": 1000, "m": 60000, "h": 3600000, "d": 86400000, "w": 604800000}
	var ms int64
	prev := int64(1 << 62) // units must go from large to small, each at most once
	for s := in; s != "" && len(d.Args) == 1; {
		m := durRE.FindStringSubmatch(s)
		if m == nil || (m[2] == "" && len(m[0]) != len(in)) || (m[2] != "" && units[m[2]] >= prev) {
			return 0, false
		}
		if m[2] != "" {
			prev = units[m[2]]
		}
		v, _ := strconv.ParseInt(m[1], 10, 64)
		ms += v * units[m[2]]
		s = s[len(m[0]):]
	}
	return int(ms / 1000), len(d.Args) == 1 && ms > 0 && ms%1000 == 0 && ms/1000 <= 1<<30
}

// parseBodySize reads client_max_body_size (k, m, g suffixes). 0 means
// "unlimited" in nginx and the model has no such value, so it is refused.
func parseBodySize(d *Block) (int64, bool) {
	m := sizeRE.FindStringSubmatch(d.Arg(0))
	if m == nil || len(d.Args) != 1 {
		return 0, false
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	shift := map[string]uint{"": 0, "k": 10, "m": 20, "g": 30}[strings.ToLower(m[2])]
	if n > math.MaxInt64>>shift {
		return 0, false
	}
	return n << shift, n > 0
}
