package invoke

// Purpose: keep one audit record on one line for every line reader.
// Inputs: the encoded audit line. Outputs: the same line with U+0085 escaped.
// Constraints: encoding/json escapes the bytes below 0x20, U+2028 and U+2029
// but not U+0085 (NEXT LINE). Readers that split text on Unicode line
// boundaries (Python str.splitlines, many log shippers) would cut a record at
// a request-supplied U+0085 and could be fed a forged second record. U+0085
// is the UTF-8 pair C2 85, which in valid UTF-8 occurs only as that character.

import "bytes"

var (
	nextLineUTF8    = []byte{0xc2, 0x85}
	nextLineEscaped = []byte(`\u0085`)
)

// escapeNextLine returns b with every U+0085 replaced by its JSON escape.
func escapeNextLine(b []byte) []byte {
	return bytes.ReplaceAll(b, nextLineUTF8, nextLineEscaped)
}
