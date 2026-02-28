# nself auth

**Category**: Security & Authentication Commands

Manage authentication, authorization, security, and user access control.

## Overview

All authentication and security operations use `nself auth <subcommand>` for managing users, roles, MFA, OAuth providers, and security policies.

**Features**:
- ✅ User management
- ✅ Role-based access control (RBAC)
- ✅ Multi-factor authentication (MFA)
- ✅ OAuth/Social login providers
- ✅ JWT token management
- ✅ Security policies
- ✅ Session management
- ✅ API key management

## Subcommands by Category

### User Management
| Subcommand | Description |
|------------|-------------|
| [users](#nself-auth-users) | Manage users |
| [create-user](#nself-auth-create-user) | Create new user |
| [delete-user](#nself-auth-delete-user) | Delete user |
| [update-user](#nself-auth-update-user) | Update user |
| [list-users](#nself-auth-list-users) | List all users |
| [reset-password](#nself-auth-reset-password) | Reset user password |
| [verify-email](#nself-auth-verify-email) | Verify user email |

### Roles & Permissions
| Subcommand | Description |
|------------|-------------|
| [roles](#nself-auth-roles) | Manage roles |
| [create-role](#nself-auth-create-role) | Create new role |
| [delete-role](#nself-auth-delete-role) | Delete role |
| [assign-role](#nself-auth-assign-role) | Assign role to user |
| [permissions](#nself-auth-permissions) | Manage permissions |
| [grant](#nself-auth-grant) | Grant permission |
| [revoke](#nself-auth-revoke) | Revoke permission |

### Multi-Factor Authentication
| Subcommand | Description |
|------------|-------------|
| [mfa](#nself-auth-mfa) | MFA management |
| [mfa-enable](#nself-auth-mfa-enable) | Enable MFA |
| [mfa-disable](#nself-auth-mfa-disable) | Disable MFA |
| [mfa-methods](#nself-auth-mfa-methods) | List MFA methods |
| [mfa-backup-codes](#nself-auth-mfa-backup-codes) | Generate backup codes |

### OAuth & Social Login
| Subcommand | Description |
|------------|-------------|
| [oauth](#nself-auth-oauth) | OAuth provider management |
| [oauth-add](#nself-auth-oauth-add) | Add OAuth provider |
| [oauth-remove](#nself-auth-oauth-remove) | Remove OAuth provider |
| [oauth-test](#nself-auth-oauth-test) | Test OAuth configuration |

### Security & Policies
| Subcommand | Description |
|------------|-------------|
| [security](#nself-auth-security) | Security settings |
| [rate-limit](#nself-auth-rate-limit) | Rate limiting |
| [ip-whitelist](#nself-auth-ip-whitelist) | IP whitelist |
| [ip-blacklist](#nself-auth-ip-blacklist) | IP blacklist |
| [password-policy](#nself-auth-password-policy) | Password requirements |
| [session-timeout](#nself-auth-session-timeout) | Session timeout |

### Tokens & API Keys
| Subcommand | Description |
|------------|-------------|
| [tokens](#nself-auth-tokens) | JWT token management |
| [token-create](#nself-auth-token-create) | Create API token |
| [token-revoke](#nself-auth-token-revoke) | Revoke token |
| [api-keys](#nself-auth-api-keys) | Manage API keys |

### SSL & Certificates
| Subcommand | Description |
|------------|-------------|
| [ssl](#nself-auth-ssl) | SSL certificate management |
| [ssl-generate](#nself-auth-ssl-generate) | Generate SSL certificate |
| [ssl-renew](#nself-auth-ssl-renew) | Renew SSL certificate |
| [ssl-verify](#nself-auth-ssl-verify) | Verify SSL certificate |

### Audit & Monitoring
| Subcommand | Description |
|------------|-------------|
| [audit](#nself-auth-audit) | Security audit |
| [sessions](#nself-auth-sessions) | Active sessions |
| [devices](#nself-auth-devices) | Trusted devices |
| [webhooks](#nself-auth-webhooks) | Auth webhooks |

## Quick Start

### Create User

```bash
nself auth create-user --email user@example.com --role user
```

**Output**:
```
Creating user...
✓ User created successfully

Email: user@example.com
Role: user
Status: active
Password: (sent via email)

User ID: user_123456789
```

### Assign Role

```bash
nself auth assign-role user@example.com admin
```

### Enable MFA

```bash
nself auth mfa-enable user@example.com --method totp
```

### Add OAuth Provider

```bash
nself auth oauth-add google \
  --client-id your-client-id \
  --client-secret your-client-secret
```

## User Management

### nself auth users

List and manage users.

**Usage**:
```bash
nself auth users [OPTIONS]
```

**Options**:
- `--filter STATUS` - Filter by status (active/inactive/banned)
- `--role ROLE` - Filter by role
- `--format FORMAT` - Output format (table/json/csv)

**Examples**:
```bash
# List all users
nself auth users

# Active users only
nself auth users --filter active

# Admins only
nself auth users --role admin
```

### nself auth create-user

Create new user account.

**Usage**:
```bash
nself auth create-user [OPTIONS]
```

**Options**:
- `--email EMAIL` - User email (required)
- `--password PASSWORD` - Initial password
- `--role ROLE` - User role (default: user)
- `--verified` - Mark email as verified
- `--send-email` - Send welcome email

**Examples**:
```bash
# Create user with auto-generated password
nself auth create-user --email user@example.com --send-email

# Create admin user
nself auth create-user --email admin@example.com --role admin --verified

# Create with specific password
nself auth create-user --email user@example.com --password SecurePass123!
```

### nself auth delete-user

Delete user account.

**Usage**:
```bash
nself auth delete-user <email|id> [OPTIONS]
```

**Options**:
- `--hard` - Permanently delete (cannot be recovered)
- `--soft` - Soft delete (can be restored)
- `--confirm` - Skip confirmation

**Examples**:
```bash
# Soft delete
nself auth delete-user user@example.com

# Hard delete with confirmation
nself auth delete-user user@example.com --hard --confirm
```

### nself auth reset-password

Reset user password.

**Usage**:
```bash
nself auth reset-password <email> [OPTIONS]
```

**Options**:
- `--password PASSWORD` - Set specific password
- `--generate` - Auto-generate password
- `--send-email` - Email new password to user

**Examples**:
```bash
# Generate and email new password
nself auth reset-password user@example.com --generate --send-email

# Set specific password
nself auth reset-password user@example.com --password NewSecurePass123!
```

## Roles & Permissions

### Default Roles

nself includes these default roles:

- **user** - Standard user (read own data)
- **manager** - Manage users and content
- **admin** - Full administrative access
- **anonymous** - Unauthenticated users

### nself auth roles

Manage user roles.

**Usage**:
```bash
nself auth roles <action> [OPTIONS]
```

**Actions**:
- `list` - List all roles
- `create` - Create new role
- `delete` - Delete role
- `assign` - Assign role to user
- `revoke` - Revoke role from user

**Examples**:
```bash
# List roles
nself auth roles list

# Create custom role
nself auth roles create editor --permissions read,write

# Assign role
nself auth roles assign user@example.com editor
```

### nself auth permissions

Manage granular permissions.

**Usage**:
```bash
nself auth permissions <action> [ROLE] [PERMISSION]
```

**Actions**:
- `list` - List role permissions
- `grant` - Grant permission to role
- `revoke` - Revoke permission from role

**Permission Format**: `table:action`

**Examples**:
```bash
# List permissions for role
nself auth permissions list editor

# Grant permissions
nself auth grant editor users:read
nself auth grant editor posts:write

# Revoke permission
nself auth revoke editor posts:delete
```

## Multi-Factor Authentication

### nself auth mfa

Manage multi-factor authentication.

**Supported Methods**:
- **TOTP** - Time-based One-Time Password (Google Authenticator, Authy)
- **SMS** - SMS text message
- **Email** - Email verification code
- **Backup Codes** - One-time use backup codes

**Usage**:
```bash
nself auth mfa <action> [USER] [OPTIONS]
```

**Actions**:
- `enable` - Enable MFA for user
- `disable` - Disable MFA for user
- `status` - Check MFA status
- `generate-codes` - Generate backup codes

**Examples**:
```bash
# Enable TOTP MFA
nself auth mfa enable user@example.com --method totp

# Generate backup codes
nself auth mfa generate-codes user@example.com

# Check status
nself auth mfa status user@example.com
```

**Output (TOTP)**:
```
MFA Setup - TOTP

Scan this QR code with your authenticator app:

█████████████████████████████
██ ▄▄▄▄▄ █ ▄ █▀█ ▄▄▄▄▄ ██
██ █   █ █▄▀▄▀█ █   █ ██
██ █▄▄▄█ █ ▄ ▀█ █▄▄▄█ ██
██▄▄▄▄▄▄▄█▀▄▀▄█▄▄▄▄▄▄▄██

Or enter this code manually:
JBSWY3DPEHPK3PXP

Backup Codes (save securely):
1. 12345-67890
2. 09876-54321
3. 11111-22222
```

## OAuth & Social Login

### nself auth oauth

Configure OAuth providers for social login.

**Supported Providers**:
- Google
- GitHub
- Facebook
- Twitter/X
- Apple
- Discord
- Microsoft

**Usage**:
```bash
nself auth oauth <action> <provider> [OPTIONS]
```

**Actions**:
- `add` - Add OAuth provider
- `remove` - Remove provider
- `update` - Update provider config
- `test` - Test OAuth flow

**Examples**:
```bash
# Add Google OAuth
nself auth oauth add google \
  --client-id your-google-client-id \
  --client-secret your-google-client-secret

# Add GitHub OAuth
nself auth oauth add github \
  --client-id your-github-client-id \
  --client-secret your-github-client-secret

# Test OAuth flow
nself auth oauth test google
```

**Configuration**:
```bash
# In .env
AUTH_PROVIDER_GOOGLE_ENABLED=true
AUTH_PROVIDER_GOOGLE_CLIENT_ID=your-client-id
AUTH_PROVIDER_GOOGLE_CLIENT_SECRET=your-secret

AUTH_PROVIDER_GITHUB_ENABLED=true
AUTH_PROVIDER_GITHUB_CLIENT_ID=your-client-id
AUTH_PROVIDER_GITHUB_CLIENT_SECRET=your-secret
```

## Security Policies

### nself auth password-policy

Configure password requirements.

**Usage**:
```bash
nself auth password-policy set [OPTIONS]
```

**Options**:
- `--min-length N` - Minimum password length
- `--require-uppercase` - Require uppercase letters
- `--require-lowercase` - Require lowercase letters
- `--require-numbers` - Require numbers
- `--require-special` - Require special characters
- `--min-score N` - Minimum password strength score (0-4)

**Examples**:
```bash
# Strong policy
nself auth password-policy set \
  --min-length 12 \
  --require-uppercase \
  --require-lowercase \
  --require-numbers \
  --require-special \
  --min-score 3

# View current policy
nself auth password-policy show
```

### nself auth rate-limit

Configure rate limiting.

**Usage**:
```bash
nself auth rate-limit set [OPTIONS]
```

**Options**:
- `--login-attempts N` - Max login attempts per hour
- `--signup-rate N` - Max signups per hour per IP
- `--api-rate N` - Max API requests per minute

**Examples**:
```bash
# Set rate limits
nself auth rate-limit set \
  --login-attempts 5 \
  --signup-rate 10 \
  --api-rate 100

# View current limits
nself auth rate-limit show
```

### nself auth ip-whitelist

Manage IP whitelist for access control.

**Usage**:
```bash
nself auth ip-whitelist <action> [IP]
```

**Actions**:
- `add` - Add IP to whitelist
- `remove` - Remove IP from whitelist
- `list` - List whitelisted IPs

**Examples**:
```bash
# Add office IP
nself auth ip-whitelist add 203.0.113.0/24

# Add specific IP
nself auth ip-whitelist add 198.51.100.42

# List whitelisted IPs
nself auth ip-whitelist list
```

## Token Management

### nself auth tokens

Manage JWT tokens and API keys.

**Usage**:
```bash
nself auth tokens <action> [OPTIONS]
```

**Actions**:
- `create` - Create new API token
- `list` - List all tokens
- `revoke` - Revoke token
- `verify` - Verify token

**Examples**:
```bash
# Create API token
nself auth token-create --name "CI/CD Token" --expires 90d

# List tokens
nself auth tokens list

# Revoke token
nself auth token-revoke token_abc123
```

## SSL Certificates

### nself auth ssl

Manage SSL/TLS certificates.

**Usage**:
```bash
nself auth ssl <action> [OPTIONS]
```

**Actions**:
- `generate` - Generate self-signed certificate
- `renew` - Renew certificate
- `verify` - Verify certificate
- `info` - Show certificate information

**Examples**:
```bash
# Generate self-signed certificate
nself auth ssl generate --domain localhost

# Verify certificate
nself auth ssl verify

# Show certificate info
nself auth ssl info
```

## Audit & Monitoring

### nself auth audit

Security audit and compliance checks.

**Usage**:
```bash
nself auth audit [OPTIONS]
```

**Checks**:
- Weak passwords
- Inactive accounts
- Expired tokens
- Suspicious activity
- Security policy compliance

**Examples**:
```bash
# Full audit
nself auth audit

# Export audit report
nself auth audit --export audit-$(date +%Y%m%d).txt
```

### nself auth sessions

View and manage active sessions.

**Usage**:
```bash
nself auth sessions <action> [OPTIONS]
```

**Actions**:
- `list` - List active sessions
- `revoke` - Revoke session
- `revoke-all` - Revoke all user sessions

**Examples**:
```bash
# List active sessions
nself auth sessions list

# Revoke specific session
nself auth sessions revoke session_abc123

# Revoke all sessions for user
nself auth sessions revoke-all user@example.com
```

## Best Practices

### 1. Strong Password Policy

```bash
nself auth password-policy set \
  --min-length 12 \
  --require-uppercase \
  --require-numbers \
  --require-special \
  --min-score 3
```

### 2. Enable MFA for Admins

```bash
# Require MFA for all admin users
nself auth mfa enable --role admin --method totp
```

### 3. Regular Security Audits

```bash
# Weekly security audit
0 0 * * 0 nself auth audit --export audit-$(date +%Y%m%d).txt
```

### 4. Rotate API Keys

```bash
# Rotate every 90 days
nself auth token-create --expires 90d
# Revoke old tokens
```

## Related Commands

- `nself config secrets` - Manage authentication secrets
- `nself db shell` - Direct database access for user management
- `nself deploy protect` - Environment protection

## See Also

- [Authentication Guide](../../guides/AUTHENTICATION.md)
- [Security Best Practices](../../guides/SECURITY.md)
- [OAuth Setup](../../guides/OAUTH.md)
- [MFA Configuration](../../guides/MFA.md)
