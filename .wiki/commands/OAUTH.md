# nself oauth - OAuth Management CLI

Manage OAuth authentication providers in nself.

## Synopsis

```bash
nself oauth <subcommand> [options]
```

## Description

The `nself oauth` command provides a complete interface for managing OAuth 2.0 authentication providers in your nself project. It handles installation, configuration, testing, and management of OAuth handlers.

## Subcommands

### install

Install the OAuth handlers service.

```bash
nself oauth install
```

**What it does:**
- Copies OAuth handlers template to `services/oauth-handlers/`
- Sets up TypeScript OAuth handlers for Google, GitHub, Microsoft, and Slack
- Creates configuration files with placeholders
- Prepares service for Docker deployment

**Example:**
```bash
nself oauth install
```

---

### enable

Enable one or more OAuth providers.

```bash
nself oauth enable --providers <provider-list>
```

**Options:**
- `--providers=<list>` - Comma-separated list of providers (google,github,slack,microsoft)

**Example:**
```bash
# Enable single provider
nself oauth enable --providers google

# Enable multiple providers
nself oauth enable --providers google,github,slack
```

---

### disable

Disable one or more OAuth providers.

```bash
nself oauth disable --providers <provider-list>
```

**Options:**
- `--providers=<list>` - Comma-separated list of providers to disable

**Example:**
```bash
# Disable single provider
nself oauth disable --providers google

# Disable multiple providers
nself oauth disable --providers google,github
```

---

### config

Configure OAuth provider credentials.

```bash
nself oauth config <provider> [options]
```

**Arguments:**
- `<provider>` - Provider name (google, github, microsoft, slack)

**Options:**
- `--client-id=<id>` - OAuth application client ID (required)
- `--client-secret=<secret>` - OAuth application client secret (required)
- `--tenant-id=<id>` - Azure AD tenant ID (Microsoft only, optional)
- `--callback-url=<url>` - Custom callback URL (optional)

**Examples:**

Google:
```bash
nself oauth config google \
  --client-id=123456.apps.googleusercontent.com \
  --client-secret=GOCSPX-xxxxx
```

GitHub:
```bash
nself oauth config github \
  --client-id=Iv1.abcdef123456 \
  --client-secret=ghp_xxxxx
```

Microsoft (multi-tenant):
```bash
nself oauth config microsoft \
  --client-id=your-app-id \
  --client-secret=your-secret \
  --tenant-id=common
```

Microsoft (single tenant):
```bash
nself oauth config microsoft \
  --client-id=your-app-id \
  --client-secret=your-secret \
  --tenant-id=your-tenant-uuid
```

Slack:
```bash
nself oauth config slack \
  --client-id=123456.apps.slack.com \
  --client-secret=xxxxx
```

Custom callback URL:
```bash
nself oauth config google \
  --client-id=xxx \
  --client-secret=xxx \
  --callback-url=https://auth.yourdomain.com/oauth/google/callback
```

---

### test

Test OAuth provider configuration.

```bash
nself oauth test <provider>
```

**Arguments:**
- `<provider>` - Provider name to test

**What it checks:**
- Provider is enabled
- Client ID is configured
- Client secret is configured
- Callback URL is set (or has default)

**Example:**
```bash
nself oauth test google
```

**Output:**
```
✓ Client ID configured
✓ Client secret configured
✓ Callback URL: http://localhost:3100/oauth/google/callback
✓ google OAuth configuration is valid
ℹ Test the OAuth flow by visiting: http://localhost:3100/oauth/google
```

---

### list

List all OAuth providers and their status.

```bash
nself oauth list
```

**Example:**
```bash
nself oauth list
```

**Output:**
```
Available OAuth Providers:

  ✓ google (enabled)
  ✓ github (enabled)
  ○ microsoft
  ○ slack
```

---

### status

Show OAuth service status and configuration summary.

```bash
nself oauth status
```

**What it shows:**
- OAuth handlers service installation status
- Enabled providers
- Service running status

**Example:**
```bash
nself oauth status
```

**Output:**
```
OAuth Service Status

✓ OAuth handlers service installed at: /path/to/services/oauth-handlers

Available OAuth Providers:

  ✓ google (enabled)
  ✓ github (enabled)
  ○ microsoft
  ○ slack

✓ OAuth handlers service is running
```

---

## Configuration Files

The `nself oauth` commands modify the following files:

### .env.dev (or .env.local)

OAuth provider configuration is stored in environment files:

