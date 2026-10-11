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
			text = re.ReplaceAllString(text, pemMarker)
			s.counts[ClassPEM] += n
		}
	}
	return s.scrubOrphanEnds(text)
}

// scrubOrphanEnds handles an END marker whose body sits on earlier lines that
// carry a log prefix or a JSON wrapper.
func (s *Scrubber) scrubOrphanEnds(text string) string {
	if !strings.Contains(text, "-----END ") {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		if !pemEndRe.MatchString(ln) {
			continue
		}
		lines[i] = pemEndRe.ReplaceAllString(ln, pemMarker)
		s.counts[ClassPEM]++
		for j := i - 1; j >= 0 && i-j <= pemMaxBack; j-- {
			red, ok := redactBodyLine(lines[j], j == i-1)
			if !ok {
				break
			}
			lines[j] = red
			s.counts[ClassPEM]++
		}
	}
	return strings.Join(lines, "\n")
}

// redactBodyLine replaces the base64 runs of a line that looks like a PEM body
// line, or reports false. The line next to the END marker may be short.
func redactBodyLine(line string, adjacent bool) (string, bool) {
	if strings.Contains(line, "[REDACTED") || len(strings.Fields(line)) > 5 {
		return "", false
	}
	re := b64Run16
	if adjacent {
		re = b64Run8
	}
	if !re.MatchString(line) {
		return "", false
	}
	return re.ReplaceAllString(line, pemMarker), true
}
