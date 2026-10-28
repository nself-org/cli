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

type composeLabels struct {
	Services map[string]struct {
		Labels yaml.Node `yaml:"labels"`
	} `yaml:"services"`
}

// initContainerServices returns services whose effective nself.type label is
// init-container after applying label overrides in Compose file order.
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
			if value, present := labelValue(&svc.Labels, "nself.type"); present {
				if value == "init-container" {
					out[name] = true
				} else {
					delete(out, name)
				}
			}
		}
	}
	return out, nil
}

// labelValue reads one Compose label, preserving whether an override sets it.
func labelValue(n *yaml.Node, key string) (string, bool) {
	switch n.Kind {
	case yaml.MappingNode:
		var labels map[string]string
		if n.Decode(&labels) == nil {
			value, ok := labels[key]
			return value, ok
		}
	case yaml.SequenceNode:
		var labels []string
		if n.Decode(&labels) == nil {
			for i := len(labels) - 1; i >= 0; i-- {
				k, value, _ := strings.Cut(labels[i], "=")
				if k == key {
					return value, true
				}
			}
		}
	}
	return "", false
}
