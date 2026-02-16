# nself Documentation Style Guide

**Version**: 1.0
**Last Updated**: January 31, 2026
**Applies To**: All markdown documentation in `/docs`

This style guide ensures consistency across all 400+ documentation files in the nself project.

---

## Table of Contents

1. [Brand & Terminology](#brand--terminology)
2. [Version References](#version-references)
3. [Command Syntax](#command-syntax)
4. [Placeholders](#placeholders)
5. [Code Blocks](#code-blocks)
6. [Headers & Formatting](#headers--formatting)
7. [Links & References](#links--references)
8. [Tables](#tables)
9. [Tone & Voice](#tone--voice)
10. [File Organization](#file-organization)

---

## Brand & Terminology

### Official Name

**Use**: `nself` (lowercase, no space)

**Symbol**: `ɳSelf` (Unicode ɳ + capital S)
- Use `ɳSelf` in:
  - Page titles
  - Hero sections
  - Marketing copy
  - Visual branding

**DO NOT USE**:
- ❌ NSelf
- ❌ Nself
- ❌ NSELF (except in environment variables)
- ❌ n-self
- ❌ n_self

### Project Description

**Official tagline**: "The complete self-hosted backend platform"

**DO NOT USE**:
- ❌ "Nhost Self-Hosted" (outdated)
- ❌ "Self-hosted Nhost" (outdated)
- ❌ Any reference to being "based on Nhost"

**Correct positioning**: nself is an **independent platform**, not a fork or derivative.

### Comparisons

When comparing to other platforms:

**Correct**:
```markdown
nself provides similar features to Supabase, Nhost, or Firebase, but runs entirely on your own infrastructure.
```

**Incorrect**:
```markdown
nself is a self-hosted version of Nhost.
nself is based on Nhost's architecture.
```

---

## Version References

### Current Version

**Always use**: The version from `/src/VERSION`

**Current version**: `0.9.8` (as of February 16, 2026)

### Version Format

- Use semantic versioning: `vX.Y.Z`
- Include `v` prefix in prose: "nself v0.9.9"
- Omit `v` in badges: `version-0.9.9-blue`

### Updating Versions

When a new version is released, update:

1. **Badge references**:
```markdown
[![Version](https://img.shields.io/badge/version-0.9.9-blue.svg)](releases/v0.9.8.md)
```

2. **Release notes links**:
```markdown
**[View v0.9.8 Release Notes](releases/v0.9.8.md)**
```

3. **Version callouts**:
```markdown
> **v0.9.8 Note:** Commands consolidated. See migration guide.
```

### Version Note Format

Use this format for version-specific notes:

```markdown
> **v0.9.8:** Brief note about change. 
```

**Example**:
```markdown
> **v0.9.8:** Environment commands moved to `nself config env`. Old syntax still works.
```

---

## Command Syntax

### Format

All command examples must follow this format:

```markdown
nself <command> <subcommand> [OPTIONS] [ARGUMENTS]
```

### Command Case

- Commands: **lowercase**
- Flags: **lowercase with dashes**
- Arguments: **UPPERCASE or descriptive**

**Correct**:
```bash
nself db migrate up
nself tenant create "Acme Corp" --slug acme --plan pro
nself deploy prod --dry-run
```

**Incorrect**:
```bash
Nself DB migrate up
nself Tenant Create "Acme Corp"
nself deploy PROD
```

### Placeholder Arguments

Use **descriptive lowercase** or **UPPERCASE** for placeholders:

**Preferred** (descriptive):
```bash
nself tenant create <tenant-name>
nself db query <sql-query>
nself service logs <service-name>
```

**Acceptable** (UPPERCASE):
```bash
nself tenant create TENANT_NAME
nself db query SQL_QUERY
```

**Choose ONE style per file** - don't mix.

### Optional vs Required

- **Required arguments**: `<argument>`
- **Optional arguments**: `[argument]`
- **Optional flags**: `[--flag]`

**Example**:
```bash
nself db migrate <direction> [steps] [--dry-run]
```

### Flag Documentation

When documenting flags:

```markdown
| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would happen without executing |
| `--force` | Skip confirmation prompts |
| `--verbose` | Show detailed output |
```

---

## Placeholders

### Environment Variables

**Use**: `UPPERCASE_WITH_UNDERSCORES`

```bash
PROJECT_NAME=myapp
POSTGRES_PASSWORD=secure123
MONITORING_ENABLED=true
```

### File Paths

**Use**: Descriptive paths with angle brackets

````markdown
Edit `.env` and set:
```bash
CUSTOM_SERVICE_1=api:express-js:8001
```
````

### Generic Examples

**Use**: Consistent example names across all docs

| Type | Example |
|------|---------|
| **Project name** | `myapp` |
| **Domain** | `example.com` |
| **Email** | `user@example.com` |
| **Tenant** | `acme` or `Acme Corp` |
| **User** | `john`, `admin` |
| **Database** | `myapp_db` |

**DO NOT use**:
- ❌ `foo`, `bar`, `baz`
- ❌ `test123`, `asdf`
- ❌ Real company names (except in real examples)

---

## Code Blocks

### Language Identifier

**Always specify** the language for syntax highlighting:

**Correct**:
````markdown
```bash
nself start
```

```sql
SELECT * FROM users;
```

```typescript
const user: User = { id: 1, email: 'test@example.com' };
```
````

**Incorrect**:
````markdown
```
nself start
```
````

### Supported Languages

Use these identifiers:

- `bash` - Shell commands
- `sql` - SQL queries
- `typescript` - TypeScript code
- `javascript` - JavaScript code
- `json` - JSON data
- `yaml` - YAML configs
- `dockerfile` - Dockerfiles
- `nginx` - Nginx configs
- `python` - Python code
- `go` - Go code
- `dbml` - Database markup

### Comments in Code

Use comments to explain non-obvious parts:

```bash
# Start all services
nself start

# Wait for health checks (default 120s)
nself health check
```

### Multi-Line Commands

Break long commands at logical points:

```bash
nself tenant create "Acme Corp" \
  --slug acme \
  --plan pro \
  --domain acme.example.com
```

---

## Headers & Formatting

### Header Hierarchy

```markdown
# Page Title (H1) - ONE per file

## Major Section (H2)

### Subsection (H3)

#### Detail Section (H4)
```

**Rules**:
1. Only ONE H1 per file
2. Don't skip levels (H2 → H4)
3. Use sentence case, not Title Case
4. No punctuation at end

**Correct**:
```markdown
# Database workflow guide

## Setting up migrations

### Creating your first migration
```

**Incorrect**:
```markdown
# Database Workflow Guide.

### Setting Up Migrations (skipped H2)
```

### Emphasis

- **Bold** for emphasis: `**important**`
- *Italic* for subtle emphasis: `*note*`
- `Code` for technical terms: `` `nself` ``

**Correct**:
```markdown
The **database** must be running before you run `nself db migrate up`.
```

### Lists

**Unordered lists**:
```markdown
- Item one
- Item two
  - Nested item
- Item three
```

**Ordered lists**:
```markdown
1. First step
2. Second step
3. Third step
```

**Checklist**:
```markdown
- [ ] Not done
- [x] Completed
```

---

## Links & References

### Internal Links

Use **relative paths** from current file:

```markdown
[Quick Start](getting-started/Quick-Start.md)
[Database Commands](commands/DB.md)
[View Architecture](../architecture/ARCHITECTURE.md)
```

**DO NOT use**:
- ❌ Absolute paths: `/do../getting-started/Quick-Start.md`
- ❌ URLs: `https://github.com/acamarata/nself/blob/main/docs/...`

### External Links

Use descriptive link text:

```markdown
[Install Docker](https://docs.docker.com/get-docker/)
[PostgreSQL Documentation](https://postgresql.org/docs)
```

**Avoid**:
```markdown
Click [here](https://example.com) to learn more.
```

### Command References

When referencing commands:

```markdown
See the `nself db migrate` command for details.
```

Link to command docs when first mentioned:

```markdown
Use [nself db migrate](commands/DB.md#migrations) to run migrations.
```

---

## Tables

### Table Format

Use aligned tables for readability:

```markdown
| Command | Description |
|---------|-------------|
| `nself start` | Start all services |
| `nself stop` | Stop all services |
```

### Table Alignment

- **Left-align** text columns
- **Right-align** numeric columns
- **Center-align** status/icon columns

```markdown
| Service | Port | Status |
|---------|-----:|:------:|
| PostgreSQL | 5432 | ✅ |
| Hasura | 8080 | ✅ |
```

### Complex Tables

For tables with code or long content, keep it simple:

```markdown
| Variable | Example |
|----------|---------|
| `PROJECT_NAME` | `myapp` |
| `BASE_DOMAIN` | `local.nself.org` |
```

---

## Tone & Voice

### Writing Style

- **Clear and concise** - Avoid unnecessary words
- **Active voice** - "Run the command" not "The command should be run"
- **Direct** - Address reader as "you"
- **Helpful** - Anticipate questions and confusion

### Example Comparisons

**Good**:
```markdown
Run `nself start` to launch all services.
```

**Bad**:
```markdown
The services can be started by executing the start command.
```

### Technical Accuracy

- Be precise with technical terms
- Don't oversimplify complex concepts
- Include warnings for dangerous operations

**Example**:
```markdown
> **Warning**: `nself db reset` will **permanently delete** all data.
> This cannot be undone. Always backup first.
```

### Assumptions

**State prerequisites clearly**:

```markdown
## Prerequisites

Before starting, ensure you have:
- Docker Desktop installed
- At least 4GB RAM available
- Port 5432 not in use
```

---

## File Organization

### File Naming

- Use **descriptive names**: `DATABASE-WORKFLOW.md` not `db.md`
- Use **UPPERCASE** for major guides: `README.md`, `Quick-Start.md`
- Use **kebab-case** for multi-word: `getting-started.md`
- Use **PascalCase** for some titles: `Quick-Start.md`

**Be consistent within a directory**.

### Directory Structure

```
docs/
├── README.md                 # Main documentation index
├── getting-started/          # New user docs
├── guides/                   # Task-focused tutorials
├── commands/                 # CLI reference
├── configuration/            # Config guides
├── deployment/               # Production deployment
├── architecture/             # System design
├── services/                 # Service documentation
├── security/                 # Security audits & guides
└── releases/                 # Release notes & roadmap
```

### Index Files

Every directory should have an `INDEX.md` or `README.md`:

```markdown
# Directory Name

Brief description of what's in this directory.

## Contents

- [File 1](database.md) - Description
- [File 2](users.md) - Description
```

---

## Frontmatter

### Optional Metadata

For migration guides and major docs:

```markdown
---
title: Migrating from Firebase
difficulty: High
time_estimate: 16-32 hours
last_updated: 2026-01-31
---
```

### Inline Metadata

For simpler docs, use a header:

```markdown
# Guide Title

**Last Updated**: January 31, 2026
**Version**: 0.9.8
**Difficulty**: Medium
```

---

## Special Sections

### Warnings

```markdown
> **Warning**: This is a destructive operation.
```

### Notes

```markdown
> **Note**: Additional context or clarification.
```

### Version Notes

```markdown
> **v0.9.8**: Command syntax has changed. See migration guide.
```

### Tips

```markdown
> **Tip**: Use `--dry-run` to preview changes.
```

---

## Examples

### Good Documentation Example

````markdown
# Database Migrations

Manage your database schema changes with versioned migrations.

## Quick Start

Run all pending migrations:

```bash
nself db migrate up
```

Create a new migration:

```bash
nself db migrate create add_user_preferences
```

## Migration Files

Migrations are stored in `nself/migrations/`:

```
nself/migrations/
├── 001_create_users.sql
├── 002_add_preferences.sql
└── 003_create_orders.sql
```

> **Warning**: Running migrations in production requires extra caution.
> Always backup your database first.

## Next Steps

- [Seeding Data](DB.md#seeding)
- [Backup & Restore](../guides/BACKUP-RECOVERY.md)
````

---

## Checklist for New Documentation

Before submitting new docs:

- [ ] Brand name is `nself` (lowercase) in code/commands
- [ ] Brand name is `ɳSelf` in titles where appropriate
- [ ] Version is current (`0.9.8` as of Feb 2026)
- [ ] All code blocks have language identifiers
- [ ] Placeholders are consistent
- [ ] Commands use correct syntax
- [ ] Internal links use relative paths
- [ ] Headers follow hierarchy (H1 → H2 → H3)
- [ ] Tables are properly formatted
- [ ] Tone is clear and helpful
- [ ] No "Nhost Self-Hosted" references
- [ ] Examples use standard names (`myapp`, `example.com`)

---

## Enforcement

This style guide should be:

1. **Applied** to all new documentation
2. **Referenced** in PR reviews
3. **Used** to refactor existing docs gradually
4. **Updated** as standards evolve

---

<div align="center">

**nself Documentation Style Guide v1.0**

*Ensuring consistency across 400+ documentation files*

</div>
