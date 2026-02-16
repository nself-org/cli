# Multi-Factor Authentication (MFA) Setup

Complete guide to enabling and managing multi-factor authentication.

## Overview

nself supports multiple MFA methods:
- Time-based One-Time Password (TOTP)
- SMS Codes
- Email Codes
- Backup Codes

## TOTP Setup (Recommended)

TOTP works with Google Authenticator, Authy, Microsoft Authenticator, and more.

```bash
# Enable TOTP for user
nself auth mfa enable --user user-id --method totp

# Get QR code for scanning
nself auth mfa qr-code --user user-id

# Verify code after user scans
nself auth mfa verify --user user-id --code 123456
```

## SMS & Email MFA

See [Authentication Guide](AUTHENTICATION.md#multi-factor-authentication-mfa) for complete SMS and Email configuration.

## Backup Codes

Generate recovery codes for account access if MFA device is lost:

```bash
nself auth mfa backup-codes --user user-id
```

Users should store these in a secure location.

## Management

```bash
# Disable MFA for user
nself auth mfa disable --user user-id

# List user's MFA methods
nself auth mfa list --user user-id

# Revoke specific method
nself auth mfa revoke --user user-id --method totp
```

## See Also

- [Authentication Guide](AUTHENTICATION.md) - Complete auth documentation
- [Security Best Practices](../configuration/SECRETS-MANAGEMENT.md)

