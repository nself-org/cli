# nself auth

Authentication management for nself applications.

## Description

The `auth` command provides comprehensive authentication management including user authentication, OAuth provider configuration, session management, and authentication settings. It supports multiple authentication methods including email/password, OAuth (Google, GitHub, Apple, etc.), phone/SMS, magic links, and anonymous auth.

**Use Cases:**
- User authentication (login/signup/logout)
- OAuth provider configuration
- Session management
- Multi-factor authentication
- Authentication debugging and testing

---

## Usage

```bash
nself auth <subcommand> [options]
```

---

## Subcommands

### `login` - Authenticate User

Authenticate a user with various methods.

```bash
nself auth login [options]
```

**Login Methods:**

#### Email/Password Login
```bash
nself auth login --email=user@example.com --password=secret
```

#### OAuth Login
```bash
# Google OAuth
nself auth login --provider=google

# GitHub OAuth
nself auth login --provider=github

# Apple OAuth
nself auth login --provider=apple
```

#### Phone/SMS Login
```bash
nself auth login --phone=+1234567890
```

#### Magic Link (Passwordless)
```bash
nself auth login --email=user@example.com
```

#### Anonymous Login
```bash
nself auth login --anonymous
```

---

### `signup` - Create New User

Create a new user account.

```bash
nself auth signup [options]
```

**Signup Methods:**

#### Email/Password Signup
```bash
nself auth signup --email=user@example.com --password=secret
```

#### OAuth Signup
```bash
nself auth signup --provider=google
```

#### Phone Signup
```bash
nself auth signup --phone=+1234567890 --password=secret
```

---

### `logout` - End Session

End the current user session.

```bash
nself auth logout [options]
```

**Options:**
- `--all` - Logout from all sessions/devices

**Examples:**
```bash
# Logout from current session
nself auth logout

# Logout from all devices
nself auth logout --all
```

---

### `status` - Show Authentication Status

Display current authentication status and session info.

```bash
nself auth status
```

**Shows:**
- Current user information
- Active session details
- Authentication method used
- MFA status
- Token expiration

---

### `providers` - Manage OAuth Providers

Manage OAuth authentication providers.

```bash
nself auth providers <action> [options]
```

#### List Providers

```bash
nself auth providers list
```

**Output:**
```
Available OAuth Providers:
========================

✓ google      [ENABLED]
  Client ID: 1234567890-abcdefg.apps.googleusercontent.com
  Scopes: email, profile

✓ github      [ENABLED]
  Client ID: Iv1.abc123def456
  Scopes: user:email, read:user

○ apple       [DISABLED]
  Not configured

○ facebook    [DISABLED]
  Not configured
```

#### Add Provider

```bash
nself auth providers add <provider> --client-id=<id> --client-secret=<secret>
```

**Examples:**
```bash
# Add Google OAuth
nself auth providers add google \
  --client-id=1234567890-abcdefg.apps.googleusercontent.com \
  --client-secret=GOCSPX-abc123def456

# Add GitHub OAuth
nself auth providers add github \
  --client-id=Iv1.abc123def456 \
  --client-secret=ghp_abc123def456

# Add Apple OAuth
nself auth providers add apple \
  --client-id=com.example.app \
  --client-secret=your-apple-secret
```

#### Remove Provider

```bash
nself auth providers remove <provider>
```

**Example:**
```bash
nself auth providers remove facebook
```

#### Enable/Disable Provider

```bash
nself auth providers enable <provider>
nself auth providers disable <provider>
```

**Examples:**
```bash
# Enable Google OAuth
nself auth providers enable google

# Disable GitHub OAuth temporarily
nself auth providers disable github
```

---

### `sessions` - Manage User Sessions

Manage active user sessions.

```bash
nself auth sessions <action> [options]
```

#### List Sessions

```bash
nself auth sessions list
```

**Output:**
```
Active Sessions:
==============

Session 1:
  ID: sess_abc123def456
  User: user@example.com
  Device: Chrome on macOS
  IP: 192.168.1.100
  Created: 2026-01-30 10:30:00
  Expires: 2026-02-13 10:30:00
  Current: ✓

Session 2:
  ID: sess_xyz789abc123
  User: user@example.com
  Device: Safari on iPhone
  IP: 192.168.1.105
  Created: 2026-01-29 14:20:00
  Expires: 2026-02-12 14:20:00
```

