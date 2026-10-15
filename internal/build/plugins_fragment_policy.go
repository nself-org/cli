package build

// Purpose: enforce fragment boundaries in the ADR 0027 decoded-input policy.
// Inputs: alias/merge-resolved fragment, owner, generated core names and env.
// Outputs: a policy refusal for inheritance, host builds or cross-owner access.
// Constraints: no reads of included files; socket exceptions cannot waive this.

import (
	"fmt"
	"maps"
	"slices"
)

func checkFragmentPolicy(top map[string]any, owner string, core map[string]any, env SocketEnv) error {
	if top["include"] != nil {
		return fmt.Errorf("%s uses include, which is not inspected", owner)
	}
	services, err := socketMap(top["services"], owner+" services")
	if err != nil {
		return err
	}
	networks, err := socketMap(top["networks"], owner+" networks")
	if err != nil {
		return err
	}
	coreServices, _ := socketMap(core["services"], "core services")
	for _, name := range slices.Sorted(maps.Keys(services)) {
		svc, _ := services[name].(map[string]any)
		bad := func(detail string) error {
			return fmt.Errorf("%s service %q %s", owner, name, detail)
		}
		if _, exists := coreServices[name]; exists {
			return bad("redefines a core service")
		}
		if svc["extends"] != nil {
			return bad("uses extends, which is not inspected")
		}
		if build, ok := svc["build"].(map[string]any); ok && build["network"] != nil {
			network, stringValue := build["network"].(string)
			x, resolved := expandSocket(network, env)
			if !stringValue || !resolved || x == "host" {
				return bad("uses build.network host or an unprovable build.network")
			}
		}
		keys, err := fragmentNetworkKeys(svc["networks"])
		if err != nil {
			return bad(err.Error())
		}
		for _, key := range keys {
			if _, declared := networks[key]; !declared && !projectNetworkRE.MatchString(key) && !isLegacyNetwork(key) {
				return bad(fmt.Sprintf("references network %q not declared by its own fragment", key))
			}
		}
	}
	return nil
}

// fragmentNetworkKeys reads only network names, never per-network options.
func fragmentNetworkKeys(value any) ([]string, error) {
	var keys []string
	switch networks := value.(type) {
	case nil:
	case map[string]any:
		keys = slices.Sorted(maps.Keys(networks))
	case []any:
		for _, raw := range networks {
			key, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("has an unsupported networks entry")
			}
			keys = append(keys, key)
		}
	default:
		return nil, fmt.Errorf("has an unsupported networks shape")
	}
	return keys, nil
}
