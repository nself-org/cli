package reconcile

// overlay_envline.go — the env-file line rewrite behind `config set`
// (P7-LIVE-05, D-0404): EnvFileWithLine and the value encoder it uses. Split
// from overlay.go for the file-size cap.

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/joho/godotenv"
)

// envStatement is one dotenv statement: the physical lines it spans (more than
// one for a quoted value that holds line breaks) and its key ("" for a blank
// line, a comment or a line without "=").
type envStatement struct {
	lines []string
	key   string
}

// EnvFileWithLine returns old with KEY=VALUE set: every statement whose key is
// key is replaced (a multi-line quoted value goes with all its lines), else the
// line is appended; comments, blank lines and other statements are kept, lines
// end in "\n". exists false means no file yet (the result is the line alone).
// found reports a replacement. This is the one rewrite `config set` writes and
// the planner previews (P7-LIVE-05).
//
// Errors never hold the value: a value the dotenv parser cannot read back
// unchanged (see quoteEnvLineValue), or a file the rewrite would corrupt. The
// result is parsed again and compared with the old file (checkEnvRewrite); a
// rewrite that would lose or change another key is an error, never a silent edit.
func EnvFileWithLine(old []byte, exists bool, key, value string) (out []byte, found bool, err error) {
	enc, err := quoteEnvLineValue(key, value)
	if err != nil {
		return nil, false, err
	}
	newLine := key + "=" + enc
	if !exists {
		return []byte(newLine + "\n"), false, nil
	}
	stmts, err := splitEnvStatements(old)
	if err != nil {
		return nil, false, err
	}
	if _, perr := godotenv.UnmarshalBytes(old); perr == nil {
		if err := proveStatements(stmts, key); err != nil {
			return nil, false, err
		}
	}
	out, found = rewriteEnvStatements(stmts, key, newLine)
	if err := checkEnvRewrite(old, out, stmts, key, value); err != nil {
		return nil, false, err
	}
	return out, found, nil
}

// proveStatements checks the statement boundaries splitEnvStatements chose:
// each statement, parsed alone, must load and yield exactly its own key (no
// key for a blank or comment statement). If every statement parses alone into
// its own key, godotenv's sequential parse of the whole file sees the same
// boundaries. The error names key only.
func proveStatements(stmts []envStatement, key string) error {
	for _, st := range stmts {
		m, err := godotenv.UnmarshalBytes([]byte(strings.Join(st.lines, "\n") + "\n"))
		_, has := m[st.key]
		if err != nil || (st.key == "" && len(m) != 0) || (st.key != "" && (len(m) != 1 || !has)) {
			return fmt.Errorf("cannot set %s without breaking the env file; edit it by hand", key)
		}
	}
	return nil
}

// rewriteEnvStatements replaces every statement whose key is key with newLine
// (appending newLine when none matches) and joins the lines.
func rewriteEnvStatements(stmts []envStatement, key, newLine string) (out []byte, found bool) {
	var lines []string
	for _, st := range stmts {
		if st.key == key {
			lines = append(lines, newLine)
			found = true
			continue
		}
		lines = append(lines, st.lines...)
	}
	if !found {
		lines = append(lines, newLine)
	}
	return []byte(strings.Join(lines, "\n") + "\n"), found
}

// splitEnvStatements groups the lines of an env file into statements, tracking
// an open quote across lines the way godotenv does (a quote ends the value at
// the first quote character not preceded by a backslash).
func splitEnvStatements(old []byte) ([]envStatement, error) {
	var stmts []envStatement
	scanner := bufio.NewScanner(bytes.NewReader(old))
	open := byte(0)
	for scanner.Scan() {
		line := scanner.Text()
		if open != 0 {
			last := &stmts[len(stmts)-1]
			last.lines = append(last.lines, line)
			if closesDotenvQuote(line, open) {
				open = 0
			}
			continue
		}
		st := envStatement{lines: []string{line}}
		trimmed := strings.TrimSpace(line)
		if eq := strings.IndexAny(trimmed, "=:"); eq > 0 && !strings.HasPrefix(trimmed, "#") {
			st.key = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(trimmed[:eq]), "export "))
			if v := strings.TrimSpace(trimmed[eq+1:]); v != "" && (v[0] == '"' || v[0] == '\'') && !closesDotenvQuote(v[1:], v[0]) {
				open = v[0]
			}
		}
		stmts = append(stmts, st)
	}
	return stmts, scanner.Err()
}