#### Revoke Session

```bash
nself auth sessions revoke <session-id>
```

**Example:**
```bash
nself auth sessions revoke sess_xyz789abc123
```

---

### `config` - Authentication Configuration

View or modify authentication configuration.

```bash
nself auth config [options]
```

#### Show Configuration

```bash
nself auth config
# or
nself auth config --show
```

**Output:**
```
Authentication Configuration:
===========================

Email/Password:
  Enabled: true
  Require Email Verification: true
  Password Min Length: 8
  Password Requirements: uppercase, lowercase, number

OAuth:
  Enabled Providers: google, github
  Allow Auto-Signup: true

MFA:
  Enabled: true
  Required: false
  Methods: totp, sms

Sessions:
  Token Lifetime: 2 weeks
  Refresh Token Lifetime: 90 days
  Max Active Sessions: 5

Security:
  Rate Limiting: enabled
  Max Failed Attempts: 5
  Lockout Duration: 15 minutes
```

#### Set Configuration

```bash
nself auth config --set key=value
```

**Examples:**
```bash
# Require email verification
nself auth config --set require_email_verification=true

# Set password minimum length
nself auth config --set password_min_length=10

# Enable MFA requirement
nself auth config --set mfa_required=true

# Set session lifetime
nself auth config --set token_lifetime=604800  # 1 week in seconds
```

---

## Login Options

| Option | Description |
|--------|-------------|
| `--provider=<name>` | OAuth provider (google, github, apple, etc.) |
| `--email=<email>` | Email address |
| `--password=<pass>` | Password for email/password auth |
| `--phone=<number>` | Phone number for SMS auth |
| `--anonymous` | Anonymous authentication |

---

## Signup Options

| Option | Description |
|--------|-------------|
| `--provider=<name>` | OAuth provider |
| `--email=<email>` | Email address |
| `--password=<pass>` | Password |
| `--phone=<number>` | Phone number |

---

## Examples

### Complete Authentication Flow

```bash
# 1. Add OAuth providers
nself auth providers add google \
  --client-id=YOUR_CLIENT_ID \
  --client-secret=YOUR_CLIENT_SECRET

nself auth providers add github \
  --client-id=YOUR_CLIENT_ID \
  --client-secret=YOUR_CLIENT_SECRET

# 2. Enable providers
nself auth providers enable google
nself auth providers enable github

# 3. Sign up a new user
nself auth signup --email=user@example.com --password=SecurePass123

# 4. Login with email/password
nself auth login --email=user@example.com --password=SecurePass123

# 5. Check status
nself auth status

# 6. List active sessions
nself auth sessions list

# 7. Logout from all devices
nself auth logout --all
```

### OAuth Provider Setup

```bash
# List available providers
nself auth providers list

# Add Google OAuth
nself auth providers add google \
  --client-id=1234567890-abc.apps.googleusercontent.com \
  --client-secret=GOCSPX-abc123

# Enable Google
nself auth providers enable google

# Test OAuth login
nself auth login --provider=google
```

### Session Management

```bash
# View all active sessions
nself auth sessions list

# Revoke a specific session
nself auth sessions revoke sess_abc123

# Logout from all sessions
nself auth logout --all
```

### Configuration Management

```bash
# View current config
nself auth config

# Require email verification
nself auth config --set require_email_verification=true

# Set stricter password requirements
nself auth config --set password_min_length=12

# Enable MFA requirement
nself auth config --set mfa_required=true
```

---

## Supported OAuth Providers

