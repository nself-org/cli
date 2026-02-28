# GitHub Plugin

Sync GitHub repository data including repos, issues, pull requests, and workflow runs.

## Installation

```bash
nself plugin install github
```

## Configuration

### Required

```bash
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
```

### Optional

```bash
GITHUB_WEBHOOK_SECRET=xxxxxxxxxxxx         # Webhook signature verification
GITHUB_ORG=your-organization               # Sync all org repos
GITHUB_REPOS=owner/repo1,owner/repo2       # Or specific repos only
```

### Token Scopes

Required scopes when creating a Personal Access Token:
- `repo` - Full repository access
- `read:org` - Organization repository access (if syncing org repos)

## Usage

### Sync Data

```bash
# Full sync
nself plugin github sync

# Repos only
nself plugin github sync --repos-only

# Initial sync (runs automatically on install)
nself plugin github sync --initial
```

### Repositories

```bash
# List repositories
nself plugin github repos list

# Filter by language
nself plugin github repos list --language TypeScript

# Filter by org
nself plugin github repos list --org myorg

# Repository statistics
nself plugin github repos stats
```

### Issues

```bash
# List issues
nself plugin github issues list

# Open issues
nself plugin github issues open

# By repository
nself plugin github issues list --repo owner/repo

# By author
nself plugin github issues list --author username

# Statistics
nself plugin github issues stats
```

### Pull Requests

```bash
# List PRs
nself plugin github prs list

# Open PRs
nself plugin github prs open

# Merged PRs
nself plugin github prs merged

# PR details
nself plugin github prs show 123

# Statistics
nself plugin github prs stats
```

### GitHub Actions

```bash
# List workflow runs
nself plugin github actions list

# Failed runs
nself plugin github actions failed

# Filter by workflow
nself plugin github actions list --workflow "CI"

# Workflow statistics
nself plugin github actions stats
```

### Webhooks

```bash
# List events
nself plugin github webhook list

# Filter by event type
nself plugin github webhook list --event push

# Pending events
nself plugin github webhook pending

# Retry event
nself plugin github webhook retry <event-id>
```

## Webhook Setup

### Repository Webhooks

1. Go to Repository > Settings > Webhooks
2. Add webhook:
   - URL: `https://your-domain.com/webhooks/github`
   - Content type: `application/json`
   - Secret: Your `GITHUB_WEBHOOK_SECRET`
3. Select events:
   - Push
   - Pull requests
   - Issues
   - Workflow runs
   - Releases

### Organization Webhooks

For org-wide webhooks:
1. Go to Organization > Settings > Webhooks
2. Same configuration as repository webhooks

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `github_repositories` | Repository metadata |
| `github_issues` | Issues with labels, assignees |
| `github_pull_requests` | PRs with merge info |
| `github_commits` | Commit history |
| `github_releases` | Release tags |
| `github_workflow_runs` | GitHub Actions runs |
| `github_deployments` | Deployment status |
| `github_webhook_events` | Webhook event log |

### Views

| View | Description |
|------|-------------|
| `github_open_items` | Open issues/PRs by repo |
| `github_recent_activity` | Last 7 days activity |
| `github_workflow_stats` | Workflow success rates |

### Example Queries

```sql
-- Open issues by repository
SELECT
  r.full_name,
  COUNT(*) as open_issues
FROM github_issues i
JOIN github_repositories r ON i.repo_id = r.id
WHERE i.state = 'open'
GROUP BY r.full_name
ORDER BY open_issues DESC;

-- PR merge rate by author
SELECT
  user_login,
  COUNT(*) as total_prs,
  COUNT(*) FILTER (WHERE merged = true) as merged,
  ROUND(COUNT(*) FILTER (WHERE merged = true)::numeric / COUNT(*) * 100) as merge_rate
FROM github_pull_requests
GROUP BY user_login
ORDER BY total_prs DESC
LIMIT 20;

-- Workflow success rate
SELECT * FROM github_workflow_stats;
```

## Environment Handling

### Development

```bash
ENV=dev
GITHUB_TOKEN=ghp_dev_token
GITHUB_REPOS=myorg/dev-repo
```

### Production

```bash
ENV=prod
GITHUB_TOKEN=ghp_prod_token
GITHUB_ORG=myorg
```

Use different tokens or sync different repos per environment.

## Uninstall

```bash
# Remove plugin and data
nself plugin remove github

# Keep database tables
nself plugin remove github --keep-data
```

## Troubleshooting

### Authentication Errors

```bash
# Test token
curl -H "Authorization: Bearer ghp_xxx" https://api.github.com/user

# Check scopes
curl -I -H "Authorization: Bearer ghp_xxx" https://api.github.com/user
# Look for X-OAuth-Scopes header
```

### Rate Limiting

```bash
# Check rate limit
curl -H "Authorization: Bearer ghp_xxx" https://api.github.com/rate_limit

# Plugin uses 0.1s delay between requests to avoid limits
```

### Webhook Issues

```bash
# Check recent deliveries in GitHub webhook settings
# Verify GITHUB_WEBHOOK_SECRET matches

# Check nginx logs
docker logs <project>_nginx | grep webhook
```

## Related

- [Plugin Command](../commands/PLUGIN.md)
- [Database Command](../commands/DB.md)
- [GitHub API Docs](https://docs.github.com/en/rest)
