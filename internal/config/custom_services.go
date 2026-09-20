package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseCustomServices parses CS_1 through CS_10 environment variables into
// CustomService structs. Each CS_N value uses the format:
//
//	name:template[:port][:route]
//
// If port is omitted or zero, it auto-assigns 8000+N.
// Per-service overrides are read from CS_N_PUBLIC, CS_N_MEMORY, CS_N_CPU,
// CS_N_PORT, CS_N_ROUTE, CS_N_HEALTHCHECK, CS_N_ENV_PASSTHROUGH,
// CS_N_IMAGE, CS_N_ENV_FILE, and CS_N_VOLUMES environment variables.
func parseCustomServices() ([]CustomService, error) {
	var services []CustomService
	for i := 1; i <= 10; i++ {
		raw := os.Getenv(fmt.Sprintf("CS_%d", i))
		if raw == "" {
			continue
		}

		// Format: name:template[:port][:route]
		parts := strings.SplitN(raw, ":", 4)
		if len(parts) < 2 {
			return nil, fmt.Errorf("CS_%d has invalid format: expected name:template[:port][:route], got %q", i, raw)
		}

		cs := CustomService{
			Index:    i,
			Name:     parts[0],
			Template: parts[1],
		}

		if err := validateCustomServiceName(cs.Name); err != nil {
			return nil, fmt.Errorf("CS_%d: %w", i, err)
		}
		// Preserve the as-configured name before sanitization so callers that
		// need to resolve an on-disk path (e.g. the compose generator's
		// default `./services/<name>` build context) can find a directory
		// named "ping_api" even though the sanitized Docker service name is
		// "ping-api". See CustomService.RawName for the full rationale.
		cs.RawName = cs.Name
		sanitized, err := SanitizeName(cs.Name)
		if err != nil {
			return nil, fmt.Errorf("CS_%d has invalid name %q: %w", i, cs.Name, err)
		}
		cs.Name = sanitized

		// Optional port
		if len(parts) >= 3 && parts[2] != "" {
			p, err := strconv.Atoi(parts[2])
			if err != nil {
				return nil, fmt.Errorf("CS_%d has invalid format: expected numeric port, got %q", i, parts[2])
			}
			cs.Port = p
		}
		if cs.Port == 0 {
			cs.Port = 8000 + i // auto-assign
		}

		// Optional route
		if len(parts) >= 4 && parts[3] != "" {
			cs.Route = parts[3]
		}

		// Additional per-service config
		cs.Public = getEnvBool(fmt.Sprintf("CS_%d_PUBLIC", i), cs.Route != "")
		cs.Memory = getEnvOr(fmt.Sprintf("CS_%d_MEMORY", i), "256m")
		cs.CPU = getEnvOr(fmt.Sprintf("CS_%d_CPU", i), "0.5")
		cs.TablePrefix = os.Getenv(fmt.Sprintf("CS_%d_TABLE_PREFIX", i))
		cs.ExtraEnv = os.Getenv(fmt.Sprintf("CS_%d_ENV", i))
		cs.HealthCheck = os.Getenv(fmt.Sprintf("CS_%d_HEALTHCHECK", i))
		cs.EnvPassthrough = os.Getenv(fmt.Sprintf("CS_%d_ENV_PASSTHROUGH", i))

		// Optional build context path override. Rejects absolute paths and
		// path traversal so a misconfigured env can't escape the project root.
		if p := os.Getenv(fmt.Sprintf("CS_%d_PATH", i)); p != "" {
			if err := validateRelativePath(p); err != nil {
				return nil, fmt.Errorf("CS_%d_PATH %w", i, err)
			}
			cs.BuildPath = p
		}

		// CS_N_IMAGE: run a pre-built (optionally digest-pinned) image instead
		// of building from a Dockerfile. Mutually exclusive with CS_N_PATH,
		// which only makes sense for the build path (G-013).
		cs.Image = os.Getenv(fmt.Sprintf("CS_%d_IMAGE", i))
		if cs.Image != "" && cs.BuildPath != "" {
			return nil, fmt.Errorf("CS_%d_IMAGE and CS_%d_PATH are mutually exclusive: a service either builds from a Dockerfile (CS_%d_PATH) or runs a pre-built image (CS_%d_IMAGE), not both", i, i, i, i)
		}

		// CS_N_ENV_FILE: dotenv-format file of extra env vars, injected at
		// build time (see coreEnvVars). Same relative-path rules as CS_N_PATH.
		if p := os.Getenv(fmt.Sprintf("CS_%d_ENV_FILE", i)); p != "" {
			if err := validateRelativePath(p); err != nil {
				return nil, fmt.Errorf("CS_%d_ENV_FILE %w", i, err)
			}
			cs.EnvFile = p
		}

		// CS_N_VOLUMES: comma-separated "host:container[:mode]" bind mounts,
		// appended to the generated service. Each relative host path is
		// subject to the same traversal check as CS_N_PATH.
		if v := os.Getenv(fmt.Sprintf("CS_%d_VOLUMES", i)); v != "" {
			if err := validateCustomServiceVolumes(v); err != nil {
				return nil, fmt.Errorf("CS_%d_VOLUMES %w", i, err)
			}
			cs.Volumes = v
		}

		// Override port/route if explicitly set
		if p := getEnvInt(fmt.Sprintf("CS_%d_PORT", i), 0); p != 0 {
			cs.Port = p
		}
		if r := os.Getenv(fmt.Sprintf("CS_%d_ROUTE", i)); r != "" {
			cs.Route = r
		}

		services = append(services, cs)
	}
	return services, nil
}
