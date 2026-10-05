# Custom Services Migration

Moving a hand-written compose fragment (`docker-compose.<name>.yml`, `docker-compose.override.yml`) to `CS_N` keys, so that `nself build` owns the only compose file. Each pattern below names the keys that replace it. Patterns that `CS_N` cannot express yet are listed as gaps with a debt id.

The key reference is in [[Config-Custom-Services]]. This page is the map from compose to keys.

## Patterns that map to keys

| Compose pattern | `CS_N` keys |
|-----------------|-------------|
| `build.context` at the monorepo root, `dockerfile` deep inside | `CS_N_PATH=../..` and `CS_N_DOCKERFILE=<path from the root>`. The root needs a `.dockerignore` with `**/.env*` and `**/.secrets`. |
| Multi-stage `build.target` | `CS_N_BUILD_TARGET=<stage>` |
| `env_file` with two or more files | `CS_N_ENV_FILE=.env.dev,.env.secrets` (later wins) |
| `depends_on` a core service with a health condition | `CS_N_DEPENDS_ON=hasura:healthy` (`started`, `healthy`, `completed`) |
| `depends_on` a service from a plugin | `CS_N_DEPENDS_ON=<plugin service name>`, resolved after the plugin step |
| `command` as a list | `CS_N_COMMAND=node dist/server.js` (whitespace split, no shell) |
| `networks` with an existing project network | `CS_N_NETWORKS=<PROJECT_NAME>_<name>` (external, must exist) |
| `image` pinned by tag or digest | `CS_N_IMAGE=...` |
| Read-only config mounted from the project | `CS_N_VOLUMES=./conf/app.conf:/etc/app.conf:ro` |
| Healthcheck on another path or with a command | `CS_N_HEALTHCHECK=/path` or `CMD ...` |
| Memory and CPU limits | `CS_N_MEMORY`, `CS_N_CPU` |
| Variables from the project `.env` | `CS_N_ENV_PASSTHROUGH=A,B` |
| Several containers of one third-party stack | One `CS_N_IMAGE` service per container, or a plugin |
| A one-shot init job that must finish first | A `CS_N_IMAGE` service with `CS_N_COMMAND`; the other service uses `CS_N_DEPENDS_ON=<job>:completed`. The restart policy gap below applies. |

## Rules that changed

- A custom service never joins a network that does not start with `<PROJECT_NAME>_` (E501). A fragment that creates its own isolated networks does not map; see gap 6.
- An unknown dependency name fails the build (E500), naming the key and the name.
- An ancestor build context above the repository root, or without an ignore file that excludes `**/.env*` and `**/.secrets`, fails (E528). A dependency cycle between services fails (E500).
- v1.5 mode (`NSELF_V15=1`): a writable absolute host bind outside the project, and any bind of `/var/run/docker.sock` or `/`, fail with E502. v1.4 mode warns once for the socket and host root. See [[Compat-V15]].

## Gaps

These compose features have no `CS_N` key. Keep them out of fragments: move the service to the closest pattern above and accept the default, or wait for the debt item.

| Gap | Where it shows up | Debt id |
|-----|-------------------|---------|
| Per-service `logging` options, restart policy (`restart: "no"` for one-shot jobs), `container_name` and `image:` tag for a built service | Every fragment repeats the same `json-file` rotation; init jobs need `restart: "no"` | D-A01-1 |
| `extra_hosts`, `tmpfs`, `entrypoint`, `working_dir`, `user` | Proxy and tool containers | D-A01-2 |
| Declaring a named volume used by `CS_N_VOLUMES` (`data:/var/lib/x`) at the top level of the generated compose | Databases, brokers and replicas that keep state | D-A01-3 |
| Overriding a core service (nginx, hasura) from outside its own config keys | Host-gateway entries, entrypoint delays, per-service memory reservations | D-A01-4 |
| Variable expansion inside `CS_N_COMMAND` (it has no shell) | A command that carries a password | D-A01-5 |
| Stacks with their own isolated networks and many containers (an observability suite) | Ten or more services, two private networks | D-A01-6 (plugin) |

Until the debt items land, a service that needs one of these stays a `CS_N_IMAGE` or build service without that feature, or becomes a plugin.

## Checking a migration

1. Set the keys, then run `nself build`. It fails with E500, E501, E502 or E528 and a message that names the key.
2. Compare the generated service with the fragment you are replacing. The generated container name is `<PROJECT_NAME>_<service>`, with the service name hyphenated, so update anything that referenced the old container name (nginx upstreams, monitoring).
3. Remove the fragment and run `nself start`.

```bash
nself build
nself start
```