```env
# Google OAuth
OAUTH_GOOGLE_ENABLED=true
OAUTH_GOOGLE_CLIENT_ID=123456.apps.googleusercontent.com
OAUTH_GOOGLE_CLIENT_SECRET=GOCSPX-xxxxx
OAUTH_GOOGLE_CALLBACK_URL=http://localhost:3100/oauth/google/callback

# GitHub OAuth
OAUTH_GITHUB_ENABLED=true
OAUTH_GITHUB_CLIENT_ID=Iv1.abcdef123456
OAUTH_GITHUB_CLIENT_SECRET=ghp_xxxxx
OAUTH_GITHUB_CALLBACK_URL=http://localhost:3100/oauth/github/callback

# Microsoft OAuth
OAUTH_MICROSOFT_ENABLED=false
OAUTH_MICROSOFT_CLIENT_ID=
OAUTH_MICROSOFT_CLIENT_SECRET=
OAUTH_MICROSOFT_TENANT_ID=common
OAUTH_MICROSOFT_CALLBACK_URL=http://localhost:3100/oauth/microsoft/callback

# Slack OAuth
OAUTH_SLACK_ENABLED=false
OAUTH_SLACK_CLIENT_ID=
OAUTH_SLACK_CLIENT_SECRET=
OAUTH_SLACK_CALLBACK_URL=http://localhost:3100/oauth/slack/callback
```

---

## Workflow

### Initial Setup

```bash
# 1. Install OAuth handlers service
nself oauth install

# 2. Enable providers you want to use
nself oauth enable --providers google,github

# 3. Configure each provider
nself oauth config google --client-id=xxx --client-secret=xxx
nself oauth config github --client-id=xxx --client-secret=xxx

# 4. Test configuration
nself oauth test google
nself oauth test github

# 5. Build and start
nself build
nself start

# 6. Verify service is running
nself oauth status
```

### Adding a New Provider

```bash
# 1. Enable the provider
nself oauth enable --providers slack

# 2. Configure credentials
nself oauth config slack \
  --client-id=your-client-id \
  --client-secret=your-secret

# 3. Test configuration
nself oauth test slack

# 4. Restart services
nself restart
```

### Removing a Provider

```bash
# 1. Disable the provider
nself oauth disable --providers slack

# 2. Restart services
nself restart
```

### Updating Credentials

```bash
# Reconfigure with new credentials
nself oauth config google \
  --client-id=new-client-id \
  --client-secret=new-secret

# Restart services
nself restart
```

---

## Environment-Specific Configuration

### Development

Use `.env.dev` or `.env.local` for development credentials:

```bash
# Development OAuth apps with localhost callbacks
nself oauth config google \
  --client-id=dev-client-id \
  --client-secret=dev-secret \
  --callback-url=http://localhost:3100/oauth/google/callback
```

### Production

Use `.env.prod` for production credentials:

```bash
# Production OAuth apps with HTTPS callbacks
nself oauth config google \
  --client-id=prod-client-id \
  --client-secret=prod-secret \
  --callback-url=https://yourdomain.com/oauth/google/callback
```

Switch environments:
```bash
nself env switch prod
```

---

## Security Considerations

### Secrets Management

1. **Never commit secrets to git**
   - Use `.env.local` (gitignored)
   - Or use `nself secrets` for production

2. **Rotate secrets regularly**
   ```bash
   nself oauth config google \
     --client-id=existing-id \
     --client-secret=new-secret
   ```

3. **Use different credentials per environment**
   - Development: localhost callbacks
   - Production: HTTPS callbacks

### Callback URLs

Always use HTTPS in production:

```bash
# ❌ Bad - HTTP in production
nself oauth config google \
  --callback-url=http://yourdomain.com/oauth/google/callback

# ✅ Good - HTTPS in production
nself oauth config google \
  --callback-url=https://yourdomain.com/oauth/google/callback
```

---

## Troubleshooting

### "OAuth handlers service not installed"

**Solution:**
```bash
nself oauth install
```

### "Provider not enabled"

**Solution:**
```bash
nself oauth enable --providers google
```

### "Missing client credentials"

**Solution:**
```bash
nself oauth config google \
  --client-id=xxx \
  --client-secret=xxx
```

### Check service logs

```bash
docker logs oauth-handlers
```

### Check configuration

```bash
nself oauth status
nself oauth list
nself oauth test google
```

---

## See Also

- [OAuth Setup Guide](../guides/OAUTH-SETUP.md)
- [Frontend Integration](../guides/OAUTH-COMPLETE-FLOWS.md)
- [nself build](./build.md)
- [nself start](./start.md)

---

**Version:** nself v0.8.0+
**Last Updated:** January 30, 2026
