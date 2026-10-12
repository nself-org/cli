package importer

// Purpose: deterministic, panic-free source registration.
// Inputs: source plugins. Outputs: sorted sources and registration failures.
// Constraints: invalid and duplicate names cannot replace a registered source.
import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"sync"
)

var registry = struct {
	sync.RWMutex
	sources  map[string]Source
	failures []error
}{sources: map[string]Source{}}
var sourceName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func Register(s Source) {
	registry.Lock()
	defer registry.Unlock()
	if s == nil || (reflect.ValueOf(s).Kind() == reflect.Pointer && reflect.ValueOf(s).IsNil()) {
		registry.failures = append(registry.failures, fmt.Errorf("nil import source"))
		return
	}
	name := s.Name()
	if !sourceName.MatchString(name) {
		registry.failures = append(registry.failures, fmt.Errorf("invalid import source name"))
		return
	}
	if _, ok := registry.sources[name]; ok {
		registry.failures = append(registry.failures, fmt.Errorf("duplicate import source %s", name))
		return
	}
	registry.sources[name] = s
}
func RegistryErrors() []error {
	registry.RLock()
	defer registry.RUnlock()
	return append([]error(nil), registry.failures...)
}
func Sources() []Source {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]Source, 0, len(registry.sources))
	for _, s := range registry.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
