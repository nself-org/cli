# nself mfa - Multi-Factor Authentication Management

> **⚠️ DEPRECATED**: `nself mfa` is deprecated and will be removed in v1.0.0.
> Please use `nself auth mfa` instead.
> Run `nself auth mfa --help` for full usage information.

**Version**: 0.6.0+
**Status**: ✅ Production Ready
**Category**: Authentication & Security

---

## Overview

The `nself mfa` command provides comprehensive multi-factor authentication (MFA) management for your nself deployment. Support for TOTP (Time-based One-Time Password), SMS, Email, and backup codes ensures maximum security flexibility.

**Why Use MFA?**
- Adds critical second layer of security beyond passwords
- Protects against credential theft and phishing
- Required for compliance in many industries (SOC 2, HIPAA, PCI-DSS)
- Supports multiple authentication methods for flexibility

---

## Quick Start

### Enable TOTP MFA (Most Common)

```bash
# 1. Enroll user in TOTP
nself mfa enable --method=totp --user=user-123 --email=john@example.com

# Output includes QR code command and secret
# Scan QR code with authenticator app (Google Authenticator, Authy, 1Password, etc.)

# 2. Verify with code from authenticator app
nself mfa verify --method=totp --user=user-123 --code=123456

# 3. Generate backup codes
nself mfa backup-codes generate --user=user-123
```

### Enable SMS MFA

```bash
# 1. Enroll phone number
nself mfa enable --method=sms --user=user-123 --phone=+1234567890

# 2. Send verification code
nself mfa verify --method=sms --user=user-123 --send

# 3. Verify code
nself mfa verify --method=sms --user=user-123 --code=123456
```

---

## Command Reference

### `nself mfa enable`

Enable MFA for a user with specified method.

**Syntax:**
```bash
nself mfa enable --method=<method> --user=<user_id> [options]
```

**Options:**
- `--method=<method>` - MFA method: `totp`, `sms`, or `email` (required)
- `--user=<user_id>` - User ID to enable MFA for (required)
- `--email=<email>` - Email address (required for TOTP and email methods)
- `--phone=<phone>` - Phone number in E.164 format (required for SMS method)

**TOTP Example:**
```bash
# Enroll with TOTP
nself mfa enable --method=totp --user=user-abc-123 --email=john@example.com

# Output:
# ✓ TOTP MFA enrolled successfully
#
# Secret (for manual entry): JBSWY3DPEHPK3PXP
#
# Scan this QR code with your authenticator app:
#   qrencode -t ansiutf8 'otpauth://totp/nself:john@example.com?secret=JBSWY3...'
#
# ⚠ Save your secret in a safe place!
#
# ℹ To complete enrollment, verify with a code:
#   nself mfa verify --method=totp --user=user-abc-123 --code=<6-digit-code>
```

**SMS Example:**
```bash
# Enroll phone number
nself mfa enable --method=sms --user=user-abc-123 --phone=+12025551234

# Output:
# ✓ SMS MFA enrolled successfully
#
# ℹ To complete enrollment, request a verification code:
#   nself mfa verify --method=sms --user=user-abc-123 --send
```

**Email Example:**
```bash
# Enroll email
nself mfa enable --method=email --user=user-abc-123 --email=john@example.com

# Output:
# ✓ Email MFA enrolled successfully
#
# ℹ To complete enrollment, request a verification code:
#   nself mfa verify --method=email --user=user-abc-123 --send
```

---

### `nself mfa verify`

Verify MFA enrollment or authenticate with MFA code.

**Syntax:**
```bash
nself mfa verify --method=<method> --user=<user_id> [--code=<code>] [--send]
```

**Options:**
- `--method=<method>` - MFA method: `totp`, `sms`, `email`, or `backup` (required)
- `--user=<user_id>` - User ID (required)
- `--code=<code>` - Verification code from authenticator/SMS/email
- `--send` - Send verification code (for SMS/email methods only)

**TOTP Verification:**
```bash
# Verify TOTP code
nself mfa verify --method=totp --user=user-abc-123 --code=836472

# Output on success:
# ℹ Verifying TOTP code...
# ✓ TOTP MFA enabled successfully

# Output on failure:
# ℹ Verifying TOTP code...
# ✗ Invalid TOTP code
```

**SMS Verification:**
```bash
# Step 1: Send code
nself mfa verify --method=sms --user=user-abc-123 --send

# Output:
# ℹ Sending SMS verification code...
# ✓ Verification code sent
#
# ℹ Verify with: nself mfa verify --method=sms --user=user-abc-123 --code=<6-digit-code>

# Step 2: Verify code
nself mfa verify --method=sms --user=user-abc-123 --code=836472
```

