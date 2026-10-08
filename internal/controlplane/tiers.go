package controlplane

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Tier states the operational class of an environment.
type Tier string

const (
	TierLocal        Tier = "local"
	TierLocalServers Tier = "local-servers"
	TierProd         Tier = "prod"
)

// DeriveTier supplies the v2 default for an environment without a tier.
func DeriveTier(name, kind string) Tier {
	if kind == "local" {
		return TierLocal
	}
	if name == "prod" || name == "production" {
		return TierProd
	}
	return TierLocalServers
}

// TierRank orders tiers from local to production.
func TierRank(t Tier) int {
	switch t {
	case TierLocal:
		return 0
	case TierLocalServers:
		return 1
	case TierProd:
		return 2
	}
	return 3
}

func validTier(t Tier) bool {
	return t == TierLocal || t == TierLocalServers || t == TierProd
}

func normalizedTier(e Environment) Tier {
	if e.Tier != "" {
		return e.Tier
	}
	return DeriveTier(strings.ToLower(e.Name), e.Kind)
}

var environmentNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)
var legacyWarningOnce sync.Once

func warnLegacyHost() {
	// compat.V15(P7-DEPL-13): silent legacy host path -> warn once until removal at v1.6.0.
	if compat.V15() {
		legacyWarningOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "warning: host:/path is deprecated and will be removed in v1.6.0; set remote_path separately")
		})
	}
}

func migrateTiers(inv *Inventory) error {
	for name, e := range inv.Environments {
		if e.Tier == "" {
			e.Tier = DeriveTier(name, e.Kind)
		}
		for j := range e.Servers {
			s := &e.Servers[j]
			if spec, err := remote.ParseHostSpec(s.Host); err == nil && spec.LegacyPath != "" {
				s.Host, s.RemotePath = spec.String(), spec.LegacyPath
				warnLegacyHost()
			}
		}
		inv.Environments[name] = e
	}
	return nil
}

func validateInventoryV2(inv *Inventory) error {
	localCount := 0
	serverNames := map[string]string{}
	for name, e := range inv.Environments {
		if !environmentNameRe.MatchString(name) {
			return errs.New("E485", fmt.Sprintf("environments.%s: invalid env name", name))
		}
		if e.Name != name {
			return errs.New("E485", fmt.Sprintf("environments.%s.name: expected %q", name, name))
		}
		if e.Kind != "local" && e.Kind != "remote" {
			return errs.New("E485", fmt.Sprintf("environments.%s.kind: invalid kind %q", name, e.Kind))
		}
		if !validTier(e.Tier) {
			return errs.New("E485", fmt.Sprintf("environments.%s.tier: invalid tier %q", name, e.Tier))
		}
		if e.Kind == "local" {
			localCount++
		}
		if e.Tier == TierProd {
			primary, hosts := 0, 0
			for _, s := range e.Servers {
				if s.Primary {
					primary++
				}
				if s.Host != "" {
					hosts++
				}
			}
			if hosts == 0 || primary != 1 {
				return errs.New("E485", fmt.Sprintf("environments.%s.servers: prod tier needs a host and exactly one primary", name))
			}
		}
		for _, s := range e.Servers {
			if prev, ok := serverNames[s.Name]; ok {
				return errs.New("E485", fmt.Sprintf("environments.%s.servers.name: duplicate %q (also in %s)", name, s.Name, prev))
			}
			serverNames[s.Name] = name
			if s.Arch != "" && s.Arch != "amd64" && s.Arch != "arm64" {
				return errs.New("E485", fmt.Sprintf("environments.%s.servers.%s.arch: invalid %q", name, s.Name, s.Arch))
			}
		}
	}
	if localCount != 1 {
		return errs.New("E485", fmt.Sprintf("environments: expected exactly one local env, got %d", localCount))
	}
	return nil
}

func rejectLegacyEnvCaseCollisions() error {
	seen := map[string]string{}
	for _, item := range os.Environ() {
		key, val, ok := strings.Cut(item, "=")
		if !ok || val == "" || !strings.HasPrefix(key, envVarPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(key, envVarPrefix)
		folded := strings.ToLower(suffix)
		if first, exists := seen[folded]; exists && first != suffix {
			return errs.New("E483", fmt.Sprintf("NSELF_DEPLOY_HOST_%s and NSELF_DEPLOY_HOST_%s differ only by case", first, suffix))
		}
		seen[folded] = suffix
	}
	return nil
}

// ValidateInventory applies the same checks used by Load to a pending edit.
func ValidateInventory(inv *Inventory) error {
	if err := Migrate(inv); err != nil {
		return err
	}
	return validateInventoryNames(inv)
}
