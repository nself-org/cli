package main

import "strings"

// quoteArg returns a single argument as the release scripts printed it:
// bare when safe, single-quoted when it holds a space, quote or shell metachar.
func quoteArg(a string) string {
	if a == "" {
		return "''"
	}
	if !strings.ContainsAny(a, " \t\n'\"\\$`!&|;<>()[]{}*?#~^") {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

// joinArgs joins args with spaces, quoting any that need it.
func joinArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = quoteArg(a)
	}
	return strings.Join(q, " ")
}
