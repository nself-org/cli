# Security Headers Guide

**Part of nself Sprint 17: Advanced Security**

## Overview

nself provides comprehensive security headers management to protect your applications from common web vulnerabilities including XSS, clickjacking, MIME-sniffing attacks, and more.

## Table of Contents

- [Quick Start](#quick-start)
- [Security Headers Explained](#security-headers-explained)
- [Content Security Policy (CSP)](#content-security-policy-csp)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [Testing & Validation](#testing--validation)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Quick Start

### View Current Configuration

```bash
nself security headers show
```

### Interactive Configuration Wizard

```bash
nself security headers configure
```

### Validate Headers on Running Server

```bash
nself security headers validate https://yourdomain.com
```

### Export to Nginx Configuration

```bash
nself security headers export nginx/includes/security-headers.conf
```

## Security Headers Explained

### Content-Security-Policy (CSP)

**Purpose**: Prevents Cross-Site Scripting (XSS) attacks by controlling which resources can be loaded.

**Default**: Moderate mode (balanced security and compatibility)

```
Content-Security-Policy: default-src 'self';
  script-src 'self' 'unsafe-inline' 'unsafe-eval';
  style-src 'self' 'unsafe-inline';
  img-src 'self' data: https:;
  font-src 'self' data:;
  connect-src 'self';
  object-src 'none';
  base-uri 'self';
  form-action 'self';
  upgrade-insecure-requests
```

**OWASP Rating**: A (Critical)

### Strict-Transport-Security (HSTS)

**Purpose**: Forces browsers to use HTTPS for all future requests.

**Default**: `max-age=31536000; includeSubDomains` (1 year)

```
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

**OWASP Rating**: A (High)

**Requirements**:
- Only works with HTTPS
- Recommended max-age: 1 year (31536000 seconds)
- Include subdomains for full protection

### X-Frame-Options

**Purpose**: Prevents clickjacking attacks by controlling if site can be framed.

**Default**: `DENY` (most secure)

**Options**:
- `DENY` - Never allow framing (recommended)
- `SAMEORIGIN` - Allow framing from same origin only

```
X-Frame-Options: DENY
```

**OWASP Rating**: A (High)

### X-Content-Type-Options

**Purpose**: Prevents MIME-sniffing attacks.

**Value**: `nosniff` (only valid value)

```
X-Content-Type-Options: nosniff
```

**OWASP Rating**: B (Medium)

### X-XSS-Protection

**Purpose**: Legacy XSS filter for older browsers (modern browsers use CSP).

**Default**: `1; mode=block`

```
X-XSS-Protection: 1; mode=block
```

**OWASP Rating**: C (Low - legacy header)

**Note**: Modern browsers rely on CSP instead. Kept for older browser compatibility.

### Referrer-Policy

**Purpose**: Controls how much referrer information is included with requests.

**Default**: `strict-origin-when-cross-origin`

**Options**:
- `no-referrer` - Never send referrer
- `no-referrer-when-downgrade` - Send only on HTTPS→HTTPS
- `same-origin` - Send only to same origin
- `strict-origin` - Send origin only (not full URL)
- `strict-origin-when-cross-origin` - Full URL same-origin, origin only cross-origin (recommended)

```
Referrer-Policy: strict-origin-when-cross-origin
```

**OWASP Rating**: B (Medium)

### Permissions-Policy

**Purpose**: Controls which browser features can be used (formerly Feature-Policy).

**Default**: Deny camera, microphone, geolocation, payment, USB, interest-cohort (FLoC)

```
Permissions-Policy: camera=(), microphone=(), geolocation=(),
  payment=(), usb=(), interest-cohort=()
```

**OWASP Rating**: B (Medium)

**Common Permissions**:
- `camera` - Camera access
- `microphone` - Microphone access
- `geolocation` - Location access
- `payment` - Payment API
- `usb` - USB device access
- `interest-cohort` - FLoC (Google's tracking, disable recommended)

### X-Permitted-Cross-Domain-Policies

**Purpose**: Prevents Adobe Flash and PDF from loading content from this domain.

**Value**: `none` (recommended)

```
X-Permitted-Cross-Domain-Policies: none
```

**OWASP Rating**: C (Low)

## Content Security Policy (CSP)

### CSP Modes

nself provides three CSP modes:

#### 1. Strict Mode (Maximum Security)

**Use Case**: Production applications, static sites
**Trade-off**: May break some features requiring inline scripts

```bash
export CSP_MODE=strict
```

**Configuration**:
- No `unsafe-inline` or `unsafe-eval`
- Scripts/styles only from `'self'`
- No external CDNs (unless whitelisted)

#### 2. Moderate Mode (Recommended)

**Use Case**: Most applications (default)
**Trade-off**: Balanced security and compatibility

```bash
export CSP_MODE=moderate
```

**Configuration**:
- Allows `unsafe-inline` and `unsafe-eval` for scripts/styles
- Images from `'self'`, `data:`, and `https:`
- Fonts from `'self'` and `data:`
- Connects to `'self'` only (unless whitelisted)

#### 3. Permissive Mode (Maximum Compatibility)

**Use Case**: Development, legacy applications
**Trade-off**: Reduced security for broader compatibility

```bash
export CSP_MODE=permissive
```

**Configuration**:
- Allows `unsafe-inline`, `unsafe-eval`, `data:`, and `https:`
- More lenient with external resources
- Still blocks `object` tags and enforces `base-uri`

### Custom CSP Configuration

For fine-grained control, use custom mode:

```bash
export CSP_MODE=custom
export CSP_DEFAULT_SRC="'self'"
export CSP_SCRIPT_SRC="'self' https://trusted-cdn.com"
export CSP_STYLE_SRC="'self' 'unsafe-inline' https://fonts.googleapis.com"
export CSP_IMG_SRC="'self' data: https:"
export CSP_FONT_SRC="'self' data: https://fonts.gstatic.com"
export CSP_CONNECT_SRC="'self' https://api.example.com"
export CSP_OBJECT_SRC="'none'"
export CSP_FRAME_SRC="'none'"
export CSP_BASE_URI="'self'"
export CSP_FORM_ACTION="'self'"
export CSP_FRAME_ANCESTORS="'none'"
export CSP_UPGRADE_INSECURE_REQUESTS="true"
```

### Adding Custom Domains to CSP

```bash
# Add CDN domain
nself security headers csp add-domain cdn.example.com

# Add API domain
nself security headers csp add-domain api.example.com

# List whitelisted domains
nself security headers csp list-domains

# Remove domain
nself security headers csp remove-domain cdn.example.com
```

### Service-Specific CSP

nself automatically generates optimized CSP for known services:

**Hasura GraphQL**:
```
default-src 'self';
script-src 'self' 'unsafe-inline' 'unsafe-eval';
connect-src 'self' ws: wss:;
```

**Grafana**:
```
default-src 'self';
script-src 'self' 'unsafe-inline' 'unsafe-eval';
connect-src 'self' https:;
img-src 'self' data: https:;
```

**MinIO Console**:
```
default-src 'self';
script-src 'self' 'unsafe-inline';
connect-src 'self' ws: wss:;
img-src 'self' data: blob:;
```

## Configuration

### Environment Variables

Add to `.env` file:

```bash
# Security Headers Mode
SECURITY_HEADERS_MODE=strict  # strict, moderate, permissive

# Content Security Policy
CSP_MODE=moderate  # strict, moderate, permissive, custom
CSP_CUSTOM_DOMAINS="cdn.example.com api.example.com"

# HSTS Configuration
HSTS_MAX_AGE=31536000  # 1 year in seconds
HSTS_INCLUDE_SUBDOMAINS=true
HSTS_PRELOAD=false  # Only enable if submitting to HSTS preload list

# X-Frame-Options
X_FRAME_OPTIONS=DENY  # DENY or SAMEORIGIN

# Referrer-Policy
REFERRER_POLICY=strict-origin-when-cross-origin

# Permissions-Policy
PERMISSIONS_POLICY_CAMERA="()"  # Deny by default
PERMISSIONS_POLICY_MICROPHONE="()"
PERMISSIONS_POLICY_GEOLOCATION="()"
PERMISSIONS_POLICY_PAYMENT="()"
PERMISSIONS_POLICY_USB="()"
```

### Apply Configuration

After updating `.env`:

```bash
# Export headers to nginx config
nself security headers export

# Rebuild nginx configuration
nself build --force

# Restart nginx
nself restart nginx
```

## CLI Commands

### Global Headers Commands

```bash
# Show current configuration
nself security headers show

# Interactive configuration wizard
nself security headers configure

# Validate headers from running server
nself security headers validate https://yourdomain.com

# Export to nginx configuration
nself security headers export [output-file]

# Generate security headers report
nself security headers report [output-file]
```

### CSP Commands

```bash
# Show CSP configuration
nself security headers csp show

# Configure CSP interactively
nself security headers csp configure

# Add domain to CSP whitelist
nself security headers csp add-domain cdn.example.com

# Remove domain from whitelist
nself security headers csp remove-domain cdn.example.com

# List whitelisted domains
nself security headers csp list-domains

# Validate CSP syntax
nself security headers csp validate

# Export CSP to nginx format
nself security headers csp export [mode] [output-file]
```

## Testing & Validation

### Manual Testing with curl

```bash
# Check headers
curl -I https://yourdomain.com

# Look for security headers
curl -I https://yourdomain.com | grep -E "Content-Security-Policy|Strict-Transport-Security|X-Frame-Options"
```

### Automated Validation

```bash
# Validate all headers
nself security headers validate https://yourdomain.com

# Security checklist (includes headers)
nself security scan
```

### Online Testing Tools

**Mozilla Observatory**: https://observatory.mozilla.org/
Comprehensive security scanner including all headers

**SecurityHeaders.com**: https://securityheaders.com/
Quick check for security header presence and configuration

**CSP Evaluator**: https://csp-evaluator.withgoogle.com/
Google's CSP validator and security analyzer

**SSL Labs**: https://www.ssllabs.com/ssltest/
Includes HSTS and security header checks

### Browser DevTools

1. Open DevTools (F12)
2. Go to Network tab
3. Click any request
4. Look at Response Headers
5. Verify security headers are present

## Best Practices

### Production Deployment

1. **Always use HTTPS in production**
   ```bash
   SSL_ENABLED=true
   SSL_PROVIDER=letsencrypt
   ```

2. **Enable strict CSP mode**
   ```bash
   CSP_MODE=strict
   ```

3. **Use DENY for X-Frame-Options**
   ```bash
   X_FRAME_OPTIONS=DENY
   ```

4. **Set HSTS max-age to 1 year**
   ```bash
   HSTS_MAX_AGE=31536000
   HSTS_INCLUDE_SUBDOMAINS=true
   ```

5. **Validate headers before deployment**
   ```bash
   nself security headers validate https://staging.yourdomain.com
   ```

### Development

1. **Use moderate or permissive CSP**
   ```bash
   CSP_MODE=moderate  # or permissive for development
   ```

2. **Allow localhost in CSP if needed**
   ```bash
   nself security headers csp add-domain localhost
   ```

3. **Test headers locally**
   ```bash
   nself security headers validate http://localhost
   ```

### Progressive Enhancement

Start permissive, gradually tighten:

```bash
# Week 1: Permissive
CSP_MODE=permissive

# Week 2: Moderate (monitor for issues)
CSP_MODE=moderate

# Week 3+: Strict (after fixing any issues)
CSP_MODE=strict
```

### Monitoring

1. **Use CSP Report-Only mode first** (TODO: add to nself)
2. **Monitor browser console for CSP violations**
3. **Set up CSP reporting endpoint** (TODO: add to nself)
4. **Run security audits regularly**
   ```bash
   nself security scan
   ```

## Troubleshooting

### CSP Blocking Resources

**Symptom**: Scripts, styles, or images not loading

**Solution**:
1. Check browser console for CSP violations
2. Add allowed domains:
   ```bash
   nself security headers csp add-domain cdn.example.com
   ```
3. Or temporarily use permissive mode:
   ```bash
   CSP_MODE=permissive nself build --force
   ```

### HSTS Not Working

**Symptom**: HSTS header not present

**Causes**:
1. SSL not enabled: `SSL_ENABLED=true` required
2. Not using HTTPS: HSTS only works on HTTPS
3. Not rebuilt: Run `nself build --force`

**Solution**:
```bash
# Ensure SSL is enabled
export SSL_ENABLED=true

# Rebuild configuration
nself build --force

# Restart nginx
nself restart nginx

# Test
nself security headers validate https://yourdomain.com
```

### Inline Scripts Blocked

**Symptom**: Inline JavaScript not executing

**Causes**: Strict CSP blocks `unsafe-inline`

**Solutions**:

1. **Best**: Move scripts to external files
2. **Good**: Use nonces (TODO: add to nself)
3. **Temporary**: Use moderate mode
   ```bash
   CSP_MODE=moderate
   ```

### Third-Party Integrations Broken

**Symptom**: Analytics, chat widgets, or other third-party tools not working

**Solution**: Whitelist the required domains
```bash
# Example: Google Analytics
nself security headers csp add-domain www.google-analytics.com
nself security headers csp add-domain ssl.google-analytics.com

# Example: Intercom
nself security headers csp add-domain widget.intercom.io
nself security headers csp add-domain js.intercomcdn.com

# Rebuild
nself build --force
```

### Headers Not Appearing

**Checklist**:
1. ✅ Configuration generated: `ls nginx/includes/security-headers.conf`
2. ✅ Included in nginx: `grep security-headers nginx/conf.d/default.conf`
3. ✅ Nginx restarted: `nself restart nginx`
4. ✅ Testing correct URL: HTTPS vs HTTP

### Frame Blocking Legitimate Use

**Symptom**: Site needs to be embedded in iframe

**Solution**: Use SAMEORIGIN instead of DENY
```bash
export X_FRAME_OPTIONS=SAMEORIGIN
nself security headers export
nself build --force
```

## Security Headers Checklist

Use this checklist for production deployments:

### Pre-Deployment

- [ ] CSP configured and tested
- [ ] HSTS enabled with 1-year max-age
- [ ] X-Frame-Options set to DENY or SAMEORIGIN
- [ ] X-Content-Type-Options set to nosniff
- [ ] Referrer-Policy configured
- [ ] Permissions-Policy denies unused features
- [ ] All headers validated with `nself security headers validate`
- [ ] Online scan passed (Mozilla Observatory, SecurityHeaders.com)

### Post-Deployment

- [ ] Headers present in production
- [ ] No console errors related to CSP
- [ ] Third-party integrations working
- [ ] Site functions correctly
- [ ] SSL Labs test passed (A rating)
- [ ] Monitor logs for CSP violations

## Related Documentation

- [SSL/TLS Configuration](../configuration/SSL.md)
- [Firewall Configuration](SECURITY-BEST-PRACTICES.md)
- [Security Checklist](SECURITY-BEST-PRACTICES.md)
- Security Audit Report (see project documentation)

## Further Reading

**OWASP Secure Headers Project**:
https://owasp.org/www-project-secure-headers/

**MDN Web Security**:
https://developer.mozilla.org/en-US/docs/Web/Security

**Content Security Policy Reference**:
https://content-security-policy.com/

**HSTS Preload List**:
https://hstspreload.org/

---

**Version**: nself v0.9.0
**Sprint**: 17 - Advanced Security
**Last Updated**: January 2026
