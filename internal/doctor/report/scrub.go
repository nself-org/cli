package report

// scrub.go: the support-bundle scrubber (EPIC D21, security core).
//
// Purpose: remove every secret from text before it enters the bundle.
// Inputs: SecretValue literals (env cascade + compose environment) and the text
//         of each file.
// Outputs: scrubbed text and per-class redaction counts.
// Constraints: pass order is fixed: (1) PEM blocks (scrub_pem.go), first so a
//         literal that spells part of a marker (KEY_FORMAT=OPENSSH) can never
//         break BEGIN/END recognition (Codex pre-merge item 1); (2) literal
//         values, overlapping hits merged into one span so a placeholder is
//         never rescanned and no tail survives; (3) URL userinfo; (4)
//         observability.Redact; (5) secret-named assignments and YAML block
//         scalars the earlier passes missed (scrub_patterns.go,
//         scrub_block.go). Every file goes through Scrub, including
//         versions.json and config-keys.txt.

import (
	"sort"
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
	text = s.scrubPEM(text)
	text = s.scrubValues(text)
	text = s.scrubUserinfo(text)
	text = s.scrubRedact(text)
	text = s.scrubExtraTokens(text)
	text = s.scrubWebhooks(text)
	return s.scrubAssignments(text)
}

// span is one literal hit [start, end) and the key of the value it belongs to.
type span struct {
	start, end int
	key        string
}

// scrubValues replaces every literal spelling. All hits of all candidates are
// collected first and overlapping or touching hits are merged into one span, so
// no tail of any candidate can survive an overlap (S3). One placeholder (named
// after the first span's key) replaces each merged span.
func (s *Scrubber) scrubValues(text string) string {
	if len(s.cands) == 0 || text == "" {
		return text
	}
	var spans []span
	for _, c := range s.cands {
		for from := 0; from < len(text); {
			j := strings.Index(text[from:], c.text)
			if j < 0 {
				break
			}
			spans = append(spans, span{from + j, from + j + len(c.text), c.key})
			from += j + 1
		}
	}
	if len(spans) == 0 {
		return text
	}
	sort.SliceStable(spans, func(a, b int) bool {
		if spans[a].start != spans[b].start {
			return spans[a].start < spans[b].start
		}
		return spans[a].end > spans[b].end
	})
	var sb strings.Builder
	pos := 0
	cur := spans[0]
	flush := func() {
		sb.WriteString(text[pos:cur.start])
		sb.WriteString("[REDACTED:" + cur.key + "]")
		pos = cur.end
		s.counts[ClassValue]++
	}
	for _, sp := range spans[1:] {
		if sp.start <= cur.end { // overlaps or touches: extend
			if sp.end > cur.end {
				cur.end = sp.end
			}
			continue
		}
		flush()
		cur = sp
	}
	flush()
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
