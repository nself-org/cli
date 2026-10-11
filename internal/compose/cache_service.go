package compose

// cache_service.go — the `redis` compose service and its engine (Valkey or
// Redis), P7-LIVE-19, EPIC D16, ADR 0029.
//
// Purpose: Redis 7.4 and later is RSALv2/SSPLv1, so the default cache becomes
// Valkey (BSD-3-Clause). The service name `redis`, the container name, the
// REDIS_* variables, the port, the user and the network stay, so apps and
// plugins keep working against REDIS_URL.
// Inputs: the resolved config (cfg.Redis) and the installed project pins.
// Outputs: the `redis` ServiceConfig and the data volume name.
// Constraints: the engine is chosen by cacheEngine and never by a side effect.
// Precedence: explicit REDIS_ENGINE (both modes) > the engine of the project
// lock's `redis` pin (an existing project keeps its engine; the build takes the
// pin from the existing docker-compose.yml when no lock exists yet, review F14)
// > the mode default (v1.4 redis, v1.5 valkey). Valkey keeps its data in the
// new volume valkey_data; redis_data is never touched, so switching back reuses
// it. Queued jobs in Redis are not migrated on a switch.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compat"
)

// Cache engines REDIS_ENGINE accepts.
const (
	EngineRedis  = "redis"
	EngineValkey = "valkey"
)

// cacheEngine returns the engine the `redis` service runs: EngineValkey or
// EngineRedis. Used by the service and the volume list so they never disagree.
func (g *Generator) cacheEngine() string {
	switch strings.ToLower(strings.TrimSpace(g.cfg.Redis.Engine)) {
	case EngineValkey:
		return EngineValkey
	case EngineRedis:
		return EngineRedis
	}
	if pin, ok := ProjectPins()["redis"]; ok {
		if strings.Contains(repoOf(pin), "valkey") {
			return EngineValkey
		}
		return EngineRedis
	}
	// compat.V15(P7-LIVE-19): the default cache engine is redis -> valkey
	if compat.V15() {
		return EngineValkey
	}
	return EngineRedis
}

// cacheVolume is the named data volume of the cache engine.
func (g *Generator) cacheVolume() string {
	if g.cacheEngine() == EngineValkey {
		return "valkey_data"
	}
	return "redis_data"
}

// cacheImage renders the image of the `redis` service for the engine. A
// REDIS_VERSION that is the default of the other engine (the config default
// 7-alpine) is the engine's own default, never a tag to apply to its repository.
func (g *Generator) cacheImage(engine string) string {
	version := g.cfg.Redis.Version
	if engine == EngineValkey {
		if r, ok := LockedRef("redis"); ok && r.IsDefaultVersion(version) {
			version = ""
		}
		return ResolveImage("redis", ImageRef("valkey", version))
	}
	return ResolveImage("redis", ImageRef("redis", version))
}

// buildRedisService returns the cache/queue service configuration for the
// engine. Command and health check depend on whether a password is set; the
// valkey-* binaries are used by name (no reliance on redis-* symlinks).
func (g *Generator) buildRedisService() ServiceConfig {
	rc := g.cfg.Redis
	engine := g.cacheEngine()

	port := rc.Port
	if port == 0 {
		port = 6379
	}
	memory := rc.Memory
	if memory == "" {
		memory = "512M"
	}
	cpu := rc.CPU
	if cpu == "" {
		cpu = "0.5"
	}
	poolSize := rc.PoolSize
	if poolSize == 0 {
		poolSize = 50
	}

	server, cli := "redis-server", "redis-cli"
	if engine == EngineValkey {
		server, cli = "valkey-server", "valkey-cli"
	}
	var command interface{}
	var env map[string]string

	if rc.Password != "" {
		// D-0282: no argv carries the password. It reaches the container as
		// REDISCLI_AUTH (Env, like every other secret), which the engine's CLI
		// reads on its own (the health check needs no -a), and a shell expands it
		// for the server ($$ survives compose interpolation as $). Valkey's CLI
		// reads REDISCLI_AUTH too.
		env = map[string]string{"REDISCLI_AUTH": secretRef("REDIS_PASSWORD", rc.Password)}
		command = []string{"sh", "-c", fmt.Sprintf(
			`exec %s --appendonly yes --protected-mode yes --requirepass "$$REDISCLI_AUTH" --maxclients %d`,
			server, poolSize,
		)}
	} else {
		command = fmt.Sprintf(
			"%s --appendonly yes --protected-mode no --maxclients %d",
			server, poolSize,
		)
	}

	return ServiceConfig{
		Image:         g.cacheImage(engine),
		ContainerName: fmt.Sprintf("%s_redis", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		User:          "999:999",
		Networks:      []string{g.cfg.DockerNetwork},
		Environment:   env,
		Command:       command,
		Volumes:       []string{g.cacheVolume() + ":/data"},
		Ports:         []string{fmt.Sprintf("127.0.0.1:%d:6379", port)},
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD", cli, "ping"},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  5,
		},
		Deploy: &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimits{
					Memory: memory,
					CPUs:   cpu,
				},
			},
		},
	}
}
