package migrate

import (
	"errors"
	"strings"
)

// ErrTxControl marks a migration file that contains transaction control. The
// per-file transaction is what keeps the ledger honest: a COMMIT inside a
// file would leave its earlier statements committed with no ledger row, and
// every later boot would replay them.
var ErrTxControl = errors.New("migration file contains transaction control; remove it, the helper wraps every file in its own transaction")

// txWords are the statement keywords refused at the start of a statement.
// START and PREPARE count only when followed by TRANSACTION.
var txWords = map[string]bool{
	"BEGIN": true, "COMMIT": true, "ROLLBACK": true, "END": true, "ABORT": true,
	"SAVEPOINT": true, "RELEASE": true,
}

type token struct {
	word string // upper-cased word, or ";" for a statement end
	line int
}

// findTxControl reports the first transaction-control statement in sql and
// its line, or "" when there is none. It is a lexer, not a parser: it skips
// comments, '...' and E'...' strings, "..." identifiers and $tag$...$tag$
// bodies, so those words never match, then looks at the first word(s) of each
// top-level statement. A BEGIN ATOMIC ... END function body is one statement.
func findTxControl(sql string) (string, int) {
	toks := tokenize(sql)
	var stmt []token
	atomic := 0 // nesting depth inside a BEGIN ATOMIC body
	check := func() (string, int) {
		if len(stmt) == 0 {
			return "", 0
		}
		w := stmt[0].word
		switch {
		case txWords[w]:
			return w, stmt[0].line
		case (w == "START" || w == "PREPARE") && len(stmt) > 1 && stmt[1].word == "TRANSACTION":
			return w + " TRANSACTION", stmt[0].line
		}
		return "", 0
	}
	for i, t := range toks {
		switch {
		case t.word == ";" && atomic == 0:
			if w, l := check(); w != "" {
				return w, l
			}
			stmt = stmt[:0]
			continue
		case t.word == "ATOMIC" && len(stmt) > 0 && stmt[len(stmt)-1].word == "BEGIN" && atomic == 0 && i > 0:
			atomic = 1
		case atomic > 0 && t.word == "CASE":
			atomic++
		case atomic > 0 && t.word == "END":
			atomic--
		}
		stmt = append(stmt, t)
	}
	return check()
}

func isWordStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}
func isWordChar(c byte) bool { return isWordStart(c) || (c >= '0' && c <= '9') || c == '$' }

// tokenize returns the words and statement separators of sql with comments,
// strings, quoted identifiers and dollar-quoted bodies removed.
func tokenize(s string) []token {
	var out []token
	line := 1
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			depth := 1
			i += 2
			for i < len(s) && depth > 0 {
				switch {
				case s[i] == '/' && i+1 < len(s) && s[i+1] == '*':
					depth++
					i += 2
				case s[i] == '*' && i+1 < len(s) && s[i+1] == '/':
					depth--
					i += 2
				default:
					if s[i] == '\n' {
						line++
					}
					i++
				}
			}
		case c == '\'':
			i, line = skipQuoted(s, i, '\'', false, line)
		case c == '"':
			i, line = skipQuoted(s, i, '"', false, line)
		case c == '$':
			if end := dollarTagEnd(s, i); end > 0 {
				tag := s[i:end]
				if j := strings.Index(s[end:], tag); j >= 0 {
					line += strings.Count(s[end:end+j], "\n")
					i = end + j + len(tag)
					continue
				}
				i = len(s) // unterminated: the server will reject it
				continue
			}
			i++
		case c == ';':
			out = append(out, token{";", line})
			i++
		case isWordStart(c):
			j := i + 1
			for j < len(s) && isWordChar(s[j]) {
				j++
			}
			w := s[i:j]
			if (w == "E" || w == "e") && j < len(s) && s[j] == '\'' {
				i, line = skipQuoted(s, j, '\'', true, line)
				continue
			}
			out = append(out, token{strings.ToUpper(w), line})
			i = j
		default:
			i++
		}
	}
	return out
}

// skipQuoted skips a quoted run starting at s[i] (the opening quote) and
// returns the index after it. A doubled quote is an escaped quote; with
// backslash true (E'...') a backslash escapes the next byte.
func skipQuoted(s string, i int, q byte, backslash bool, line int) (int, int) {
	i++
	for i < len(s) {
		switch {
		case backslash && s[i] == '\\' && i+1 < len(s):
			if s[i+1] == '\n' {
				line++
			}
			i += 2
		case s[i] == q && i+1 < len(s) && s[i+1] == q:
			i += 2
		case s[i] == q:
			return i + 1, line
		default:
			if s[i] == '\n' {
				line++
			}
			i++
		}
	}
	return i, line
}

// dollarTagEnd returns the index just past an opening $tag$ at s[i], or 0
// when s[i] does not open a dollar quote ($1 is a parameter, not a quote).
func dollarTagEnd(s string, i int) int {
	j := i + 1
	if j < len(s) && s[j] == '$' {
		return j + 1
	}
	if j >= len(s) || !isWordStart(s[j]) || s[j] >= 0x80 {
		return 0
	}
	for j < len(s) && s[j] != '$' {
		if !isWordChar(s[j]) || s[j] == '$' {
			return 0
		}
		j++
	}
	if j < len(s) {
		return j + 1
	}
	return 0
}
