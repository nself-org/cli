# nself history - Operation Audit Trail

**Version 0.4.6** | Deployment and operation history

---

## Overview

The `nself history` command provides a comprehensive audit trail for deployments, migrations, rollbacks, and command executions. Track who did what, when, and to which environment.

---

## Usage

```bash
nself history [subcommand] [options]
```

---

## Subcommands

### `show` (default)

Show recent history.

```bash
nself history                   # Recent history
nself history show              # Same as above
```

### `deployments`

Show deployment history.

```bash
nself history deployments           # All deployments
nself history deployments --limit 50  # Last 50
```

### `migrations`

Show database migration history.

```bash
nself history migrations
```

### `rollbacks`

Show rollback history.

```bash
nself history rollbacks
```

### `commands`

Show command execution history.

```bash
nself history commands          # Recent commands
nself history commands --limit 100  # Last 100
```

### `search <query>`

Search history.

```bash
nself history search "migration"
nself history search "prod"
nself history search "failed"
```

### `export`

Export history to file.

```bash
nself history export              # JSON export
nself history export --csv        # CSV export
nself history export --output history.json
```

### `clear`

Clear history (with confirmation).

```bash
nself history clear               # Interactive
nself history clear --force       # No confirmation
```

---

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--limit N` | Number of entries | 20 |
| `--since DATE` | Show entries since date | - |
| `--until DATE` | Show entries until date | - |
| `--env NAME` | Filter by environment | all |
| `--type TYPE` | Filter by type | all |
| `--json` | Output in JSON format | false |
| `--csv` | Output in CSV format | false |
| `--output FILE` | Export to file | - |
| `--force` | Skip confirmation | false |
| `-h, --help` | Show help message | - |

---

## Event Types

| Type | Description |
|------|-------------|
| `deploy` | Deployment event |
| `migration` | Database migration |
| `rollback` | Rollback operation |
| `command` | CLI command execution |

---

## Examples

```bash
# View recent history
nself history

# Filter by environment
nself history --env prod

# Filter by type
nself history --type deploy

# Search for specific events
nself history search "production"

# Export as JSON
nself history export --json > history.json

# Export as CSV
nself history export --csv --output history.csv

# Clear old history
nself history clear
```

---

## Output Example

### Table Format

```
  ➞ Operation History

  Timestamp            Type         Env        Status     Description
  ---------            ----         ---        ------     -----------
  2026-01-23 10:30:00  deploy       staging    success    Deploy to staging
  2026-01-23 10:25:00  migration    local      success    Run migration 003
  2026-01-23 10:20:00  command      local      success    nself db backup

  ℹ Showing last 20 of 156 entries
```

### JSON Format

```json
{
  "history": [
    {
      "timestamp": "2026-01-23T10:30:00Z",
      "type": "deploy",
      "env": "staging",
      "status": "success",
      "description": "Deploy to staging",
      "user": "admin"
    }
  ],
  "count": 156
}
```

---

## History Storage

History is stored in `.nself/history/`:

```
.nself/history/
├── all.jsonl           # All events
├── deployments.jsonl   # Deployment events
├── migrations.jsonl    # Migration events
├── rollbacks.jsonl     # Rollback events
└── commands.jsonl      # Command events
```

---

## Recording Events

Events are automatically recorded by nself commands. To record custom events:

```bash
# In scripts, source and use record_event
source /path/to/history.sh
record_event "custom" "Description" "staging" "success"
```

---

## Data Retention

By default, history is retained indefinitely. To manage size:

```bash
# Export and archive old history
nself history export --json > archive-$(date +%Y%m).json

# Clear history
nself history clear
```

---

## Related Commands

- [deploy](DEPLOY.md) - Deployment
- [db migrate](DB.md) - Database migrations
- [rollback](ROLLBACK.md) - Rollback operations

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
