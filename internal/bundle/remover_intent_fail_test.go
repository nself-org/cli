package bundle

// remover_intent_fail_test.go — D-0368: a failed member keeps the intent, and a
// dry run never writes one. Fixtures come from remover_intent_test.go.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestRemoveMemberFailureKeepsIntent(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := familyDir(t)
		var out bytes.Buffer
		rec := &recorder{failOn: "d2"}
		res, err := Remove(context.Background(), "family", baseOpts(dir, &out, rec))
		if err == nil || !strings.Contains(err.Error(), "stopped at d2") || !strings.Contains(err.Error(), "'nself bundle remove family' again") {
			t.Fatalf("err = %v, want the stopped-at wording", err)
		}
		if !sameStrings(res.Removed, []string{"d3"}) {
			t.Errorf("removed %v, want [d3]", res.Removed)
		}
		if _, ok := loadLedger(t, dir).Bundles["family"]; !ok {
			t.Error("the bundle must stay recorded after a failure")
		}
		if in, ok := intentOf(t, dir, "family"); !ok || !sameStrings(in.Remaining, []string{"d1", "d2"}) {
			t.Errorf("intent = %+v ok=%v, want remaining [d1 d2]", in, ok)
		}
		out.Reset()
		if _, err := Remove(context.Background(), "family", baseOpts(dir, &out, &recorder{})); err != nil {
			t.Fatalf("rerun: %v", err)
		}
		if intentFileExists(dir) || isAlreadyInstalled(dir, "d1") || isAlreadyInstalled(dir, "d2") {
			t.Error("rerun did not finish the remove")
		}
	})
}

func TestRemoveDryRunWritesNoIntent(t *testing.T) {
	dir := familyDir(t)
	var out bytes.Buffer
	opts := baseOpts(dir, &out, &recorder{})
	opts.DryRun = true
	if _, err := Remove(context.Background(), "family", opts); err != nil {
		t.Fatal(err)
	}
	if ledgerBytes(t, dir) != nil || intentFileExists(dir) {
		t.Error("a dry run wrote state")
	}
}
