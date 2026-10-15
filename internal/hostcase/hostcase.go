// Package hostcase supplies the case normalization shared by HTTP allowlists.
// Purpose: keep Host and Origin comparison limited to ASCII case folding.
// Inputs: raw host or origin strings. Outputs: strings with A-Z lowercased.
// Constraints: preserve every other byte, including non-ASCII characters.
package hostcase

// LowerASCII lowercases A-Z only. Unicode-aware folding would make a Kelvin
// sign equal "k", allowing non-ASCII hosts to match ASCII allowlist entries.
func LowerASCII(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}
