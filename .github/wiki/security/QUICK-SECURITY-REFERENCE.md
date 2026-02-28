# Quick Security Reference - nself v0.9.9

**For**: Developers and DevOps Engineers
**Updated**: January 31, 2026

---

## Common Security Tasks

### Run Security Scan
```bash
# Quick scan
nself security scan

# Comprehensive scan
nself security scan --comprehensive

# Check file permissions
nself security check-permissions

# Scan for secrets
nself security scan-secrets
```

### Run Security Tests
```bash
# All tests
bash src/tests/security/test-sql-injection.sh
bash src/tests/security/test-command-injection.sh
bash src/tests/security/test-permissions.sh
bash src/tests/security/test-secrets.sh

# Or run all at once
for test in src/tests/security/test-*.sh; do bash "$test"; done
```

### Fix Common Issues
```bash
# Fix file permissions
nself security fix-permissions

# Rotate secrets
nself auth secrets rotate jwt
nself auth apikey rotate-all --grace-period 7d

# Update dependencies
nself update check
nself update apply
```

---

## Safe Database Queries

### Use Safe Query Wrapper

**Always use `safe-query.sh` for database operations with user input**

```bash
# Source the safe query library
source "$(dirname "${BASH_SOURCE[0]}")/database/safe-query.sh"

# SELECT by ID (safe)
pg_select_by_id "auth.users" "id" "$user_id" "id, email, created_at"

# INSERT with parameters (safe)
pg_insert_returning_id "auth.users" "email, phone" "$email" "$phone"

# UPDATE by ID (safe)
pg_update_by_id "auth.users" "id" "$user_id" "email, phone" "$new_email" "$new_phone"

# DELETE by ID (safe)
pg_delete_by_id "auth.users" "id" "$user_id"

# Custom query with parameters (safe)
pg_query_safe "SELECT * FROM auth.users WHERE email = :'param1' AND active = true" "$email"

# Get single value (safe)
count=$(pg_query_value "SELECT COUNT(*) FROM auth.users WHERE role = :'param1'" "$role")
```

### Input Validation

```bash
# Validate UUID
uuid=$(validate_uuid "$input_uuid") || { echo "Invalid UUID"; exit 1; }

# Validate email
email=$(validate_email "$input_email") || { echo "Invalid email"; exit 1; }

# Validate integer
age=$(validate_integer "$input_age" 0 120) || { echo "Invalid age"; exit 1; }

# Validate identifier (alphanumeric + underscore/hyphen)
name=$(validate_identifier "$input_name" 50) || { echo "Invalid name"; exit 1; }

# Validate JSON
json=$(validate_json "$input_json") || { echo "Invalid JSON"; exit 1; }
```

### Escape Strings (last resort - prefer parameterized queries)

```bash
# Only if parameterized queries are not possible
escaped=$(sql_escape "$user_input")
# Then use with caution
```

---

## Avoid These Anti-Patterns

### ❌ DON'T: Direct String Interpolation in SQL
```bash
# DANGEROUS - SQL injection risk
db_query "DELETE FROM users WHERE email = '$email'"

# DANGEROUS - can execute arbitrary SQL
db_query "SELECT * FROM users WHERE name = '$name'"
```

### ✅ DO: Use Parameterized Queries
```bash
# SAFE - parameter binding prevents injection
pg_query_safe "DELETE FROM auth.users WHERE email = :'param1'" "$email"

# SAFE - validated and parameterized
pg_query_safe "SELECT * FROM auth.users WHERE name = :'param1'" "$name"
```

### ❌ DON'T: Unquoted Variables in Commands
```bash
# DANGEROUS - command injection risk
docker exec $container command

# DANGEROUS - variable expansion in eval
eval "command $user_input"
```

### ✅ DO: Properly Quote Variables
```bash
# SAFE - quoted variables
docker exec "$container" command

# SAFE - validate before use
if [[ "$input" =~ ^[a-zA-Z0-9_-]+$ ]]; then
  docker exec "$container" command "$input"
fi
```

---

## Rate Limiting Quick Reference

### Configure Rate Limits
```bash
# Global default
nself auth rate-limit set default 100 per-minute

# Specific endpoints
nself auth rate-limit set /api/* 100 per-minute
nself auth rate-limit set /graphql 50 per-minute
nself auth rate-limit set /auth/login 5 per-minute
nself auth rate-limit set /auth/signup 3 per-minute
```

