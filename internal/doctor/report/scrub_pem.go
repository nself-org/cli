package report

// scrub_pem.go: the PEM pass of the scrubber.
//
// Purpose: remove key and certificate blocks, whole, truncated at either end,
// prefixed per line by a log shipper, or JSON-wrapped with \n escapes (S1).
// Inputs: text.
// Outputs: text with [REDACTED:PEM]; counts added to the Scrubber.
// Constraints: a BEGIN with no END redacts to the end of the text; an END with
// no BEGIN redacts the base64-looking lines before it (at most pemMaxBack
// lines, at most five words per line) and the marker itself.

import (
	"regexp"
	"strings"
)

const (
	pemLabel   = `[A-Z0-9 ]+`
	pemMaxBack = 120
	pemMarker  = "[REDACTED:PEM]"
)

var (
	// A whole PEM block (any label: keys, certificates, PGP, age).
	pemBlockRe = regexp.MustCompile(`(?s)-----BEGIN ` + pemLabel + `-----.*?-----END ` + pemLabel + `-----`)
	// A BEGIN with no END (a log tail cut the block): redact to the end.
	pemOpenRe = regexp.MustCompile(`(?s)-----BEGIN ` + pemLabel + `-----.*$`)
	// An END with its body on the same line but no BEGIN (the tail started mid-block).
	pemTailRe = regexp.MustCompile(`(?s)(?:[A-Za-z0-9+/=]{16,}(?:\r?\n|\\n)?)+-----END ` + pemLabel + `-----`)
	pemEndRe  = regexp.MustCompile(`-----END ` + pemLabel + `-----`)
	b64Run16  = regexp.MustCompile(`[A-Za-z0-9+/=]{16,}`)
	b64Run8   = regexp.MustCompile(`[A-Za-z0-9+/=]{8,}`)
)

func (s *Scrubber) scrubPEM(text string) string {
	for _, re := range []*regexp.Regexp{pemBlockRe, pemOpenRe, pemTailRe} {
		n := len(re.FindAllStringIndex(text, -1))
		if n > 0 {
			repl := pemMarker
			if re == pemTailRe {
				repl += tailMark // an END whose body was cut: the walk goes on above it
			}
			text = re.ReplaceAllString(text, repl)
			s.counts[ClassPEM] += n
		}
	}
	return s.scrubOrphanEnds(text)
}

// tailMark tags the marker a pemTailRe hit left, so scrubOrphanEnds can walk
// up from it (body lines above a line the literal pass hit survive otherwise).
// It is removed before scrubOrphanEnds returns.
const tailMark = "\x00"

// scrubOrphanEnds handles an END marker whose body sits on earlier lines that
// carry a log prefix or a JSON wrapper.
func (s *Scrubber) scrubOrphanEnds(text string) string {
	if !strings.Contains(text, "-----END ") && !strings.Contains(text, tailMark) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		isEnd := pemEndRe.MatchString(ln)
		if !isEnd && !strings.Contains(ln, tailMark) {
			continue
		}
		if isEnd {
			lines[i] = pemEndRe.ReplaceAllString(ln, pemMarker)
			s.counts[ClassPEM]++
		}
		for j := i - 1; j >= 0 && i-j <= pemMaxBack; j-- {
			red, ok := redactBodyLine(lines[j], j == i-1)
			if !ok {
				break
			}
			if red != lines[j] {
				lines[j] = red
				s.counts[ClassPEM]++
			}
		}
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), tailMark, "")
}

// redactBodyLine replaces the base64 body of a line that looks like a PEM body
// line, or reports false. A line of at most five words (and no placeholder) may
// hold base64 runs anywhere; a longer line, such as one with a log prefix
// (Postgres: "2026-10-11 12:00:00.123 UTC [42] LOG:  <body>"), is judged by its
// LAST word (S3), which must be base64 characters and placeholders only: a
// body line the literal pass hit in the middle still counts (R6 d). A last word
// that is only placeholders was a body line removed earlier: the walk goes on.
// The line next to the END marker may be short.
func redactBodyLine(line string, adjacent bool) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", false
	}
	re, minLen := b64Run16, 16
	if adjacent {
		re, minLen = b64Run8, 8
	}
	last := strings.Trim(fields[len(fields)-1], "\"',;\\")
	if onlyPlaceholders(last) {
		return line, true
	}
	if len(fields) <= 5 && !strings.Contains(line, "[REDACTED") {
		if !re.MatchString(line) {
			return "", false
		}
		return re.ReplaceAllString(line, pemMarker), true
	}
	core := placeholderRe.ReplaceAllString(last, "")
	if core == "" || strings.Trim(core, b64Chars) != "" || (core == last && len(core) < minLen) {
		return "", false
	}
	at := strings.LastIndex(line, last)
	return line[:at] + pemMarker + line[at+len(last):], true
}

const b64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
