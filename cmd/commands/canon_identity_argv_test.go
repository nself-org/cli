package commands

import (
	"reflect"
	"testing"
)

func TestCanonIdentityRewrite(t *testing.T) {
	for _, name := range []string{"ci", "functions"} {
		t.Run(name, func(t *testing.T) {
			tb := &canonTableT{Breakouts: []canonRowT{{From: []string{name}, To: []string{name}, Plugin: name}}}
			args := []string{name, "status"}
			got, notes, err := rewriteOldToNew(tb, nil, args, 0, args)
			if err != nil || !reflect.DeepEqual(got, args) || len(notes) != 0 {
				t.Fatalf("identity rewrite: %v, notes=%v, err=%v", got, notes, err)
			}
		})
	}
}