**Email Verification:**
```bash
# Step 1: Send code
nself mfa verify --method=email --user=user-abc-123 --send

# Step 2: Verify code
nself mfa verify --method=email --user=user-abc-123 --code=836472
```

**Backup Code Verification:**
```bash
# Verify backup code (one-time use)
nself mfa verify --method=backup --user=user-abc-123 --code=ABC123-DEF456
```

---

### `nself mfa disable`

Disable MFA method for a user.

**Syntax:**
```bash
nself mfa disable --method=<method> --user=<user_id>
```

**Options:**
- `--method=<method>` - MFA method to disable (required)
- `--user=<user_id>` - User ID (required)

**Examples:**
```bash
# Disable TOTP
nself mfa disable --method=totp --user=user-abc-123

# Disable SMS
nself mfa disable --method=sms --user=user-abc-123

# Disable email MFA
nself mfa disable --method=email --user=user-abc-123
```

**Output:**
```
✓ MFA method 'totp' disabled
```

---

### `nself mfa status`

Show MFA status and enabled methods for a user.

**Syntax:**
```bash
nself mfa status --user=<user_id>
```

**Options:**
- `--user=<user_id>` - User ID (required)

**Example:**
```bash
nself mfa status --user=user-abc-123
```

**Output:**
```
ℹ MFA status for user: user-abc-123

  MFA Enabled: true
  MFA Required: false
  Exempt: false

  Enabled Methods:
    TOTP: true
    SMS: false
    Email: false

  Backup Codes Remaining: 8
```

---

### `nself mfa backup-codes`

Manage backup codes for MFA recovery.

**Syntax:**
```bash
nself mfa backup-codes <action> --user=<user_id>
```

**Actions:**
- `generate` - Generate new backup codes
- `list` - List existing backup codes
- `status` - Show backup codes status

**Generate Backup Codes:**
```bash
nself mfa backup-codes generate --user=user-abc-123
```

**Output:**
```
ℹ Generating backup codes for user: user-abc-123
✓ Backup codes generated

⚠ Store these codes in a safe place. Each code can only be used once.

  ABC123-DEF456
  GHI789-JKL012
  MNO345-PQR678
  STU901-VWX234
  YZA567-BCD890
  EFG123-HIJ456
  KLM789-NOP012
  QRS345-TUV678
```

**List Backup Codes:**
```bash
nself mfa backup-codes list --user=user-abc-123
```

**Output (JSON):**
```json
{
  "codes": [
    {
      "code": "ABC123-DEF456",
      "used": false,
      "used_at": null
    },
    {
      "code": "GHI789-JKL012",
      "used": true,
      "used_at": "2026-01-29T15:30:00Z"
    }
  ]
}
```

**Check Status:**
```bash
nself mfa backup-codes status --user=user-abc-123
```

**Output:**
```
ℹ Backup codes status:
  Unused: 7
  Used: 1
  Total: 8
```

---

### `nself mfa policy`

Manage global MFA policies.

**Syntax:**
```bash
nself mfa policy <action> [options]
```

**Actions:**
- `set` - Set global MFA policy
- `get` - Get current MFA policy

**Set Policy:**
```bash
# Make MFA optional (default)
nself mfa policy set --type=optional

# Make MFA required for all users
nself mfa policy set --type=required

# Make MFA required for admins only
nself mfa policy set --type=admin-required
```

**Policy Types:**
- `optional` - Users can choose to enable MFA
- `required` - All users must enable MFA
- `admin-required` - Only admin users must enable MFA
- `role-based` - MFA requirement based on user role

**Get Policy:**
```bash
nself mfa policy get
```

**Output (JSON):**
```json
{
  "global_policy": "optional",
  "admin_required": false,
  "grace_period_days": 7,
  "allowed_methods": ["totp", "sms", "email", "backup"],
  "totp_window": 1,
  "sms_timeout": 300,
  "email_timeout": 600
}
```

---

### `nself mfa methods`

List available MFA methods and usage examples.

**Syntax:**
```bash
nself mfa methods
```

**Output:**
```
ℹ Available MFA methods:

  totp     - Time-based One-Time Password (Google Authenticator, Authy, etc.)
  sms      - SMS verification code
  email    - Email verification code
  backup   - One-time backup codes

ℹ Usage examples:
  nself mfa enable --method=totp --user=<user_id> --email=<email>
  nself mfa enable --method=sms --user=<user_id> --phone=<phone>
  nself mfa verify --method=totp --user=<user_id> --code=<code>
  nself mfa status --user=<user_id>
  nself mfa backup-codes generate --user=<user_id>
```

