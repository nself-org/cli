package reconcile

import (
	"strings"
	"testing"
)

func TestUnifiedBlockSecretContinuation(t *testing.T) {
	for _, header := range []string{"|-", ">-", "&cmd |2- # literal"} {
		for _, assignment := range []string{"--requirepass ", "--api-key=", "API_TOKEN="} {
			t.Run(header+assignment, func(t *testing.T) {
				before := "command: " + header + "\n  redis-server " + assignment + "first-secret\n  second-secret\n\n  third-secret # literal\n  --maxclients 50\n  echo keep-visible\nimage: redis\n"
				after := strings.Replace(before, "50", "60", 1)
				runShapes(t, []shapeCase{{name: "continuations", before: before, after: after,
					secrets: []string{"first-secret", "second-secret", "third-secret"},
					want:    []string{"--maxclients 50", "--maxclients 60", "echo keep-visible", "image: redis"}}})
			})
		}
	}
}

func TestUnifiedBlockContinuationStopsAtFlagAndDedent(t *testing.T) {
	runShapes(t, []shapeCase{{name: "inline next flag", before: "command: |-\n  redis --requirepass first-secret\n  continuation-secret --maxclients 50\nimage: redis\n",
		after:   "command: |-\n  redis --requirepass first-secret\n  continuation-secret --maxclients 60\nimage: redis\n",
		secrets: []string{"first-secret", "continuation-secret"}, want: []string{"--maxclients 50", "--maxclients 60", "image: redis"}},
		{name: "dedent", before: "command: |-\n  redis --requirepass first-secret\n  continuation-secret\nimage: redis-old\n",
			after:   "command: |-\n  redis --requirepass first-secret\n  continuation-secret\nimage: redis-new\n",
			secrets: []string{"first-secret", "continuation-secret"}, want: []string{"image: redis-old", "image: redis-new"}},
	})
}

func TestUnifiedSequenceBlockSecretContinuation(t *testing.T) {
	for _, header := range []string{"|-", ">-", "&cmd |2- # literal"} {
		before := "command:\n  - " + header + "\n    redis --requirepass first-secret\n    continuation-secret\n    --maxclients 50\n  - keep-visible\nimage: redis\n"
		after := strings.Replace(before, "50", "60", 1)
		runShapes(t, []shapeCase{{name: header, before: before, after: after,
			secrets: []string{"first-secret", "continuation-secret"},
			want:    []string{"--maxclients 50", "--maxclients 60", "- keep-visible", "image: redis"}}})
	}
}

func TestUnifiedBlockQuotedPairClosesBeforeDedent(t *testing.T) {
	runShapes(t, []shapeCase{{name: "quoted pair", before: "command: |-\n  API_TOKEN=\"first-secret\n  continuation-secret\"\nimage: redis-old\n",
		after:   "command: |-\n  API_TOKEN=\"first-secret\n  continuation-secret\"\nimage: redis-new\n",
		secrets: []string{"first-secret", "continuation-secret"},
		want:    []string{"image: redis-old", "image: redis-new"}}})
}

func TestUnifiedLiteralRedactionMarkerSecret(t *testing.T) {
	for _, secret := range []string{"prefix[REDACTED]suffix", "[REDACTED]", "prefix\x00redacted\x00suffix"} {
		t.Run(secret, func(t *testing.T) {
			before := "PASSWORD: " + secret + "\nhealthcheck: echo " + secret + "\n"
			after := "PASSWORD: changed-secret\nhealthcheck: echo " + secret + "\n"
			got := Unified("docker-compose.yml", []byte(before), []byte(after))
			if strings.Contains(got, "prefix") || strings.Contains(got, "changed-secret") || strings.Contains(got, "\x00") {
				t.Fatalf("secret survived: %q", got)
			}
			if !strings.Contains(got, "healthcheck: echo [REDACTED]\n") {
				t.Fatalf("repeated secret was not masked: %q", got)
			}
		})
	}
}

func TestRedactSecretSidesSentinelCollision(t *testing.T) {
	// Only the after side holds the candidate sentinel, as ordinary text. The
	// before side's redaction must use a different marker shared by both sides.
	before := []string{"PASSWORD: shared-secret\n"}
	after := []string{"note: \x00redacted\x00\n", "healthcheck: shared-secret\n"}
	ao, bo := redactSecretSides(before, after)
	if ao[0] != "PASSWORD: [REDACTED]\n" || bo[0] != after[0] || bo[1] != "healthcheck: [REDACTED]\n" {
		t.Fatalf("redaction confused input and sentinel: %q, %q", ao, bo)
	}
}

func TestRedactSecretLinesBlockContinuationRepeats(t *testing.T) {
	lines := []string{"command: |-\n", "  redis --requirepass first-secret\n", "  continuation-secret # literal\n", "healthcheck: continuation-secret # literal\n"}
	got := redactSecretLines(lines)
	if got[2] != "  [REDACTED]\n" || got[3] != "healthcheck: [REDACTED]\n" {
		t.Fatalf("literal continuation was not collected: %q", got)
	}
}
