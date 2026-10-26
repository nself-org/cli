package destinations

import (
	"regexp"
	"strings"
)

// inlineOptionRe is the `,key=` shape of an inline rclone option ("myr,pass=x").
var inlineOptionRe = regexp.MustCompile(`,[A-Za-z0-9_]+=`)

// HasInlineOption reports whether a relative uri carries an inline rclone
// option with no ":" after it ("myr,pass=ab/cd"): a malformed remote, not a
// directory. "./" and "../" always mean a path, so they never match.
func HasInlineOption(uri string) bool {
	if strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../") {
		return false
	}
	return inlineOptionRe.MatchString(uri)
}
