# White-Label Troubleshooting Guide

Comprehensive troubleshooting guide for nself white-label functionality, covering common issues, diagnostics, and solutions.

**Last Updated**: January 2026
**Version**: 1.0.0

---

## Table of Contents

1. [Branding Not Applying](#1-branding-not-applying)
2. [Logo & Asset Problems](#2-logo--asset-problems)
3. [Custom Domain Issues](#3-custom-domain-issues)
4. [Email Template Problems](#4-email-template-problems)
5. [Theme Issues](#5-theme-issues)
6. [Multi-Tenant Branding](#6-multi-tenant-branding)
7. [Performance Issues](#7-performance-issues)
8. [Security Issues](#8-security-issues)
9. [Browser Compatibility](#9-browser-compatibility)
10. [Common Error Messages](#10-common-error-messages)
11. [Diagnostic Tools](#diagnostic-tools)
12. [Quick Reference](#quick-reference)

---

## 1. Branding Not Applying

### Issue: Changes Not Visible After Update

**Symptoms:**
- Updated branding settings but old branding still shows
- Logo changes not appearing
- Color changes not reflected

**Common Causes:**
1. Browser cache
2. CDN cache
3. Redis cache not invalidated
4. Database transaction not committed
5. Service not restarted

**Diagnosis:**

```bash
# 1. Check if branding exists in database
nself db query "SELECT * FROM public.branding WHERE tenant_id = 'YOUR_TENANT_ID';"

# 2. Check Redis cache
nself redis get "branding:YOUR_TENANT_ID"

# 3. Check CDN cache headers
curl -I https://yourdomain.com/assets/logo.png

# 4. Check service logs
nself logs branding --tail 100

# 5. Verify branding service is running
docker ps | grep branding
```

**Solution:**

```bash
# Step 1: Clear all caches
nself cache clear --all

# Step 2: Force Redis cache invalidation
nself redis del "branding:*"

# Step 3: Clear browser cache
# Chrome: Ctrl+Shift+Del (Cmd+Shift+Del on Mac)
# Firefox: Ctrl+Shift+Del
# Or use incognito/private mode

# Step 4: Purge CDN cache (if using CDN)
# Cloudflare example:
curl -X POST "https://api.cloudflare.com/client/v4/zones/ZONE_ID/purge_cache" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purge_everything":true}'

# Step 5: Restart branding-related services
nself restart branding
nself restart nginx

# Step 6: Verify update
curl -H "X-Tenant-ID: YOUR_TENANT_ID" https://api.yourdomain.com/branding
```

**Verification:**

```bash
# Check branding API response
curl -s https://api.yourdomain.com/branding \
  -H "X-Tenant-ID: YOUR_TENANT_ID" | jq .

# Expected output:
# {
#   "tenant_id": "YOUR_TENANT_ID",
#   "brand_name": "Updated Name",
#   "logo_url": "https://cdn.yourdomain.com/logos/new-logo.png",
#   "updated_at": "2026-01-30T12:34:56Z"
# }
```

**Prevention:**
- Implement proper cache invalidation in your update workflow
- Use versioned asset URLs (e.g., `logo.png?v=1234567890`)
- Set appropriate cache headers (short TTL during updates)
- Add cache-busting parameters to asset URLs

---

### Issue: Cache Propagation Delays

**Symptoms:**
- Changes appear for some users but not others
- Different branding on different servers/regions
- Intermittent old branding appearances

**Diagnosis:**

```bash
# Test from multiple regions
curl -I https://yourdomain.com/assets/logo.png
# Look for: Cache-Control, Age, X-Cache headers

# Check CDN edge locations
curl -I https://yourdomain.com/assets/logo.png -H "X-Debug: 1"

# Verify asset version
curl -s https://yourdomain.com/assets/logo.png | md5sum
```

**Solution:**

```bash
# 1. Use cache-busting in asset URLs
# Update your asset URL generation to include version/hash
# Example: logo.png?v=abc123def456

# 2. Set short cache TTL during updates
# Add to nginx config:
location /assets/ {
  expires 5m;  # Short cache during updates
  add_header Cache-Control "public, must-revalidate";
}

# 3. Use CDN API to purge specific URLs
# Cloudflare example:
curl -X POST "https://api.cloudflare.com/client/v4/zones/ZONE_ID/purge_cache" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"files":["https://yourdomain.com/assets/logo.png"]}'

# 4. Implement stale-while-revalidate
# nginx config:
add_header Cache-Control "public, max-age=300, stale-while-revalidate=60";
```

**Prevention:**
- Always use versioned asset URLs
- Implement proper cache invalidation strategy
- Use short cache TTLs for branding assets
- Monitor cache hit rates and edge locations

---

## 2. Logo & Asset Problems

### Issue: Upload Failures

**Symptoms:**
- "Upload failed" error
- Request timeout
- 413 Request Entity Too Large
- 500 Internal Server Error

**Diagnosis:**

```bash
# 1. Check file size limits
nself config get UPLOAD_MAX_SIZE

# 2. Check nginx limits
grep client_max_body_size /etc/nginx/nginx.conf

# 3. Check storage space
df -h /var/lib/nself/storage

# 4. Check MinIO/S3 permissions
nself storage test-upload

# 5. Check logs
nself logs storage --tail 50 | grep -i error
nself logs nginx --tail 50 | grep -i "413\|502\|504"
```

**Solution:**

```bash
# Step 1: Increase nginx upload limits
# Edit nginx.conf
cat >> /etc/nginx/conf.d/upload-limits.conf <<EOF
client_max_body_size 50M;
client_body_timeout 300s;
EOF

# Step 2: Increase PHP/application upload limits (if applicable)
# Edit .env
echo "UPLOAD_MAX_SIZE=50M" >> .env
echo "UPLOAD_TIMEOUT=300" >> .env

# Step 3: Verify MinIO/S3 bucket permissions
nself storage verify-permissions

# Step 4: Check disk space and clean if needed
docker system prune -a --volumes

# Step 5: Restart services
nself restart nginx
nself restart storage

# Step 6: Test upload
curl -X POST https://api.yourdomain.com/branding/logo \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "logo=@/path/to/logo.png"
```

**Verification:**

```bash
# Upload a test file
dd if=/dev/zero of=/tmp/test-10mb.bin bs=1M count=10
curl -X POST https://api.yourdomain.com/branding/logo \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "logo=@/tmp/test-10mb.bin"

# Should return 200 OK or specific error
```

**Prevention:**
- Set reasonable file size limits (10-50MB for logos)
- Implement client-side file size validation
- Add file type validation
- Show clear error messages with size limits
- Implement chunked upload for large files

---

### Issue: Image Format Issues

**Symptoms:**
- "Invalid file format" error
- Uploaded image appears corrupted
- Image not displaying correctly
- MIME type errors

**Diagnosis:**

```bash
# 1. Check file type
file /path/to/uploaded/logo.png

# 2. Verify MIME type
curl -I https://cdn.yourdomain.com/logos/logo.png | grep Content-Type

# 3. Check image validity
identify /path/to/uploaded/logo.png

# 4. Check for EXIF corruption
exiftool /path/to/uploaded/logo.png

# 5. Check logs for format errors
nself logs branding | grep -i "format\|mime\|invalid"
```

**Solution:**

```bash
# Step 1: Validate and convert images on upload
# Add to your upload handler:

#!/bin/bash
validate_and_convert_image() {
  local input="$1"
  local output="$2"

  # Check if it's a valid image
  if ! identify "$input" &>/dev/null; then
    echo "Invalid image file"
    return 1
  fi

  # Convert to standard format (remove EXIF, optimize)
  convert "$input" \
    -strip \
    -quality 90 \
    -resize 2000x2000\> \
    "$output"

  # Verify output
  if ! identify "$output" &>/dev/null; then
    echo "Conversion failed"
    return 1
  fi

  return 0
}

# Step 2: Set proper MIME types in nginx
# Add to nginx config:
types {
  image/png png;
  image/jpeg jpg jpeg;
  image/svg+xml svg;
  image/webp webp;
}

# Step 3: Implement server-side validation
# Example validation rules:
# - Allowed formats: PNG, JPEG, SVG, WebP
# - Max dimensions: 2000x2000px
# - Max file size: 10MB
# - Strip metadata/EXIF

# Step 4: Generate multiple formats
convert logo.png -quality 90 logo.jpg
convert logo.png -define webp:lossless=true logo.webp

# Step 5: Restart nginx
nself restart nginx
```

**Prevention:**
- Accept common formats: PNG, JPEG, SVG, WebP
- Validate file headers, not just extensions
- Strip EXIF/metadata on upload
- Generate optimized versions automatically
- Provide clear format requirements to users

---

### Issue: Broken Asset URLs

**Symptoms:**
- 404 errors for logo/assets
- Mixed content warnings (HTTP/HTTPS)
- Incorrect CDN URLs
- Assets not loading on custom domains

**Diagnosis:**

```bash
# 1. Check asset URL in database
nself db query "SELECT logo_url, favicon_url FROM public.branding WHERE tenant_id = 'YOUR_TENANT_ID';"

# 2. Test asset accessibility
curl -I https://cdn.yourdomain.com/logos/logo.png

# 3. Check for mixed content issues
curl -v https://yourdomain.com 2>&1 | grep -i "http:"

# 4. Verify CDN configuration
nslookup cdn.yourdomain.com

# 5. Check nginx routing
nself logs nginx | grep "logo.png"
```

**Solution:**

```bash
# Step 1: Fix asset URLs in database
nself db query "
UPDATE public.branding
SET logo_url = REPLACE(logo_url, 'http://', 'https://'),
    favicon_url = REPLACE(favicon_url, 'http://', 'https://')
WHERE logo_url LIKE 'http://%' OR favicon_url LIKE 'http://%';
"

# Step 2: Configure proper asset base URL
# Add to .env:
ASSET_BASE_URL=https://cdn.yourdomain.com
CDN_ENABLED=true

# Step 3: Update nginx to handle assets
cat > /etc/nginx/sites-enabled/assets.conf <<'EOF'
server {
  listen 443 ssl http2;
  server_name cdn.yourdomain.com;

  ssl_certificate /etc/ssl/certs/yourdomain.pem;
  ssl_certificate_key /etc/ssl/private/yourdomain.key;

  location /logos/ {
    alias /var/lib/nself/storage/branding/logos/;
    expires 30d;
    add_header Cache-Control "public, immutable";
    add_header Access-Control-Allow-Origin "*";
  }

  location /assets/ {
    alias /var/lib/nself/storage/branding/assets/;
    expires 30d;
    add_header Cache-Control "public, immutable";
    add_header Access-Control-Allow-Origin "*";
  }
}
EOF

# Step 4: Test asset URLs
curl -I https://cdn.yourdomain.com/logos/logo.png

# Step 5: Update asset URLs in application
# Use helper function for consistent URL generation:
get_asset_url() {
  local path="$1"
  local base_url="${ASSET_BASE_URL:-https://yourdomain.com}"
  echo "${base_url}${path}"
}

# Step 6: Restart nginx
nself restart nginx
```

**Prevention:**
- Always use HTTPS for asset URLs
- Use environment variables for base URLs
- Implement URL helpers/functions
- Validate URLs on save
- Test assets in different environments

---

## 3. Custom Domain Issues

### Issue: Domain Verification Failures

**Symptoms:**
- "Domain verification failed" error
- DNS records not found
- Verification timeout
- TXT record not propagating

**Diagnosis:**

```bash
# 1. Check DNS TXT record
dig TXT _nself-verify.yourdomain.com +short

# 2. Check from multiple DNS servers
dig @8.8.8.8 TXT _nself-verify.yourdomain.com +short
dig @1.1.1.1 TXT _nself-verify.yourdomain.com +short

# 3. Check DNS propagation globally
# Use online tool: https://www.whatsmydns.net/

# 4. Verify expected value
nself db query "SELECT verification_token FROM public.custom_domains WHERE domain = 'yourdomain.com';"

# 5. Check verification logs
nself logs domain-verification --tail 50
```

**Solution:**

```bash
# Step 1: Get verification token
TOKEN=$(nself domain get-verification-token yourdomain.com)
echo "Add TXT record: _nself-verify.yourdomain.com = $TOKEN"

# Step 2: Add DNS TXT record
# Example for Cloudflare API:
curl -X POST "https://api.cloudflare.com/client/v4/zones/ZONE_ID/dns_records" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "TXT",
    "name": "_nself-verify.yourdomain.com",
    "content": "'"$TOKEN"'",
    "ttl": 120
  }'

# Step 3: Wait for DNS propagation (usually 5-60 minutes)
# Check propagation:
while ! dig TXT _nself-verify.yourdomain.com +short | grep -q "$TOKEN"; do
  echo "Waiting for DNS propagation..."
  sleep 30
done

# Step 4: Trigger verification
nself domain verify yourdomain.com

# Step 5: Verify status
nself domain status yourdomain.com
```

**Verification:**

```bash
# Check verification status
nself db query "SELECT domain, verified, verified_at FROM public.custom_domains WHERE domain = 'yourdomain.com';"

# Should show:
#   domain        | verified | verified_at
# ----------------+----------+-------------------------
#  yourdomain.com | true     | 2026-01-30 12:34:56+00
```

**Prevention:**
- Set low TTL (120s) for verification records
- Provide clear DNS setup instructions
- Implement retry logic with exponential backoff
- Check multiple DNS servers for verification
- Allow manual re-verification

---

### Issue: SSL Certificate Problems

**Symptoms:**
- SSL certificate errors in browser
- "Certificate not valid" warnings
- HTTPS not working on custom domain
- Certificate expired

**Diagnosis:**

```bash
# 1. Check certificate validity
openssl s_client -connect yourdomain.com:443 -servername yourdomain.com </dev/null 2>/dev/null | \
  openssl x509 -noout -dates

# 2. Check certificate details
openssl s_client -connect yourdomain.com:443 -servername yourdomain.com </dev/null 2>/dev/null | \
  openssl x509 -noout -text

# 3. Check Let's Encrypt logs (if using certbot)
cat /var/log/letsencrypt/letsencrypt.log | tail -50

# 4. Check certificate in database
nself db query "SELECT domain, ssl_enabled, ssl_certificate_expires_at FROM public.custom_domains WHERE domain = 'yourdomain.com';"

# 5. Test SSL/TLS configuration
nmap --script ssl-enum-ciphers -p 443 yourdomain.com
```

**Solution:**

```bash
# Step 1: Generate new Let's Encrypt certificate
certbot certonly \
  --dns-cloudflare \
  --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  -d yourdomain.com \
  -d www.yourdomain.com

# Alternative: HTTP-01 challenge
certbot certonly \
  --webroot \
  -w /var/www/html \
  -d yourdomain.com \
  -d www.yourdomain.com

# Step 2: Update nginx configuration
cat > /etc/nginx/sites-enabled/yourdomain.com.conf <<'EOF'
server {
  listen 443 ssl http2;
  server_name yourdomain.com www.yourdomain.com;

  ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;
  ssl_trusted_certificate /etc/letsencrypt/live/yourdomain.com/chain.pem;

  ssl_protocols TLSv1.2 TLSv1.3;
  ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256';
  ssl_prefer_server_ciphers off;

  location / {
    proxy_pass http://upstream_backend;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
  }
}

server {
  listen 80;
  server_name yourdomain.com www.yourdomain.com;
  return 301 https://$server_name$request_uri;
}
EOF

# Step 3: Test nginx configuration
nginx -t

# Step 4: Reload nginx
nginx -s reload

# Step 5: Set up auto-renewal
# Add to crontab:
# 0 0,12 * * * certbot renew --quiet --deploy-hook "nginx -s reload"

# Step 6: Update database
nself db query "
UPDATE public.custom_domains
SET ssl_enabled = true,
    ssl_certificate_expires_at = NOW() + INTERVAL '90 days'
WHERE domain = 'yourdomain.com';
"
```

**Verification:**

```bash
# Test SSL certificate
curl -vI https://yourdomain.com 2>&1 | grep -A 5 "SSL certificate"

# Check certificate expiry
echo | openssl s_client -servername yourdomain.com -connect yourdomain.com:443 2>/dev/null | \
  openssl x509 -noout -dates

# Test SSL Labs rating
# Visit: https://www.ssllabs.com/ssltest/analyze.html?d=yourdomain.com
```

**Prevention:**
- Set up automatic certificate renewal
- Monitor certificate expiration (alert 30 days before)
- Use DNS-01 challenge for wildcard certificates
- Keep certbot updated
- Test renewal process regularly

---

### Issue: Domain Routing Not Working

**Symptoms:**
- Custom domain shows 404 or default page
- Traffic not routing to correct tenant
- Wrong branding displayed on custom domain

**Diagnosis:**

```bash
# 1. Check DNS resolution
nslookup yourdomain.com

# 2. Check nginx configuration
nginx -T | grep -A 20 "yourdomain.com"

# 3. Check domain mapping in database
nself db query "SELECT domain, tenant_id, enabled FROM public.custom_domains WHERE domain = 'yourdomain.com';"

# 4. Test routing
curl -v https://yourdomain.com

# 5. Check nginx logs
nself logs nginx | grep "yourdomain.com"
```

**Solution:**

```bash
# Step 1: Verify DNS points to your server
# DNS should have:
# A record: yourdomain.com -> YOUR_SERVER_IP
# CNAME record: www.yourdomain.com -> yourdomain.com

# Step 2: Update nginx configuration
cat > /etc/nginx/sites-enabled/custom-domain-routing.conf <<'EOF'
# Map custom domains to tenant IDs
map $host $tenant_id {
  default "default";
  yourdomain.com "tenant-abc123";
  www.yourdomain.com "tenant-abc123";
  anotherdomain.com "tenant-xyz789";
}

server {
  listen 443 ssl http2;
  server_name yourdomain.com;

  ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

  location / {
    proxy_pass http://app_backend;
    proxy_set_header Host $host;
    proxy_set_header X-Tenant-ID $tenant_id;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
  }
}
EOF

# Step 3: Reload nginx configuration
# Reload domain mappings from database
nself domain refresh-nginx-config

# Step 4: Test nginx config
nginx -t

# Step 5: Reload nginx
nginx -s reload

# Step 6: Verify routing
curl -v https://yourdomain.com 2>&1 | grep "X-Tenant-ID"
```

**Verification:**

```bash
# Test domain routing
curl -s https://yourdomain.com/api/branding | jq '.tenant_id'

# Should return: "tenant-abc123"

# Check logs for correct tenant
nself logs app | grep "tenant-abc123"
```

**Prevention:**
- Automate nginx config generation from database
- Validate domain before enabling
- Implement health checks for custom domains
- Monitor domain routing in logs
- Use dynamic upstream configuration

---

## 4. Email Template Problems

### Issue: Template Rendering Errors

**Symptoms:**
- Emails showing raw HTML/code
- Variables not replaced (showing {{variable}})
- Broken layout in email clients
- Missing images or styles

**Diagnosis:**

```bash
# 1. Check template syntax
nself email validate-template --template welcome

# 2. Test template rendering
nself email render-test \
  --template welcome \
  --data '{"user_name":"John","verification_link":"https://..."}'

# 3. Check logs
nself logs email | grep -i "template\|render\|error"

# 4. Verify template exists
nself db query "SELECT id, name, subject, body FROM public.email_templates WHERE name = 'welcome';"

# 5. Test variable substitution
echo "Hello {{user_name}}" | nself email render-string --data '{"user_name":"John"}'
```

**Solution:**

```bash
# Step 1: Validate template syntax
# Check for common issues:
# - Unclosed tags: {{variable
# - Invalid variable names: {{user-name}} (use {{user_name}})
# - Missing quotes in HTML attributes

# Step 2: Fix variable syntax
# WRONG:
# <p>Hello {{user-name}}</p>

# RIGHT:
# <p>Hello {{user_name}}</p>

# Step 3: Test with sample data
cat > /tmp/test-template.html <<'EOF'
<!DOCTYPE html>
<html>
<body>
  <h1>Welcome {{user_name}}</h1>
  <p>Click here to verify: <a href="{{verification_link}}">Verify Email</a></p>
  <img src="{{logo_url}}" alt="Logo">
</body>
</html>
EOF

nself email render-test \
  --file /tmp/test-template.html \
  --data '{
    "user_name": "John Doe",
    "verification_link": "https://yourdomain.com/verify?token=abc123",
    "logo_url": "https://cdn.yourdomain.com/logos/logo.png"
  }'

# Step 4: Update template in database
nself db query "
UPDATE public.email_templates
SET body = '$(cat /tmp/test-template.html)'
WHERE name = 'welcome';
"

# Step 5: Test actual email sending
nself email send-test \
  --to your@email.com \
  --template welcome \
  --data '{"user_name":"John Doe",...}'
```

**Verification:**

```bash
# Send test email and check inbox
nself email send-test --to your@email.com --template welcome

# Check email queue
nself db query "SELECT * FROM public.email_queue WHERE status = 'failed';"

# Check email logs
nself logs email --tail 20
```

**Prevention:**
- Use template validation before saving
- Test templates with real data
- Implement template versioning
- Use a template preview system
- Validate all variables exist before rendering

---

### Issue: HTML/CSS Issues in Emails

**Symptoms:**
- Layout broken in Gmail/Outlook
- CSS not applying
- Images not displaying
- Mobile responsiveness issues

**Diagnosis:**

```bash
# 1. Test in multiple email clients
# Use service like: https://www.emailonacid.com/
# Or: https://litmus.com/

# 2. Validate HTML
tidy -errors -q /path/to/template.html

# 3. Check inline CSS
# CSS should be inlined for email compatibility
nself email inline-css --template welcome

# 4. Test image URLs
curl -I https://cdn.yourdomain.com/email/logo.png
```

**Solution:**

```bash
# Step 1: Use email-safe HTML structure
cat > /tmp/email-template-base.html <<'EOF'
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>{{subject}}</title>
</head>
<body style="margin: 0; padding: 0; font-family: Arial, sans-serif;">
  <table border="0" cellpadding="0" cellspacing="0" width="100%">
    <tr>
      <td style="padding: 20px 0;">
        <table align="center" border="0" cellpadding="0" cellspacing="0" width="600" style="border-collapse: collapse;">
          <!-- Header -->
          <tr>
            <td align="center" bgcolor="{{brand_primary_color}}" style="padding: 40px 0;">
              <img src="{{logo_url}}" alt="{{brand_name}}" width="200" style="display: block;" />
            </td>
          </tr>
          <!-- Content -->
          <tr>
            <td bgcolor="#ffffff" style="padding: 40px 30px;">
              {{content}}
            </td>
          </tr>
          <!-- Footer -->
          <tr>
            <td bgcolor="#f4f4f4" style="padding: 30px 30px; text-align: center; font-size: 12px; color: #666666;">
              &copy; {{current_year}} {{brand_name}}. All rights reserved.
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>
EOF

# Step 2: Inline all CSS
npm install -g juice
juice /tmp/email-template-base.html /tmp/email-template-inlined.html

# Step 3: Test responsive design
# Add media queries in <style> tag (not inline):
cat >> /tmp/email-template-inlined.html <<'EOF'
<style type="text/css">
  @media only screen and (max-width: 600px) {
    table[class="container"] {
      width: 100% !important;
    }
    img {
      max-width: 100% !important;
      height: auto !important;
    }
  }
</style>
EOF

# Step 4: Use absolute URLs for images
# WRONG: src="/images/logo.png"
# RIGHT: src="https://cdn.yourdomain.com/images/logo.png"

# Step 5: Test in real clients
# Send test emails
nself email send-test --to gmail@example.com --template welcome
nself email send-test --to outlook@example.com --template welcome
nself email send-test --to yahoo@example.com --template welcome
```

**Email CSS Best Practices:**

```css
/* Use inline styles for maximum compatibility */
style="font-family: Arial, sans-serif; font-size: 14px; color: #333333;"

/* Avoid these in email CSS: */
/* - position: absolute/fixed */
/* - float */
/* - CSS animations */
/* - flexbox/grid */
/* - background images (use table backgrounds instead) */
/* - web fonts (use safe fallbacks) */

/* Email-safe font stack: */
style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;"
```

**Prevention:**
- Use table-based layouts for emails
- Inline all CSS styles
- Test in major email clients
- Use email testing services
- Provide plain text alternative
- Use absolute URLs for all resources

---

## 5. Theme Issues

### Issue: CSS Not Applying

**Symptoms:**
- Theme colors not showing
- Custom CSS ignored
- Styles inconsistent across pages
- Theme switching not working

**Diagnosis:**

```bash
# 1. Check theme configuration
nself db query "SELECT * FROM public.themes WHERE tenant_id = 'YOUR_TENANT_ID';"

# 2. Verify CSS file exists
ls -lh /var/lib/nself/storage/themes/YOUR_TENANT_ID/custom.css

# 3. Check CSS is being loaded
curl -s https://yourdomain.com | grep -i "stylesheet\|<style"

# 4. Check browser console for errors
# Open DevTools (F12) -> Console tab

# 5. Verify CSS variables
curl -s https://yourdomain.com/themes/YOUR_TENANT_ID/variables.css
```

**Solution:**

```bash
# Step 1: Generate theme CSS from database
nself theme generate --tenant YOUR_TENANT_ID

# Step 2: Verify CSS output
cat /var/lib/nself/storage/themes/YOUR_TENANT_ID/theme.css

# Expected content:
# :root {
#   --brand-primary: #007bff;
#   --brand-secondary: #6c757d;
#   --brand-accent: #28a745;
# }

# Step 3: Ensure CSS is included in HTML
cat > /tmp/theme-include.html <<'EOF'
<head>
  <!-- Base theme CSS -->
  <link rel="stylesheet" href="/themes/{{tenant_id}}/theme.css">

  <!-- Custom CSS (if any) -->
  {{#if has_custom_css}}
  <link rel="stylesheet" href="/themes/{{tenant_id}}/custom.css">
  {{/if}}

  <!-- Inline critical CSS -->
  <style>
    :root {
      --brand-primary: {{brand_primary_color}};
      --brand-secondary: {{brand_secondary_color}};
    }
  </style>
</head>
EOF

# Step 4: Clear CSS cache
rm -rf /var/cache/nginx/themes/*
nself cache clear --pattern "theme:*"

# Step 5: Restart services
nself restart nginx
nself restart app

# Step 6: Test theme loading
curl -I https://yourdomain.com/themes/YOUR_TENANT_ID/theme.css
```

**Verification:**

```bash
# Check CSS variables are defined
curl -s https://yourdomain.com | grep -o "var(--brand-[^)]*)"

# Test theme colors are applied
# Open browser DevTools -> Elements tab
# Inspect element -> Computed styles
# Should show custom colors
```

**Prevention:**
- Version theme CSS files (theme.css?v=123456)
- Validate CSS before saving
- Use CSS variables for consistency
- Implement theme preview
- Test theme changes in staging

---

### Issue: Dark/Light Mode Not Working

**Symptoms:**
- Theme toggle doesn't switch modes
- Wrong theme applied on page load
- Theme preference not saved
- Flash of wrong theme on load

**Diagnosis:**

```bash
# 1. Check theme preference storage
# Browser localStorage
# Open DevTools -> Application -> Local Storage
# Look for: theme_mode or darkMode

# 2. Check database preference
nself db query "SELECT user_id, theme_preference FROM public.user_preferences WHERE user_id = 'USER_ID';"

# 3. Check CSS media query
curl -s https://yourdomain.com/theme.css | grep "@media.*prefers-color-scheme"

# 4. Test JavaScript theme switcher
curl -s https://yourdomain.com/js/theme-switcher.js

# 5. Check for conflicting styles
# DevTools -> Elements -> Styles panel
```

**Solution:**

```bash
# Step 1: Implement proper theme detection and application
cat > /var/www/html/js/theme-manager.js <<'EOF'
// Theme Manager - Load before page renders to prevent flash
(function() {
  // Get saved preference or system preference
  const getThemePreference = () => {
    const saved = localStorage.getItem('theme_mode');
    if (saved) return saved;

    // Check system preference
    if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
      return 'dark';
    }
    return 'light';
  };

  // Apply theme immediately
  const theme = getThemePreference();
  document.documentElement.setAttribute('data-theme', theme);

  // Theme toggle function
  window.toggleTheme = function() {
    const current = document.documentElement.getAttribute('data-theme');
    const newTheme = current === 'dark' ? 'light' : 'dark';

    document.documentElement.setAttribute('data-theme', newTheme);
    localStorage.setItem('theme_mode', newTheme);

    // Sync to server (optional)
    fetch('/api/user/preferences', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ theme_preference: newTheme })
    });
  };

  // Listen for system theme changes
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
    if (!localStorage.getItem('theme_mode')) {
      document.documentElement.setAttribute('data-theme', e.matches ? 'dark' : 'light');
    }
  });
})();
EOF

# Step 2: Add theme CSS with proper specificity
cat > /var/www/html/css/theme.css <<'EOF'
/* Default (light) theme */
:root,
[data-theme="light"] {
  --bg-primary: #ffffff;
  --bg-secondary: #f8f9fa;
  --text-primary: #212529;
  --text-secondary: #6c757d;
  --border-color: #dee2e6;
}

/* Dark theme */
[data-theme="dark"] {
  --bg-primary: #1a1a1a;
  --bg-secondary: #2d2d2d;
  --text-primary: #f8f9fa;
  --text-secondary: #adb5bd;
  --border-color: #495057;
}

/* Apply variables */
body {
  background-color: var(--bg-primary);
  color: var(--text-primary);
  transition: background-color 0.3s, color 0.3s;
}

/* Support system preference if no explicit choice */
@media (prefers-color-scheme: dark) {
  :root:not([data-theme]) {
    --bg-primary: #1a1a1a;
    --bg-secondary: #2d2d2d;
    --text-primary: #f8f9fa;
    --text-secondary: #adb5bd;
  }
}
EOF

# Step 3: Load theme script in <head> (before body)
cat > /tmp/theme-html-structure.html <<'EOF'
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="color-scheme" content="light dark">

  <!-- Load theme immediately to prevent flash -->
  <script src="/js/theme-manager.js"></script>

  <!-- Theme CSS -->
  <link rel="stylesheet" href="/css/theme.css">
</head>
<body>
  <!-- Theme toggle button -->
  <button onclick="toggleTheme()" class="theme-toggle">
    <span class="theme-icon">🌙</span>
  </button>
</body>
</html>
EOF

# Step 4: Test theme persistence
curl -c cookies.txt -b cookies.txt https://yourdomain.com

# Step 5: Restart services
nself restart nginx
```

**Verification:**

```bash
# Test theme switching
# 1. Open browser DevTools -> Console
# 2. Run: toggleTheme()
# 3. Check: document.documentElement.getAttribute('data-theme')
# 4. Refresh page - theme should persist

# Check localStorage
# DevTools -> Application -> Local Storage -> theme_mode
```

**Prevention:**
- Load theme script before page renders
- Use CSS variables for all colors
- Respect system preferences
- Persist preference in localStorage and server
- Test on page refresh and navigation
- Implement smooth transitions

---

## 6. Multi-Tenant Branding

### Issue: Tenant Isolation Failures

**Symptoms:**
- User sees wrong tenant's branding
- Data leakage between tenants
- Assets accessible across tenants
- Session mixing

**Diagnosis:**

```bash
# 1. Check tenant identification
curl -v https://yourdomain.com/api/branding 2>&1 | grep "X-Tenant-ID"

# 2. Verify database queries use tenant filter
nself db query "EXPLAIN SELECT * FROM public.branding WHERE tenant_id = 'TENANT_ID';"

# 3. Check for missing tenant filters
nself db query "
SELECT query
FROM pg_stat_statements
WHERE query LIKE '%FROM public.branding%'
  AND query NOT LIKE '%tenant_id%';
"

# 4. Check session isolation
nself redis keys "session:*" | head -20

# 5. Audit asset access
nself logs nginx | grep "GET.*assets" | grep -v "X-Tenant-ID"
```

**Solution:**

```bash
# Step 1: Enforce tenant context in all queries
# Add database view that automatically filters by tenant

nself db query "
-- Create secure view that enforces tenant filtering
CREATE OR REPLACE VIEW tenant_branding AS
SELECT * FROM public.branding
WHERE tenant_id = current_setting('app.current_tenant_id', true);

-- Set row-level security
ALTER TABLE public.branding ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON public.branding
  USING (tenant_id = current_setting('app.current_tenant_id', true));
"

# Step 2: Set tenant context in application middleware
cat > /tmp/tenant-middleware.js <<'EOF'
// Express.js middleware example
const tenantMiddleware = async (req, res, next) => {
  // Get tenant from various sources
  const tenantId =
    req.headers['x-tenant-id'] ||
    req.subdomains[0] ||
    await getTenantFromDomain(req.hostname) ||
    req.session?.tenantId;

  if (!tenantId) {
    return res.status(400).json({ error: 'Tenant ID required' });
  }

  // Verify tenant exists and is active
  const tenant = await db.query(
    'SELECT id, status FROM public.tenants WHERE id = $1',
    [tenantId]
  );

  if (!tenant || tenant.status !== 'active') {
    return res.status(403).json({ error: 'Invalid tenant' });
  }

  // Set tenant context for database queries
  req.tenantId = tenantId;
  await db.query("SET app.current_tenant_id = $1", [tenantId]);

  // Set header for downstream services
  res.setHeader('X-Tenant-ID', tenantId);

  next();
};

module.exports = tenantMiddleware;
EOF

# Step 3: Enforce tenant in asset URLs
cat > /tmp/asset-middleware.js <<'EOF'
// Asset access control
const assetMiddleware = async (req, res, next) => {
  const assetPath = req.path; // e.g., /logos/logo.png
  const tenantId = req.headers['x-tenant-id'];

  if (!tenantId) {
    return res.status(403).json({ error: 'Tenant ID required' });
  }

  // Verify asset belongs to tenant
  const asset = await db.query(
    'SELECT tenant_id FROM public.assets WHERE path = $1',
    [assetPath]
  );

  if (!asset || asset.tenant_id !== tenantId) {
    return res.status(404).json({ error: 'Asset not found' });
  }

  next();
};
EOF

# Step 4: Use tenant-scoped Redis keys
cat > /tmp/redis-helper.js <<'EOF'
// Redis key helper
const getTenantKey = (tenantId, key) => {
  return `tenant:${tenantId}:${key}`;
};

// Usage:
const brandingKey = getTenantKey(tenantId, 'branding');
await redis.get(brandingKey);
EOF

# Step 5: Audit tenant isolation
cat > /tmp/audit-tenant-isolation.sh <<'EOF'
#!/bin/bash
# Check for queries without tenant filtering

echo "Checking for unsafe queries..."

# Check application code
grep -r "SELECT.*FROM.*branding" src/ | grep -v "WHERE.*tenant_id"

# Check database queries
nself db query "
SELECT query, calls
FROM pg_stat_statements
WHERE query LIKE '%FROM%branding%'
  AND query NOT LIKE '%tenant_id%'
ORDER BY calls DESC;
"
EOF

chmod +x /tmp/audit-tenant-isolation.sh
bash /tmp/audit-tenant-isolation.sh
```

**Verification:**

```bash
# Test tenant isolation
# 1. Request branding as Tenant A
curl -H "X-Tenant-ID: tenant-a" https://yourdomain.com/api/branding

# 2. Request branding as Tenant B
curl -H "X-Tenant-ID: tenant-b" https://yourdomain.com/api/branding

# 3. Try to access Tenant A's assets as Tenant B
curl -H "X-Tenant-ID: tenant-b" https://yourdomain.com/assets/tenant-a/logo.png
# Should return 404

# 4. Check database policies
nself db query "SELECT * FROM pg_policies WHERE tablename = 'branding';"
```

**Prevention:**
- Enable row-level security on all tenant tables
- Use database policies for automatic filtering
- Audit all queries for tenant filtering
- Use middleware to set tenant context
- Implement tenant-scoped caching
- Regular security audits

---

### Issue: Brand Inheritance Not Working

**Symptoms:**
- Sub-tenant not inheriting parent branding
- Override settings not applying
- Fallback branding not working

**Diagnosis:**

```bash
# 1. Check tenant hierarchy
nself db query "
SELECT t1.id, t1.name, t2.name as parent_name
FROM public.tenants t1
LEFT JOIN public.tenants t2 ON t1.parent_tenant_id = t2.id
WHERE t1.id = 'SUB_TENANT_ID';
"

# 2. Check branding inheritance
nself db query "
WITH RECURSIVE tenant_hierarchy AS (
  SELECT id, parent_tenant_id, 1 as level
  FROM public.tenants
  WHERE id = 'SUB_TENANT_ID'

  UNION ALL

  SELECT t.id, t.parent_tenant_id, th.level + 1
  FROM public.tenants t
  JOIN tenant_hierarchy th ON t.id = th.parent_tenant_id
)
SELECT th.level, b.*
FROM tenant_hierarchy th
JOIN public.branding b ON b.tenant_id = th.id
ORDER BY th.level;
"

# 3. Check override settings
nself db query "SELECT * FROM public.branding_overrides WHERE tenant_id = 'SUB_TENANT_ID';"

# 4. Test branding API
curl -H "X-Tenant-ID: SUB_TENANT_ID" https://api.yourdomain.com/branding
```

**Solution:**

```bash
# Step 1: Implement branding inheritance function
nself db query "
CREATE OR REPLACE FUNCTION get_effective_branding(p_tenant_id UUID)
RETURNS TABLE (
  tenant_id UUID,
  brand_name TEXT,
  logo_url TEXT,
  primary_color TEXT,
  -- ... other fields
) AS \$\$
BEGIN
  RETURN QUERY
  WITH RECURSIVE tenant_hierarchy AS (
    -- Start with requested tenant
    SELECT t.id, t.parent_tenant_id, 1 as level
    FROM public.tenants t
    WHERE t.id = p_tenant_id

    UNION ALL

    -- Recursively get parent tenants
    SELECT t.id, t.parent_tenant_id, th.level + 1
    FROM public.tenants t
    JOIN tenant_hierarchy th ON t.id = th.parent_tenant_id
    WHERE th.level < 10  -- Prevent infinite loops
  ),
  branding_chain AS (
    SELECT
      th.level,
      b.*
    FROM tenant_hierarchy th
    LEFT JOIN public.branding b ON b.tenant_id = th.id
    ORDER BY th.level ASC  -- Child first, then parents
  )
  SELECT DISTINCT ON (1)
    p_tenant_id as tenant_id,
    COALESCE(
      (SELECT brand_name FROM branding_chain WHERE brand_name IS NOT NULL ORDER BY level LIMIT 1),
      'Default Brand'
    ) as brand_name,
    COALESCE(
      (SELECT logo_url FROM branding_chain WHERE logo_url IS NOT NULL ORDER BY level LIMIT 1),
      '/assets/default-logo.png'
    ) as logo_url,
    COALESCE(
      (SELECT primary_color FROM branding_chain WHERE primary_color IS NOT NULL ORDER BY level LIMIT 1),
      '#007bff'
    ) as primary_color;
    -- ... repeat for all fields
END;
\$\$ LANGUAGE plpgsql;
"

# Step 2: Use inheritance function in API
cat > /tmp/branding-api-with-inheritance.js <<'EOF'
// GET /api/branding
app.get('/api/branding', async (req, res) => {
  const tenantId = req.tenantId;

  // Get effective branding (with inheritance)
  const branding = await db.query(
    'SELECT * FROM get_effective_branding($1)',
    [tenantId]
  );

  // Apply overrides
  const overrides = await db.query(
    'SELECT * FROM public.branding_overrides WHERE tenant_id = $1',
    [tenantId]
  );

  // Merge branding with overrides
  const effective = { ...branding, ...overrides };

  res.json(effective);
});
EOF

# Step 3: Create override management UI
cat > /tmp/override-example.sql <<'EOF'
-- Branding overrides table
CREATE TABLE IF NOT EXISTS public.branding_overrides (
  tenant_id UUID PRIMARY KEY REFERENCES public.tenants(id),
  override_brand_name BOOLEAN DEFAULT false,
  override_logo BOOLEAN DEFAULT false,
  override_colors BOOLEAN DEFAULT false,
  brand_name TEXT,
  logo_url TEXT,
  primary_color TEXT,
  -- ... other override fields
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Example: Sub-tenant overrides only logo
INSERT INTO public.branding_overrides (tenant_id, override_logo, logo_url)
VALUES ('sub-tenant-id', true, 'https://cdn.example.com/sub-tenant-logo.png');
EOF

# Step 4: Test inheritance
nself db query "SELECT * FROM get_effective_branding('SUB_TENANT_ID');"
```

**Verification:**

```bash
# Test inheritance hierarchy
curl -H "X-Tenant-ID: PARENT_TENANT_ID" https://api.yourdomain.com/branding | jq .
curl -H "X-Tenant-ID: SUB_TENANT_ID" https://api.yourdomain.com/branding | jq .

# Sub-tenant should inherit parent's branding except where overridden
```

**Prevention:**
- Document tenant hierarchy clearly
- Implement inheritance testing
- Provide override UI
- Cache effective branding per tenant
- Audit inheritance chain

---

## 7. Performance Issues

### Issue: Slow Asset Loading

**Symptoms:**
- Logo/images loading slowly
- High Time to First Byte (TTFB)
- Slow page load times
- Timeout errors

**Diagnosis:**

```bash
# 1. Measure asset load times
curl -w "@-" -o /dev/null -s https://cdn.yourdomain.com/logos/logo.png <<'EOF'
time_namelookup: %{time_namelookup}s
time_connect: %{time_connect}s
time_appconnect: %{time_appconnect}s
time_pretransfer: %{time_pretransfer}s
time_starttransfer: %{time_starttransfer}s
time_total: %{time_total}s
size_download: %{size_download} bytes
speed_download: %{speed_download} bytes/sec
EOF

# 2. Check asset file sizes
ls -lh /var/lib/nself/storage/branding/logos/

# 3. Check CDN performance
curl -I https://cdn.yourdomain.com/logos/logo.png | grep -i "cache\|age\|hit"

# 4. Check nginx performance
nself logs nginx | grep "upstream_response_time"

# 5. Test with WebPageTest
# Visit: https://www.webpagetest.org/
```

**Solution:**

```bash
# Step 1: Optimize images
cd /var/lib/nself/storage/branding/logos/

# Install optimization tools
apt-get install -y optipng jpegoptim webp

# Optimize PNGs
find . -name "*.png" -exec optipng -o7 {} \;

# Optimize JPEGs
find . -name "*.jpg" -exec jpegoptim --strip-all --max=85 {} \;

# Generate WebP versions
find . -name "*.png" -exec sh -c 'cwebp -q 85 "$1" -o "${1%.png}.webp"' _ {} \;
find . -name "*.jpg" -exec sh -c 'cwebp -q 85 "$1" -o "${1%.jpg}.webp"' _ {} \;

# Step 2: Enable nginx caching
cat >> /etc/nginx/conf.d/asset-caching.conf <<'EOF'
# Asset caching
proxy_cache_path /var/cache/nginx/assets
  levels=1:2
  keys_zone=assets:10m
  max_size=1g
  inactive=30d
  use_temp_path=off;

server {
  location /assets/ {
    proxy_cache assets;
    proxy_cache_valid 200 30d;
    proxy_cache_valid 404 1m;
    proxy_cache_use_stale error timeout updating http_500 http_502 http_503 http_504;
    proxy_cache_lock on;

    add_header X-Cache-Status $upstream_cache_status;
    add_header Cache-Control "public, max-age=2592000, immutable";

    expires 30d;
  }
}
EOF

# Step 3: Enable compression
cat >> /etc/nginx/conf.d/compression.conf <<'EOF'
# Gzip compression
gzip on;
gzip_vary on;
gzip_proxied any;
gzip_comp_level 6;
gzip_types
  text/plain
  text/css
  text/xml
  text/javascript
  application/json
  application/javascript
  application/xml+rss
  application/rss+xml
  image/svg+xml;

# Brotli compression (if module available)
brotli on;
brotli_comp_level 6;
brotli_types
  text/plain
  text/css
  text/xml
  text/javascript
  application/json
  application/javascript
  application/xml+rss;
EOF

# Step 4: Implement lazy loading
cat > /tmp/lazy-load-images.html <<'EOF'
<!-- Use native lazy loading -->
<img
  src="https://cdn.yourdomain.com/logos/logo.png"
  loading="lazy"
  alt="Logo"
>

<!-- Or use Intersection Observer -->
<script>
const lazyImages = document.querySelectorAll('img[data-src]');
const imageObserver = new IntersectionObserver((entries) => {
  entries.forEach(entry => {
    if (entry.isIntersecting) {
      const img = entry.target;
      img.src = img.dataset.src;
      img.removeAttribute('data-src');
      imageObserver.unobserve(img);
    }
  });
});

lazyImages.forEach(img => imageObserver.observe(img));
</script>
EOF

# Step 5: Use CDN for assets
# Configure CDN (Cloudflare example)
# 1. Add CNAME: cdn.yourdomain.com -> yourdomain.com
# 2. Enable "Cache Everything" page rule
# 3. Set Browser Cache TTL: 1 month
# 4. Enable Auto Minify: JS, CSS, HTML

# Step 6: Implement HTTP/2 and HTTP/3
cat >> /etc/nginx/sites-enabled/default.conf <<'EOF'
server {
  listen 443 ssl http2;
  listen 443 quic reuseport;  # HTTP/3

  ssl_certificate /etc/ssl/certs/yourdomain.pem;
  ssl_certificate_key /etc/ssl/private/yourdomain.key;

  # HTTP/3 advertisement
  add_header Alt-Svc 'h3=":443"; ma=86400';
}
EOF

# Step 7: Restart nginx
nginx -t && nginx -s reload
```

**Verification:**

```bash
# Test asset load time (should be < 500ms)
curl -w "Total time: %{time_total}s\n" -o /dev/null -s https://cdn.yourdomain.com/logos/logo.png

# Check cache status
curl -I https://cdn.yourdomain.com/logos/logo.png | grep "X-Cache-Status"
# Should show: HIT

# Test compression
curl -H "Accept-Encoding: gzip" -I https://cdn.yourdomain.com/assets/styles.css | grep "Content-Encoding"
# Should show: gzip

# Measure full page load
curl -w "@-" -o /dev/null -s https://yourdomain.com <<'EOF'
Total time: %{time_total}s
Size: %{size_download} bytes
Speed: %{speed_download} bytes/sec
EOF
```

**Prevention:**
- Optimize images before upload
- Use modern formats (WebP, AVIF)
- Implement CDN from day one
- Monitor asset sizes
- Set up performance budgets
- Use lazy loading for images

---

### Issue: Database Query Performance

**Symptoms:**
- Slow branding API responses
- High database CPU usage
- Query timeouts
- Application lag

**Diagnosis:**

```bash
# 1. Check slow queries
nself db query "
SELECT
  query,
  calls,
  total_time,
  mean_time,
  max_time
FROM pg_stat_statements
WHERE query LIKE '%branding%'
ORDER BY mean_time DESC
LIMIT 10;
"

# 2. Check missing indexes
nself db query "
SELECT
  schemaname,
  tablename,
  attname,
  n_distinct,
  correlation
FROM pg_stats
WHERE tablename = 'branding'
  AND schemaname = 'public';
"

# 3. Analyze table
nself db query "ANALYZE public.branding;"

# 4. Check table statistics
nself db query "
SELECT
  relname,
  n_tup_ins,
  n_tup_upd,
  n_tup_del,
  n_live_tup,
  n_dead_tup,
  last_vacuum,
  last_autovacuum
FROM pg_stat_user_tables
WHERE relname = 'branding';
"

# 5. Explain query performance
nself db query "EXPLAIN ANALYZE SELECT * FROM public.branding WHERE tenant_id = 'TENANT_ID';"
```

**Solution:**

```bash
# Step 1: Add missing indexes
nself db query "
-- Index on tenant_id (most common filter)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_branding_tenant_id
ON public.branding(tenant_id);

-- Index on domain for custom domain lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_custom_domains_domain
ON public.custom_domains(domain);

-- Composite index for common queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_branding_tenant_enabled
ON public.branding(tenant_id, enabled)
WHERE enabled = true;

-- Index on updated_at for cache invalidation
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_branding_updated_at
ON public.branding(updated_at DESC);
"

# Step 2: Implement query result caching
cat > /tmp/branding-cache.js <<'EOF'
const redis = require('redis');
const client = redis.createClient();

async function getBrandingCached(tenantId) {
  const cacheKey = `branding:${tenantId}`;

  // Try cache first
  const cached = await client.get(cacheKey);
  if (cached) {
    return JSON.parse(cached);
  }

  // Query database
  const branding = await db.query(
    'SELECT * FROM public.branding WHERE tenant_id = $1',
    [tenantId]
  );

  // Cache for 5 minutes
  await client.setex(cacheKey, 300, JSON.stringify(branding));

  return branding;
}
EOF

# Step 3: Optimize database connection pooling
cat >> .env <<'EOF'
# Database connection pool
DB_POOL_MIN=2
DB_POOL_MAX=10
DB_POOL_IDLE_TIMEOUT=10000
DB_CONNECTION_TIMEOUT=2000
EOF

# Step 4: Partition large tables (if needed)
nself db query "
-- Partition branding table by tenant_id hash
CREATE TABLE public.branding_partitioned (
  LIKE public.branding INCLUDING ALL
) PARTITION BY HASH (tenant_id);

-- Create 8 partitions
CREATE TABLE public.branding_part_0 PARTITION OF public.branding_partitioned
  FOR VALUES WITH (MODULUS 8, REMAINDER 0);
CREATE TABLE public.branding_part_1 PARTITION OF public.branding_partitioned
  FOR VALUES WITH (MODULUS 8, REMAINDER 1);
-- ... repeat for 2-7

-- Migrate data (during maintenance window)
-- INSERT INTO public.branding_partitioned SELECT * FROM public.branding;
"

# Step 5: Vacuum and analyze
nself db query "VACUUM ANALYZE public.branding;"

# Step 6: Enable query plan caching (PostgreSQL 12+)
nself db query "ALTER SYSTEM SET plan_cache_mode = 'auto';"
nself db query "SELECT pg_reload_conf();"
```

**Verification:**

```bash
# Test query performance
time nself db query "SELECT * FROM public.branding WHERE tenant_id = 'TENANT_ID';"
# Should be < 10ms

# Check index usage
nself db query "
SELECT
  schemaname,
  tablename,
  indexname,
  idx_scan,
  idx_tup_read,
  idx_tup_fetch
FROM pg_stat_user_indexes
WHERE tablename = 'branding';
"

# Verify cache hit rate
nself db query "
SELECT
  sum(heap_blks_hit) / (sum(heap_blks_hit) + sum(heap_blks_read)) AS cache_hit_ratio
FROM pg_statio_user_tables
WHERE relname = 'branding';
"
# Should be > 0.99 (99%)
```

**Prevention:**
- Create indexes for all filtered columns
- Implement application-level caching
- Monitor query performance regularly
- Set up slow query logging
- Regular VACUUM and ANALYZE
- Use connection pooling

---

## 8. Security Issues

### Issue: XSS Vulnerabilities in Custom CSS

**Symptoms:**
- JavaScript execution in style tags
- Malicious content injection
- Cross-site scripting attacks
- Data exfiltration attempts

**Diagnosis:**

```bash
# 1. Scan custom CSS for dangerous patterns
nself db query "SELECT tenant_id, custom_css FROM public.branding WHERE custom_css LIKE '%<script%' OR custom_css LIKE '%javascript:%';"

# 2. Check for expression() attacks (IE)
nself db query "SELECT tenant_id, custom_css FROM public.branding WHERE custom_css LIKE '%expression(%';"

# 3. Check for import attacks
nself db query "SELECT tenant_id, custom_css FROM public.branding WHERE custom_css LIKE '%@import%';"

# 4. Audit CSS sanitization
grep -r "sanitizeCSS\|purify" src/

# 5. Test with known XSS payloads
curl -X POST https://api.yourdomain.com/branding/css \
  -H "Content-Type: application/json" \
  -d '{"custom_css":"body{background:url(javascript:alert(1))}"}'
```

**Solution:**

```bash
# Step 1: Implement CSS sanitization
npm install css-tree

cat > /tmp/css-sanitizer.js <<'EOF'
const csstree = require('css-tree');

function sanitizeCSS(css) {
  try {
    // Parse CSS
    const ast = csstree.parse(css);

    // Dangerous patterns to remove
    const dangerousPatterns = [
      /javascript:/gi,
      /expression\(/gi,
      /behavior:/gi,
      /@import/gi,
      /vbscript:/gi,
      /data:text\/html/gi,
    ];

    // Walk AST and remove dangerous nodes
    csstree.walk(ast, function(node) {
      if (node.type === 'Url') {
        const value = node.value.value;
        for (const pattern of dangerousPatterns) {
          if (pattern.test(value)) {
            this.remove(node);
            return;
          }
        }
      }

      if (node.type === 'Atrule' && node.name === 'import') {
        this.remove(node);
      }
    });

    // Generate sanitized CSS
    return csstree.generate(ast);

  } catch (err) {
    console.error('CSS parsing error:', err);
    return ''; // Return empty string on parse error
  }
}

// Whitelist only safe properties
const SAFE_CSS_PROPERTIES = [
  'color', 'background-color', 'background',
  'font-family', 'font-size', 'font-weight',
  'border', 'border-radius', 'padding', 'margin',
  'width', 'height', 'display', 'text-align',
  // ... add more as needed
];

function sanitizeCSSStrict(css) {
  const ast = csstree.parse(css);

  csstree.walk(ast, function(node) {
    if (node.type === 'Declaration') {
      if (!SAFE_CSS_PROPERTIES.includes(node.property)) {
        this.remove(node);
      }
    }
  });

  return csstree.generate(ast);
}

module.exports = { sanitizeCSS, sanitizeCSSStrict };
EOF

# Step 2: Add Content Security Policy
cat >> /etc/nginx/conf.d/csp.conf <<'EOF'
# Content Security Policy
add_header Content-Security-Policy "
  default-src 'self';
  script-src 'self' 'unsafe-inline' https://cdn.yourdomain.com;
  style-src 'self' 'unsafe-inline' https://cdn.yourdomain.com;
  img-src 'self' data: https:;
  font-src 'self' data: https:;
  connect-src 'self' https://api.yourdomain.com;
  frame-ancestors 'none';
  base-uri 'self';
  form-action 'self';
" always;

# Additional security headers
add_header X-Content-Type-Options "nosniff" always;
add_header X-Frame-Options "DENY" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
EOF

# Step 3: Validate CSS on input
cat > /tmp/css-validation.js <<'EOF'
const { sanitizeCSS } = require('./css-sanitizer');

app.post('/api/branding/css', async (req, res) => {
  const { custom_css } = req.body;

  // Validate input
  if (!custom_css || typeof custom_css !== 'string') {
    return res.status(400).json({ error: 'Invalid CSS' });
  }

  // Size limit (e.g., 50KB)
  if (custom_css.length > 50 * 1024) {
    return res.status(400).json({ error: 'CSS too large' });
  }

  // Sanitize CSS
  const sanitized = sanitizeCSS(custom_css);

  // Save sanitized version
  await db.query(
    'UPDATE public.branding SET custom_css = $1 WHERE tenant_id = $2',
    [sanitized, req.tenantId]
  );

  res.json({ message: 'CSS updated', css: sanitized });
});
EOF

# Step 4: Audit existing CSS
nself db query "
-- Find potentially dangerous CSS
SELECT
  tenant_id,
  LENGTH(custom_css) as css_size,
  custom_css
FROM public.branding
WHERE custom_css IS NOT NULL
  AND (
    custom_css LIKE '%javascript:%'
    OR custom_css LIKE '%expression(%'
    OR custom_css LIKE '%@import%'
    OR custom_css LIKE '%behavior:%'
  );
"

# Step 5: Sanitize existing CSS in database
cat > /tmp/sanitize-existing-css.js <<'EOF'
const { sanitizeCSS } = require('./css-sanitizer');

async function sanitizeAllCSS() {
  const results = await db.query('SELECT tenant_id, custom_css FROM public.branding WHERE custom_css IS NOT NULL');

  for (const row of results) {
    const sanitized = sanitizeCSS(row.custom_css);

    await db.query(
      'UPDATE public.branding SET custom_css = $1 WHERE tenant_id = $2',
      [sanitized, row.tenant_id]
    );

    console.log(`Sanitized CSS for tenant ${row.tenant_id}`);
  }
}

sanitizeAllCSS().then(() => console.log('Done'));
EOF

node /tmp/sanitize-existing-css.js

# Step 6: Restart nginx
nginx -s reload
```

**Verification:**

```bash
# Test XSS prevention
curl -X POST https://api.yourdomain.com/branding/css \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer TOKEN" \
  -d '{"custom_css":"body{background:url(javascript:alert(1))}"}'

# Should reject or sanitize the input

# Check CSP headers
curl -I https://yourdomain.com | grep "Content-Security-Policy"

# Audit for unsafe CSS
nself db query "SELECT COUNT(*) FROM public.branding WHERE custom_css LIKE '%javascript:%';"
# Should return: 0
```

**Prevention:**
- Always sanitize CSS input
- Use CSS parsers, not regex
- Implement strict CSP
- Whitelist safe CSS properties
- Regular security audits
- Limit CSS size
- Educate users about safe CSS

---

### Issue: Domain Verification Bypass

**Symptoms:**
- Unauthorized domains verified
- Domain takeover attempts
- Verification token leakage

**Diagnosis:**

```bash
# 1. Check for domains without proper verification
nself db query "
SELECT
  domain,
  verified,
  verification_token,
  verified_at,
  created_at
FROM public.custom_domains
WHERE verified = true
  AND (verified_at IS NULL OR verified_at < created_at);
"

# 2. Check for reused verification tokens
nself db query "
SELECT
  verification_token,
  COUNT(*) as count
FROM public.custom_domains
GROUP BY verification_token
HAVING COUNT(*) > 1;
"

# 3. Check for expired but verified domains
nself db query "
SELECT *
FROM public.custom_domains
WHERE verified = true
  AND verified_at < NOW() - INTERVAL '90 days';
"

# 4. Audit verification logs
nself logs domain-verification | grep -i "bypass\|failure\|error"
```

**Solution:**

```bash
# Step 1: Generate secure verification tokens
cat > /tmp/generate-verification-token.js <<'EOF'
const crypto = require('crypto');

function generateVerificationToken() {
  // Generate cryptographically secure random token
  const token = crypto.randomBytes(32).toString('hex');

  // Format: nself-verify-{timestamp}-{random}
  const timestamp = Date.now();
  return `nself-verify-${timestamp}-${token}`;
}

// Usage:
const token = generateVerificationToken();
// Example: nself-verify-1706630400000-a1b2c3d4e5f6...
EOF

# Step 2: Implement proper verification flow
cat > /tmp/domain-verification.js <<'EOF'
const dns = require('dns').promises;

async function verifyDomain(domain, expectedToken) {
  try {
    // 1. Check TXT record
    const records = await dns.resolveTxt(`_nself-verify.${domain}`);
    const flatRecords = records.flat();

    // 2. Find matching token
    const found = flatRecords.some(record => record === expectedToken);

    if (!found) {
      return {
        verified: false,
        error: 'Verification token not found in DNS'
      };
    }

    // 3. Check from multiple DNS servers
    const dnsServers = ['8.8.8.8', '1.1.1.1', '9.9.9.9'];
    const verifications = await Promise.all(
      dnsServers.map(server =>
        verifyFromDNSServer(domain, expectedToken, server)
      )
    );

    // 4. Require majority consensus (2 out of 3)
    const successCount = verifications.filter(v => v.verified).length;
    if (successCount < 2) {
      return {
        verified: false,
        error: 'DNS verification failed on multiple servers'
      };
    }

    // 5. Update database
    await db.query(`
      UPDATE public.custom_domains
      SET verified = true,
          verified_at = NOW(),
          verification_attempts = verification_attempts + 1
      WHERE domain = $1
        AND verification_token = $2
        AND verified = false
    `, [domain, expectedToken]);

    // 6. Generate SSL certificate
    await generateSSLCertificate(domain);

    return { verified: true };

  } catch (err) {
    console.error('Verification error:', err);

    // Log failed attempt
    await db.query(`
      UPDATE public.custom_domains
      SET verification_attempts = verification_attempts + 1,
          last_verification_error = $1
      WHERE domain = $2
    `, [err.message, domain]);

    return {
      verified: false,
      error: err.message
    };
  }
}

async function verifyFromDNSServer(domain, token, dnsServer) {
  const resolver = new dns.Resolver();
  resolver.setServers([dnsServer]);

  try {
    const records = await resolver.resolveTxt(`_nself-verify.${domain}`);
    const found = records.flat().some(r => r === token);
    return { verified: found, server: dnsServer };
  } catch {
    return { verified: false, server: dnsServer };
  }
}
EOF

# Step 3: Add rate limiting
cat > /tmp/verification-rate-limit.js <<'EOF'
const rateLimit = require('express-rate-limit');

const verificationLimiter = rateLimit({
  windowMs: 15 * 60 * 1000, // 15 minutes
  max: 5, // 5 attempts per window
  message: 'Too many verification attempts, try again later',
  standardHeaders: true,
  legacyHeaders: false,
  keyGenerator: (req) => {
    // Rate limit per domain
    return req.body.domain || req.ip;
  }
});

app.post('/api/domains/verify', verificationLimiter, async (req, res) => {
  // Verification logic
});
EOF

# Step 4: Implement re-verification
nself db query "
-- Add re-verification schedule
ALTER TABLE public.custom_domains
ADD COLUMN IF NOT EXISTS next_verification_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '90 days';

-- Create function to schedule re-verification
CREATE OR REPLACE FUNCTION schedule_domain_reverification()
RETURNS void AS \$\$
BEGIN
  UPDATE public.custom_domains
  SET verified = false,
      next_verification_at = NOW() + INTERVAL '7 days'
  WHERE verified = true
    AND verified_at < NOW() - INTERVAL '90 days';
END;
\$\$ LANGUAGE plpgsql;
"

# Step 5: Set up automated re-verification cron job
cat > /tmp/domain-reverification.sh <<'EOF'
#!/bin/bash
# Run daily via cron

nself db query "SELECT schedule_domain_reverification();"

# Re-verify domains
nself db query "
SELECT domain, verification_token
FROM public.custom_domains
WHERE next_verification_at < NOW()
  AND enabled = true;
" | while read domain token; do
  echo "Re-verifying $domain..."
  nself domain verify "$domain"
done
EOF

chmod +x /tmp/domain-reverification.sh

# Add to crontab
(crontab -l 2>/dev/null; echo "0 2 * * * /tmp/domain-reverification.sh") | crontab -
```

**Verification:**

```bash
# Test verification process
nself domain verify yourdomain.com

# Check verification status
nself db query "SELECT * FROM public.custom_domains WHERE domain = 'yourdomain.com';"

# Test rate limiting
for i in {1..10}; do
  curl -X POST https://api.yourdomain.com/domains/verify \
    -H "Content-Type: application/json" \
    -d '{"domain":"test.com"}'
done
# Should get rate limited after 5 attempts
```

**Prevention:**
- Use cryptographically secure tokens
- Verify from multiple DNS servers
- Implement rate limiting
- Regular re-verification (every 90 days)
- Log all verification attempts
- Monitor for suspicious patterns
- Validate token format

---

## 9. Browser Compatibility

### Issue: Cross-Browser CSS Issues

**Symptoms:**
- Layout broken in Safari/Firefox
- Colors rendering differently
- Fonts not loading
- CSS features not working

**Diagnosis:**

```bash
# 1. Check CSS for browser-specific issues
grep -E "(:-moz-|-webkit-|-ms-)" /var/www/html/css/theme.css

# 2. Validate CSS
npm install -g csslint
csslint /var/www/html/css/theme.css

# 3. Check for modern CSS features
grep -E "(grid|flex|var\()" /var/www/html/css/theme.css

# 4. Test in BrowserStack
# Visit: https://www.browserstack.com/

# 5. Check console errors in different browsers
# Safari: Develop → Show Error Console
# Firefox: Tools → Web Developer → Browser Console
```

**Solution:**

```bash
# Step 1: Add vendor prefixes
npm install -g autoprefixer postcss-cli

# Process CSS with autoprefixer
postcss /var/www/html/css/theme.css \
  --use autoprefixer \
  -o /var/www/html/css/theme-prefixed.css \
  --no-map

# Step 2: Create browser compatibility CSS
cat > /var/www/html/css/theme-compat.css <<'EOF'
/* CSS Custom Properties fallbacks */
:root {
  --brand-primary: #007bff;
  --brand-secondary: #6c757d;
}

.button {
  /* Modern */
  background-color: var(--brand-primary);

  /* Fallback for IE11 */
  background-color: #007bff;
}

/* Flexbox with fallback */
.flex-container {
  display: flex;
  display: -webkit-flex;  /* Safari */
  display: -ms-flexbox;   /* IE10 */
}

/* Grid with fallback */
.grid-container {
  display: grid;

  /* Fallback for older browsers */
  display: flex;
  flex-wrap: wrap;
}

@supports (display: grid) {
  .grid-container {
    display: grid;
  }
}

/* Font rendering */
body {
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-rendering: optimizeLegibility;
}

/* Smooth scrolling */
html {
  scroll-behavior: smooth;
}

@media (prefers-reduced-motion: reduce) {
  html {
    scroll-behavior: auto;
  }
}
EOF

# Step 3: Add feature detection
cat > /var/www/html/js/feature-detection.js <<'EOF'
// Feature detection
const supportsGrid = CSS.supports('display', 'grid');
const supportsCustomProperties = CSS.supports('--fake-var', '0');
const supportsFlex = CSS.supports('display', 'flex');

// Add classes to html element
if (supportsGrid) document.documentElement.classList.add('supports-grid');
if (supportsCustomProperties) document.documentElement.classList.add('supports-custom-props');
if (supportsFlex) document.documentElement.classList.add('supports-flex');

// Load polyfills if needed
if (!supportsCustomProperties) {
  // Load CSS Variables polyfill
  const script = document.createElement('script');
  script.src = 'https://cdn.jsdelivr.net/npm/css-vars-ponyfill@2';
  document.head.appendChild(script);
}
EOF

# Step 4: Add browserslist configuration
cat > /var/www/html/.browserslistrc <<'EOF'
# Browsers to support
> 0.5%
last 2 versions
Firefox ESR
not dead
not IE 11
EOF

# Step 5: Test with different user agents
# Test Safari
curl -A "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Safari/605.1.15" \
  https://yourdomain.com > /tmp/safari-test.html

# Test Firefox
curl -A "Mozilla/5.0 (X11; Linux x86_64; rv:95.0) Gecko/20100101 Firefox/95.0" \
  https://yourdomain.com > /tmp/firefox-test.html

# Test Chrome
curl -A "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36" \
  https://yourdomain.com > /tmp/chrome-test.html
```

**Browser-Specific CSS:**

```css
/* Safari-specific fixes */
@supports (-webkit-appearance: none) {
  .safari-fix {
    /* Safari-specific styles */
  }
}

/* Firefox-specific fixes */
@-moz-document url-prefix() {
  .firefox-fix {
    /* Firefox-specific styles */
  }
}

/* IE11 fallbacks (if needed) */
@media all and (-ms-high-contrast: none), (-ms-high-contrast: active) {
  .ie11-fix {
    /* IE11-specific styles */
  }
}
```

**Prevention:**
- Use autoprefixer in build process
- Test in multiple browsers regularly
- Use feature detection, not browser detection
- Provide progressive enhancement
- Use modern CSS with fallbacks
- Follow web standards

---

## 10. Common Error Messages

### Error: "Branding Not Found"

**Message**: `Error: Branding configuration not found for tenant`

**Causes:**
1. Tenant ID missing or incorrect
2. Branding not initialized for tenant
3. Database connection issue
4. Cache inconsistency

**Fix:**

```bash
# Check if branding exists
nself db query "SELECT * FROM public.branding WHERE tenant_id = 'TENANT_ID';"

# If empty, initialize branding
nself db query "
INSERT INTO public.branding (tenant_id, brand_name, enabled)
VALUES ('TENANT_ID', 'Default Brand', true)
ON CONFLICT (tenant_id) DO NOTHING;
"

# Clear cache
nself cache clear --pattern "branding:TENANT_ID"

# Verify
curl -H "X-Tenant-ID: TENANT_ID" https://api.yourdomain.com/branding
```

---

### Error: "Domain Already Registered"

**Message**: `Error: Domain already registered to another tenant`

**Causes:**
1. Domain already in use
2. Previous tenant not cleaned up
3. Database constraint violation

**Fix:**

```bash
# Check domain ownership
nself db query "SELECT * FROM public.custom_domains WHERE domain = 'yourdomain.com';"

# If domain should be transferred:
# 1. Verify current tenant is inactive or approves transfer
# 2. Update domain ownership
nself db query "
UPDATE public.custom_domains
SET tenant_id = 'NEW_TENANT_ID',
    verified = false,
    verification_token = 'NEW_TOKEN'
WHERE domain = 'yourdomain.com';
"

# Clear cache
nself cache clear --pattern "domain:yourdomain.com"
```

---

### Error: "SSL Certificate Generation Failed"

**Message**: `Error: Failed to generate SSL certificate`

**Causes:**
1. Domain not verified
2. Rate limit exceeded (Let's Encrypt)
3. DNS not propagated
4. Certbot configuration issue

**Fix:**

```bash
# Check domain verification
dig TXT _nself-verify.yourdomain.com +short

# Check Let's Encrypt rate limits
# Visit: https://letsencrypt.org/docs/rate-limits/

# Wait for rate limit reset or use staging
certbot certonly --staging -d yourdomain.com

# Check certbot logs
tail -50 /var/log/letsencrypt/letsencrypt.log

# Manual certificate generation
certbot certonly \
  --manual \
  --preferred-challenges dns \
  -d yourdomain.com \
  --agree-tos \
  --email admin@yourdomain.com
```

---

### Error: "Asset Upload Failed"

**Message**: `Error: Asset upload failed`

**Causes:**
1. File too large
2. Invalid file type
3. Storage quota exceeded
4. Permission issues

**Fix:**

```bash
# Check storage space
df -h /var/lib/nself/storage

# Check file permissions
ls -la /var/lib/nself/storage/branding/

# Fix permissions
chown -R nself:nself /var/lib/nself/storage/
chmod -R 755 /var/lib/nself/storage/

# Clean up old files
find /var/lib/nself/storage/branding/ -mtime +90 -type f -delete

# Test upload
curl -X POST https://api.yourdomain.com/branding/logo \
  -H "Authorization: Bearer TOKEN" \
  -F "logo=@test-logo.png"
```

---

### Error: "Template Rendering Failed"

**Message**: `Error: Failed to render email template`

**Causes:**
1. Missing template variables
2. Invalid template syntax
3. Database connection issue
4. Template not found

**Fix:**

```bash
# Check template exists
nself db query "SELECT * FROM public.email_templates WHERE name = 'welcome';"

# Validate template syntax
nself email validate-template --template welcome

# Test with sample data
nself email render-test \
  --template welcome \
  --data '{"user_name":"Test","verification_link":"https://example.com"}'

# Check required variables
nself db query "SELECT required_variables FROM public.email_templates WHERE name = 'welcome';"

# Fix template
nself db query "
UPDATE public.email_templates
SET body = 'corrected template HTML'
WHERE name = 'welcome';
"
```

---

## Diagnostic Tools

### Quick Health Check Script

```bash
#!/bin/bash
# nself-branding-health-check.sh

echo "=== nself White-Label Health Check ==="
echo

# 1. Database connectivity
echo "1. Checking database..."
nself db query "SELECT 1;" >/dev/null 2>&1 && echo "✓ Database connected" || echo "✗ Database connection failed"

# 2. Redis connectivity
echo "2. Checking Redis..."
nself redis ping >/dev/null 2>&1 && echo "✓ Redis connected" || echo "✗ Redis connection failed"

# 3. Storage accessibility
echo "3. Checking storage..."
[ -d /var/lib/nself/storage ] && echo "✓ Storage accessible" || echo "✗ Storage not accessible"

# 4. Branding service
echo "4. Checking branding service..."
docker ps | grep -q branding && echo "✓ Branding service running" || echo "✗ Branding service not running"

# 5. Nginx
echo "5. Checking nginx..."
docker ps | grep -q nginx && echo "✓ Nginx running" || echo "✗ Nginx not running"

# 6. SSL certificates
echo "6. Checking SSL certificates..."
cert_count=$(find /etc/letsencrypt/live/ -name "cert.pem" 2>/dev/null | wc -l)
echo "   Found $cert_count certificates"

# 7. Branding count
echo "7. Checking branding configurations..."
brand_count=$(nself db query "SELECT COUNT(*) FROM public.branding;" 2>/dev/null | tail -1)
echo "   Total: $brand_count configurations"

# 8. Custom domains
echo "8. Checking custom domains..."
domain_count=$(nself db query "SELECT COUNT(*) FROM public.custom_domains WHERE verified = true;" 2>/dev/null | tail -1)
echo "   Verified domains: $domain_count"

# 9. Recent errors
echo "9. Checking recent errors..."
error_count=$(nself logs branding --since 1h 2>/dev/null | grep -i error | wc -l)
echo "   Errors in last hour: $error_count"

# 10. Cache status
echo "10. Checking cache..."
cache_keys=$(nself redis keys "branding:*" 2>/dev/null | wc -l)
echo "    Cached branding: $cache_keys keys"

echo
echo "=== Health Check Complete ==="
```

**Usage:**

```bash
chmod +x nself-branding-health-check.sh
./nself-branding-health-check.sh
```

---

### Branding Diagnostic Script

```bash
#!/bin/bash
# nself-branding-diagnostics.sh TENANT_ID

TENANT_ID="$1"

if [ -z "$TENANT_ID" ]; then
  echo "Usage: $0 TENANT_ID"
  exit 1
fi

echo "=== Branding Diagnostics for $TENANT_ID ==="
echo

# 1. Database record
echo "1. Database Record:"
nself db query "SELECT * FROM public.branding WHERE tenant_id = '$TENANT_ID';"
echo

# 2. Cache status
echo "2. Cache Status:"
nself redis get "branding:$TENANT_ID"
echo

# 3. Custom domains
echo "3. Custom Domains:"
nself db query "SELECT domain, verified, ssl_enabled FROM public.custom_domains WHERE tenant_id = '$TENANT_ID';"
echo

# 4. Assets
echo "4. Assets:"
ls -lh "/var/lib/nself/storage/branding/$TENANT_ID/" 2>/dev/null || echo "No assets found"
echo

# 5. Recent logs
echo "5. Recent Logs (last 20 lines):"
nself logs branding | grep "$TENANT_ID" | tail -20
echo

# 6. API test
echo "6. API Test:"
curl -s -H "X-Tenant-ID: $TENANT_ID" "https://api.yourdomain.com/branding" | jq .
echo

echo "=== Diagnostics Complete ==="
```

**Usage:**

```bash
chmod +x nself-branding-diagnostics.sh
./nself-branding-diagnostics.sh YOUR_TENANT_ID
```

---

## Quick Reference

### Common Commands

```bash
# Health check
nself status

# Clear all caches
nself cache clear --all

# Restart branding services
nself restart branding nginx

# Check logs
nself logs branding --tail 50
nself logs nginx --tail 50

# Database queries
nself db query "SELECT * FROM public.branding WHERE tenant_id = 'ID';"

# Verify domain
nself domain verify yourdomain.com

# Generate SSL certificate
nself ssl generate yourdomain.com

# Test email template
nself email send-test --to you@example.com --template welcome

# Check storage space
df -h /var/lib/nself/storage
```

---

### Log Locations

```
/var/log/nself/branding.log        # Branding service logs
/var/log/nself/domain-verification.log  # Domain verification
/var/log/nginx/access.log          # Nginx access
/var/log/nginx/error.log           # Nginx errors
/var/log/letsencrypt/letsencrypt.log    # SSL certificates
/var/log/postgresql/postgresql.log # Database logs
```

---

### Performance Benchmarks

| Metric | Target | Acceptable | Poor |
|--------|--------|------------|------|
| Asset load time | < 200ms | < 500ms | > 1s |
| API response time | < 100ms | < 300ms | > 1s |
| Database query time | < 10ms | < 50ms | > 200ms |
| Cache hit rate | > 95% | > 80% | < 60% |
| SSL handshake time | < 100ms | < 200ms | > 500ms |
| Email send time | < 2s | < 5s | > 10s |

---

### Security Checklist

- [ ] All custom CSS sanitized
- [ ] Content Security Policy enabled
- [ ] Domain verification active
- [ ] SSL certificates valid
- [ ] Row-level security enabled
- [ ] Asset access control working
- [ ] Rate limiting configured
- [ ] Regular security audits scheduled

---

### Escalation Path

1. **Self-service**: Check this troubleshooting guide
2. **Diagnostics**: Run health check and diagnostic scripts
3. **Documentation**: Check main white-label documentation
4. **Support**: Contact nself support with diagnostic output
5. **Emergency**: For production issues, escalate immediately

---

## Additional Resources

- [White-Label Architecture Documentation](../architecture/WHITE-LABEL-ARCHITECTURE.md)
- [Multi-Tenant Guide](../architecture/MULTI-TENANCY.md)
- [Security Best Practices](../security/SECURITY-BEST-PRACTICES.md)
- [Performance Optimization](../performance/PERFORMANCE-OPTIMIZATION-V0.9.8.md)
- [API Documentation](../reference/api/WHITE-LABEL-API.md)

---

**Document Version**: 1.0.0
**Last Updated**: January 30, 2026
**Maintained By**: nself Team
