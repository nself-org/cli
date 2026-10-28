// Copyright © 2024 nself-org. All rights reserved.

package frontcheck

import "strings"

// SameNames reports whether two server_name sets are equal. An empty set is no
// evidence of ownership, so it equals nothing, not even another empty set.
func SameNames(a, b []string) bool {
	return len(a) > 0 && len(b) > 0 && strings.Join(a, " ") == strings.Join(b, " ")
}
