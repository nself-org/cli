# Documentation Quick Reference

**For**: nself contributors and maintainers
**Purpose**: Fast lookup of documentation standards

---

## Brand & Naming

| Context | Format | Example |
|---------|--------|---------|
| Code/Commands | `nself` | `nself start` |
| Titles/Branding | `ɳSelf` | `# ɳSelf Documentation` |
| Environment Vars | `NSELF_*` | `NSELF_VERSION=0.9.8` |

**Tagline**: "The complete self-hosted backend platform"

---

## Version Format

- **Current**: `0.9.8` (check `/src/VERSION`)
- **In prose**: `nself v0.9.9`
- **In badges**: `version-0.9.9-blue`
- **In links**: `[Release Notes](releases/v0.9.8.md)`

---

## Command Syntax

```bash
nself <command> <subcommand> [OPTIONS] [ARGUMENTS]

# Examples:
nself db migrate up
nself tenant create <tenant-name> --slug <slug>
```

**Rules**:
- Command: `nself` (lowercase only)
- Subcommands: `lowercase`
- Flags: `--kebab-case`

---

## Placeholders

| Type | Format | Example |
|------|--------|---------|
| Commands | `<kebab-case>` | `<project-name>` |
| Env Vars | `UPPERCASE` | `PROJECT_NAME` |
| Examples | Concrete | `myapp`, `acme` |

---

## Code Blocks

**Always use language identifier**:

````markdown
```bash
nself start
```

```sql
SELECT * FROM users;
```

```typescript
const user: User = { ... };
```
````

**Common identifiers**: `bash`, `sql`, `typescript`, `javascript`, `python`, `json`, `yaml`, `dbml`

---

## Headers

**Use sentence case**:

```markdown
✅ # Database workflow guide
✅ ## Creating your first migration

❌ # Database Workflow Guide
❌ ## Creating Your First Migration
```

**Exceptions**: Proper nouns (PostgreSQL, Docker, GitHub)

---

## Links

**Use relative paths**:

```markdown
✅ [Quick Start](getting-started/Quick-Start.md)
✅ [Commands](commands/README.md)

❌ [Quick Start](getting-started/Quick-Start.md)
❌ [Quick Start](https://github.com/.../docs/...)
```

---

## Standard Examples

| Type | Use |
|------|-----|
| Project | `myapp` |
| Domain | `example.com` |
| Email | `user@example.com` |
| Tenant | `acme`, `Acme Corp` |
| Database | `myapp_db` |

**Avoid**: `foo`, `bar`, `test123`, `asdf`

---

## Common Patterns

### Version Note

```markdown
> **v0.9.8:** Brief note. 
```

### Warning

```markdown
> **Warning**: This is destructive.
```

### Code with Comments

```bash
# Start all services
nself start

# Check status
nself status
```

---

## Pre-Commit Checklist

- [ ] Code blocks have language identifiers
- [ ] Links are relative paths
- [ ] Version is current (0.9.9)
- [ ] Brand name is `nself` in code
- [ ] Placeholders use `<kebab-case>`
- [ ] Headers use sentence case
- [ ] Examples use standard names

---

## Quick Links

- **[STYLE-GUIDE.md](STYLE-GUIDE.md)** - Full standards
- **[CONSISTENCY-AUDIT-REPORT.md](CONSISTENCY-AUDIT-REPORT.md)** - Analysis
- **[CONSISTENCY-FIX-GUIDE.md](CONSISTENCY-FIX-GUIDE.md)** - Fix scripts

---

<div align="center">

**Keep this handy when writing docs!**

</div>
