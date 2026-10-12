package importer

// Purpose: let a source declare the fixed, credential-free error texts that may reach the user.
// Inputs: static strings from a source package's init. Outputs: membership checks for publicError.
// Constraints: only constant text; a message built from input (a URL, a key, a table) must not be
// registered, because publicError otherwise replaces every source error with a generic one.
import "sync"

var allowed = struct {
	sync.RWMutex
	set map[string]bool
}{set: map[string]bool{}}

// AllowMessages registers texts that publicError may surface unchanged for E517 and E520 source failures.
func AllowMessages(messages ...string) {
	allowed.Lock()
	defer allowed.Unlock()
	for _, m := range messages {
		allowed.set[m] = true
	}
}

func isSafeMessage(what string) bool {
	for _, safe := range safeMessages {
		if what == safe {
			return true
		}
	}
	allowed.RLock()
	defer allowed.RUnlock()
	return allowed.set[what]
}