### Manage IP Lists
```bash
# Whitelist (bypass rate limits)
nself auth rate-limit whitelist add 192.168.1.100
nself auth rate-limit whitelist list

# Blacklist (block completely)
nself auth rate-limit blacklist add 10.0.0.50
nself auth rate-limit blacklist list

# Remove from lists
nself auth rate-limit whitelist remove 192.168.1.100
nself auth rate-limit blacklist remove 10.0.0.50
```

### Monitor Rate Limiting
```bash
# View statistics
nself auth rate-limit stats

# Top blocked IPs
nself auth rate-limit stats --top-blocked

# View in Grafana
open https://grafana.yourdomain.com/d/rate-limiting
```

---

## Secret Management

### Environment File Hierarchy
```
.env.dev      → Base config (committed to git)
.env.local    → Your overrides (gitignored)
.env.staging  → Staging overrides (on server only)
.env.prod     → Production overrides (on server only)
.secrets      → Ultra-sensitive (on server only, SSH-synced)
```

### Sync Secrets (Role-based)
```bash
# Dev - local only
nself env switch local

# Sr Dev - can access staging
nself sync pull staging
nself env switch staging

# Lead Dev - can access production
nself sync pull prod
nself sync pull secrets
nself env switch prod
```

### Check Access Level
```bash
# See what you can access
nself env access

# Test specific environment
nself env access --check staging
nself env access --check prod
```

### Rotate Secrets
```bash
# JWT secrets
nself auth secrets rotate jwt --grace-period 7d

# API keys
nself auth apikey rotate-all --grace-period 14d

# Database password (manual - see docs)
# SSL certificates (automatic with Let's Encrypt)
```

---

## SSL/TLS Management

### Set Up SSL
```bash
# Let's Encrypt (recommended)
nself auth ssl certbot --domain yourdomain.com --email admin@yourdomain.com

# Custom certificate
nself auth ssl install --cert /path/to/cert.pem --key /path/to/key.pem

# Self-signed (development only)
nself auth ssl generate-self-signed --domain local.nself.org
```

### Verify SSL
```bash
# Quick test
nself auth ssl test

# Detailed check
nself auth ssl status

# Test with external tool
openssl s_client -connect yourdomain.com:443 -tls1_2

# SSL Labs (for production)
# Visit: https://www.ssllabs.com/ssltest/analyze.html?d=yourdomain.com
```

### Auto-Renewal
```bash
# Enable automatic renewal
nself auth ssl auto-renew enable

# Check renewal status
nself auth ssl auto-renew status

# Test renewal
nself auth ssl auto-renew test

# Verify cron job
crontab -l | grep certbot
```

---

## Authentication Security

### User Management
```bash
# List users
nself auth users list

# List admins
nself auth users list --role admin

# Delete user
nself auth users delete user@example.com

# Clean up inactive users
nself auth users cleanup --inactive 90d
```

### Multi-Factor Authentication (MFA)
```bash
# Enable MFA for user
nself auth mfa enable user@example.com

# Enforce MFA for all admins
nself auth mfa enforce --role admin

# Check MFA status
nself auth mfa status

# Disable MFA for user
nself auth mfa disable user@example.com
```

### API Keys
```bash
# Create API key
nself auth apikey create "My App" --scopes read,write

# List keys
nself auth apikey list

# Revoke key
nself auth apikey revoke <key-id>

# Rotate all keys (with grace period)
nself auth apikey rotate-all --grace-period 7d
```

### Sessions
```bash
# List active sessions
nself auth sessions list

# Revoke session
nself auth sessions revoke <session-id>

# Revoke all sessions for user
nself auth sessions revoke-all user@example.com

# Clean up expired sessions
nself auth sessions cleanup
```

---

## Monitoring & Alerts

### View Dashboards
```bash
# Open Grafana
nself monitor dashboard

# Specific dashboard
nself monitor dashboard open nself-overview
nself monitor dashboard open database-performance
nself monitor dashboard open api-metrics
```

### Configure Alerts
```bash
# CPU alert
nself monitor alert add cpu-high --threshold 80 --duration 5m

# Disk space alert
nself monitor alert add disk-low --threshold 90 --duration 10m

# Service down alert
nself monitor alert add service-down --service postgres
nself monitor alert add service-down --service hasura

# Error rate alert
nself monitor alert add error-rate-high --threshold 5% --duration 5m
```

