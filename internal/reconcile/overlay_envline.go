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
// result is parsed again; it must load, hold key=value, and leave every other
// key of a loadable old file unchanged (keys whose statement uses $ are
// skipped, because they may expand the key being set).
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
	out = []byte(strings.Join(lines, "\n") + "\n")
	if err := checkEnvRewrite(old, out, stmts, key, value); err != nil {
		return nil, false, err
	}
	return out, found, nil
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
		if eq := strings.IndexByte(trimmed, '='); eq > 0 && !strings.HasPrefix(trimmed, "#") {
			st.key = strings.TrimSpace(trimmed[:eq])
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

// checkEnvRewrite parses out and fails, without the value in the text, when
// out does not load, does not hold key=value, or changed another key that old
// held (see EnvFileWithLine).
func checkEnvRewrite(old, out []byte, stmts []envStatement, key, value string) error {
	m, err := godotenv.UnmarshalBytes(out)
	if err != nil || m[key] != value {
		return fmt.Errorf("cannot set %s without breaking the env file; edit it by hand", key)
	}
	om, err := godotenv.UnmarshalBytes(old)
	if err != nil {
		return nil // the old file did not load either; nothing to preserve
	}
	for _, st := range stmts {
		prev, held := om[st.key]
		if st.key == "" || st.key == key || !held || strings.Contains(strings.Join(st.lines, "\n"), "$") {
			continue
		}
		if m[st.key] != prev {
			return fmt.Errorf("cannot set %s without changing %s; edit the env file by hand", key, st.key)
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