---

## MFA Methods Comparison

| Method | Security | Convenience | Requirements | Best For |
|--------|----------|-------------|--------------|----------|
| **TOTP** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Authenticator app | Most users (recommended) |
| **SMS** | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | SMS provider, phone | Users without smartphone apps |
| **Email** | ⭐⭐⭐ | ⭐⭐⭐⭐ | Email service | Fallback option |
| **Backup** | ⭐⭐⭐⭐ | ⭐⭐ | Safe storage | Recovery only |

---

## Complete Workflow

### 1. Enable MFA for User

```bash
# Enable TOTP (most secure and recommended)
nself mfa enable --method=totp --user=user-123 --email=john@example.com

# User scans QR code with authenticator app
```

### 2. Verify Enrollment

```bash
# User provides code from authenticator app
nself mfa verify --method=totp --user=user-123 --code=123456
```

### 3. Generate Backup Codes

```bash
# Generate 8 backup codes for recovery
nself mfa backup-codes generate --user=user-123

# User saves codes in password manager or prints them
```

### 4. Optionally Enable Additional Methods

```bash
# Add SMS as backup method
nself mfa enable --method=sms --user=user-123 --phone=+1234567890
nself mfa verify --method=sms --user=user-123 --send
nself mfa verify --method=sms --user=user-123 --code=836472
```

### 5. Check Status

```bash
# Verify all methods are enabled
nself mfa status --user=user-123
```

---

## Authenticator Apps

### Recommended Apps

**iOS:**
- Google Authenticator (Free)
- Authy (Free, cloud backup)
- 1Password (Paid, integrated password manager)
- Duo Mobile (Free)

**Android:**
- Google Authenticator (Free)
- Authy (Free, cloud backup)
- Microsoft Authenticator (Free)
- andOTP (Free, open source)

**Desktop:**
- 1Password (Paid, Mac/Windows/Linux)
- Authy (Free, Mac/Windows/Linux)

---

## Database Schema

### Tables

**`auth.mfa_enrollments`**
```sql
CREATE TABLE auth.mfa_enrollments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  method VARCHAR(20) NOT NULL, -- 'totp', 'sms', 'email'
  secret TEXT, -- Encrypted TOTP secret
  phone VARCHAR(20), -- For SMS
  email VARCHAR(255), -- For email
  verified BOOLEAN DEFAULT FALSE,
  enabled BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  verified_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ
);
```

**`auth.mfa_backup_codes`**
```sql
CREATE TABLE auth.mfa_backup_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  code VARCHAR(20) NOT NULL,
  used BOOLEAN DEFAULT FALSE,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Security Best Practices

### 1. Always Generate Backup Codes

```bash
# Generate immediately after enabling MFA
nself mfa backup-codes generate --user=user-123
```

**Why:** Backup codes are critical for account recovery if primary device is lost.

### 2. Use TOTP When Possible

TOTP is more secure than SMS (vulnerable to SIM swapping) and doesn't require external services.

### 3. Enforce MFA for Sensitive Roles

```bash
# Require MFA for all admin users
nself mfa policy set --type=admin-required
```

### 4. Monitor MFA Status

```bash
# Regular audits
nself auth users list --format json | jq '.[] | select(.mfa_enabled == false)'
```

### 5. Test MFA Before Enforcing

```bash
# Test with your account first
nself mfa enable --method=totp --user=$(nself auth whoami --id)
nself mfa verify --method=totp --user=$(nself auth whoami --id) --code=123456
```

---

## Troubleshooting

### TOTP Code Not Working

**Problem:** "Invalid TOTP code" error

**Solutions:**
1. Check time synchronization:
   ```bash
   # On Linux/Mac
   date

   # Time must be accurate within 30 seconds
   ```

2. Verify authenticator app clock:
   - Open authenticator app settings
   - Enable "Time Correction for Codes" (Google Authenticator)
   - Force time sync

3. Check TOTP window setting:
   ```bash
   # Allow wider time window (1 = ±30 seconds)
   nself mfa policy set --totp-window=1
   ```

### SMS Not Arriving

**Problem:** SMS verification code not received

**Solutions:**
1. Check SMS provider configuration:
   ```bash
   nself config get SMS_PROVIDER
   nself config get TWILIO_ACCOUNT_SID
   ```

2. Verify phone number format:
   ```bash
   # Must be E.164 format: +[country code][number]
   # Good: +12025551234
   # Bad: (202) 555-1234
   ```

3. Check SMS service status:
   ```bash
   nself status sms
   nself logs sms --tail 100
   ```

### Locked Out Without Backup Codes

**Problem:** Lost phone and no backup codes

**Solutions:**
1. Admin can disable MFA:
   ```bash
   # Requires admin privileges
   nself mfa disable --method=totp --user=user-123
   ```

2. Database-level recovery (last resort):
   ```sql
   -- As postgres user
   UPDATE auth.mfa_enrollments
   SET enabled = FALSE
   WHERE user_id = 'user-123';
   ```

### Backup Codes Not Working

**Problem:** Backup code marked as used

**Solutions:**
1. Check backup code status:
   ```bash
   nself mfa backup-codes list --user=user-123
   ```

2. Generate new codes:
   ```bash
   nself mfa backup-codes generate --user=user-123
   ```

---

## Integration with Authentication

### Client-Side Integration

**JavaScript/TypeScript Example:**
```typescript
import { Auth } from '@nself/client';

