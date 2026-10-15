package importer

// Purpose: let a source declare the fixed, credential-free error texts that may reach the user.
// Inputs: static strings from a source package's init. Outputs: membership checks for publicError.
// Constraints: only constant text; a message built from input (a URL, a key, a table) must not be
// registered, because publicError otherwise replaces every source error with a generic one.
import (
	"fmt"
	"sync"
)

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

// SafeError is a source failure whose text is fixed by the source and carries no input. publicError
// keeps its text, so a source can tell the user what failed without a registry entry per message.
type SafeError struct{ Code, What string }

// NewSafe returns a SafeError. code is one of E517, E518, E520 or E521; any other code is shown generically.
func NewSafe(code, what string) error { return &SafeError{Code: code, What: what} }

func (e *SafeError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.What) }

func passesText(code string) bool {
	return code == "E517" || code == "E518" || code == "E520" || code == "E521"
}
