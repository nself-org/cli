# nself auth devices

> **DEPRECATED COMMAND NAME**: This command was formerly `nself devices` in v0.x. It has been consolidated to `nself auth devices` in v1.0. The old command name may still work as an alias.

Device management and trusted device authentication.

## Description

The `nself auth devices` command manages user devices, device trust, and device-based authentication. It provides tools for tracking user devices, managing trusted devices, and implementing device-based security policies.

**Use Cases:**
- Track devices accessing user accounts
- Implement trusted device authentication
- Revoke access from compromised devices
- Monitor device usage patterns
- Enhanced security with device fingerprinting

---

## Usage

```bash
nself auth devices <command> [options]
```

---

## Commands

### `init` - Initialize Device Management

Initialize the device management system in your database.

```bash
nself auth devices init
```

**What it does:**
- Creates device management tables
- Sets up device fingerprinting
- Configures device trust policies
- Enables device tracking

**Example:**
```bash
nself auth devices init
```

**Output:**
```
✓ Device management initialized
```

---

### `list` - List User Devices

List all devices for a specific user.

```bash
nself auth devices list <user_id>
```

**Parameters:**
- `user_id` - User UUID

**Example:**
```bash
nself auth devices list 550e8400-e29b-41d4-a716-446655440000
```

**Output (JSON):**
```json
[
  {
    "id": "dev_abc123def456",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "fingerprint": "chrome-macos-192.168.1.100",
    "user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)...",
    "device_type": "desktop",
    "os": "macOS",
    "browser": "Chrome 120",
    "ip_address": "192.168.1.100",
    "trusted": true,
    "last_used": "2026-01-30T12:34:56Z",
    "created_at": "2026-01-15T10:00:00Z",
    "trust_expires_at": "2026-04-15T10:00:00Z"
  },
  {
    "id": "dev_xyz789abc123",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "fingerprint": "safari-ios-192.168.1.105",
    "user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X)...",
    "device_type": "mobile",
    "os": "iOS 17.2",
    "browser": "Safari Mobile",
    "ip_address": "192.168.1.105",
    "trusted": false,
    "last_used": "2026-01-29T08:20:00Z",
    "created_at": "2026-01-20T14:30:00Z",
    "trust_expires_at": null
  }
]
```

---

### `trust` - Trust a Device

Mark a device as trusted for a user.

```bash
nself auth devices trust <device_id>
```

**Parameters:**
- `device_id` - Device ID

**Example:**
```bash
nself auth devices trust dev_xyz789abc123
```

**Output:**
```
✓ Device trusted
```

**What happens:**
- Device is marked as trusted
- Trust expiration is set (default: 90 days)
- User can skip MFA on this device
- Device added to trusted device list

---

### `revoke` - Revoke Device Access

Revoke trust and access for a device.

```bash
nself auth devices revoke <device_id>
```

**Parameters:**
- `device_id` - Device ID

**Example:**
```bash
nself auth devices revoke dev_xyz789abc123
```

**Output:**
```
✓ Device revoked
```

**What happens:**
- Device trust is removed
- All sessions on this device are invalidated
- Device can no longer skip MFA
- User must re-authenticate on this device

---

## Options

| Option | Description |
|--------|-------------|
| `-h, --help` | Show help message |

---

## Device Fields

### Device Information

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Unique device identifier |
| `user_id` | UUID | User who owns the device |
| `fingerprint` | String | Device fingerprint hash |
| `user_agent` | String | Full user agent string |
| `device_type` | Enum | `desktop`, `mobile`, `tablet`, `other` |
| `os` | String | Operating system (e.g., "macOS", "iOS") |
| `browser` | String | Browser name and version |
| `ip_address` | String | Last known IP address |
| `trusted` | Boolean | Whether device is trusted |
| `last_used` | Timestamp | Last authentication time |
| `created_at` | Timestamp | First seen timestamp |
| `trust_expires_at` | Timestamp | When trust expires (if trusted) |

---

## Examples

### Basic Device Management

```bash
# Initialize device management
nself auth devices init

# List devices for a user
nself auth devices list 550e8400-e29b-41d4-a716-446655440000

# Trust a device
nself auth devices trust dev_abc123

# Revoke a compromised device
nself auth devices revoke dev_suspicious123
```

### Security Response Workflow

```bash
# 1. User reports suspicious activity
# 2. List all devices for the user
nself auth devices list <user-uuid>

# 3. Revoke unrecognized devices
nself auth devices revoke dev_unknown123
nself auth devices revoke dev_foreign456

# 4. User re-authenticates on trusted devices
```

### Trusted Device Enrollment

```bash
# 1. User logs in from new device
# Device is automatically registered but untrusted

# 2. User verifies device via email/SMS
# Device is marked as trusted
nself auth devices trust <device-id>

# 3. User can now skip MFA on this device for 90 days
```

---

## Device Fingerprinting

### How Fingerprints Work

Device fingerprints are generated from:
- User agent string
- IP address
- Browser type and version
- Operating system
- Screen resolution
- Timezone
- Language settings
- Installed plugins

**Example fingerprint:**
```
chrome-120-macos-en-US-1920x1080-America/New_York
```

**Hashed fingerprint:**
```
a3f5e8d9c2b1a0e7f6d5c4b3a2f1e0d9c8b7a6e5
```

### Fingerprint Security

- Fingerprints are **hashed** - original data not stored
- **Collision-resistant** - unlikely for two devices to match
- **Privacy-preserving** - no personal data in fingerprint
- **Volatile** - changes if user changes browser/OS/settings

---

## Device Trust Policies

### Default Trust Policy

