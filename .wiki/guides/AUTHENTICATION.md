# Authentication Setup Guide

Complete guide to setting up and managing user authentication in nself.

## Overview

nself provides a complete authentication solution:

- **User Management** - Create, update, delete users
- **JWT Tokens** - Secure API authentication
- **Sessions** - Track logged-in users
- **Multi-Factor Auth** - Optional 2FA for security
- **OAuth Integration** - Connect Google, GitHub, etc.
- **Row-Level Security** - Data access control per user

## Quick Start

### 1. Access Authentication Service

The auth service runs automatically when you start nself:

```bash
nself start
```

Default URL: `http://localhost:8080/auth`

### 2. Create Your First User

```bash
# Using the auth API
curl -X POST http://localhost:8080/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "secure-password",
    "username": "user"
  }'
```

Response:
```json
{
  "user": {
    "id": "12345",
    "email": "user@example.com",
    "username": "user"
  },
  "token": "eyJhbGciOiJIUzI1NiIs..."
}
```

### 3. Use JWT Token in GraphQL

Store the token and use it in requests:

```bash
curl -X POST http://localhost:8080/graphql \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..." \
  -H "Content-Type: application/json" \
  -d '{"query": "{ me { id email } }"}'
```

## User Management

### Create Users

**Via API**:
```bash
curl -X POST http://localhost:8080/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "newuser@example.com",
    "password": "secure-password"
  }'
```

**Via nself CLI**:
```bash
nself auth users create \
  --email user@example.com \
  --password secure-password
```

### List Users

```bash
nself auth users list
```

### Update User

```bash
nself auth users update user-id \
  --email newemail@example.com \
  --role admin
```

### Delete User

```bash
nself auth users delete user-id
```

## JWT Tokens

### Token Structure

nself uses JWT tokens with standard claims:

```json
{
  "sub": "user-123",
  "email": "user@example.com",
  "role": "user",
  "aud": "your-api",
  "iat": 1234567890,
  "exp": 1234571490
}
```

### Token Expiration

Configure in `.env`:

```bash
# Token expires in 1 hour (3600 seconds)
JWT_TOKEN_EXPIRY=3600

# Refresh token expires in 7 days
JWT_REFRESH_EXPIRY=604800
```

### Refresh Tokens

Get a new token without re-authenticating:

```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "eyJhbGc..."}'
```

## Hasura Permissions

### Connect Auth to Hasura

Hasura validates JWT tokens and extracts user info:

```bash
# In .env
HASURA_GRAPHQL_JWT_SECRET='{"type": "HS256", "key": "your-jwt-secret"}'
```

### Add User Claims to Tokens

Configure the auth service to include custom claims:

```bash
# In .env
JWT_CUSTOM_CLAIMS='{
  "x-hasura-user-id": "{{ user.id }}",
  "x-hasura-user-role": "{{ user.role }}"
}'
```

These claims are available in Hasura permissions.

### Set Table Permissions

In Hasura console, set permissions for the `users` table:

**Select (Read)**:
```
user_id = X-Hasura-User-Id
```

This means: Users can only see their own user record.

**Insert (Create)**:
```
Deny (users cannot insert their own records)
```

**Update**:
```
user_id = X-Hasura-User-Id
```

Users can only update their own record.

**Delete**:
```
Deny
```

Users cannot delete their own record.

### Admin Permissions

For admin users:

**Select**:
```
role = "admin" OR user_id = X-Hasura-User-Id
```

Admins see all records, users see only their own.

## Multi-Factor Authentication (MFA)

### Enable MFA

```bash
nself auth mfa enable --user user-id
```

### MFA Methods

1. **Time-based One-Time Password (TOTP)**
   - Works with Google Authenticator, Authy, Microsoft Authenticator
   - Most common and secure

2. **SMS Code**
   - Sends 6-digit code via SMS
   - Requires Twilio or similar

3. **Email Code**
   - Sends code to verified email
   - Easiest to implement

### Setup TOTP (Recommended)

```bash
# Enable TOTP for user
nself auth mfa enable --user user-id --method totp

# Get QR code
nself auth mfa qr-code --user user-id

# User scans with authenticator app
# Verify code
nself auth mfa verify --user user-id --code 123456
```

### Backup Codes

Generate backup codes for account recovery:

```bash
nself auth mfa backup-codes --user user-id
```

Users should save these in a secure location.

## OAuth Integration

### Setup Google OAuth

