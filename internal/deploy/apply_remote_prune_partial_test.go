package deploy

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// D-0823: a prune that fails in a later batch has already deleted the earlier
// batches' generated files. They must stay in the changed list, so the in-place
// restore of the failed apply puts them back.
func TestApplyRemotePartialPruneFailureRestoresDeletedFiles(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	var shipped string
	var old []string
	for i := 0; i < 60; i++ { // 60 candidates: batches of 50 and 10
		p := fmt.Sprintf("nginx/sites/g%02d.conf", i)
		old = append(old, p)
		shipped += p + "\n"
		writeFile(t, rp+"/"+p, markedConf+"# v1\n", 0o644)
	}
	writeFile(t, rp+"/"+ShippedFile, shipped, 0o644)
	o.Previous = old
	f.fail["g55.conf"] = errors.New("ssh dropped") // only the second batch names it

	res, err := ApplyRemote(context.Background(), o)
	if err == nil {
		t.Fatal("the second prune batch failed but the apply succeeded")
	}
	if !contains(res.Changed, "nginx/sites/g00.conf") {
		t.Errorf("a file the first batch already deleted is missing from Changed: %v", res.Changed)
	}
	if !exists(rp + "/nginx/sites/g00.conf") {
		t.Error("a file pruned before the failure was not restored")
	}
}
