# Getting Started

Get ɳSelf running in under 5 minutes.

## Pages

| Page | What you'll learn |
|------|------------------|
| [[Installation]] | Install via Homebrew, curl, or from source |
| [[Quick-Start]] | init, build, start , your first stack in 2 minutes |
| [[First-Project]] | Guided walkthrough building a real backend |
| [[Upgrading-from-v1]] | Migration guide from the legacy Bash CLI |

## Prerequisites

- **Docker** 24+ with Docker Compose v2
- **macOS** (Intel or Apple Silicon) or **Linux** (x86_64 or ARM64)
- **2 GB RAM** minimum (4 GB recommended)
- **5 GB disk** free space (10 GB recommended)

## Quick Path

```bash
# Install
brew install nself-org/nself/nself

# Create a project
mkdir myproject && cd myproject
nself init

# Launch everything
nself start
```

That's it. You now have Postgres, Hasura GraphQL, Auth, and Nginx running locally with SSL.

See [[Quick-Start]] for the full walkthrough with expected output.

## What's Next

- [[First-Project]], Guided walkthrough building a real backend with tables, permissions, and auth
- [[Plugin-Overview]], Browse the free and licensed plugins for AI, video, commerce, CMS, and more
- [[Guide-Production-Deployment]], Deploy your stack to a real server with SSL and backups
- [[Commands]], Full reference for every top-level command and subcommand
