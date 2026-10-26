package destinations

import "testing"

func TestHasInlineOption(t *testing.T) {
	for uri, want := range map[string]bool{
		"myr,pass=ab/cd":    true,
		"./myr,pass=ab/cd":  false,
		"../myr,pass=ab/cd": false,
		"backups":           false,
		"back,ups":          false,
	} {
		if got := HasInlineOption(uri); got != want {
			t.Errorf("HasInlineOption(%q) = %v, want %v", uri, got, want)
		}
	}
}
