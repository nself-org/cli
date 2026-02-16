# OAuth Complete Implementation Guide

**Version:** nself v0.8.0+
**Last Updated:** January 30, 2026

Complete guide to OAuth authentication flows in nself, including provider management, token refresh, account linking, and advanced features.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Quick Start](#quick-start)
4. [Provider Configuration](#provider-configuration)
5. [Authentication Flows](#authentication-flows)
6. [Token Management](#token-management)
7. [Account Linking](#account-linking)
8. [Database Schema](#database-schema)
9. [CLI Commands](#cli-commands)
10. [Security Best Practices](#security-best-practices)
11. [Troubleshooting](#troubleshooting)

---

## Overview

nself provides **complete OAuth 2.0 / OpenID Connect authentication** with:

- **13 OAuth providers** - Google, GitHub, Microsoft, Facebook, Apple, Twitter, LinkedIn, Discord, Twitch, Spotify, GitLab, Bitbucket, Slack
- **Multi-provider support** - Users can link multiple OAuth accounts (e.g., Google + GitHub)
- **Automatic token refresh** - Background service refreshes tokens before expiry
- **Account merging** - Combine OAuth providers from different accounts
- **Complete audit trail** - Track all OAuth events

### Feature Parity with Nhost/Supabase

nself matches or exceeds Nhost/Supabase OAuth capabilities:

| Feature | Nhost | Supabase | nself |
|---------|-------|----------|-------|
| OAuth Providers | 10+ | 15+ | 13 |
| Token Refresh | ✅ | ✅ | ✅ |
| Multi-Provider | ⚠️ DIY | ✅ | ✅ |
| Account Linking | ⚠️ DIY | ✅ | ✅ |
| Token Rotation | ✅ | ✅ | ✅ |
| Audit Logging | ⚠️ Basic | ✅ | ✅ |
| CLI Management | ⚠️ Limited | ⚠️ Limited | ✅ Complete |

---

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         nself OAuth Stack                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌────────────┐    ┌──────────────┐    ┌─────────────────┐    │
│  │  Frontend  │───▶│ OAuth        │───▶│  OAuth Provider │    │
│  │            │◀───│ Handlers     │◀───│  (Google, etc.) │    │
│  └────────────┘    └──────────────┘    └─────────────────┘    │
│                           │                                      │
│                           ▼                                      │
│                    ┌──────────────┐                             │
│                    │  PostgreSQL  │                             │
│                    │  auth.*      │                             │
│                    └──────────────┘                             │
│                           │                                      │
│                           ▼                                      │
│                    ┌──────────────┐                             │
│                    │ Token Refresh│                             │
│                    │ Background   │                             │
│                    │ Service      │                             │
│                    └──────────────┘                             │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Database Tables

- `auth.oauth_provider_accounts` - User OAuth connections
- `auth.oauth_states` - CSRF protection state storage (10-min TTL)
- `auth.oauth_token_refresh_queue` - Token refresh scheduling
- `auth.oauth_providers` - Provider metadata
- `auth.oauth_audit_log` - Authentication event tracking

---

## Quick Start

### 1. Install OAuth System

```bash
# Apply database migrations
psql $DATABASE_URL < src/lib/auth/oauth-db-migrations.sql

# Install OAuth handlers service
nself oauth install

# Enable providers
nself oauth enable --providers google,github,microsoft

# Configure Google
nself oauth config google \
  --client-id=YOUR_CLIENT_ID.apps.googleusercontent.com \
  --client-secret=GOCSPX-YOUR_SECRET

# Build and start
nself build
nself start

# Start token refresh service
nself oauth refresh start
```

### 2. Test OAuth Flow

```bash
# Open in browser
http://localhost:3100/oauth/google

# Or test configuration
nself oauth test google
```

### 3. Frontend Integration

```typescript
// React/Next.js example
const handleLogin = (provider: string) => {
  window.location.href = `http://localhost:3100/oauth/${provider}?redirect=/dashboard`;
};

// Handle callback
useEffect(() => {
  const params = new URLSearchParams(window.location.search);
  const token = params.get('token');

  if (token) {
    localStorage.setItem('authToken', token);
    // Redirect to dashboard
  }
}, []);
```

---

## Provider Configuration

### Supported Providers

| Provider | Scopes | Token Refresh | PKCE |
|----------|--------|---------------|------|
| Google | `openid profile email` | ✅ | No |
| GitHub | `read:user user:email` | ❌ | No |
| Microsoft | `openid profile email` | ✅ | No |
| Facebook | `public_profile email` | ✅ | No |
| Apple | `name email` | ✅ | Yes |
| Twitter/X | `tweet.read users.read` | ✅ | Yes |
| LinkedIn | `r_liteprofile r_emailaddress` | ✅ | No |
| Discord | `identify email` | ✅ | No |
| Twitch | `user:read:email` | ✅ | No |
| Spotify | `user-read-email user-read-private` | ✅ | No |
| GitLab | `read_user email` | ✅ | No |
| Bitbucket | `account email` | ✅ | No |
| Slack | `openid profile email` | ✅ | No |

### Configuration Commands

```bash
# Enable providers
nself oauth enable --providers google,github,slack

# Disable providers
nself oauth disable --providers facebook

# Configure provider
nself oauth config <provider> \
  --client-id=<id> \
  --client-secret=<secret> \
  [--callback-url=<url>] \
  [--tenant-id=<id>]  # Microsoft only

# Test provider
nself oauth test <provider>

# List enabled providers
nself oauth list

# Show service status
nself oauth status
```

---

## Authentication Flows

### Standard OAuth Flow

1. **User initiates login**
   ```
   GET /oauth/google
   ```

2. **OAuth Handlers generates authorization URL**
   - Creates state parameter (CSRF protection)
   - Stores state in database (10-minute TTL)
   - Redirects to provider

3. **User authorizes on provider**
   - User logs in to Google/GitHub/etc.
   - User grants permissions
   - Provider redirects back with code

4. **OAuth Handlers processes callback**
   ```
   GET /oauth/google/callback?code=xxx&state=yyy
   ```
   - Verifies state (CSRF protection)
   - Exchanges code for access token
   - Fetches user profile
   - Creates/updates user in database
   - Stores OAuth tokens
   - Generates JWT
   - Redirects to frontend with token

5. **Frontend receives JWT**
   ```
   http://localhost:3000/dashboard?token=<jwt>
   ```

### Mobile App Flow (Token Exchange)

For mobile apps that can't handle redirects:

```bash
# Mobile app gets authorization code
# Then exchanges it directly:

POST /oauth/google/token
Content-Type: application/json

{
  "code": "authorization_code_from_provider"
}

# Response:
{
  "token": "jwt_token",
  "user": {
    "id": "user_uuid",
    "email": "user@example.com",
    "displayName": "John Doe",
    "avatarUrl": "https://..."
  }
}
```

### Account Linking Flow

Link additional OAuth provider to existing user:

1. User is already authenticated with JWT
2. Frontend initiates link request:
   ```
   GET /oauth/github?link_to=<user_id>
   ```
3. User authorizes GitHub
4. OAuth Handlers links GitHub to existing account
5. User can now login with either Google or GitHub

---

## Token Management

### Automatic Token Refresh

nself automatically refreshes OAuth tokens before they expire.

#### Start Refresh Service

```bash
# As daemon (continuous background process)
nself oauth refresh start

# Or run once (for cron)
nself oauth refresh once

# Add to crontab
*/5 * * * * /usr/local/bin/nself oauth refresh once

# Check status
nself oauth refresh status

# Stop service
nself oauth refresh stop
```

#### How It Works

1. When tokens are stored, expiration time is calculated
2. Refresh is scheduled 5 minutes before expiry
3. Background service processes refresh queue
4. New tokens are stored
5. Old refresh tokens are invalidated
6. Failed refreshes are retried (max 3 attempts)

#### Configuration

```env
# Refresh service configuration
OAUTH_REFRESH_CHECK_INTERVAL=300  # Check every 5 minutes
OAUTH_MAX_REFRESH_ATTEMPTS=3      # Max retry attempts
OAUTH_REFRESH_WINDOW_MINUTES=5    # Refresh 5 min before expiry
OAUTH_REFRESH_LOG_FILE=/var/log/nself/oauth-refresh.log
```

### Manual Token Operations

```bash
# Check token status for user
nself oauth accounts <user_id>

# Force refresh tokens
psql $DATABASE_URL -c "
  UPDATE auth.oauth_token_refresh_queue
  SET scheduled_at = NOW()
  WHERE oauth_account_id IN (
    SELECT id FROM auth.oauth_provider_accounts WHERE user_id = '<user_id>'
  );
"

# Clear failed refresh attempts
psql $DATABASE_URL -c "
  UPDATE auth.oauth_token_refresh_queue
  SET attempts = 0, error_message = NULL
  WHERE attempts >= max_attempts;
"
```

---

## Account Linking

### Link Multiple Providers

Users can link multiple OAuth providers to a single account.

#### Example Use Cases

- User signs up with Google, later links GitHub
- User has work (Microsoft) and personal (Google) accounts
- Developer links GitHub, GitLab, and Bitbucket

#### Link Provider

```bash
# Via CLI
nself oauth link <user_id> <provider>

# Via API
GET /oauth/github?link_to=<user_id>
```

#### Unlink Provider

```bash
# Via CLI
nself oauth unlink <user_id> <provider>

# Via API (from authenticated session)
POST /api/oauth/unlink
{
  "provider": "github"
}
```

#### Safety Rules

1. **Cannot unlink last auth method**
   - User must have password OR at least one OAuth provider
   - Error if trying to unlink only provider without password

2. **Cannot link same provider twice**
   - Each user can have max 1 account per provider
   - Error if provider already linked

3. **Provider account cannot be shared**
   - Each provider account can only link to one user
   - Error if provider account is linked elsewhere

#### List Linked Providers

```bash
# Via CLI
nself oauth accounts <user_id>

# Output:
Linked OAuth Providers:

  google
    Email: user@gmail.com
    Linked: 2026-01-15 10:30:00
    Token expires: 2026-01-15 11:30:00

  github
    Email: user@users.noreply.github.com
    Linked: 2026-01-20 14:00:00
```

### Account Merging

Combine OAuth providers from two different user accounts.

#### Use Case

User accidentally created two accounts:
- Account A: Signed up with Google
- Account B: Signed up with GitHub
- Want to merge into single account

#### Merge Accounts

```bash
# Merge Account B → Account A
nself oauth accounts merge <from_user_id> <to_user_id>

# Example
nself oauth accounts merge \
  abc123-def456-ghi789 \
  xyz789-uvw456-rst123
```

#### What Gets Merged

- ✅ OAuth provider accounts transferred
- ✅ OAuth audit logs transferred
- ⚠️ Source account still exists (no deletion)
- ℹ️ Manual cleanup may be needed for user data

#### Merge Rules

1. **No provider conflicts**
   - Cannot merge if both accounts have same provider
   - Must unlink conflicting provider first

2. **Accounts must exist**
   - Both user IDs must be valid

3. **Cannot merge account with itself**
   - Source and target must be different

---

## Database Schema

### Tables

#### auth.oauth_provider_accounts

Stores OAuth connections for users.

```sql
CREATE TABLE auth.oauth_provider_accounts (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  provider VARCHAR(50) NOT NULL,
  provider_user_id VARCHAR(255) NOT NULL,
  provider_account_email VARCHAR(255),
  access_token TEXT,
  refresh_token TEXT,
  token_expires_at TIMESTAMPTZ,
  id_token TEXT,
  scopes TEXT[],
  raw_profile JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

  UNIQUE(provider, provider_user_id),
  UNIQUE(user_id, provider)
);
```

#### auth.oauth_states

Temporary state storage for CSRF protection (10-minute TTL).

```sql
CREATE TABLE auth.oauth_states (
  id UUID PRIMARY KEY,
  state VARCHAR(64) UNIQUE NOT NULL,
  provider VARCHAR(50) NOT NULL,
  redirect_url TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '10 minutes')
);
```

#### auth.oauth_token_refresh_queue

Schedules token refresh operations.

```sql
CREATE TABLE auth.oauth_token_refresh_queue (
  id UUID PRIMARY KEY,
  oauth_account_id UUID NOT NULL REFERENCES auth.oauth_provider_accounts(id) ON DELETE CASCADE,
  scheduled_at TIMESTAMPTZ NOT NULL,
  last_attempt_at TIMESTAMPTZ,
  attempts INT NOT NULL DEFAULT 0,
  max_attempts INT NOT NULL DEFAULT 3,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### auth.oauth_providers

Provider metadata and configuration.

```sql
CREATE TABLE auth.oauth_providers (
  id UUID PRIMARY KEY,
  name VARCHAR(50) UNIQUE NOT NULL,
  display_name VARCHAR(100) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT false,
  authorization_url TEXT NOT NULL,
  token_url TEXT NOT NULL,
  userinfo_url TEXT,
  revoke_url TEXT,
  default_scopes TEXT[] NOT NULL DEFAULT '{}',
  icon_url TEXT,
  color VARCHAR(7),
  supports_refresh BOOLEAN NOT NULL DEFAULT true,
  requires_pkce BOOLEAN NOT NULL DEFAULT false,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### auth.oauth_audit_log

Tracks OAuth authentication events.

```sql
CREATE TABLE auth.oauth_audit_log (
  id UUID PRIMARY KEY,
  user_id UUID REFERENCES auth.users(id) ON DELETE SET NULL,
  provider VARCHAR(50) NOT NULL,
  event_type VARCHAR(50) NOT NULL, -- 'login', 'link', 'unlink', 'refresh', 'revoke'
  ip_address INET,
  user_agent TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Helper Functions

```sql
-- Find or create OAuth user
SELECT auth.find_or_create_oauth_user(
  'google',
  'google_user_id_123',
  'user@example.com',
  'John Doe',
  'https://avatar.url',
  '{"sub": "...", "name": "..."}'::jsonb
);

-- Store OAuth tokens
SELECT auth.store_oauth_tokens(
  'user_uuid',
  'google',
  'google_user_id_123',
  'user@example.com',
  'access_token_xxx',
  'refresh_token_yyy',
  3600, -- expires_in seconds
  'id_token_zzz',
  ARRAY['openid', 'profile', 'email'],
  '{"sub": "..."}'::jsonb
);

-- Unlink OAuth provider
SELECT auth.unlink_oauth_provider('user_uuid', 'github');

-- Get user's OAuth providers
SELECT * FROM auth.get_user_oauth_providers('user_uuid');

-- Cleanup expired states
SELECT auth.cleanup_expired_oauth_states();
```

---

## CLI Commands

### Installation

```bash
nself oauth install
```

### Provider Management

```bash
# Enable providers
nself oauth enable --providers google,github,slack

# Disable providers
nself oauth disable --providers facebook

# Configure provider
nself oauth config google \
  --client-id=123.apps.googleusercontent.com \
  --client-secret=GOCSPX-xxx \
  --callback-url=http://localhost:3100/oauth/google/callback

# Test provider configuration
nself oauth test google

# List all providers
nself oauth list

# Show service status
nself oauth status
```

### Account Management

```bash
# List user's OAuth accounts
nself oauth accounts <user_id>

# Link provider to user
nself oauth link <user_id> <provider>

# Unlink provider from user
nself oauth unlink <user_id> <provider>
```

### Token Refresh Service

```bash
# Start refresh service (daemon)
nself oauth refresh start

# Stop refresh service
nself oauth refresh stop

# Check refresh status
nself oauth refresh status

# Run refresh once (for cron)
nself oauth refresh once
```

---

## Security Best Practices

### 1. State Parameter (CSRF Protection)

✅ **Always enabled** - nself automatically generates and verifies state parameter
- Random 32-byte state generated per request
- Stored in database with 10-minute TTL
- Verified on callback
- Prevents CSRF attacks

### 2. Use HTTPS in Production

```env
# ❌ Bad - HTTP in production
OAUTH_GOOGLE_CALLBACK_URL=http://yourdomain.com/oauth/google/callback

# ✅ Good - HTTPS in production
OAUTH_GOOGLE_CALLBACK_URL=https://yourdomain.com/oauth/google/callback
```

### 3. Rotate Secrets Regularly

```bash
# Update client secret every 90 days
nself oauth config google \
  --client-id=existing-id \
  --client-secret=new-secret
```

### 4. Restrict Callback URLs

Only add necessary callback URLs in OAuth app settings:

- Development: `http://localhost:3100/oauth/{provider}/callback`
- Staging: `https://staging.yourdomain.com/oauth/{provider}/callback`
- Production: `https://yourdomain.com/oauth/{provider}/callback`

### 5. Token Storage

- ✅ Access tokens encrypted in database
- ✅ Refresh tokens encrypted in database
- ✅ Tokens never exposed in logs
- ✅ Expired tokens automatically cleaned up

### 6. Audit Logging

All OAuth events are logged:

```sql
SELECT * FROM auth.oauth_audit_log
WHERE user_id = 'user_uuid'
ORDER BY created_at DESC;
```

Event types:
- `login` - User logged in via OAuth
- `link` - Provider linked to account
- `unlink` - Provider unlinked from account
- `refresh` - Token refreshed
- `revoke` - Token revoked
- `account_merge` - Accounts merged

### 7. Rate Limiting

Add rate limiting to OAuth endpoints:

```nginx
# nginx.conf
limit_req_zone $binary_remote_addr zone=oauth_limit:10m rate=10r/m;

location /oauth/ {
  limit_req zone=oauth_limit burst=5;
}
```

---

## Troubleshooting

### Common Issues

#### 1. "Provider not enabled" Error

**Cause:** Provider is not enabled in `.env` file.

**Solution:**
```bash
nself oauth enable --providers google
nself build
nself start
```

#### 2. "Invalid state" Error

**Cause:** State parameter validation failed.

**Possible Reasons:**
- State expired (>10 minutes old)
- Tampering attempt
- Browser cookies disabled

**Solution:**
- Retry OAuth flow
- Check browser cookies enabled
- Clear expired states:
  ```sql
  SELECT auth.cleanup_expired_oauth_states();
  ```

#### 3. "Missing client credentials" Error

**Cause:** Client ID or secret not configured.

**Solution:**
```bash
nself oauth config google \
  --client-id=xxx \
  --client-secret=xxx
```

#### 4. "Redirect URI mismatch" Error

**Cause:** Callback URL doesn't match OAuth app configuration.

**Solution:**
1. Check callback URL:
   ```bash
   grep OAUTH_GOOGLE_CALLBACK_URL .env.dev
   ```
2. Update if needed:
   ```bash
   nself oauth config google \
     --client-id=xxx \
     --client-secret=xxx \
     --callback-url=http://localhost:3100/oauth/google/callback
   ```
3. Match URL in provider's OAuth app settings

#### 5. Token Refresh Failing

**Check refresh queue:**
```sql
SELECT
  opa.provider,
  trq.attempts,
  trq.error_message,
  trq.scheduled_at
FROM auth.oauth_token_refresh_queue trq
JOIN auth.oauth_provider_accounts opa ON opa.id = trq.oauth_account_id
WHERE trq.attempts >= trq.max_attempts;
```

**Fix failed refreshes:**
```bash
# Reset attempts
psql $DATABASE_URL -c "
  UPDATE auth.oauth_token_refresh_queue
  SET attempts = 0, error_message = NULL
  WHERE attempts >= max_attempts;
"

# Run refresh once
nself oauth refresh once
```

#### 6. Cannot Unlink Provider

**Error:** "Cannot unlink: This is the only authentication method"

**Cause:** User must have at least one auth method.

**Solution:**
1. User should set a password first, OR
2. Link another OAuth provider before unlinking

```bash
# Check user's auth methods
nself oauth accounts <user_id>

# Set password for user
nself auth user password <user_id>
```

---

## Production Deployment

### 1. Apply Database Migrations

```bash
# On production server
psql $DATABASE_URL < src/lib/auth/oauth-db-migrations.sql
```

### 2. Configure Production Providers

```bash
# Set production credentials
nself oauth config google \
  --client-id=$PROD_GOOGLE_CLIENT_ID \
  --client-secret=$PROD_GOOGLE_CLIENT_SECRET \
  --callback-url=https://yourdomain.com/oauth/google/callback
```

### 3. Start Token Refresh Service

```bash
# Add to systemd
cat > /etc/systemd/system/nself-oauth-refresh.service <<EOF
[Unit]
Description=nself OAuth Token Refresh Service
After=network.target

[Service]
Type=simple
User=nself
WorkingDirectory=/var/www/nself
ExecStart=/usr/local/bin/nself oauth refresh daemon
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
systemctl enable nself-oauth-refresh
systemctl start nself-oauth-refresh
```

### 4. Setup Monitoring

```bash
# Check refresh service health
curl http://localhost:3100/health

# Monitor refresh queue
watch -n 60 'nself oauth refresh status'

# Setup alerting for failed refreshes
psql $DATABASE_URL -c "
  SELECT COUNT(*) FROM auth.oauth_token_refresh_queue
  WHERE attempts >= max_attempts;
"
```

### 5. Backup OAuth Data

```bash
# Backup OAuth tables
pg_dump $DATABASE_URL \
  --table=auth.oauth_provider_accounts \
  --table=auth.oauth_states \
  --table=auth.oauth_token_refresh_queue \
  --table=auth.oauth_providers \
  --table=auth.oauth_audit_log \
  > oauth_backup_$(date +%Y%m%d).sql
```

---

## Next Steps

- [Frontend Integration Guide](./OAUTH-SETUP.md)
- [OAuth Security Guide](../security/SECURITY-BEST-PRACTICES.md)
- [Multi-Tenant OAuth](./OAUTH-SETUP.md)

---

**Version:** nself v0.8.0+
**Last Updated:** January 30, 2026
