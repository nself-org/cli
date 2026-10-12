package config

// envfile.go — the one reader for dotenv files, with parser errors that never
// carry the file's text.
//
// Purpose: godotenv quotes the offending part of the file in its parse errors
//          (the rest of the file for a bad key, the line for an unterminated
//          quote). A secret on that line then lands in a terminal, a log or an
//          MCP tool result. ReadEnvFile and ParseEnvBytes replace every direct
//          godotenv read and return an *EnvSyntaxError (file + line only).
// Inputs:  a file path, or a name plus the bytes (a pending overlay, a
//          resolved snapshot held in memory).
// Outputs: the parsed map, or an error whose text is safe to print.
// Constraints: file-system errors from ReadEnvFile are returned as-is: they
//          hold a path, never file text, and callers rely on os.IsNotExist and
//          errors.Is(err, fs.ErrNotExist). The line number is derived from
//          godotenv v1.5.1's message shapes and is best effort: when a shape is
//          not recognised the error simply carries no line. envfile_lint_test.go
//          fails the build if a new godotenv read appears outside this file.
// SPORT:   cli/internal/config — D-0444, D-0558.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// EnvSyntaxError reports that godotenv could not parse a dotenv file. It holds
// the file name and line only, never parser text (see the file comment).
// Line 0 means the line could not be worked out.
type EnvSyntaxError struct {
	File string
	Line int
}

// Error renders the message without any of the file's content.
func (e *EnvSyntaxError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("invalid dotenv syntax in %s (line %d)", e.File, e.Line)
	}
	return "invalid dotenv syntax in " + e.File
}

// ReadEnvFile parses the dotenv file at path. A file-system error is returned
// unchanged; a parse error is an *EnvSyntaxError naming path.
func ReadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseEnvBytes(path, data)
}

// ParseEnvBytes parses dotenv bytes. name is only used to label an
// *EnvSyntaxError (a path, or a phrase such as "the resolved deploy env file").
func ParseEnvBytes(name string, data []byte) (map[string]string, error) {
	m, err := godotenv.UnmarshalBytes(data)
	if err != nil {
		return nil, &EnvSyntaxError{File: name, Line: envSyntaxLine(data, err)}
	}
	return m, nil
}

// EnvReadError turns the error of a ReadEnvFile call into the error a command
// returns. An *EnvSyntaxError already names the file, so it passes through;
// anything else gets the usual "reading <path>: ..." wrapper.
func EnvReadError(path string, err error) error {
	var se *EnvSyntaxError
	if errors.As(err, &se) {
		return err
	}
	return fmt.Errorf("reading %s: %w", path, err)
}

const (
	nameErrMarker   = " in variable name near "
	unterminatedMsg = "unterminated quoted value "
)

// envSyntaxLine finds the 1-based line godotenv v1.5.1 failed on, from the
// shape of its error, and returns 0 when it cannot. The text of err is read
// here and never returned or stored.
//
//	(a) `unexpected character 'c' in variable name near "<rest of file>"`: the
//	    quoted rest is a suffix of the CRLF-normalised data starting at the
//	    statement, so the line is where that suffix begins.
//	(b) `unterminated quoted value <quote>...`: no unescaped matching quote
//	    follows the opening one, so the last unescaped quote of that kind in
//	    the file is the opening quote.
func envSyntaxLine(data []byte, err error) int {
	msg := err.Error()
	d := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	off := -1
	switch {
	case strings.Contains(msg, nameErrMarker):
		rest := msg[strings.Index(msg, nameErrMarker)+len(nameErrMarker):]
		if frag, uerr := strconv.Unquote(rest); uerr == nil && bytes.HasSuffix(d, []byte(frag)) {
			off = len(d) - len(frag)
		}
	case strings.HasPrefix(msg, unterminatedMsg) && len(msg) > len(unterminatedMsg):
		q := msg[len(unterminatedMsg)]
		for i := len(d) - 1; i >= 0; i-- {
			if d[i] == q && (i == 0 || d[i-1] != '\\') {
				off = i
				break
			}
		}
	}
	if off < 0 {
		return 0
	}
	return 1 + bytes.Count(d[:off], []byte("\n"))
}
