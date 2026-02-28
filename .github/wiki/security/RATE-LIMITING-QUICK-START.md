# Rate Limiting Quick Start Guide

Get started with rate limiting and DDoS protection in 5 minutes.

## 1. Initialize (First Time Only)

```bash
# Create rate limiting database tables
nself auth rate-limit init
```

## 2. Check Current Configuration

```bash
# View all rate limit zones
nself auth rate-limit list
```

Output:
```
Current Rate Limit Configuration:

ZONE                           RATE            DESCRIPTION
----                           ----            -----------
RATE_LIMIT_GENERAL_RATE        10r/s (default) General API
RATE_LIMIT_GRAPHQL_RATE        100r/m (default) GraphQL API
RATE_LIMIT_AUTH_RATE           10r/m (default) Authentication
RATE_LIMIT_UPLOAD_RATE         5r/m (default)  File Uploads
RATE_LIMIT_STATIC_RATE         1000r/m (default) Static Assets
RATE_LIMIT_WEBHOOK_RATE        30r/m (default) Webhooks
RATE_LIMIT_FUNCTIONS_RATE      50r/m (default) Functions
RATE_LIMIT_USER_RATE           1000r/m (default) Per-User (Auth)
```

## 3. Customize for Your Needs

### Production (Strict Security)

```bash
# Prevent brute force attacks
nself auth rate-limit set auth 5r/m

# Limit GraphQL queries
nself auth rate-limit set graphql_api 100r/m

# Prevent upload abuse
nself auth rate-limit set uploads 5r/m

# Apply changes
nself build
nself restart nginx
```

### High-Traffic Application

```bash
# Allow more GraphQL queries
nself auth rate-limit set graphql_api 500r/m

# Higher static asset throughput
nself auth rate-limit set static 5000r/m

# Apply changes
nself build
nself restart nginx
```

### Development (Lenient)

```bash
# No rate limiting needed
# Leave defaults or increase them
nself auth rate-limit set graphql_api 1000r/m
nself auth rate-limit set auth 100r/m
```

## 4. Whitelist Trusted IPs

```bash
# Internal servers
nself auth rate-limit whitelist add 10.0.0.1 "Internal API"

# Monitoring services
nself auth rate-limit whitelist add 203.0.113.50 "UptimeRobot"

# CI/CD
nself auth rate-limit whitelist add 198.51.100.10 "GitHub Actions"

# Apply changes
nself build
nself restart nginx
```

## 5. Monitor & Protect

### Daily Monitoring

```bash
# Check for violations
nself auth rate-limit violations

# Check for alerts
nself auth rate-limit alerts

# View status
nself auth rate-limit status
```

### Block Abusive IPs

```bash
# View top violators
nself auth rate-limit violations

# Block an IP permanently
nself auth rate-limit block add 1.2.3.4 "DDoS attack"

# Block temporarily (1 hour)
nself auth rate-limit block add 5.6.7.8 "Brute force" 3600

# Apply changes
nself build
nself restart nginx
```

### Analyze Logs

```bash
# Analyze nginx error logs
nself auth rate-limit analyze
```

## 6. Verify Protection

```bash
# Run production checks
nself doctor
```

Look for:
```
✓ Rate limiting configuration exists
✓ Rate limiting is active in nginx
✓ Low violations: 23 in last hour
✓ Custom rate limits configured
```

## Common Scenarios

### Scenario 1: User Reports "Too Many Requests" Error

**Problem**: Legitimate user hitting rate limits

**Solution**:
```bash
# Option 1: Reset their limits
nself auth rate-limit reset <user_ip>

# Option 2: Whitelist their IP
nself auth rate-limit whitelist add <user_ip> "High-volume customer"

# Option 3: Increase zone limit
nself auth rate-limit set graphql_api 500r/m
nself build && nself restart nginx
```

### Scenario 2: Under DDoS Attack

**Problem**: Sudden spike in traffic from multiple IPs

**Solution**:
```bash
# 1. Check alerts
nself auth rate-limit alerts

# 2. View top violators
nself auth rate-limit violations 1  # last 1 hour

# 3. Block attacking IPs
nself auth rate-limit block add <ip1> "DDoS"
nself auth rate-limit block add <ip2> "DDoS"

# 4. Temporarily lower limits
nself auth rate-limit set graphql_api 50r/m
nself build && nself restart nginx

# 5. Monitor
watch -n 10 'nself auth rate-limit status'
```

### Scenario 3: Setting Up New Production Server

**Checklist**:
```bash
# 1. Initialize
nself auth rate-limit init

# 2. Configure strict limits
nself auth rate-limit set auth 5r/m
nself auth rate-limit set graphql_api 100r/m
nself auth rate-limit set uploads 5r/m

# 3. Whitelist known IPs
nself auth rate-limit whitelist add <internal_ip> "Office"

# 4. Apply and verify
nself build
nself start
nself doctor

# 5. Set up monitoring cron
crontab -e
# Add: 0 * * * * /usr/local/bin/nself auth rate-limit alerts
```

## Environment-Specific Configuration

### .env.dev (Development)
```bash
# Lenient limits
RATE_LIMIT_GRAPHQL_RATE=1000r/m
RATE_LIMIT_AUTH_RATE=100r/m
```

### .env.staging (Staging)
```bash
# Production-like limits
RATE_LIMIT_GRAPHQL_RATE=200r/m
RATE_LIMIT_AUTH_RATE=15r/m
```

### .env.prod (Production)
```bash
# Strict limits
RATE_LIMIT_GRAPHQL_RATE=100r/m
RATE_LIMIT_AUTH_RATE=10r/m
RATE_LIMIT_UPLOAD_RATE=5r/m
```

## Quick Reference Commands

```bash
# Configuration
nself auth rate-limit list                      # List all zones
nself auth rate-limit set <zone> <rate>        # Set limit
nself auth rate-limit status                    # Check status

# Monitoring
nself auth rate-limit violations [hours]        # View violations
nself auth rate-limit alerts                    # Check alerts
nself auth rate-limit analyze                   # Analyze logs

# Whitelist
nself auth rate-limit whitelist add <ip> [desc] # Add IP
nself auth rate-limit whitelist list            # List whitelisted
nself auth rate-limit whitelist remove <ip>     # Remove IP

# Blacklist
nself auth rate-limit block add <ip> [reason] [duration] # Block IP
nself auth rate-limit block remove <ip>         # Unblock IP

# Management
nself auth rate-limit reset <ip>                # Reset IP limits
nself auth rate-limit init                      # Initialize DB

# Apply changes
nself build && nself restart nginx              # Apply config
```

## Next Steps

- Read full documentation: [docs/security/RATE-LIMITING.md](./RATE-LIMITING.md)
- Set up Grafana dashboards for visualization
- Configure alerting to your notification channel
- Implement automated blocking for repeat offenders

## Need Help?

- Check violations: `nself auth rate-limit violations`
- Run diagnostics: `nself doctor`
- View logs: `nself logs nginx`
- Full docs: https://docs.nself.org/security/rate-limiting
