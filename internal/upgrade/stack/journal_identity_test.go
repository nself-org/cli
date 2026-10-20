package stack

import "testing"

func TestJournalRunIDCharset(t *testing.T) {
	for _, tc := range []struct {
		id    string
		valid bool
	}{
		{"run-123-AbC", true}, {"../outside", false}, {"nested/file", false}, {`nested\file`, false}, {"run.id", false}, {"", false}, {"run\n", false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			j := newJournal(false)
			j.RunID = tc.id
			if err := validateJournal(j); (err == nil) != tc.valid {
				t.Fatalf("validateJournal(%q)=%v, valid=%v", tc.id, err, tc.valid)
			}
		})
	}
}
