package reconcile

// containers_init.go — services whose containers `nself start` removes.
//
// Purpose: keep init containers out of the "create" action (D-0505 review).
// Inputs: the planned compose files.
// Outputs: the set of services labelled nself.type=init-container.
// Constraints: the label is the one docker.cleanupInitContainers removes
// by, run after every start (docker.RunPostStartCleanup), so a missing
// container of such a service is the normal state, not a pending create.
// Labels may be a map or a "key=value" list; missing files are skipped, as
// docker.StatefulServices does.

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// initContainerLabel is the label docker.cleanupInitContainers removes by.
const initContainerLabel = "nself.type=init-container"

type composeLabels struct {
	Services map[string]struct {
		Labels yaml.Node `yaml:"labels"`
	} `yaml:"services"`
}

// initContainerServices returns the services carrying initContainerLabel in
// the effective labels, merging overrides in file order by label key.
func initContainerServices(files []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading compose file %s: %w", path, err)
		}
		var doc composeLabels
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parsing compose file %s: %w", path, err)
		}
		for name, svc := range doc.Services {
			key, value, _ := strings.Cut(initContainerLabel, "=")
			if got, present := composeLabelValue(&svc.Labels, key); present {
				if got == value {
					out[name] = true
				} else {
					delete(out, name)
				}
			}
		}
	}
	return out, nil
}

// composeLabelValue reads a label from map or list syntax. Absence preserves
// an earlier file's value; a present empty value overrides it.
func composeLabelValue(n *yaml.Node, key string) (string, bool) {
	if n.Kind == yaml.AliasNode {
		return composeLabelValue(n.Alias, key)
	}
	switch n.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if n.Decode(&m) == nil {
			value, present := m[key]
			return value, present
		}
	case yaml.SequenceNode:
		var l []string
		if n.Decode(&l) != nil {
			return "", false
		}
		var value string
		var present bool
		for _, e := range l {
			k, v, _ := strings.Cut(strings.TrimSpace(e), "=")
			if k == key {
				value, present = v, true
			}
		}
		return value, present
	}
	return "", false
}