1. **Create OAuth 2.0 Credentials**
   - Go to [Google Cloud Console](https://console.cloud.google.com/)
   - Create OAuth 2.0 Client ID
   - Note: Client ID and Client Secret

2. **Configure in nself**

   In `.env`:
   ```bash
   OAUTH_GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
   OAUTH_GOOGLE_CLIENT_SECRET=your-client-secret
   OAUTH_GOOGLE_REDIRECT_URI=http://localhost:8080/auth/callback
   ```

3. **Build and restart**
   ```bash
   nself build
   nself restart auth
   ```

4. **Use in your app**
   ```html
   <a href="http://localhost:8080/auth/oauth/google">
     Sign in with Google
   </a>
   ```

### Setup GitHub OAuth

1. **Create OAuth App**
   - Go to [GitHub Settings → Developer Settings](https://github.com/settings/developers)
   - Click "New OAuth App"
   - Fill in form with your redirect URI

2. **Configure in nself**

   In `.env`:
   ```bash
   OAUTH_GITHUB_CLIENT_ID=your-app-id
   OAUTH_GITHUB_CLIENT_SECRET=your-app-secret
   OAUTH_GITHUB_REDIRECT_URI=http://localhost:8080/auth/callback
   ```

3. **Restart**
   ```bash
   nself restart auth
   ```

### Using OAuth Tokens

After OAuth login, users get a JWT token:

```javascript
// In your frontend
const response = await fetch('/auth/oauth/google');
const { token } = await response.json();

// Use token in API calls
fetch('/graphql', {
  headers: {
    'Authorization': `Bearer ${token}`
  }
})
```

## Session Management

### Configure Session Duration

In `.env`:

```bash
# Session expires after 30 days of inactivity
SESSION_TIMEOUT=2592000

# Session data stored in Redis
SESSION_STORE=redis
```

### Track Active Sessions

```bash
# List user's active sessions
nself auth sessions list --user user-id

# Revoke a session
nself auth sessions revoke --session-id session-id

# Revoke all sessions (logout everywhere)
nself auth sessions revoke --user user-id --all
```

## Password Management

### Reset Password

User requests password reset:

```bash
curl -X POST http://localhost:8080/auth/password-reset \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com"}'
```

Sends email with reset link. User clicks link and creates new password.

### Require Password Change

Force user to change password on next login:

```bash
nself auth users update user-id --require-password-change
```

### Password Policy

Configure in `.env`:

```bash
# Minimum password length
PASSWORD_MIN_LENGTH=12

# Require uppercase, lowercase, numbers, symbols
PASSWORD_REQUIRE_UPPERCASE=true
PASSWORD_REQUIRE_LOWERCASE=true
PASSWORD_REQUIRE_NUMBERS=true
PASSWORD_REQUIRE_SYMBOLS=true

# Password expires every 90 days
PASSWORD_EXPIRY_DAYS=90
```

## Role-Based Access Control (RBAC)

### Define Roles

In `.env`:

```bash
AUTH_ROLES='["admin", "user", "guest"]'
```

### Assign Roles

```bash
# Assign role to user
nself auth users update user-id --role admin

# Remove role
nself auth users update user-id --role user
```

### Use Roles in Permissions

In Hasura, use the `x-hasura-user-role` claim:

**Admin can do everything**:
```
role = "admin"
```

**Users can only see their own data**:
```
role = "user" AND user_id = X-Hasura-User-Id
```

**Guests can only read published content**:
```
role = "guest" AND published = true
```

## Security Best Practices

### 1. HTTPS Only

In production, always use HTTPS:

```bash
# .env.prod
BASE_DOMAIN=myapp.com  # Automatic HTTPS via Nginx
```

### 2. Secure Tokens

- Store tokens in HTTP-only cookies (not localStorage)
- Never log tokens
- Rotate keys regularly

```bash
# In .env, set JWT secret
JWT_SECRET=long-random-string-min-32-chars
```

### 3. Rate Limiting

Prevent brute force attacks:

```bash
# .env
AUTH_RATE_LIMIT=10  # 10 login attempts per minute
AUTH_LOCKOUT_TIME=900  # 15 minute lockout after failed attempts
```

### 4. Audit Logging

Track authentication events:

```bash
nself audit logs --service auth
```

Shows login attempts, failed logins, password changes, etc.

### 5. Keep Secrets Secret

Never commit secrets to git:

```bash
# .env (committed)
JWT_SECRET=${JWT_SECRET}  # Reference env var

# .env.secrets (not committed, only on server)
JWT_SECRET=actual-secret-key-here
```

## Troubleshooting

### JWT Token Invalid

**Error**: `Invalid token` or `Unauthorized`

**Solutions**:
1. Token expired - get new token with refresh token
2. Token malformed - check token format
3. Secret mismatch - verify JWT_SECRET in .env

```bash
# Debug token
nself auth debug --token eyJhbGc...
```

### User Can't Login

**Error**: `Invalid credentials`

**Solutions**:
1. User doesn't exist - create user first
2. Wrong password - reset password
3. User disabled - re-enable user

```bash
# Check user status
nself auth users get user-id
```

### OAuth Callback Error

**Error**: `Redirect URI mismatch` or `Client ID invalid`

**Solutions**:
1. OAuth app not configured - double-check credentials
2. Redirect URI wrong - must match exactly (including protocol)
3. Environment not restarted - restart auth service

```bash
# Verify configuration
nself config env --filter OAUTH
```

## Next Steps

- [Database Guide](../guides/DATABASE-WORKFLOW.md) - Secure your data
- [Deployment](../guides/DEPLOYMENT-ARCHITECTURE.md) - Production setup
- [Monitoring](../configuration/MONITORING.md) - Track auth events

---

**Key Takeaway**: nself auth is production-ready. Use it for all user authentication needs.
