package portable

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// foldName returns the key under which two member names clash: NFC first, then
// Unicode case folding, then NFC again (folding can leave a string
// unnormalised). Two names with the same key can land on one file of a
// case-insensitive or normalising file system.
func foldName(name string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(name)))
}
