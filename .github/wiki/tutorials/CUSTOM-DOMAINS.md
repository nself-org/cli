# Custom Domains Setup Guide

Complete guide to configuring custom domains with SSL certificates, DNS verification, and multi-tenant domain management.

**Time Estimate**: 15-30 minutes (+ DNS propagation time)
**Difficulty**: Intermediate
**Prerequisites**: Domain registrar access, nself project running

---

## Table of Contents

1. [Overview](#overview)
2. [Single Domain Setup](#single-domain-setup)
3. [Multi-Tenant Domains](#multi-tenant-domains)
4. [SSL Certificate Management](#ssl-certificate-management)
5. [DNS Configuration](#dns-configuration)
6. [Wildcard Domains](#wildcard-domains)
7. [Domain Verification](#domain-verification)
8. [Production Deployment](#production-deployment)
9. [Troubleshooting](#troubleshooting)

---

## Overview

### What You Can Do

- **Single domain**: `app.yourcompany.com`
- **Multiple domains**: `app1.com`, `app2.com`, `app3.com`
- **Wildcard domains**: `*.yourdomain.com`
- **Multi-tenant**: Each client gets their own domain
- **Automatic SSL**: Let's Encrypt with auto-renewal
- **Custom SSL**: Upload your own certificates

### Domain Types

**Apex domain** (root):
```
example.com
```

**Subdomain**:
```
app.example.com
api.example.com
```

**Wildcard**:
```
*.example.com
# Matches: app.example.com, api.example.com, anything.example.com
```

---

## Single Domain Setup

### Scenario: Change from default to custom domain

**Default**: `https://api.local.nself.org`
**Goal**: `https://api.myapp.com`

### Step 1: Update Configuration (2 minutes)

Edit `.env`:

```bash
# Change from:
BASE_DOMAIN=local.nself.org

# To:
BASE_DOMAIN=myapp.com
```

### Step 2: Rebuild (1 minute)

```bash
nself build
```

This regenerates:
- `docker-compose.yml`
- `nginx/` configurations
- SSL certificates

### Step 3: Configure DNS (5 minutes)

**At your domain registrar** (GoDaddy, Namecheap, Cloudflare, etc.):

Add these DNS records:

```
Type    Name    Value               TTL
A       @       your-server-ip      300
A       *       your-server-ip      300
```

**Or use specific subdomains**:

```
Type    Name    Value               TTL
A       api     your-server-ip      300
A       auth    your-server-ip      300
A       admin   your-server-ip      300
```

**For local development**:

Edit `/etc/hosts` (macOS/Linux) or `C:\Windows\System32\drivers\etc\hosts` (Windows):

```
127.0.0.1 api.myapp.local
127.0.0.1 auth.myapp.local
127.0.0.1 admin.myapp.local
```

### Step 4: Provision SSL (3 minutes)

**Self-signed** (development):
```bash
nself ssl generate myapp.com --self-signed
```

**Let's Encrypt** (production):
```bash
nself ssl generate myapp.com --letsencrypt
```

**Automatic renewal**:
```bash
nself ssl enable-auto-renew myapp.com
```

### Step 5: Restart Services (1 minute)

```bash
nself restart nginx
```

### Step 6: Verify (2 minutes)

```bash
# Check DNS
nslookup api.myapp.com

# Check SSL
curl -I https://api.myapp.com

# List all URLs
nself urls
```

**Expected output**:
```
✓ DNS: api.myapp.com → your-server-ip
✓ SSL: Valid certificate
✓ Service: Running

URLs:
  https://api.myapp.com       (Hasura GraphQL)
  https://auth.myapp.com      (Authentication)
  https://admin.myapp.com     (Admin Dashboard)
```

---

## Multi-Tenant Domains

### Scenario: Each client gets their own domain

**Clients**:
- Acme Corp: `app.acme.com`
- TechCo: `app.techco.com`
- StartupXYZ: `app.startupxyz.com`

### Step 1: Initialize White-Label System (2 minutes)

```bash
nself whitelabel init
```

### Step 2: Add Domain for Tenant (3 minutes per domain)

**Tenant 1: Acme Corp**

```bash
nself whitelabel domain add app.acme.com --tenant acme-corp
```

**Output**:
```
Domain added: app.acme.com
Tenant: acme-corp
Status: pending_verification

Next steps:
1. Configure DNS (see below)
2. Run: nself whitelabel domain verify app.acme.com
```

**DNS Instructions**:
```
Add these records to acme.com DNS:

Type    Name    Value                           TTL
A       app     your-server-ip                  300
TXT     _verify.app "nself-verify-abc123def"    300
```

### Step 3: Client Configures DNS (5 minutes)

**Instruct client to add**:
```
Type    Name        Value               TTL
A       app         your-server-ip      300
CNAME   app         yourplatform.com    300  # Alternative
```

### Step 4: Verify DNS (2 minutes)

**Wait for DNS propagation** (1-48 hours, usually 5-15 minutes)

```bash
# Check DNS
nslookup app.acme.com

# Verify domain
nself whitelabel domain verify app.acme.com
```

**Output**:
```
✓ DNS propagated
✓ A record points to your-server-ip
✓ Domain verified

Status: verified
```

### Step 5: Provision SSL (3 minutes)

**Automatic (Let's Encrypt)**:
```bash
nself whitelabel domain ssl app.acme.com --auto-renew
```

**Manual (custom certificate)**:
```bash
nself whitelabel domain ssl app.acme.com \
  --cert /path/to/cert.pem \
  --key /path/to/key.pem \
  --chain /path/to/chain.pem
```

### Step 6: Activate Domain (1 minute)

```bash
nself whitelabel domain activate app.acme.com
```

### Step 7: Test (2 minutes)

```bash
# Check health
nself whitelabel domain health app.acme.com

# Test HTTPS
curl -I https://app.acme.com

# Check certificate
nself ssl check app.acme.com
```

**Repeat for all tenants**:
```bash
# TechCo
nself whitelabel domain add app.techco.com --tenant techco
nself whitelabel domain verify app.techco.com
nself whitelabel domain ssl app.techco.com --auto-renew

# StartupXYZ
nself whitelabel domain add app.startupxyz.com --tenant startupxyz
nself whitelabel domain verify app.startupxyz.com
nself whitelabel domain ssl app.startupxyz.com --auto-renew
```

---

## SSL Certificate Management

### Option 1: Let's Encrypt (Recommended for Production)

**Automatic issuance and renewal**

#### Initial Setup (5 minutes)

```bash
# Install certbot
sudo apt-get install certbot python3-certbot-nginx  # Ubuntu/Debian

# Or via snap
sudo snap install --classic certbot
```

#### Generate Certificate

```bash
nself ssl generate myapp.com --letsencrypt
```

**Behind the scenes**:
```bash
certbot certonly --standalone \
  -d myapp.com \
  -d api.myapp.com \
  -d auth.myapp.com \
  -d admin.myapp.com \
  --non-interactive \
  --agree-tos \
  --email admin@myapp.com
```

#### Auto-Renewal (1 minute)

```bash
# Enable auto-renewal
nself ssl enable-auto-renew myapp.com

# Test renewal
nself ssl renew myapp.com --dry-run

# Check renewal status
nself ssl status myapp.com
```

**Cron job created**:
```bash
# Renew certificates daily at 2am
0 2 * * * /usr/bin/certbot renew --quiet && nself restart nginx
```

### Option 2: Self-Signed (Development Only)

```bash
nself ssl generate myapp.com --self-signed
```

**Trust certificate locally**:

**macOS**:
```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ssl/myapp.com/cert.pem
```

**Linux**:
```bash
sudo cp ssl/myapp.com/cert.pem /usr/local/share/ca-certificates/myapp.crt
sudo update-ca-certificates
```

**Windows**:
```powershell
Import-Certificate -FilePath ssl\myapp.com\cert.pem -CertStoreLocation Cert:\LocalMachine\Root
```

### Option 3: Custom Certificate (Enterprise)

**If you have your own certificate**:

```bash
nself ssl upload myapp.com \
  --cert /path/to/certificate.crt \
  --key /path/to/private.key \
  --chain /path/to/ca-bundle.crt
```

**Certificate requirements**:
- PEM format
- Includes full certificate chain
- Private key is unencrypted
- Valid for at least 30 days

---

## DNS Configuration

### Common DNS Providers

#### Cloudflare

1. **Log in** to [Cloudflare Dashboard](https://dash.cloudflare.com)
2. **Select** your domain
3. **DNS** tab
4. **Add record**:
   - Type: `A`
   - Name: `app` (or `@` for apex)
   - IPv4 address: `your-server-ip`
   - Proxy status: **DNS only** (gray cloud, not orange)
   - TTL: `Auto` or `300`
5. **Save**

**Important**: Set proxy to **DNS only** (gray cloud), not proxied (orange cloud), for SSL to work correctly.

#### GoDaddy

1. **Log in** to [GoDaddy](https://sso.godaddy.com)
2. **My Products** > **DNS**
3. **Add** record:
   - Type: `A`
   - Host: `app` (or `@`)
   - Points to: `your-server-ip`
   - TTL: `1 hour`
4. **Save**

#### Namecheap

1. **Log in** to [Namecheap](https://www.namecheap.com/myaccount/login/)
2. **Domain List** > **Manage**
3. **Advanced DNS**
4. **Add New Record**:
   - Type: `A Record`
   - Host: `app`
   - Value: `your-server-ip`
   - TTL: `Automatic`
5. **Save**

#### AWS Route 53

1. **Console** > **Route 53** > **Hosted zones**
2. **Select** domain
3. **Create record**:
   - Record name: `app`
   - Record type: `A`
   - Value: `your-server-ip`
   - TTL: `300`
   - Routing policy: `Simple`
4. **Create records**

### DNS Verification

```bash
# Check A record
dig app.myapp.com A

# Check with specific DNS server
dig @8.8.8.8 app.myapp.com A

# Check propagation globally
nself dns check app.myapp.com
```

**Expected output**:
```
app.myapp.com.    300    IN    A    your-server-ip
```

### DNS Propagation Time

**Typical**:
- Cloudflare: 1-5 minutes
- GoDaddy: 10-30 minutes
- Namecheap: 30 minutes - 2 hours
- AWS Route 53: 1-5 minutes

**Check propagation**:
- https://www.whatsmydns.net
- https://dnschecker.org

---

## Wildcard Domains

### Use Case: Multi-tenant subdomains

**Goal**: `{tenant}.myplatform.com`

Examples:
- `acme.myplatform.com`
- `techco.myplatform.com`
- `startup.myplatform.com`

### Step 1: DNS Wildcard Record (2 minutes)

```
Type    Name    Value               TTL
A       *       your-server-ip      300
```

This matches:
- `anything.myplatform.com`
- `acme.myplatform.com`
- `subdomain123.myplatform.com`

### Step 2: Wildcard SSL Certificate (5 minutes)

**Let's Encrypt wildcard**:

```bash
nself ssl generate myplatform.com --wildcard --letsencrypt
```

**DNS challenge required**:
```
Add this TXT record to myplatform.com:

_acme-challenge    "random-string-from-letsencrypt"
```

**Wait for DNS, then continue**:
```bash
nself ssl verify myplatform.com
```

### Step 3: Configure Nginx (3 minutes)

Edit `.env`:
```bash
WILDCARD_DOMAIN_ENABLED=true
WILDCARD_DOMAIN=myplatform.com
TENANT_ROUTING=subdomain  # Route by subdomain
```

Rebuild:
```bash
nself build
nself restart nginx
```

### Step 4: Test (2 minutes)

```bash
# Test different subdomains
curl -I https://acme.myplatform.com
curl -I https://techco.myplatform.com
curl -I https://any-subdomain.myplatform.com

# All should work
```

---

## Domain Verification

### Why Verify?

- Proves domain ownership
- Required for SSL issuance
- Prevents unauthorized use

### Verification Methods

#### Method 1: TXT Record (Recommended)

```bash
nself whitelabel domain verify app.acme.com --method txt
```

**Add to DNS**:
```
Type    Name                    Value
TXT     _nself-verify.app      "verification-token-here"
```

**Verify**:
```bash
nself whitelabel domain verify app.acme.com --check
```

#### Method 2: HTTP Challenge

```bash
nself whitelabel domain verify app.acme.com --method http
```

**Host this file**:
```
URL: http://app.acme.com/.well-known/nself-verification.txt
Content: verification-token-here
```

**Verify**:
```bash
nself whitelabel domain verify app.acme.com --check
```

#### Method 3: DNS CNAME

```bash
nself whitelabel domain verify app.acme.com --method cname
```

**Add to DNS**:
```
Type    Name    Value
CNAME   app     yourplatform.com
```

### Verification Status

```bash
# Check status
nself whitelabel domain status app.acme.com
```

**Output**:
```
Domain: app.acme.com
Status: verified
Method: txt
Verified at: 2026-01-30 10:15:23
SSL: active
Health: healthy
```

---

## Production Deployment

### Pre-Deployment Checklist

- [ ] DNS records configured
- [ ] DNS propagated (verified with `dig` or `nslookup`)
- [ ] Domain verified
- [ ] SSL certificate issued
- [ ] SSL auto-renewal enabled
- [ ] Firewall allows ports 80 (HTTP) and 443 (HTTPS)
- [ ] Server IP is correct in DNS
- [ ] nginx configuration tested
- [ ] Health checks passing

### Deploy Custom Domain

**Step 1: Update production config**

Edit `.env.prod`:
```bash
BASE_DOMAIN=myapp.com
ENVIRONMENT=production
SSL_PROVIDER=letsencrypt
SSL_AUTO_RENEW=true
```

**Step 2: Deploy**

```bash
nself deploy prod
```

**Step 3: Generate SSL**

```bash
# SSH to production server
ssh user@your-server-ip

cd /var/www/myapp

# Generate certificate
nself ssl generate myapp.com --letsencrypt --email admin@myapp.com

# Enable auto-renewal
nself ssl enable-auto-renew myapp.com
```

**Step 4: Verify**

```bash
# Check certificate
nself ssl check myapp.com

# Test HTTPS
curl -I https://api.myapp.com

# Check all URLs
nself urls
```

### Multi-Domain Deployment

**For multiple domains on one server**:

```bash
# Domain 1
nself whitelabel domain add app1.com --tenant tenant1
nself whitelabel domain ssl app1.com --letsencrypt

# Domain 2
nself whitelabel domain add app2.com --tenant tenant2
nself whitelabel domain ssl app2.com --letsencrypt

# Domain 3
nself whitelabel domain add app3.com --tenant tenant3
nself whitelabel domain ssl app3.com --letsencrypt

# Activate all
nself whitelabel domain activate app1.com
nself whitelabel domain activate app2.com
nself whitelabel domain activate app3.com

# Restart
nself restart nginx
```

---

## Troubleshooting

### DNS Not Resolving

**Symptom**: `nslookup` returns `NXDOMAIN` or wrong IP

**Fix**:
```bash
# Check DNS records
nself dns check app.myapp.com

# Expected:
# A record: app.myapp.com → your-server-ip

# If wrong, update DNS at registrar
# Wait for propagation (5 minutes - 48 hours)

# Check globally
curl "https://dns.google/resolve?name=app.myapp.com&type=A"
```

### SSL Certificate Failed

**Symptom**: `certbot` errors

**Common causes**:

1. **Port 80 blocked**:
   ```bash
   # Check firewall
   sudo ufw status
   sudo ufw allow 80/tcp
   sudo ufw allow 443/tcp
   ```

2. **Domain not pointing to server**:
   ```bash
   # Verify DNS first
   dig app.myapp.com A

   # Must return your server IP
   ```

3. **Rate limit hit**:
   ```
   Error: too many certificates already issued for myapp.com
   ```

   **Solution**: Wait 1 week, or use staging:
   ```bash
   nself ssl generate myapp.com --letsencrypt --staging
   ```

### Certificate Expired

**Symptom**: Browser shows "Certificate expired"

**Fix**:
```bash
# Renew certificate
nself ssl renew myapp.com

# Check expiry
nself ssl check myapp.com

# Enable auto-renewal
nself ssl enable-auto-renew myapp.com

# Test renewal
nself ssl renew myapp.com --dry-run
```

### Domain Shows Wrong Site

**Symptom**: `app.myapp.com` shows default nginx page

**Fix**:
```bash
# Check nginx configuration
nself config show nginx | grep server_name

# Should include: server_name app.myapp.com;

# If missing, rebuild
nself build
nself restart nginx

# Check site configuration
cat nginx/sites/app.myapp.com.conf
```

### SSL Mixed Content Warning

**Symptom**: Page loads but shows "Not Secure" warning

**Cause**: HTTP resources on HTTPS page

**Fix**:

1. **Force HTTPS redirect**:
   ```bash
   # In .env
   FORCE_HTTPS=true

   nself build
   nself restart nginx
   ```

2. **Update frontend**:
   ```javascript
   // Change all URLs to HTTPS
   // Bad:
   <img src="http://example.com/image.jpg" />

   // Good:
   <img src="https://example.com/image.jpg" />
   ```

### Wildcard Certificate Not Covering Subdomain

**Symptom**: `app.myapp.com` works, but `api.myapp.com` fails

**Cause**: Wildcard only covers one level

**Fix**:
```bash
# Get certificate for both wildcard and apex
nself ssl generate myapp.com \
  --letsencrypt \
  --domains "myapp.com,*.myapp.com"
```

---

## Best Practices

### Security

1. **Always use HTTPS**
   - Redirect HTTP → HTTPS
   - Set `FORCE_HTTPS=true`

2. **Enable HSTS**
   ```bash
   # In .env
   HSTS_ENABLED=true
   HSTS_MAX_AGE=31536000  # 1 year
   ```

3. **Monitor certificate expiry**
   ```bash
   # Check expiry
   nself ssl check myapp.com

   # Set up alerts (30 days before expiry)
   nself ssl alert myapp.com --days 30 --email admin@myapp.com
   ```

### Performance

1. **Enable HTTP/2**
   ```bash
   # In .env
   HTTP2_ENABLED=true
   ```

2. **Use CDN** (Cloudflare, CloudFront)
   ```bash
   # Configure CDN in .env
   CDN_ENABLED=true
   CDN_URL=https://cdn.myapp.com
   ```

3. **Enable caching**
   ```bash
   # In .env
   NGINX_CACHE_ENABLED=true
   NGINX_CACHE_SIZE=100m
   ```

### Reliability

1. **Monitor domain health**
   ```bash
   # Automated checks
   nself whitelabel domain health --all --interval 5m
   ```

2. **Set up uptime monitoring**
   - Use: Pingdom, UptimeRobot, StatusCake
   - Monitor: `https://api.myapp.com/health`

3. **Keep SSL auto-renewal enabled**
   ```bash
   # Verify renewal is configured
   nself ssl status myapp.com | grep auto-renew
   ```

---

## Quick Reference

### Common Commands

```bash
# Add domain
nself whitelabel domain add app.myapp.com

# Verify domain
nself whitelabel domain verify app.myapp.com

# Generate SSL
nself ssl generate myapp.com --letsencrypt

# Check SSL status
nself ssl check myapp.com

# Renew SSL
nself ssl renew myapp.com

# Check DNS
nself dns check app.myapp.com

# View all domains
nself whitelabel domain list

# Domain health
nself whitelabel domain health app.myapp.com
```

### DNS Record Templates

**Apex domain**:
```
A       @       your-server-ip      300
A       www     your-server-ip      300
```

**Subdomain**:
```
A       app     your-server-ip      300
```

**Wildcard**:
```
A       *       your-server-ip      300
```

**CNAME** (alternative):
```
CNAME   app     yourplatform.com    300
```

**Verification**:
```
TXT     _nself-verify.app    "token"    300
```

---

## Resources

- **[Let's Encrypt](https://letsencrypt.org)** - Free SSL certificates
- **[DNS Checker](https://dnschecker.org)** - Check DNS propagation
- **[SSL Labs](https://www.ssllabs.com/ssltest/)** - Test SSL configuration
- **[nself SSL Command](../commands/SSL.md)** - Full SSL command reference

---

## Support

- **Documentation**: https://docs.nself.org
- **GitHub**: https://github.com/nself-org/cli
- **Discord**: https://discord.gg/nself

---

**Your custom domain is configured! Your app is live.**
