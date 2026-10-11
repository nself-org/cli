package report

// scrub_patterns_extra.go: pattern passes added by security CR r2 (lead ruling
// R5 d): licence key prefixes, webhook URL path tokens, SQL passwords and curl
// credentials. ("="-joined flag values that start with a dash are already
// caught by assignRe, whose identifier takes the leading dashes; see the
// "dash value equals-joined" row of TestScrubR5Corpus.)
//
// Purpose: catch secrets whose value is not a scrub input.
// Inputs: text.
// Outputs: text with placeholders; counts added to the Scrubber.
// Constraints: the regexps that go through rewriteValue keep exactly two
//         capture groups (prefix, value); already-redacted values, ${REF}
//         values and boolean words are left alone (redactedForm).

import "regexp"

// licenseKeyRe matches a licensed key by its product prefix. The prefix list
// mirrors validProductPrefixes in internal/license/keys.go (unexported there;
// TestLicensePrefixesMatchLicensePackage fails when the two drift apart), plus
// nself_lic_ which observability.Redact already knows.
var licenseKeyRe = regexp.MustCompile(`\bnself_(?:owner|plus|claw|clawde|chat|media|family|pro|ent|max|lic)_[A-Za-z0-9_\-]{6,}`)

// webhookRe matches the secret path token of a webhook URL; the service host
// and fixed path (group 1 or 2) are kept. A Telegram bot token has no slash.
var webhookRe = regexp.MustCompile(`(?i)(?:(hooks\.slack\.com/(?:services|workflows|triggers)/|discord(?:app)?\.com/api(?:/v[0-9]+)?/webhooks/)[A-Za-z0-9/_.:\-]{4,}|(api\.telegram\.org/(?:file/)?bot)[A-Za-z0-9:_\-]{4,})`)

var (
	// ALTER ROLE x PASSWORD 'v'; IDENTIFIED BY 'v'; WITH ENCRYPTED PASSWORD "v".
	sqlPasswordRe = regexp.MustCompile(`(?i)(\b(?:PASSWORD|IDENTIFIED[ \t]+BY)[ \t]+)('(?:[^'\r\n]|'')*'|"[^"\r\n]*")`)
	// curl -u user:value, curl --user user:value, also inside a JSON/YAML list.
	curlUserRe = regexp.MustCompile(`(\bcurl\b[^\r\n]*?[\s"',\[](?:-u|--user)(?:=|[ \t]+|["']?[ \t]*,[ \t]*["']?)["']?(?:\[REDACTED(?::[^\]\s]*)?\]|[^\s"':,])*:)([^\s"',\]]+)`)
)

// Codex pre-merge items 3, 6 and 7 (all through rewriteValue or extraTokenRe).
const (
	// jwtShape: a JWS or JWE compact token by its eyJ header and any base64url
	// payload, however short ({} is "e30"); observability.Redact wants 10+ chars
	// in the payload.
	jwtShape = `eyJ[A-Za-z0-9_\-]{5,}\.(?:[A-Za-z0-9_\-]*\.){1,3}[A-Za-z0-9_\-]{10,}`
)

var (
	// nginx: proxy_set_header Authorization "Basic x"; in every directive and
	// quote form (Authorization names carry no secret word, so nginxRe misses
	// them). A $variable pass-through is left alone by redactedForm.
	nginxAuthRe = regexp.MustCompile(`(?i)(\b(?:(?:proxy|grpc)_set_header|add_header|more_set_headers|more_set_input_headers|(?:fastcgi|uwsgi|scgi)_param)[ \t]+["']?(?:HTTP_)?(?:PROXY[-_])?AUTHORIZATION["']?:?[ \t]+)("[^"\r\n]*"|'[^'\r\n]*'|[^;\r\n]+)`)
	// Signed-URL signatures: Azure SAS sig=, plus the presigned-URL spellings.
	// Only the signature is secret; sv, sp, sr, se stay. A leading ; covers
	// &amp; and %26 covers an encoded outer URL.
	sasSigRe = regexp.MustCompile(`(?i)((?:[?&;]|%26)(?:sig|signature|x-amz-signature|x-goog-signature)=)([^&#\s"']+)`)
)

// scrubWebhooks redacts licence keys and webhook URL path tokens.
func (s *Scrubber) scrubWebhooks(text string) string {
	if n := len(licenseKeyRe.FindAllStringIndex(text, -1)); n > 0 {
		text = licenseKeyRe.ReplaceAllString(text, placeholder)
		s.counts[ClassPattern] += n
	}
	if n := len(webhookRe.FindAllStringIndex(text, -1)); n > 0 {
		text = webhookRe.ReplaceAllString(text, "${1}${2}"+placeholder)
		s.counts[ClassPattern] += n
	}
	return text
}
