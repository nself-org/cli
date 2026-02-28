# White-Label System - Quick Start Guide

Get your white-label branding up and running in 5 minutes.

---

## Prerequisites

- nself installed and configured
- PostgreSQL database running
- Basic understanding of DNS configuration

---

## Quick Start (5 Minutes)

### Step 1: Initialize White-Label System (30 seconds)

```bash
# Initialize the white-label system
nself whitelabel init
```

This creates:
- Default brand configuration
- Branding directories
- Default themes (light, dark, high-contrast)
- Default email templates

### Step 2: Create Your Brand (1 minute)

```bash
# Create your brand
nself whitelabel branding create "My Company"

# Set brand colors (use your brand's hex colors)
nself whitelabel branding set-colors \
  --primary #0066cc \
  --secondary #ff6600 \
  --accent #00cc66

# Set brand fonts
nself whitelabel branding set-fonts \
  --primary "Inter, system-ui, sans-serif" \
  --secondary "Georgia, serif"
```

### Step 3: Upload Your Logo (30 seconds)

```bash
# Upload main logo
nself whitelabel logo upload /path/to/your/logo.png --type main

# Optional: Upload icon and email logos
nself whitelabel logo upload /path/to/icon.png --type icon
nself whitelabel logo upload /path/to/email-logo.png --type email
```

**Recommended logo specs:**
- Main logo: 200-400px width, PNG/SVG
- Icon: 64x64px, PNG/SVG
- Email logo: 200px width, PNG

### Step 4: Configure Custom Domain (2 minutes)

```bash
# Add your custom domain
nself whitelabel domain add app.mycompany.com
```

**Configure DNS** (do this in your DNS provider):

Option A - A Record:
```
Type: A
Name: app
Value: YOUR_SERVER_IP
```

Option B - CNAME Record:
```
Type: CNAME
Name: app
Value: your-server.com
```

Then verify:
```bash
# Verify DNS is configured
nself whitelabel domain verify app.mycompany.com

# Provision SSL certificate (automatic with Let's Encrypt)
nself whitelabel domain ssl app.mycompany.com --auto-renew
```

### Step 5: Preview Your Branding (30 seconds)

```bash
# View complete branding configuration
nself whitelabel branding preview
```

**Done!** Your white-label branding is now active.

---

## Next Steps

### Customize Email Templates

```bash
# List available templates
nself whitelabel email list

# Edit welcome email
nself whitelabel email edit welcome

# Preview with sample data
nself whitelabel email preview welcome

# Send test email
nself whitelabel email test welcome your@email.com
```

### Create Custom Theme

```bash
# Create new theme
nself whitelabel theme create my-theme

# Edit theme configuration
nself whitelabel theme edit my-theme

# Preview theme
nself whitelabel theme preview my-theme

# Activate theme
nself whitelabel theme activate my-theme
```

### Add Custom CSS

```bash
# Create custom.css file with your styles
echo ".custom-class { color: red; }" > custom.css

# Apply custom CSS
nself whitelabel branding set-css custom.css
```

---

## Common Customizations

### Change Theme Mode

```bash
# Activate dark mode
nself whitelabel theme activate dark

# Activate high contrast mode
nself whitelabel theme activate high-contrast

# Back to light mode
nself whitelabel theme activate light
```

### Multi-Language Email Templates

```bash
# Add Spanish email templates
nself whitelabel email set-language es

# Edit Spanish welcome email
nself whitelabel email edit welcome --language es
```

### Export/Backup Configuration

```bash
# Export complete branding
nself whitelabel export --format json > branding-backup.json

# Later, import on different server
nself whitelabel import branding-backup.json
```

---

## Verification Checklist

✅ **Branding**
- [ ] Brand name set
- [ ] Colors configured (primary, secondary, accent)
- [ ] Fonts configured
- [ ] Main logo uploaded
- [ ] Preview looks correct

✅ **Domain**
- [ ] Domain added
- [ ] DNS configured (A or CNAME record)
- [ ] DNS verified
- [ ] SSL certificate provisioned
- [ ] HTTPS working

✅ **Theme**
- [ ] Theme activated (light/dark/custom)
- [ ] CSS variables generated
- [ ] Custom CSS applied (if needed)

✅ **Email**
- [ ] Templates customized
- [ ] Test emails sent
- [ ] Variables replaced correctly

---

## Troubleshooting

### Issue: DNS Verification Fails

