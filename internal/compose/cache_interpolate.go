package compose

// cacheImageExpanded resolves Compose image references before classifying the
// engine. Variable values are inserted verbatim; only fallback/alternate text
// is recursively interpolated. Invalid or required-but-unset references give
// no image evidence, allowing mounted data to decide instead.
import (
	"os"
	"strings"
)

func cacheImageExpanded(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			out.WriteByte(s[i])
			i++
			continue
		}
		i++
		if i == len(s) {
			return ""
		}
		if s[i] == '$' {
			out.WriteByte('$')
			i++
			continue
		}
		start := i
		braced := s[i] == '{'
		if braced {
			i++
			start = i
		}
		for i < len(s) && (s[i] == '_' || s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z' || i > start && s[i] >= '0' && s[i] <= '9') {
			i++
		}
		if i == start {
			return ""
		}
		name := s[start:i]
		value, set := os.LookupEnv(name)
		if braced {
			end, depth := i, 1
			for end < len(s) {
				if s[end] == '{' {
					depth++
				}
				if s[end] == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				end++
			}
			if end == len(s) {
				return ""
			}
			rest := s[i:end]
			if rest != "" {
				op, arg := "", ""
				for _, candidate := range []string{":-", ":+", ":?", "-", "+", "?"} {
					if strings.HasPrefix(rest, candidate) {
						op, arg = candidate, rest[len(candidate):]
						break
					}
				}
				switch op {
				case ":-":
					if value == "" {
						value = cacheImageExpanded(arg)
					}
				case "-":
					if !set {
						value = cacheImageExpanded(arg)
					}
				case ":+":
					if value != "" {
						value = cacheImageExpanded(arg)
					} else {
						value = ""
					}
				case "+":
					if set {
						value = cacheImageExpanded(arg)
					} else {
						value = ""
					}
				case ":?":
					if value == "" {
						return ""
					}
				case "?":
					if !set {
						return ""
					}
				default:
					return ""
				}
			}
			i = end + 1
		}
		out.WriteString(value)
	}
	return out.String()
}

func cacheImageEngine(image string) string {
	// Classify the repository, never a variable name, tag, or registry hostname.
	image = strings.ToLower(strings.SplitN(image, "@", 2)[0])
	parts := strings.Split(image, "/")
	repo := strings.SplitN(parts[len(parts)-1], ":", 2)[0]
	switch repo {
	case "valkey":
		return EngineValkey
	case "redis":
		return EngineRedis
	}
	return ""
}
