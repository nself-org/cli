# Advanced Security System

**Sprint 17: Advanced Security (25pts)**

Complete security infrastructure for self-hosted nself backends.

## Features

### 1. WebAuthn/FIDO2 Support
- Hardware security keys (YubiKey, Titan, etc.)
- Platform authenticators (Touch ID, Face ID, Windows Hello)
- FIDO2 compliant implementation
- Anti-replay protection with signature counters
- Multiple keys per user

### 2. Device Management
- Track all user devices
- Device fingerprinting
- Trust/untrust devices
- Risk scoring per device
- Session correlation

### 3. Security Event Logging
- Comprehensive audit trail
- Real-time event capture
- Suspicious activity detection
- Risk scoring
- Event categorization (authentication, authorization, device, suspicious)

### 4. Security Incidents
- Automated incident creation
- Incident tracking and management
- Priority and severity levels
- Assignment and resolution workflow
- Forensics and evidence collection

### 5. Security Scanning
- Weak password detection
- Missing MFA warnings
- Expired session cleanup
- Suspicious activity patterns
- Brute force detection
- Credential stuffing detection

### 6. Password Security
- Password history tracking
- Reuse prevention
- Age tracking
- Strength validation
- Common password detection

## Database Schema

### Tables Created

1. **auth.webauthn_credentials** - WebAuthn/FIDO2 keys
2. **auth.user_devices** - Device tracking
3. **auth.security_events** - Security audit log
4. **auth.security_incidents** - Incident management
5. **auth.security_metrics** - Security analytics
6. **auth.password_history** - Password change tracking

### Helper Functions

- `auth.log_security_event()` - Log security events
- `auth.calculate_device_risk_score()` - Device risk assessment
- `auth.detect_suspicious_activity()` - Pattern detection
- `auth.get_weak_passwords()` - Vulnerability scanning

## CLI Commands

### Security Scanning

```bash
# Run full security scan
nself security scan

# Scan for weak passwords
nself security scan passwords

# Scan for missing MFA
nself security scan mfa

# Scan for expired sessions
nself security scan sessions

# Scan for suspicious activity
nself security scan suspicious
```

### Device Management

```bash
# List devices for a user
nself security devices list <user_id>

# Trust a device
nself security devices trust <device_id>

# Untrust a device
nself security devices untrust <device_id>

# Remove a device
nself security devices remove <device_id>
```

### Security Incidents

```bash
# List open incidents
nself security incidents list

# List resolved incidents
nself security incidents list resolved

# Show incident details
nself security incidents show <incident_id>

# Create manual incident
nself security incidents create "Incident Title" high "Description"

# Resolve incident
nself security incidents resolve <incident_id> "Resolution notes"
```

### Security Events

```bash
# View recent security events
nself security events

# View events for specific user
nself security events <user_id> [limit]
```

### WebAuthn Management

```bash
# List WebAuthn keys for user
nself security webauthn list <user_id>

# Remove WebAuthn key
nself security webauthn remove <key_id>

# Note: Adding keys must be done through web UI
```

### MFA Shortcut

```bash
# Delegate to MFA CLI
nself security mfa enable --method=totp --user=<user_id>
```

## Security Libraries

### scanner.sh
- Password strength checking
- Session anomaly detection
- Device fingerprinting
- User agent parsing
- Brute force detection
- Credential stuffing detection
- Account takeover detection
- Risk scoring
- Vulnerability scanning (SQL injection, XSS)

### webauthn.sh
- Challenge generation
- Registration options
- Authentication options
- Credential storage
- Signature verification
- Counter management
- Attestation processing
- Transport detection
- Authenticator type identification
- Security level assessment

### incident-response.sh
- Incident detection automation
- Automated response playbooks
- Account locking
- IP blocking
- Session revocation
- Password reset enforcement
- MFA requirement
- Escalation workflows
- Forensics collection
- Incident analysis
- Resolution tracking
- Metrics (MTTD, MTTR)

## Usage Examples

### Enable Security for a Project

1. **Run the migration:**
   ```bash
   nself migrate run 014_create_security_system.sql
   ```

2. **Scan for vulnerabilities:**
   ```bash
   nself security scan
   ```

3. **Enable MFA for users:**
   ```bash
   nself security mfa enable --method=totp --user=<user_id>
   ```

4. **Monitor security events:**
   ```bash
   nself security events
   ```

### Respond to Security Incident

1. **Detect incident:**
   ```bash
   nself security scan suspicious
   ```

2. **Review incident:**
   ```bash
   nself security incidents list
   nself security incidents show <incident_id>
   ```

3. **Take action:**
   ```bash
   # Lock compromised account
   # (would be done through API)

   # Block malicious IP
   # (would be done through API)

   # Revoke sessions
   # (would be done through API)
   ```

4. **Resolve incident:**
   ```bash
   nself security incidents resolve <incident_id> "Incident resolved, user notified"
   ```

### Monitor Device Security

```bash
# List devices for user
nself security devices list <user_id>

# Trust known devices
nself security devices trust <device_id>

# Remove suspicious devices
nself security devices remove <device_id>
```

## Security Best Practices

### For Self-Hosted Users

1. **Enable MFA for all admin accounts**
   - Use TOTP (Google Authenticator, Authy)
   - Add hardware keys (YubiKey) for critical accounts
   - Generate backup codes