// closesDotenvQuote reports whether s holds a q that is not preceded by a
// backslash, godotenv v1.5.1's rule for the end of a quoted value.
func closesDotenvQuote(s string, q byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == q && (i == 0 || s[i-1] != '\\') {
			return true
		}
	}
	return false
}

// checkEnvRewrite is the grammar-independent check of a rewrite (see
// EnvFileWithLine). It fails, with key names only, when out does not load or
// does not hold key=value. When old loads: the key set of out must be old's
// keys plus key; if key was new, every old value must be unchanged (the line is
// appended last, and godotenv expands only from earlier statements); if key was
// there, a probe (the same rewrite with key's old value) must reproduce old
// exactly, which proves no other statement was touched, so out may differ from
// old only where a value expands key.
func checkEnvRewrite(old, out []byte, stmts []envStatement, key, value string) error {
	broken := fmt.Errorf("cannot set %s without breaking the env file; edit it by hand", key)
	nm, err := godotenv.UnmarshalBytes(out)
	if err != nil || nm[key] != value {
		return broken
	}
	om, err := godotenv.UnmarshalBytes(old)
	if err != nil {
		return nil // the old file did not load either; nothing to preserve
	}
	_, had := om[key]
	want := len(om)
	if !had {
		want++
	}
	if len(nm) != want {
		return broken
	}
	for k := range om {
		if _, ok := nm[k]; !ok {
			return broken
		}
	}
	if !had {
		for k, v := range om {
			if nm[k] != v {
				return broken
			}
		}
		return nil
	}
	enc, err := quoteEnvLineValue(key, om[key])
	if err != nil {
		return broken
	}
	probe, _ := rewriteEnvStatements(stmts, key, key+"="+enc)
	pm, err := godotenv.UnmarshalBytes(probe)
	if err != nil || len(pm) != len(om) {
		return broken
	}
	for k, v := range om {
		if pm[k] != v {
			return broken
		}
	}
	return nil
}

// quoteEnvLineValue returns the text written after "KEY=" so godotenv v1.5.1
// (the loader's parser) reads value back unchanged.
//
// Inputs: the key (for the error text) and the value. Outputs: the encoded
// value, or an error that never holds the value. Candidates in order, the
// first that round-trips through godotenv.UnmarshalBytes wins:
//  1. raw, for a non-empty value with no space, tab, #, quote, backslash, $,
//     backtick, newline or CR;
//  2. double-quoted with \ -> \\, " -> \", $ -> \$, newline -> \n, CR -> \r
//     (godotenv expands \n, \\X and $VAR inside double quotes, so a literal
//     backslash or $ must be escaped, or `a\nb` loads as a newline and `$HOME`
//     as the variable);
//  3. raw unquoted, for a value that holds only backslashes as special
//     characters: godotenv ends a quoted value at the first quote not preceded
//     by a backslash, so a value ending in a backslash cannot be terminated
//     inside quotes, and an unquoted value gets no escape processing;
//  4. single-quoted (no processing), for a value with no ' or line break that
//     (2) cannot hold, such as one ending in a double quote (godotenv trims
//     every trailing quote character off the quoted text).
//
// No candidate round-tripping is an error (a value such as `a b\`).
func quoteEnvLineValue(key, v string) (string, error) {
	var cands []string
	switch {
	case v == "":
		cands = append(cands, `""`)
	case !strings.ContainsAny(v, " \t#\"'\\$`\n\r"):
		cands = append(cands, v)
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`, "\r", `\r`)
	cands = append(cands, `"`+esc.Replace(v)+`"`)
	if v != "" && !strings.ContainsAny(v, " \t#\"'$`\n\r") {
		cands = append(cands, v)
	}
	if !strings.ContainsAny(v, "'\n\r") {
		cands = append(cands, "'"+v+"'")
	}
	for _, c := range cands {
		if m, err := godotenv.UnmarshalBytes([]byte(key + "=" + c)); err == nil && m[key] == v {
			return c, nil
		}
	}
	if strings.HasSuffix(v, `\`) {
		return "", fmt.Errorf("value for %s ends in a backslash and holds characters that need quoting; a dotenv file cannot store it", key)
	}
	return "", fmt.Errorf("value for %s cannot be stored in a dotenv file", key)
}