**Solution:**
```bash
# Check if DNS has propagated
dig app.mycompany.com A

# Wait for propagation (can take 5-60 minutes)
# Then retry verification
nself whitelabel domain verify app.mycompany.com
```

### Issue: SSL Provisioning Fails

**Solution:**
```bash
# Check if domain is accessible via HTTP
curl -I http://app.mycompany.com

# Use self-signed certificate for testing
SSL_PROVIDER=selfsigned nself whitelabel domain ssl app.mycompany.com

# For production, ensure port 80 and 443 are open
```

### Issue: Logo Not Displaying

**Solution:**
```bash
# Check if logo was uploaded
nself whitelabel logo list

# Verify logo file exists
ls -la branding/logos/

# Re-upload logo
nself whitelabel logo upload /path/to/logo.png --type main
```

### Issue: Colors Not Applying

**Solution:**
```bash
# Regenerate CSS variables
nself whitelabel branding set-colors \
  --primary #0066cc \
  --secondary #ff6600

# Check generated CSS
cat branding/css/variables.css
```

---

## Production Checklist

Before going to production:

### Security
- [ ] Custom domain SSL enabled
- [ ] Auto-renewal configured
- [ ] HTTPS redirect enabled
- [ ] Self-signed certificates replaced with Let's Encrypt

### Performance
- [ ] Logos optimized (compressed)
- [ ] CSS minified
- [ ] CDN configured (if available)

### Branding
- [ ] All email templates customized
- [ ] Brand colors match guidelines
- [ ] Fonts load correctly
- [ ] Logos display properly on all screens

### Testing
- [ ] Test emails sent and verified
- [ ] Domain health checks passing
- [ ] SSL certificate valid
- [ ] Mobile responsive
- [ ] Dark/light themes tested

---

## CLI Reference

### Most Used Commands

```bash
# View help
nself whitelabel --help

# Initialize system
nself whitelabel init

# Branding
nself whitelabel branding create "Brand Name"
nself whitelabel branding set-colors --primary #hex --secondary #hex
nself whitelabel branding preview

# Domains
nself whitelabel domain add example.com
nself whitelabel domain verify example.com
nself whitelabel domain ssl example.com --auto-renew
nself whitelabel domain health example.com

# Themes
nself whitelabel theme create theme-name
nself whitelabel theme activate theme-name
nself whitelabel theme preview theme-name

# Email
nself whitelabel email list
nself whitelabel email edit template-name
nself whitelabel email preview template-name

# Logos
nself whitelabel logo upload path/to/logo.png --type main
nself whitelabel logo list

# System
nself whitelabel settings
nself whitelabel export --format json
nself whitelabel import config.json
```

---

## Example: Complete Setup Script

Save this as `setup-branding.sh`:

```bash
#!/usr/bin/env bash
# Complete white-label setup script

# 1. Initialize
nself whitelabel init

# 2. Create brand
nself whitelabel branding create "TechCorp"

# 3. Configure colors
nself whitelabel branding set-colors \
  --primary #1a73e8 \
  --secondary #34a853 \
  --accent #fbbc04 \
  --background #ffffff \
  --text #202124

# 4. Configure fonts
nself whitelabel branding set-fonts \
  --primary "Roboto, system-ui, sans-serif" \
  --secondary "Merriweather, Georgia, serif" \
  --code "Fira Code, monospace"

# 5. Upload logos
nself whitelabel logo upload ./assets/logo-main.png --type main
nself whitelabel logo upload ./assets/logo-icon.png --type icon
nself whitelabel logo upload ./assets/logo-email.png --type email

# 6. Configure domain
nself whitelabel domain add app.techcorp.com
echo "Configure DNS: A record pointing to $(curl -s ifconfig.me)"
read -p "Press enter when DNS is configured..."
nself whitelabel domain verify app.techcorp.com
nself whitelabel domain ssl app.techcorp.com --auto-renew

# 7. Activate dark theme
nself whitelabel theme activate dark

# 8. Preview
nself whitelabel branding preview

echo "✓ White-label setup complete!"
```

Run it:
```bash
chmod +x setup-branding.sh
./setup-branding.sh
```

---

## Getting Help

- **Documentation**: https://docs.nself.org/whitelabel
- **Examples**: https://github.com/nself-org/cli/tree/main/src/examples/whitelabel
- **Issues**: https://github.com/nself-org/cli/issues
- **Discord**: https://discord.gg/nself

---

**Ready to customize further?** Check out the [complete documentation](../features/WHITELABEL-SYSTEM.md).
