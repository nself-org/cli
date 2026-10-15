package compose

// Purpose: resolve existing cache image and mounted data-volume evidence.
// Inputs: Compose strings and the process environment loaded by config.Load.
// Outputs: the engine name, or empty when the evidence cannot identify it.
// Constraints: no commands or filesystem writes; image evidence wins over mounts.

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func cacheImageEngine(image string) string {
	if strings.ContainsAny(image, "${}") {
		return ""
	}
	image = strings.ToLower(image)
	for _, engine := range []string{EngineValkey, EngineRedis} {
		if strings.Contains(image, engine) {
			return engine
		}
	}
	return ""
}

func cacheMountEngine(node yaml.Node) string {
	var source, target string
	if node.Kind == yaml.ScalarNode {
		parts := strings.Split(cacheInterpolate(node.Value), ":")
		if len(parts) >= 2 {
			source, target = parts[0], parts[1]
		}
	} else {
		var mount struct{ Type, Source, Target string }
		if node.Decode(&mount) != nil || mount.Type != "volume" {
			return ""
		}
		source, target = cacheInterpolate(mount.Source), cacheInterpolate(mount.Target)
	}
	if target == "/data" {
		switch source {
		case "valkey_data":
			return EngineValkey
		case "redis_data":
			return EngineRedis
		}
	}
	return ""
}

// cacheInterpolate handles Compose references without expanding substituted values again.
// Malformed or required-but-unset references retain '$', so they cannot name an engine.
func cacheInterpolate(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' || i+1 == len(s) {
			out.WriteByte(s[i])
			i++
			continue
		}
		switch s[i+1] {
		case '$':
			out.WriteByte('$')
			i += 2
		case '{':
			end, depth := i+2, 1
			for ; end < len(s); end++ {
				if s[end] == '{' {
					depth++
				}
				if s[end] == '}' {
					depth--
				}
				if depth == 0 {
					break
				}
			}
			if depth != 0 {
				return s
			}
			out.WriteString(cacheReference(s[i+2 : end]))
			i = end + 1
		default:
			end := i + 1
			for end < len(s) && cacheNameByte(s[end], end == i+1) {
				end++
			}
			if end == i+1 {
				return s
			}
			out.WriteString(os.Getenv(s[i+1 : end]))
			i = end
		}
	}
	return out.String()
}

func cacheNameByte(c byte, first bool) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || !first && c >= '0' && c <= '9'
}

func cacheReference(body string) string {
	n := 0
	for n < len(body) && cacheNameByte(body[n], n == 0) {
		n++
	}
	if n == 0 {
		return "$"
	}
	value, set := os.LookupEnv(body[:n])
	rest := body[n:]
	if rest == "" {
		return value
	}
	for _, op := range []string{":-", ":+", ":?", "-", "+", "?"} {
		if !strings.HasPrefix(rest, op) {
			continue
		}
		present := set && (!strings.HasPrefix(op, ":") || value != "")
		switch op[len(op)-1] {
		case '-':
			if present {
				return value
			}
		case '+':
			if !present {
				return ""
			}
		case '?':
			if present {
				return value
			}
			return "$"
		}
		return cacheInterpolate(rest[len(op):])
	}
	return "$"
}
