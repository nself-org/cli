# nself nginx - Multi-Project Shared Nginx Management

**Version 0.9.9** | Manage a shared nginx reverse proxy across multiple nself projects

---

## Overview

When running multiple nself projects on the same machine, each project normally starts its own nginx container. The `nself nginx` command provides an alternative: a single shared nginx container that aggregates routes from all registered projects. This avoids port conflicts, reduces resource usage, and gives you a unified entry point for all your local projects.

The shared nginx feature is opt-in. Projects that are not registered continue to use their own standalone nginx container as before.

---

## Usage

```bash
nself nginx <subcommand> [options]
```

---

## Subcommands

### `register`

Register a project with the shared nginx registry. Allocates a 20-port block and records the project's domains.

```bash
nself nginx register                       # Register current directory
nself nginx register --path /opt/backend   # Register specific project
```

**Requirements:**
- Project must have an `nginx/sites/` directory (run `nself build` first)
- Project must have a `.env` file with `PROJECT_NAME` and `BASE_DOMAIN`
- Project must not already be registered

**What happens:**
1. Validates the project directory
2. Extracts project name and base domain from `.env`
3. Allocates a 20-port block (starting at 10000)
4. Checks for subdomain conflicts against all registered projects
5. Adds the project to `~/.nself/nginx/registry.json`

---

### `unregister`

Remove a project from the shared nginx registry. Frees its allocated port range.

```bash
nself nginx unregister                     # Unregister current directory
nself nginx unregister --path /opt/backend # Unregister specific project
```

If the shared nginx container is running, you will see a reminder to run `nself nginx shared reload` to apply the change.

---

### `shared start`

Start the shared nginx container. Generates the aggregated configuration and launches a single `nginx:alpine` container that serves all registered projects.

```bash
nself nginx shared start
```

Fails if no projects are registered. Run `nself nginx register` first.

---

### `shared stop`

Stop the shared nginx container.

```bash
nself nginx shared stop
```

---

### `shared status`

Show the shared nginx container status and a table of all registered projects with their paths, base domains, and allocated port ranges.

```bash
nself nginx shared status
```

---

### `shared reload`

Reload the shared nginx configuration without restarting the container. Use this after registering or unregistering a project, or after running `nself build` in any registered project.

```bash
nself nginx shared reload
```

Fails if the shared nginx container is not running.

---

### `shared logs`

Tail the shared nginx container logs.

```bash
nself nginx shared logs                    # Last 100 lines
nself nginx shared logs --tail 50          # Last 50 lines
```

---

## Options

| Option | Description | Default |
| --- | --- | --- |
| `--path PATH` | Project path for register/unregister | Current directory |
| `--tail N` | Number of log lines for `shared logs` | 100 |
| `-h, --help` | Show help message | |

---

## How It Works

### Registry

All registered projects are stored in `~/.nself/nginx/registry.json`. Each entry tracks the project name, path, base domain, and allocated port range. The registry is created automatically on first use.

### Port Allocation

Each project gets a dedicated 20-port block starting at port 10000. When a project is unregistered, its port range is freed and can be reused by the next registration. The allocator fills gaps before extending.

| Project | Port Range |
| --- | --- |
| First registered | 10000-10019 |
| Second registered | 10020-10039 |
| Third registered | 10040-10059 |

### Conflict Detection

Before registration, the CLI parses `server_name` directives from all `nginx/sites/*.conf` files across all registered projects. If the new project claims a domain that is already owned by another registered project, registration is rejected with a clear error message.

### Build Integration

When a registered project runs `nself build`:
- `.env.computed` includes `NGINX_MODE=shared`
- The main `nginx.conf` is skipped (the shared container has its own)
- Site configs (`nginx/sites/*.conf`) are still generated (the shared container includes them)
- The per-project nginx service is excluded from `docker-compose.yml`

Unregistered projects are unaffected. Their builds produce standalone nginx as before.

### Shared Container

The shared nginx container:
- Name: `nself-shared-nginx`
- Image: `nginx:alpine`
- Ports: 80 and 443
- Config: `~/.nself/nginx/nginx.conf` (auto-generated)
- Compose file: `~/.nself/nginx/docker-compose.yml` (auto-generated)
- Mounts each registered project's `nginx/sites/` directory read-only

---

## Workflow

### First-time setup

```bash
# 1. Build your projects (generates nginx/sites/ configs)
cd ~/project-a && nself build
cd ~/project-b && nself build

# 2. Register both projects
nself nginx register --path ~/project-a
nself nginx register --path ~/project-b

# 3. Start the shared nginx
nself nginx shared start

# 4. Verify
nself nginx shared status
```

### After changing a project's .env

```bash
cd ~/project-a
# Edit .env (change BASE_DOMAIN, add services, etc.)
nself build                     # Regenerates site configs
nself nginx shared reload       # Shared nginx picks up new configs
```

### Removing a project

```bash
nself nginx unregister --path ~/project-a
nself nginx shared reload       # Apply the change
```

---

## Backward Compatibility

The shared nginx feature is entirely opt-in. By default, every nself project uses its own standalone nginx container. The shared mode is activated only when a project is explicitly registered via `nself nginx register`. Unregistered projects see no behavior change.

---

## Troubleshooting

### Shared nginx won't start

- Check that at least one project is registered: `nself nginx shared status`
- Check that registered projects have `nginx/sites/` directories
- Check Docker is running: `docker ps`

### Port conflicts

If another service is using port 80 or 443, stop it before starting the shared nginx. The shared container binds to the standard HTTP/HTTPS ports.

### Stale configs after build

Run `nself nginx shared reload` after any `nself build` in a registered project. The shared nginx needs to reload to pick up the regenerated site configs.

### Domain conflict on register

If two projects claim the same `server_name`, registration is rejected. Check which project owns the domain with `nself nginx shared status`, then adjust the conflicting project's `.env` (change `BASE_DOMAIN` or service routes).

---

## See Also

- [BUILD](BUILD.md) - Generate project configs
- [START](START.md) - Start project services
- [Command Tree](COMMAND-TREE-V1.md) - Full command hierarchy
