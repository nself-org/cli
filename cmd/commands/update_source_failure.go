package commands

// Purpose: how `nself update --plan|--yes` codes a source that could not answer
// (D-0560): the HTTP status is read from the typed releaseStatusError, not from
// the words of an error, so a rate limit and a plain outage have their own codes.
// Inputs: the error a source returned.
// Outputs: an E456 (unavailable) or E457 (rate limited) coded error.
// Constraints: only the plugin registry fetch still words its status as text, so
// that is the one place the text fallback applies.

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/nself-org/cli/internal/errs"
)

// httpStatusRE pulls the status out of an untyped error's text: only the plugin
// registry fetch still words its status that way.
var httpStatusRE = regexp.MustCompile(`status (\d{3})`)

// httpStatusOf is the HTTP status a source answered with, 0 when it did not
// answer with one. The release lookup carries it typed; other errors fall back
// to their text.
func httpStatusOf(err error) int {
	var se *releaseStatusError
	if errors.As(err, &se) {
		return se.Status
	}
	if m := httpStatusRE.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// sourceFailure codes a source that could not answer: E457 for a rate limit
// (HTTP 429), E456 for anything else.
func sourceFailure(source string, err error) error {
	if httpStatusOf(err) == http.StatusTooManyRequests {
		return errs.Wrap("E457", "update plan: "+source+" is rate limited", err).
			WithWhy("The source rate-limited this request (HTTP 429).").WithFix("Retry in a few minutes. Nothing was changed.")
	}
	return errs.Wrap("E456", "update plan: "+source+" is unavailable", err).
		WithWhy("The source did not answer: " + err.Error()).WithFix("Retry in a few minutes. Nothing was changed.")
}
