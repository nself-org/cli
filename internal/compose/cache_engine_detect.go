package compose

// cache_engine_detect.go — classification of an existing cache service from a
// docker-compose.yml (P7-LIVE-19, EPIC D16, review F14). Split out of
// cache_service.go to keep that file under the 300-line budget.

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// ExistingCacheEngine reads the cache engine out of an existing docker-compose.yml
// with no environment (only ${VAR:-default} fallbacks apply); see
// ExistingCacheEngineEnv.
func ExistingCacheEngine(composeYAML []byte) string {
	return ExistingCacheEngineEnv(composeYAML, nil)
}

// ExistingCacheEngineEnv reads the cache engine out of an existing
// docker-compose.yml. The `redis` service image is classified after Compose
// interpolation (lookup first, then the ${VAR:-default} fallback; nil lookup =
// empty environment): EngineValkey when it names valkey, EngineRedis when it
// names redis. When the image cannot name the engine (a ${VAR} with no value, a
// build) the engine comes from the data volume the service mounts (valkey_data
// or redis_data), then EngineRedis for a service that shows neither. Without a
// redis service, a redis_data volume is EngineRedis and a valkey_data volume
// EngineValkey; "" when the file shows no cache at all. An unparseable file
// shows nothing.
func ExistingCacheEngineEnv(composeYAML []byte, lookup func(string) (string, bool)) string {
	var doc struct {
		Services map[string]struct {
			Image   string        `yaml:"image"`
			Volumes []interface{} `yaml:"volumes"`
		} `yaml:"services"`
		Volumes map[string]interface{} `yaml:"volumes"`
	}
	if yaml.Unmarshal(composeYAML, &doc) != nil {
		return ""
	}
	if svc, ok := doc.Services["redis"]; ok {
		img := strings.ToLower(interpolateCompose(svc.Image, lookup))
		switch {
		case strings.Contains(img, "valkey"):
			return EngineValkey
		case strings.Contains(img, "redis"):
			return EngineRedis
		}
		for _, v := range svc.Volumes {
			src := ""
			switch t := v.(type) {
			case string:
				src, _, _ = strings.Cut(t, ":")
			case map[string]interface{}:
				src, _ = t["source"].(string)
			}
			switch src {
			case "valkey_data":
				return EngineValkey
			case "redis_data":
				return EngineRedis
			}
		}
		return EngineRedis
	}
	if _, ok := doc.Volumes["redis_data"]; ok {
		return EngineRedis
	}
	if _, ok := doc.Volumes["valkey_data"]; ok {
		return EngineValkey
	}
	return ""
}

// interpolateCompose expands ${VAR}, ${VAR:-default}, ${VAR-default} and $VAR in
// s the way Compose does for an image reference: a set (and, for :-, non-empty)
// variable wins, then the default. An unset variable with no default is empty.
func interpolateCompose(s string, lookup func(string) (string, bool)) string {
	get := func(k string) (string, bool) {
		if lookup == nil {
			return "", false
		}
		return lookup(k)
	}
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '$':
			out.WriteByte('$')
			i++
		case '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				out.WriteString(s[i:])
				return out.String()
			}
			expr := s[i+2 : i+end]
			i += end
			name, def, op := expr, "", ""
			if j := strings.Index(expr, ":-"); j >= 0 {
				name, def, op = expr[:j], expr[j+2:], ":-"
			} else if j := strings.IndexByte(expr, '-'); j >= 0 {
				name, def, op = expr[:j], expr[j+1:], "-"
			}
			v, set := get(name)
			if op == ":-" && v == "" || op == "-" && !set {
				v = interpolateCompose(def, lookup)
			}
			out.WriteString(v)
		default:
			j := i + 1
			for j < len(s) && (s[j] == '_' || s[j] >= '0' && s[j] <= '9' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z') {
				j++
			}
			if j == i+1 {
				out.WriteByte('$')
				continue
			}
			v, _ := get(s[i+1 : j])
			out.WriteString(v)
			i = j - 1
		}
	}
	return out.String()
}