```bash
# Trust duration: 90 days
NSELF_DEVICE_TRUST_DURATION=7776000  # seconds

# Require MFA for untrusted devices
NSELF_DEVICE_REQUIRE_MFA_UNTRUSTED=true

# Auto-trust after N successful logins
NSELF_DEVICE_AUTO_TRUST_THRESHOLD=5

# Max trusted devices per user
NSELF_DEVICE_MAX_TRUSTED=10
```

### Trust Expiration

Trusted devices automatically expire after the trust duration:
- **Desktop devices:** 90 days (default)
- **Mobile devices:** 180 days
- **Tablets:** 180 days

Users must re-verify devices after expiration.

---

## Use Cases

### Enhanced Security

**Require MFA on untrusted devices:**
```typescript
// Middleware example
if (!device.trusted && !session.mfa_verified) {
  return requireMFA();
}
```

**Detect suspicious devices:**
```bash
# List devices from unusual locations
nself auth devices list <user-id> | jq '.[] | select(.ip_address | startswith("203."))'
```

### User Experience

**Remember this device:**
```bash
# User checks "Remember this device"
# Device is automatically trusted after login
nself auth devices trust <device-id>
```

**Skip MFA on trusted devices:**
```bash
# User logs in from trusted device
# MFA is skipped automatically
```

### Compliance

**Track all access points:**
```bash
# Audit all devices accessing sensitive data
nself auth devices list <user-id>
```

**Revoke access for ex-employees:**
```bash
# Revoke all devices for a user
for device_id in $(nself auth devices list <user-id> | jq -r '.[].id'); do
  nself auth devices revoke $device_id
done
```

---

## Integration Examples

### Automatic Device Registration

When a user logs in, automatically register their device:

```typescript
// Example login handler
async function handleLogin(req, res) {
  const user = await authenticateUser(req.body);

  // Register device
  const deviceFingerprint = generateFingerprint(req);
  const device = await registerDevice({
    user_id: user.id,
    fingerprint: deviceFingerprint,
    user_agent: req.headers['user-agent'],
    ip_address: req.ip
  });

  // Check if device is trusted
  if (!device.trusted) {
    // Require MFA
    return res.json({ requires_mfa: true, device_id: device.id });
  }

  // Allow login
  return res.json({ token: generateToken(user) });
}
```

### Device-Based MFA

Implement device-based MFA policy:

```typescript
// Check device trust before allowing access
async function checkDeviceTrust(userId, deviceId) {
  const device = await getDevice(deviceId);

  if (!device || device.user_id !== userId) {
    throw new Error('Invalid device');
  }

  if (!device.trusted) {
    return { requiresMFA: true, device };
  }

  if (device.trust_expires_at < new Date()) {
    return { requiresMFA: true, device, expired: true };
  }

  return { requiresMFA: false, device };
}
```

---

## Database Schema

### Devices Table

```sql
CREATE TABLE devices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  fingerprint VARCHAR(255) NOT NULL,
  user_agent TEXT,
  device_type VARCHAR(50),
  os VARCHAR(100),
  browser VARCHAR(100),
  ip_address INET,
  trusted BOOLEAN DEFAULT false,
  last_used TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  trust_expires_at TIMESTAMP WITH TIME ZONE,

  -- Indexes
  INDEX idx_devices_user_id (user_id),
  INDEX idx_devices_fingerprint (fingerprint),
  INDEX idx_devices_trusted (trusted),
  UNIQUE (user_id, fingerprint)
);
```

### Device Events Table

```sql
CREATE TABLE device_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id UUID REFERENCES devices(id) ON DELETE CASCADE,
  event_type VARCHAR(50) NOT NULL,
  ip_address INET,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  INDEX idx_device_events_device_id (device_id),
  INDEX idx_device_events_type (event_type)
);
```

**Event types:**
- `device.registered` - Device first registered
- `device.trusted` - Device marked as trusted
- `device.revoked` - Device trust revoked
- `device.login` - Login from device
- `device.suspicious` - Suspicious activity detected

---

## Troubleshooting

### Device Not Found

```bash
# Verify device ID
nself auth devices list <user-id> | jq '.[].id'

# Check device was deleted
# Devices are automatically deleted after 365 days of inactivity
```

### Trust Not Working

```bash
# Verify device is trusted
nself auth devices list <user-id> | jq '.[] | select(.id=="<device-id>")'

# Check trust expiration
# Trust expires after configured duration (default: 90 days)

# Re-trust device
nself auth devices trust <device-id>
```

### Too Many Devices

```bash
# List all devices
nself auth devices list <user-id>

# Revoke old/unused devices
nself auth devices revoke <old-device-id>

# Configure max trusted devices
NSELF_DEVICE_MAX_TRUSTED=20
```

---

## Related Commands

- **[auth](AUTH.md)** - Authentication management
- **[mfa](MFA.md)** - Multi-factor authentication
- **[audit](AUDIT.md)** - Audit log management
- **[security](SECURITY.md)** - Security tools

---

## Security Best Practices

1. **Always verify new devices** via email/SMS before trusting
2. **Set reasonable trust expiration** (30-90 days recommended)
3. **Monitor device list regularly** for suspicious devices
4. **Revoke lost/stolen devices** immediately
5. **Limit max trusted devices** per user (5-10 recommended)
6. **Require MFA for high-risk actions** even on trusted devices
7. **Log device events** for audit trail
8. **Implement device blocking** for repeated failed attempts

---

**Version:** v0.6.0+
**Category:** Authentication & Security
**Related Documentation:**
- [Authentication Guide](../guides/SECURITY.md)
- [Trusted Device Setup](AUTH.md)
- [Security Best Practices](../security/SECURITY-BEST-PRACTICES.md)