2. **Monitor security events regularly**
   - Check for suspicious activity daily
   - Review open incidents weekly
   - Investigate high-risk events immediately

3. **Implement password policies**
   - Enforce strong passwords
   - Require password changes every 90 days
   - Prevent password reuse (last 5 passwords)

4. **Trust user devices**
   - Review devices monthly
   - Remove inactive devices
   - Trust only known devices

5. **Enable automated responses**
   - Configure incident response playbooks
   - Set up alerting (email, Slack, PagerDuty)
   - Automate account locking for brute force

### For Production Deployments

1. **Hardware Security Keys**
   - Require hardware keys for admin access
   - Support multiple keys per user (backup)
   - Use FIPS-certified keys for compliance

2. **Security Monitoring**
   - Set up Prometheus alerts for security events
   - Use Grafana dashboards for visualization
   - Export security events to SIEM

3. **Incident Response**
   - Define incident severity levels
   - Create escalation procedures
   - Conduct post-incident reviews
   - Document lessons learned

4. **Compliance**
   - Regular security audits
   - Penetration testing
   - Vulnerability scanning
   - Access reviews

## API Integration

Security features are exposed through Hasura GraphQL:

```graphql
# Query security events
query GetSecurityEvents($userId: uuid!) {
  auth_security_events(
    where: { user_id: { _eq: $userId } }
    order_by: { created_at: desc }
    limit: 50
  ) {
    id
    event_type
    severity
    description
    is_suspicious
    risk_score
    created_at
  }
}

# Query user devices
query GetUserDevices($userId: uuid!) {
  auth_user_devices(
    where: { user_id: { _eq: $userId } }
    order_by: { last_seen_at: desc }
  ) {
    id
    device_name
    device_type
    is_trusted
    risk_score
    last_seen_at
  }
}

# Query WebAuthn credentials
query GetWebAuthnKeys($userId: uuid!) {
  auth_webauthn_credentials(
    where: { user_id: { _eq: $userId } }
    order_by: { created_at: desc }
  ) {
    id
    name
    credential_type
    authenticator_attachment
    created_at
    last_used_at
  }
}
```

## Architecture

### Event Flow

```
User Action → Application → Security Event Logger
                               ↓
                    Security Event Table
                               ↓
                    Suspicious Activity Detector
                               ↓
                         Incident Creator
                               ↓
                    Automated Response Playbook
                               ↓
                    Resolution & Metrics
```

### Device Tracking Flow

```
User Login → Device Fingerprint → Device Lookup
                                      ↓
                              New Device?
                              ↓         ↓
                            Yes        No
                             ↓          ↓
                   Create Device    Update Last Seen
                             ↓          ↓
                   Calculate Risk Score
                             ↓
                   Log Security Event
                             ↓
                   Check If Suspicious
                             ↓
                   Create Incident If Needed
```

### WebAuthn Flow

```
Registration:
User → Generate Challenge → Frontend Creates Credential
       ↓
    Store Public Key & Counter
       ↓
    Mark As Enrolled

Authentication:
User → Generate Challenge → Frontend Signs Challenge
       ↓
    Verify Signature
       ↓
    Check Counter (anti-replay)
       ↓
    Update Counter
       ↓
    Log Security Event
```

## Performance Considerations

### Indexing
- All foreign keys are indexed
- Time-series queries optimized (created_at DESC)
- Risk score queries indexed
- Suspicious activity filtered with partial index

### Partitioning (Optional)
- Security events can be partitioned by month
- Metrics table can be partitioned by day
- Improves query performance for large datasets

### Retention
- Consider archiving old security events (>1 year)
- Keep incident records indefinitely (compliance)
- Rotate security metrics based on policy

## Testing

### Manual Testing

```bash
# Create test user
nself auth signup --email=test@example.com --password=test123

# Enable MFA
nself security mfa enable --method=totp --user=<user_id>

# Simulate brute force (would trigger incident)
# Multiple failed logins

# View security events
nself security events <user_id>

# Check incidents
nself security incidents list
```

### Integration Testing

Security features integrate with:
- Authentication system (auth.sh, mfa.sh)
- Rate limiting (rate-limit.sh)
- Session management
- Hasura GraphQL
- PostgreSQL RLS

## Troubleshooting

### Security Events Not Logging
- Check PostgreSQL connection
- Verify auth schema exists
- Check RLS policies
- Verify user permissions

### WebAuthn Not Working
- Ensure HTTPS (required for WebAuthn)
- Check browser compatibility
- Verify RP ID matches domain
- Check credential storage

### Incidents Not Creating
- Check suspicious activity detection
- Verify incident table exists
- Check detection thresholds
- Review security event logs

## Future Enhancements

- [ ] IP geolocation integration
- [ ] Behavioral biometrics
- [ ] Threat intelligence feeds
- [ ] SIEM integration
- [ ] Automated penetration testing
- [ ] Security compliance reports (SOC 2, HIPAA)
- [ ] Advanced ML-based anomaly detection
- [ ] Real-time security dashboards

## Support

For issues and questions:
- Documentation: https://docs.nself.org/security
- GitHub Issues: https://github.com/nself-org/cli/issues
- Discord: https://discord.gg/nself

## License

Part of nself - MIT License
