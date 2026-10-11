package report

// scrub.go: the support-bundle scrubber (EPIC D21, security core).
//
// Purpose: remove every secret from text before it enters the bundle.
// Inputs: SecretValue literals (env cascade + compose environment) and the text
//         of each file.
// Outputs: scrubbed text and per-class redaction counts.
// Constraints: pass order is fixed: (1) literal values, longest first, one scan
//         so a placeholder is never rescanned; (2) PEM blocks; (3) URL
//         userinfo; (4) observability.Redact; (5) secret-named assignments
//         the earlier passes missed (scrub_patterns.go). Every file goes
//         through Scrub, including versions.json and config-keys.txt.

import (
	"strings"

	"github.com/nself-org/cli/internal/observability"
)

// Redaction classes counted in manifest.json.
const (
	ClassValue      = "value"
	ClassPEM        = "pem"
	ClassUserinfo   = "url_userinfo"
	ClassPattern    = "pattern"
	ClassAssignment = "assignment"
)

// Scrubber holds the literal inputs and the running counts.
type Scrubber struct {
	cands  []candidate
	counts map[string]int
}

// NewScrubber builds a scrubber over the given secret values.
func NewScrubber(values []SecretValue) *Scrubber {
	return &Scrubber{cands: candidates(values), counts: map[string]int{}}
}

// Counts returns a copy of the per-class redaction counts.
func (s *Scrubber) Counts() map[string]int {
	out := make(map[string]int, len(s.counts))
	for k, v := range s.counts {
		out[k] = v
	}
	return out
}

// Total is the sum of all classes.
func (s *Scrubber) Total() int {
	n := 0
	for _, v := range s.counts {
		n += v
	}
	return n
}

// Scrub runs all passes over text.
func (s *Scrubber) Scrub(text string) string {
	text = s.scrubValues(text)
	text = s.scrubPEM(text)
	text = s.scrubUserinfo(text)
	text = s.scrubRedact(text)
	text = s.scrubExtraTokens(text)
	return s.scrubAssignments(text)
}

// scrubValues replaces every literal spelling in one left-to-right scan; at the
// same position the longest candidate wins (cands are sorted longest first).
func (s *Scrubber) scrubValues(text string) string {
	if len(s.cands) == 0 || text == "" {
		return text
	}
	next := make([]int, len(s.cands)) // absolute index of the next hit, -1 none
	for i, c := range s.cands {
		next[i] = strings.Index(text, c.text)
	}
	var sb strings.Builder
	pos := 0
	for {
		best := -1
		for i := range s.cands {
			if next[i] >= 0 && next[i] < pos { // stale: overlapped by a replacement
				if j := strings.Index(text[pos:], s.cands[i].text); j >= 0 {
					next[i] = pos + j
				} else {
					next[i] = -1
				}
			}
			if next[i] >= 0 && (best < 0 || next[i] < next[best]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		sb.WriteString(text[pos:next[best]])
		sb.WriteString("[REDACTED:" + s.cands[best].key + "]")
		pos = next[best] + len(s.cands[best].text)
		s.counts[ClassValue]++
	}
	sb.WriteString(text[pos:])
	return sb.String()
}

// scrubRedact applies observability.Redact and counts its placeholders.
func (s *Scrubber) scrubRedact(text string) string {
	out := observability.Redact(text)
	if n := strings.Count(out, "[REDACTED]") - strings.Count(text, "[REDACTED]"); n > 0 {
		s.counts[ClassPattern] += n
	}
	return out
}