| Provider | Status | Setup Guide |
|----------|--------|-------------|
| **Google** | ✅ Fully supported | [Google OAuth Setup](../guides/OAUTH-SETUP.md#google) |
| **GitHub** | ✅ Fully supported | [GitHub OAuth Setup](../guides/OAUTH-SETUP.md#github) |
| **Microsoft** | ✅ Fully supported | [Microsoft OAuth Setup](../guides/OAUTH-SETUP.md#microsoft) |
| **Slack** | ✅ Fully supported | [Slack OAuth Setup](../guides/OAUTH-SETUP.md#slack) |
| **Apple** | 🔄 Planned (v1.0) | Implementation in progress |
| **Facebook** | 🔄 Planned (v1.0+) | Awaiting implementation |
| **Twitter/X** | 🔄 Planned (v1.0+) | Awaiting implementation |
| **LinkedIn** | 🔄 Planned (v1.0+) | Awaiting implementation |

---

## Authentication Methods

### Email/Password (Built-in)

Default authentication method with:
- Email verification
- Password requirements
- Password reset
- Account recovery

### OAuth (Social Login)

Delegate authentication to third-party providers:
- Single sign-on (SSO)
- No password management
- Trusted providers
- Auto-fill user data

### Phone/SMS (Coming Soon)

SMS-based authentication:
- Phone number verification
- OTP delivery
- No password required
- Regional support

### Magic Link (Coming Soon)

Passwordless email authentication:
- Email-only login
- Time-limited links
- No password management
- High security

### Anonymous (Coming Soon)

Temporary authentication:
- No credentials required
- Guest access
- Convertible to permanent account
- Session-based

---

## Configuration Options

### Password Policy

```bash
# Minimum length
NSELF_AUTH_PASSWORD_MIN_LENGTH=8

# Require uppercase
NSELF_AUTH_PASSWORD_REQUIRE_UPPERCASE=true

# Require lowercase
NSELF_AUTH_PASSWORD_REQUIRE_LOWERCASE=true

# Require numbers
NSELF_AUTH_PASSWORD_REQUIRE_NUMBER=true

# Require special characters
NSELF_AUTH_PASSWORD_REQUIRE_SPECIAL=false
```

### Session Settings

```bash
# Token lifetime (seconds)
NSELF_AUTH_TOKEN_LIFETIME=1209600  # 2 weeks

# Refresh token lifetime (seconds)
NSELF_AUTH_REFRESH_TOKEN_LIFETIME=7776000  # 90 days

# Max active sessions per user
NSELF_AUTH_MAX_SESSIONS=5
```

### Security Settings

```bash
# Enable rate limiting
NSELF_AUTH_RATE_LIMIT_ENABLED=true

# Max failed login attempts
NSELF_AUTH_MAX_FAILED_ATTEMPTS=5

# Lockout duration (seconds)
NSELF_AUTH_LOCKOUT_DURATION=900  # 15 minutes

# Require email verification
NSELF_AUTH_REQUIRE_EMAIL_VERIFICATION=true
```

---

## Troubleshooting

### OAuth Provider Not Working

```bash
# Check provider configuration
nself auth providers list

# Verify client ID and secret
nself auth config

# Check OAuth redirect URLs
# Should be: https://auth.yourdomain.com/callback/google
```

### Session Expired

```bash
# Check session status
nself auth status

# Login again
nself auth login --email=user@example.com --password=secret
```

### Too Many Failed Attempts

```bash
# Check lockout status
nself auth status

# Wait for lockout period to expire (default: 15 minutes)
# Or reset via database if needed
```

---

## Related Commands

- **[mfa](MFA.md)** - Multi-factor authentication
- **[oauth](OAUTH.md)** - OAuth management
- **[devices](DEVICES.md)** - Device management
- **[audit](AUDIT.md)** - Audit log management
- **[security](SECURITY.md)** - Security tools

---

## Security Best Practices

1. **Always use HTTPS** in production
2. **Enable email verification** for new accounts
3. **Require strong passwords** (min 12 characters)
4. **Enable MFA** for sensitive accounts
5. **Use OAuth providers** when possible
6. **Implement rate limiting** to prevent brute force
7. **Monitor audit logs** for suspicious activity
8. **Rotate secrets regularly** for OAuth providers

---

**Version:** v0.6.0+
**Category:** Authentication & Security
**Related Documentation:**
- [Authentication Guide](../guides/SECURITY.md)
- [OAuth Setup Guide](../guides/OAUTH-SETUP.md)
- [OAuth Quick Start](../guides/OAUTH-QUICK-START.md)
- [Security System](../security/SECURITY-SYSTEM.md)
- [MFA Setup](MFA.md)