const auth = new Auth({ url: 'https://auth.yourdomain.com' });

// Sign in (first factor)
const { user, mfa_required } = await auth.signIn({
  email: 'john@example.com',
  password: 'password123'
});

if (mfa_required) {
  // Prompt user for MFA code
  const code = prompt('Enter MFA code:');

  // Verify MFA (second factor)
  const { session } = await auth.verifyMFA({
    user_id: user.id,
    code: code,
    method: 'totp' // or 'sms', 'email', 'backup'
  });

  // Authenticated with MFA
  console.log('Session:', session);
}
```

**React Hook Example:**
```typescript
import { useAuth, useMFA } from '@nself/react';

function LoginForm() {
  const { signIn } = useAuth();
  const { verifyMFA, mfaRequired } = useMFA();

  const handleLogin = async (email: string, password: string) => {
    const { user, mfa_required } = await signIn(email, password);

    if (mfa_required) {
      // Show MFA input form
      setShowMFAForm(true);
    }
  };

  const handleMFAVerify = async (code: string) => {
    await verifyMFA(code, 'totp');
    // Redirect to dashboard
  };

  return mfaRequired ? <MFAForm onSubmit={handleMFAVerify} /> : <LoginForm onSubmit={handleLogin} />;
}
```

---

## API Reference

### REST Endpoints

**Enable MFA:**
```http
POST /auth/mfa/enroll
Content-Type: application/json

{
  "user_id": "user-123",
  "method": "totp",
  "email": "john@example.com"
}
```

**Verify MFA:**
```http
POST /auth/mfa/verify
Content-Type: application/json

{
  "user_id": "user-123",
  "method": "totp",
  "code": "123456"
}
```

**Get MFA Status:**
```http
GET /auth/mfa/status?user_id=user-123
```

See [API Reference](../reference/api/README.md) for complete API reference.

---

## Compliance

### Standards Supported

- **SOC 2 Type II:** MFA as additional security control
- **HIPAA:** Technical safeguards for PHI access
- **PCI-DSS:** Requirement 8.3 (Multi-factor authentication)
- **NIST 800-63B:** Level 2 and 3 authenticators
- **GDPR:** Enhanced security for personal data access

### Audit Trail

All MFA events are logged:

```bash
# View MFA audit log
nself audit mfa --user=user-123 --start="2026-01-01"

# Example output:
# 2026-01-30 10:00:00 | user-123 | MFA_ENABLED | method=totp
# 2026-01-30 10:05:00 | user-123 | MFA_VERIFIED | method=totp
# 2026-01-30 11:00:00 | user-123 | MFA_LOGIN | method=totp success=true
```

---

## Performance

### Latency

- **TOTP Verification:** <10ms (local computation)
- **SMS Delivery:** 1-5 seconds (depends on provider)
- **Email Delivery:** 1-10 seconds (depends on email service)

### Scaling

MFA verification is stateless and scales horizontally:

```yaml
# docker-compose.yml
services:
  auth:
    image: nhost/hasura-auth:latest
    deploy:
      replicas: 5  # Scale MFA verification
```

---

## Related Commands

- [`nself auth`](./AUTH.md) - User authentication management
- [`nself security`](./SECURITY.md) - Security settings
- [`nself audit`](./AUDIT.md) - Security audit logs
- [`nself roles`](./ROLES.md) - Role-based access control

---

## Related Documentation

- [Authentication Guide](../guides/SECURITY.md)
- [Security Best Practices](../security/SECURITY-BEST-PRACTICES.md)
- [API Reference](../reference/api/README.md)
- [Compliance Documentation](../security/COMPLIANCE-GUIDE.md)

---

**Last Updated:** January 30, 2026
**Version:** 0.9.5
**Status:** Production Ready ✅