### Manage Alerts
```bash
# List alerts
nself monitor alerts list

# Test alert
nself monitor alert test <alert-name>

# Disable alert
nself monitor alert disable <alert-name>

# View alert history
nself monitor alert history --since 7d
```

### Alert Channels
```bash
# Configure email
nself monitor alert configure --email team@company.com

# Configure Slack
nself monitor alert configure --slack-webhook https://hooks.slack.com/...

# Configure PagerDuty
nself monitor alert configure --pagerduty-key <integration-key>
```

---

## Backup & Recovery

### Create Backups
```bash
# Manual backup
nself db backup create "pre-deploy"

# Automated backups
nself backup configure --type database --frequency daily --time 02:00
nself backup configure --type full --frequency weekly --day sunday
```

### Restore Backups
```bash
# List backups
nself db backup list

# Restore latest
nself db backup restore latest

# Restore specific backup
nself db backup restore backup_20260131_020000.sql

# Restore to staging
nself db backup restore latest --to staging
```

### Backup Management
```bash
# Set retention
nself backup retention daily 7
nself backup retention weekly 4
nself backup retention monthly 12

# Clean old backups
nself backup clean --older-than 30d

# Verify backups
nself backup verify latest
```

---

## File Permissions

### Check Permissions
```bash
# Check all sensitive files
nself security check-permissions

# Check specific file
stat -c "%a" .env.prod  # Linux
stat -f "%OLp" .env.prod  # macOS
```

### Fix Permissions
```bash
# Auto-fix all
nself security fix-permissions

# Manual fixes
chmod 600 .env*
chmod 600 .secrets
chmod 600 ssl/*.key
chmod 600 ssl/*.pem
chmod 755 nself
chmod 755 src/cli/*.sh
```

### Required Permissions

| File Type | Permission | Reason |
|-----------|-----------|---------|
| `.env*` | 600 | Contains secrets |
| `.secrets` | 600 | Ultra-sensitive |
| `*.pem` | 600 | SSL private keys |
| `*.key` | 600 | Private keys |
| Scripts | 755 | Executable |
| Configs | 644 | Readable |

---

## Quick Security Checklist

### Before Production Deployment

- [ ] Run `nself security scan`
- [ ] Run all security tests
- [ ] Change all default passwords
- [ ] Rotate all secrets
- [ ] Configure SSL/TLS
- [ ] Enable rate limiting
- [ ] Set up monitoring
- [ ] Configure alerts
- [ ] Set up backups
- [ ] Review production checklist

### Monthly Maintenance

- [ ] Run security scan
- [ ] Review access logs
- [ ] Update dependencies
- [ ] Test backups
- [ ] Review alerts
- [ ] Rotate secrets

### Quarterly Review

- [ ] Full security audit
- [ ] Penetration testing
- [ ] Update security documentation
- [ ] Review incident response plan

---

## Emergency Procedures

### Security Breach Detected

```bash
# 1. Create emergency backup
nself db backup create emergency-$(date +%s)

# 2. Block attacker IP
nself auth rate-limit blacklist add <attacker-ip>

# 3. Rotate all secrets immediately
nself auth secrets rotate jwt --no-grace-period
nself auth apikey rotate-all --no-grace-period

# 4. Force logout all users
nself auth sessions revoke-all

# 5. Review logs
nself logs nginx --since 24h | grep <attacker-ip>
nself audit history --since 24h --type security

# 6. Notify team and stakeholders
```

### Service Down

```bash
# 1. Check status
nself status
nself doctor

# 2. Try restart
nself restart

# 3. If restart fails, rollback
git checkout <previous-tag>
nself build
nself restart

# 4. If still down, restore from backup
nself db backup restore latest
```

---

## References

- [Full Security Audit](./SECURITY-AUDIT-V0.9.8.md)
- [Production Checklist](../guides/PRODUCTION-SECURITY-CHECKLIST.md)
- [Security Improvements](./SECURITY-IMPROVEMENTS-V0.9.8.md)
- [Test Results](./SECURITY-TEST-RESULTS.md)

---

**Questions?** Run `nself security --help`
**Emergency?** See [Emergency Procedures](#emergency-procedures)
