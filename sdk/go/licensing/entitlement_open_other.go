//go:build !unix

package licensing

import "os"

// Platforms without Unix open flags rely on the opened descriptor's SameFile check.
const tokenOpenFlags = os.O_RDONLY
