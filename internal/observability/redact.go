package observability

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
)

// PII redaction patterns.
//
// Expanded coverage includes IPs, JWTs, SSNs, license keys, additional
// Stripe/Cloudflare token shapes, AWS keys, and generic UUIDs. Patterns are
// documented in .claude/docs/operations/telemetry-privacy.md § "Redaction patterns".
var (
	// Communication identifiers.
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRegex = regexp.MustCompile(`\b(\+?1?[-.\s]?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4})\b`)

	// Financial identifiers.
	ccRegex   = regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`)
	ssnRegex  = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	ibanRegex = regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{10,30}\b`)

	// Network identifiers (IPv4 + IPv6 + MAC). Loopback (127.0.0.1, ::1) is
	// intentionally NOT redacted — it appears widely in dev docs and never
	// identifies a user.
	ipv4Regex = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\.){3}(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\b`)
	// IPv6 covers full + compressed (::) + mixed-IPv4 forms. We accept any
	// run of hex segments separated by `:` that includes either ≥2 colons
	// or one `::` compression marker. The regex below uses a single
	// alternation: full form (7 colons) OR compressed (>=1 :: occurrence).
	ipv6Regex = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4}\b|\b(?:[0-9a-fA-F]{1,4}:)*[0-9a-fA-F]{0,4}::(?:[0-9a-fA-F]{1,4}:?)*[0-9a-fA-F]{1,4}\b`)
	macRegex  = regexp.MustCompile(`\b(?:[0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}\b`)

	// Auth tokens. JWT regex is conservative — three base64url segments
	// joined by `.` with min length per segment.
	jwtRegex = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`)

	// API key prefixes that should be redacted.
	apiKeyPrefixes = []string{
		"nself_pro_",
		"nself_lic_",
		"sk-",
		"sk_live_",
		"sk_test_",
		"pk_live_",
		"pk_test_",
		"rk_live_",
		"rk_test_",
		"whsec_",
		"ghp_",
		"gho_",
		"ghs_",
		"github_pat_",
		"AKIA", // AWS access key prefix
		"AIza", // Google API key prefix
		"Bearer ",
	}

	redactionPlaceholder = "[REDACTED]"

	// Loopback addresses that must survive redaction. These never identify
	// a user and appear in nearly every doctor/diag log.
	// Keep the tokens free of digits and terminate them with a non-hex word.
	// A numeric suffix beside :port:port can itself match the IPv6 regex and
	// destroy the token before restoration (D-0559).
	loopbackV4Placeholder = "\x00LOOPBACKVFOUR\x00"
	loopbackV6Placeholder = "\x00LOOPBACKVSIX\x00"
	loopbackV4            = "127.0.0.1"
	loopbackV6            = "::1"

	// Global counter for redaction failures.
	redactionFailures atomic.Int64
)

// RedactionFailures returns the current count of redaction failures.
func RedactionFailures() int64 {
	return redactionFailures.Load()
}

// Redact applies PII pattern matching and replaces matches with [REDACTED].
//
// Coverage (S12.T09): emails, phones, SSNs, IBANs, credit cards, IPv4/IPv6,
// MAC addresses, JWTs, and API/license/Stripe/AWS/GitHub keys via the
// apiKeyPrefixes table. Loopback IPs (127.0.0.1, ::1) are preserved.
//
// COPPA + GDPR Article 9 / "special category" data (race, religion, health,
// minor status, sexual orientation, political opinion, biometric identifiers)
// is NEVER allowed into telemetry payloads by construction — the doctor check
// OBS-REDACT-01 enforces that callers do not send such fields at all rather
// than relying on textual redaction.
func Redact(s string) string {
	result := s

	// Stash loopback addresses so the IP regexes don't clobber them.
	result = strings.ReplaceAll(result, loopbackV4, loopbackV4Placeholder)
	result = strings.ReplaceAll(result, loopbackV6, loopbackV6Placeholder)

	// JWT first (matches before any prefix logic would).
	result = jwtRegex.ReplaceAllString(result, redactionPlaceholder)

	result = emailRegex.ReplaceAllString(result, redactionPlaceholder)
	result = phoneRegex.ReplaceAllString(result, redactionPlaceholder)
	result = ssnRegex.ReplaceAllString(result, redactionPlaceholder)
	result = ibanRegex.ReplaceAllString(result, redactionPlaceholder)
	result = ccRegex.ReplaceAllString(result, redactionPlaceholder)
	result = ipv6Regex.ReplaceAllString(result, redactionPlaceholder)
	result = ipv4Regex.ReplaceAllString(result, redactionPlaceholder)
	result = macRegex.ReplaceAllString(result, redactionPlaceholder)

	for _, prefix := range apiKeyPrefixes {
		for {
			idx := strings.Index(result, prefix)
			if idx < 0 {
				break
			}
			// Redact from prefix to next whitespace or end of string.
			end := idx + len(prefix)
			for end < len(result) && result[end] != ' ' && result[end] != '"' && result[end] != '\'' && result[end] != ',' && result[end] != '\n' {
				end++
			}
			result = result[:idx] + redactionPlaceholder + result[end:]
		}
	}

	// Restore loopback addresses.
	result = strings.ReplaceAll(result, loopbackV4Placeholder, loopbackV4)
	result = strings.ReplaceAll(result, loopbackV6Placeholder, loopbackV6)

	return result
}

// RedactHandler wraps a slog.Handler and redacts PII from string attribute values.
type RedactHandler struct {
	inner slog.Handler
}

// NewRedactHandler creates a handler that redacts PII patterns from log attributes.
func NewRedactHandler(inner slog.Handler) *RedactHandler {
	return &RedactHandler{inner: inner}
}

func (h *RedactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactHandler) Handle(ctx context.Context, r slog.Record) error {
	// Redact the message itself.
	r.Message = Redact(r.Message)

	// Redact all string attributes.
	var redacted []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		redacted = append(redacted, redactAttr(a))
		return true
	})

	// Create a new record with redacted attrs.
	newR := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	newR.AddAttrs(redacted...)

	return h.inner.Handle(ctx, newR)
}

func (h *RedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	ra := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		ra[i] = redactAttr(a)
	}
	return &RedactHandler{inner: h.inner.WithAttrs(ra)}
}

func (h *RedactHandler) WithGroup(name string) slog.Handler {
	return &RedactHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	// Skip known sensitive keys entirely.
	key := strings.ToLower(a.Key)
	if key == "password" || key == "token" || key == "secret" || key == "authorization" {
		return slog.String(a.Key, redactionPlaceholder)
	}

	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Redact(a.Value.String()))
	case slog.KindGroup:
		attrs := a.Value.Group()
		ra := make([]slog.Attr, len(attrs))
		for i, ga := range attrs {
			ra[i] = redactAttr(ga)
		}
		return slog.Group(a.Key, attrsToAny(ra)...)
	}
	return a
}

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, len(attrs))
	for i, a := range attrs {
		out[i] = a
	}
	return out
}
