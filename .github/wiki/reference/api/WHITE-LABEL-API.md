# White-Label System API Reference

Complete API documentation for the nself White-Label & Customization system.

**Version:** 0.9.0
**Sprint:** 14 - White-Label & Customization (60pts)

---

## Table of Contents

1. [Overview](#overview)
2. [Branding Commands](#branding-commands)
3. [Logo & Assets](#logo--assets)
4. [Custom Domains](#custom-domains)
5. [Email Templates](#email-templates)
6. [Theme System](#theme-system)
7. [Configuration Management](#configuration-management)
8. [Error Handling](#error-handling)
9. [Function Reference](#function-reference)

---

## Overview

The white-label system provides comprehensive customization capabilities including branding, custom domains, email templates, and theme management.

### System Architecture

```
branding/
├── config.json              # Main branding configuration
├── logos/                   # Logo assets
├── css/                     # Generated CSS variables
├── fonts/                   # Custom fonts
├── assets/                  # Additional assets
├── domains/                 # Domain configurations
│   ├── domains.json
│   ├── ssl/                # SSL certificates
│   └── dns-challenges/     # DNS verification tokens
├── email-templates/        # Email templates
│   ├── languages/         # Multi-language support
│   │   ├── en/           # English templates
│   │   └── [lang]/       # Other languages
│   └── previews/         # Template previews
└── themes/                # Theme definitions
    ├── .active           # Active theme marker
    ├── light/           # Light theme
    ├── dark/            # Dark theme
    └── [custom]/        # Custom themes
```

### Configuration Files

**Branding Config** (`branding/config.json`):
```json
{
  "version": "1.0.0",
  "brand": {
    "name": "nself",
    "tagline": "Powerful Backend for Modern Applications",
    "description": "Open-source backend infrastructure platform"
  },
  "colors": {
    "primary": "#0066cc",
    "secondary": "#ff6600",
    "accent": "#00cc66"
  },
  "fonts": {
    "primary": "Inter, system-ui, sans-serif",
    "secondary": "Georgia, serif"
  },
  "logos": {
    "main": "logo-main.png",
    "icon": "logo-icon.png",
    "email": "logo-email.png",
    "favicon": "logo-favicon.png"
  },
  "customCSS": "custom.css",
  "theme": "light"
}
```

**Domain Config** (`branding/domains/domains.json`):
```json
{
  "version": "1.0.0",
  "domains": [
    {
      "domain": "app.example.com",
      "status": "active",
      "verified": true,
      "sslEnabled": true,
      "sslIssuer": "letsencrypt",
      "sslExpiryDate": "2026-04-30T00:00:00Z",
      "healthStatus": "healthy"
    }
  ],
  "defaultDomain": "app.example.com",
  "sslProvider": "letsencrypt",
  "autoRenew": true
}
```

---

## Branding Commands

### `nself whitelabel branding create <brand-name>`

Create a new brand configuration.

**Syntax:**
```bash
nself whitelabel branding create <brand-name> [tenant-id]
```

**Parameters:**
- `brand-name` (required): Name of the brand
- `tenant-id` (optional): Tenant ID for multi-tenant setups (default: "default")

**Example:**
```bash
# Create single-tenant brand
nself whitelabel branding create "My Company"

# Create brand for specific tenant
nself whitelabel branding create "Acme Corp" tenant-123
```

**Output:**
```
Creating brand: My Company
✓ Brand 'My Company' created successfully

Next steps:
  1. Set brand colors: nself whitelabel branding set-colors
  2. Upload logo: nself whitelabel logo upload <path>
  3. Customize fonts: nself whitelabel branding set-fonts
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::create_brand()`
- Calls: `initialize_branding_system()`, `jq` for JSON manipulation
- Config File: `branding/config.json`

**Error Codes:**
- `0`: Success
- `1`: Invalid brand name or configuration error

---

### `nself whitelabel branding set-colors`

Configure brand color palette.

**Syntax:**
```bash
nself whitelabel branding set-colors [options]
```

**Options:**
- `--primary <color>`: Primary brand color (hex format)
- `--secondary <color>`: Secondary brand color (hex format)
- `--accent <color>`: Accent color (hex format)
- `--background <color>`: Background color (hex format)
- `--text <color>`: Text color (hex format)

**Color Format:**
- Hex colors: `#RRGGBB` or `#RGB`
- Examples: `#0066cc`, `#f00`

**Example:**
```bash
# Set primary and secondary colors
nself whitelabel branding set-colors \
  --primary #0066cc \
  --secondary #ff6600

# Set complete color scheme
nself whitelabel branding set-colors \
  --primary #2563eb \
  --secondary #7c3aed \
  --accent #10b981 \
  --background #ffffff \
  --text #1f2937
```

**Output:**
```
Updating brand colors...
✓ Brand colors updated successfully
  Primary: #0066cc
  Secondary: #ff6600
  Accent: #00cc66
```

**Side Effects:**
- Updates `branding/config.json`
- Regenerates `branding/css/variables.css`
- Auto-generates CSS custom properties

**Generated CSS Variables:**
```css
:root {
  --color-primary: #0066cc;
  --color-secondary: #ff6600;
  --color-accent: #00cc66;
  /* ... */
}
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::set_brand_colors()`
- Validator: `validate_hex_color()`
- CSS Generator: `generate_css_variables()`

**Error Codes:**
- `0`: Success
- `1`: Invalid color format or branding not initialized

---

### `nself whitelabel branding set-fonts`

Configure brand typography.

**Syntax:**
```bash
nself whitelabel branding set-fonts [options]
```

**Options:**
- `--primary <font>`: Primary font family
- `--secondary <font>`: Secondary font family (headings)
- `--code <font>`: Monospace font for code

**Font Format:**
- CSS font-family syntax
- Include fallbacks: `"Inter, system-ui, sans-serif"`
- Web fonts or system fonts

**Example:**
```bash
# Set primary font
nself whitelabel branding set-fonts \
  --primary "Inter, system-ui, sans-serif"

# Set complete font stack
nself whitelabel branding set-fonts \
  --primary "Helvetica Neue, Arial, sans-serif" \
  --secondary "Georgia, Times, serif" \
  --code "Fira Code, Consolas, monospace"
```

**Output:**
```
Updating brand fonts...
✓ Brand fonts updated successfully
  Primary: Helvetica Neue, Arial, sans-serif
  Secondary: Georgia, Times, serif
```

**Generated CSS:**
```css
:root {
  --font-primary: "Helvetica Neue, Arial, sans-serif";
  --font-secondary: "Georgia, Times, serif";
  --font-code: "Fira Code, Consolas, monospace";
}

body {
  font-family: var(--font-primary);
}
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::set_brand_fonts()`
- CSS Generator: `generate_css_variables()`

**Notes:**
- Web fonts must be loaded separately via `@font-face` or external CSS
- Custom fonts can be placed in `branding/fonts/`

---

### `nself whitelabel branding set-css <path>`

Add custom CSS overrides.

**Syntax:**
```bash
nself whitelabel branding set-css <path-to-css-file>
```

**Parameters:**
- `path-to-css-file` (required): Path to custom CSS file

**Example:**
```bash
# Set custom CSS
nself whitelabel branding set-css ./custom-styles.css

# Example custom CSS file
cat > custom.css << EOF
/* Custom brand styles */
.btn-primary {
  background-color: var(--color-primary);
  border-radius: 8px;
  font-weight: 600;
}

.hero-section {
  background-image: url('/assets/hero-bg.jpg');
}
EOF

nself whitelabel branding set-css custom.css
```

**Output:**
```
Setting custom CSS...
✓ Custom CSS set successfully: /path/to/branding/css/custom.css
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::set_custom_css()`
- Destination: `branding/css/custom.css`

**Best Practices:**
- Use CSS custom properties for colors: `var(--color-primary)`
- Avoid `!important` when possible
- Keep specificity low for easier overrides

---

### `nself whitelabel branding preview`

Preview current branding configuration.

**Syntax:**
```bash
nself whitelabel branding preview
```

**Example:**
```bash
nself whitelabel branding preview
```

**Output:**
```
Branding Preview
============================================================

Brand: My Company
Primary Color: #0066cc
Secondary Color: #ff6600
Primary Font: Inter, system-ui, sans-serif

Logos:
  main: logo-main.png
  icon: logo-icon.png
  email: not set
  favicon: not set

Updated: 2026-01-30T12:34:56Z
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::preview_branding()`
- Data Source: `branding/config.json`

---

## Logo & Assets

### `nself whitelabel logo upload <path>`

Upload brand logo.

**Syntax:**
```bash
nself whitelabel logo upload <path> [--type <logo-type>]
```

**Parameters:**
- `path` (required): Path to logo file
- `--type <type>` (optional): Logo type (default: `main`)

**Logo Types:**
- `main`: Primary logo (header, homepage)
- `icon`: Small icon/favicon (16x16 to 64x64)
- `email`: Email header logo (optimized for email clients)
- `favicon`: Browser favicon

**Supported Formats:**
- PNG (recommended)
- JPG/JPEG
- SVG (vector, scalable)
- WebP (modern browsers)

**Example:**
```bash
# Upload main logo
nself whitelabel logo upload ./logo.png

# Upload specific logo types
nself whitelabel logo upload ./logo-main.svg --type main
nself whitelabel logo upload ./icon-64x64.png --type icon
nself whitelabel logo upload ./email-logo.png --type email
nself whitelabel logo upload ./favicon.ico --type favicon
```

**Output:**
```
Uploading main logo...
✓ Logo uploaded successfully: /path/to/branding/logos/logo-main.png
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::upload_logo()`
- Called Function: `upload_brand_logo()`
- Destination: `branding/logos/logo-{type}.{ext}`

**Logo Recommendations:**
| Type | Size | Format | Usage |
|------|------|--------|-------|
| main | 200x60px | SVG/PNG | Header, navigation |
| icon | 64x64px | PNG/ICO | Favicon, app icon |
| email | 600x200px | PNG | Email headers |
| favicon | 32x32px | ICO/PNG | Browser tab |

**Error Codes:**
- `0`: Success
- `1`: File not found or unsupported format

---

### `nself whitelabel logo list`

List all configured logos.

**Syntax:**
```bash
nself whitelabel logo list
```

**Example:**
```bash
nself whitelabel logo list
```

**Output:**
```
Configured Logos:

main: logo-main.png
icon: logo-icon.png
email: logo-email.png
favicon: not set
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::list_logos()`

---

### `nself whitelabel logo remove <logo-type>`

Remove a logo.

**Syntax:**
```bash
nself whitelabel logo remove <logo-type>
```

**Parameters:**
- `logo-type` (required): Type of logo to remove (main, icon, email, favicon)

**Example:**
```bash
# Remove email logo
nself whitelabel logo remove email
```

**Output:**
```
Removing email logo...
✓ Logo removed successfully
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::remove_logo()`
- Deletes: Logo file and config reference

---

## Custom Domains

### `nself whitelabel domain add <domain>`

Add a custom domain.

**Syntax:**
```bash
nself whitelabel domain add <domain> [--primary]
```

**Parameters:**
- `domain` (required): Domain name (e.g., app.example.com)
- `--primary` (optional): Set as primary domain

**Domain Validation:**
- Valid format: `subdomain.domain.tld`
- Examples: `app.example.com`, `api.mycompany.io`
- Invalid: `localhost`, `192.168.1.1`, `example`

**Example:**
```bash
# Add custom domain
nself whitelabel domain add app.mycompany.com

# Add as primary domain
nself whitelabel domain add app.mycompany.com --primary
```

**Output:**
```
Adding custom domain: app.mycompany.com
✓ Domain added: app.mycompany.com

Next steps:
  1. Configure DNS: Point app.mycompany.com to your server IP
  2. Verify domain: nself whitelabel domain verify app.mycompany.com
  3. Provision SSL: nself whitelabel domain ssl app.mycompany.com
```

**Domain Record:**
```json
{
  "domain": "app.mycompany.com",
  "status": "pending",
  "verified": false,
  "sslEnabled": false,
  "sslIssuer": null,
  "dnsVerified": false,
  "healthStatus": "unknown",
  "isPrimary": false,
  "createdAt": "2026-01-30T12:34:56Z"
}
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/domains.sh::add_custom_domain()`
- Validator: `validate_domain_format()`
- Config File: `branding/domains/domains.json`

**Error Codes:**
- `0`: Success
- `1`: Invalid domain format or already exists

---

### `nself whitelabel domain verify <domain>`

Verify domain ownership.

**Syntax:**
```bash
nself whitelabel domain verify <domain>
```

**Parameters:**
- `domain` (required): Domain to verify

**Verification Methods:**

**Method 1: DNS TXT Record**
```
Name:  _nself-verification.app.example.com
Type:  TXT
Value: <verification-token>
```

**Method 2: DNS A/CNAME Record**
```
Name:  app.example.com
Type:  A (or CNAME)
Value: <your-server-ip>
```

**Example:**
```bash
nself whitelabel domain verify app.mycompany.com
```

**Output:**
```
Verifying domain: app.mycompany.com

DNS Verification Instructions:
------------------------------------------------------------
Add this TXT record to your DNS:

  Name:  _nself-verification.app.mycompany.com
  Type:  TXT
  Value: a3f8d92e4c1b5a6f7e8d9c0b1a2f3e4d5c6b7a8f9e0d1c2b3a4f5e6d7c8b9a0f

Or configure A/CNAME record:

  Name:  app.mycompany.com
  Type:  A (or CNAME)
  Value: <your-server-ip>

Checking DNS propagation...
Attempt 1/30 - waiting for DNS propagation...
Attempt 2/30 - waiting for DNS propagation...
✓ Domain verified successfully: app.mycompany.com
```

**DNS Propagation:**
- Timeout: 300 seconds (5 minutes)
- Check interval: 10 seconds
- Max attempts: 30

**Function Reference:**
- Implementation: `src/lib/whitelabel/domains.sh::verify_domain()`
- DNS Checker: `check_dns_propagation()`
- Token Generator: `generate_verification_token()`
- Challenge File: `branding/domains/dns-challenges/{domain}.txt`

**Tools Used:**
- `dig` (preferred)
- `nslookup` (fallback)
- `host` (fallback)

**Error Codes:**
- `0`: Domain verified
- `1`: Verification timeout or failed

---

### `nself whitelabel domain ssl <domain>`

Provision SSL certificate for domain.

**Syntax:**
```bash
nself whitelabel domain ssl <domain> [--auto-renew]
```

**Parameters:**
- `domain` (required): Domain for SSL certificate
- `--auto-renew` (optional): Enable automatic renewal

**SSL Providers:**

**Let's Encrypt** (default, production):
```bash
# Set provider (in .env)
SSL_PROVIDER=letsencrypt

# Provision certificate
nself whitelabel domain ssl app.example.com --auto-renew
```

**Self-Signed** (development):
```bash
SSL_PROVIDER=selfsigned nself whitelabel domain ssl app.example.com
```

**Custom** (bring your own):
```bash
SSL_PROVIDER=custom nself whitelabel domain ssl app.example.com
# Then manually copy certificates to:
# branding/domains/ssl/app.example.com/cert.pem
# branding/domains/ssl/app.example.com/key.pem
# branding/domains/ssl/app.example.com/chain.pem
```

**Example:**
```bash
# Provision Let's Encrypt SSL
nself whitelabel domain ssl app.mycompany.com --auto-renew
```

**Output (Let's Encrypt):**
```
Provisioning SSL certificate for: app.mycompany.com
Using Let's Encrypt for SSL...
Requesting certificate from Let's Encrypt...
✓ SSL certificate provisioned successfully
  Issuer: Let's Encrypt
  Expires: 2026-04-30T00:00:00Z
✓ Auto-renewal configured
Add to crontab: 0 0 * * * /path/to/ssl/renew-app.mycompany.com.sh
```

**Output (Self-Signed):**
```
Provisioning SSL certificate for: app.mycompany.com
Generating self-signed certificate...
✓ Self-signed certificate generated
Note: Self-signed certificates are not trusted by browsers
For production, use Let's Encrypt or a commercial CA
```

**Certificate Files:**
```
branding/domains/ssl/app.mycompany.com/
├── cert.pem        # Public certificate
├── key.pem         # Private key
└── chain.pem       # Certificate chain
```

**Auto-Renewal:**
- Generates renewal script: `ssl/renew-{domain}.sh`
- Cron schedule: Daily at midnight (`0 0 * * *`)
- Uses `certbot renew` command

**Function Reference:**
- Implementation: `src/lib/whitelabel/domains.sh::provision_ssl()`
- Let's Encrypt: `provision_letsencrypt_ssl()`
- Self-Signed: `provision_selfsigned_ssl()`
- Renewal: `setup_ssl_auto_renewal()`

**Prerequisites:**
- Domain verified (recommended)
- `certbot` installed (for Let's Encrypt)
- `openssl` installed (for self-signed)

**Error Codes:**
- `0`: Success
- `1`: Domain not found, provisioning failed, or missing tools

---

### `nself whitelabel domain health <domain>`

Check domain health status.

**Syntax:**
```bash
nself whitelabel domain health <domain>
```

**Parameters:**
- `domain` (required): Domain to check

**Health Checks:**
1. **DNS Resolution**: Domain resolves to IP
2. **SSL Certificate**: Valid and not expired
3. **HTTP Response**: Server responds to requests

**Example:**
```bash
nself whitelabel domain health app.mycompany.com
```

**Output:**
```
Checking domain health: app.mycompany.com
============================================================

1. DNS Resolution: ✓ Resolved
2. SSL Certificate: ✓ Valid
3. HTTP Response: ✓ Responding

Health Status: Healthy
```

**Degraded Output:**
```
Checking domain health: app.mycompany.com
============================================================

1. DNS Resolution: ✓ Resolved
2. SSL Certificate: ! No SSL
3. HTTP Response: ✓ Responding

Health Status: Degraded

Issues Found:
  - No SSL certificate
```

**Unhealthy Output:**
```
Checking domain health: app.mycompany.com
============================================================

1. DNS Resolution: ✗ Not resolved
2. SSL Certificate: ✗ Expired
3. HTTP Response: ✗ Not responding

Health Status: Unhealthy

Issues Found:
  - DNS not resolving
  - SSL certificate expired
  - HTTP not responding
```

**Health Status Values:**
- `healthy`: All checks passed
- `degraded`: Minor issues (missing SSL, warnings)
- `unhealthy`: Critical issues (DNS failed, not responding)

**Function Reference:**
- Implementation: `src/lib/whitelabel/domains.sh::check_domain_health()`
- DNS Check: `check_dns_propagation()`
- SSL Check: `has_ssl_certificate()`, `is_ssl_expired()`
- HTTP Check: `check_http_response()`

**Tools Used:**
- DNS: `dig`, `nslookup`, or `host`
- HTTP: `curl` or `wget`

---

### `nself whitelabel domain remove <domain>`

Remove a custom domain.

**Syntax:**
```bash
nself whitelabel domain remove <domain>
```

**Parameters:**
- `domain` (required): Domain to remove

**Example:**
```bash
nself whitelabel domain remove app.mycompany.com
```

**Output:**
```
Removing custom domain: app.mycompany.com
Removing SSL certificates...
✓ Domain removed: app.mycompany.com
```

**Side Effects:**
- Removes domain from `domains.json`
- Deletes SSL certificates from `ssl/{domain}/`
- Removes DNS challenge files

**Function Reference:**
- Implementation: `src/lib/whitelabel/domains.sh::remove_custom_domain()`

**Warning:**
This operation is permanent and cannot be undone. SSL certificates will be deleted.

---

## Email Templates

### `nself whitelabel email list`

List available email templates.

**Syntax:**
```bash
nself whitelabel email list [language]
```

**Parameters:**
- `language` (optional): Language code (default: `en`)

**Available Templates:**
- `welcome`: New user welcome email
- `password-reset`: Password reset with secure link
- `verify-email`: Email verification
- `invite`: User invitation
- `password-change`: Password change confirmation
- `account-update`: Account information update
- `notification`: Generic notification
- `alert`: Alert/warning notification

**Example:**
```bash
# List English templates
nself whitelabel email list

# List Spanish templates
nself whitelabel email list es
```

**Output:**
```
Email Templates (Language: en)
============================================================

welcome              New user welcome email upon registration
  Subject: Welcome to {{BRAND_NAME}}!

password-reset       Password reset email with secure reset link
  Subject: Reset Your Password

verify-email         Email verification with confirmation link
  Subject: Verify Your Email Address

invite               User invitation email
  Subject: You're invited to {{BRAND_NAME}}

password-change      Password change confirmation
  Subject: Your password has been changed

account-update       General account update notification
  Subject: Account Update Notification

notification         Generic notification template
  Subject: {{NOTIFICATION_TITLE}}

alert                Alert/warning notification template
  Subject: ⚠️ {{ALERT_TITLE}}
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/email-templates.sh::list_email_templates()`
- Template Directory: `branding/email-templates/languages/{lang}/`

---

### `nself whitelabel email edit <template>`

Edit an email template.

**Syntax:**
```bash
nself whitelabel email edit <template-name> [language]
```

**Parameters:**
- `template-name` (required): Name of template to edit
- `language` (optional): Language code (default: `en`)

**Template Files:**
- HTML version: `{template}.html`
- Plain text version: `{template}.txt`
- Metadata: `{template}.json`

**Example:**
```bash
# Edit welcome email
nself whitelabel email edit welcome

# Edit Spanish password reset
nself whitelabel email edit password-reset es

# Uses $EDITOR environment variable (default: vi)
export EDITOR=nano
nself whitelabel email edit welcome
```

**Template Variables:**
Templates support variable injection using `{{VARIABLE_NAME}}` syntax.

**Common Variables:**
```html
<!-- Brand Variables -->
{{BRAND_NAME}}          - Brand/company name
{{LOGO_URL}}            - URL to brand logo
{{APP_URL}}             - Main application URL
{{COMPANY_ADDRESS}}     - Company address
{{SUPPORT_EMAIL}}       - Support email

<!-- User Variables -->
{{USER_NAME}}           - User's display name
{{USER_EMAIL}}          - User's email address

<!-- Action Variables -->
{{RESET_URL}}           - Password reset URL
{{VERIFY_URL}}          - Email verification URL
{{INVITE_URL}}          - Invitation URL
{{ACTION_URL}}          - Generic action URL

<!-- Date Variables -->
{{CURRENT_YEAR}}        - Current year
{{CHANGE_DATE}}         - Date of change
{{EXPIRY_TIME}}         - Link expiration time
```

**Example Template (HTML):**
```html
<!DOCTYPE html>
<html>
<head>
  <title>Welcome to {{BRAND_NAME}}</title>
  <style>
    body { font-family: var(--font-primary, Arial, sans-serif); }
    .button {
      background-color: var(--color-primary, #0066cc);
      color: #fff;
      padding: 12px 30px;
    }
  </style>
</head>
<body>
  <h1>Welcome to {{BRAND_NAME}}!</h1>
  <p>Hi {{USER_NAME}},</p>
  <p>Welcome aboard! We're excited to have you.</p>
  <a href="{{APP_URL}}" class="button">Get Started</a>
</body>
</html>
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/email-templates.sh::edit_email_template()`
- Opens: `$EDITOR {template}.html`
- Path: `branding/email-templates/languages/{lang}/{template}.html`

**Output:**
```
✓ Template updated: welcome
```

---

### `nself whitelabel email preview <template>`

Preview email template with sample data.

**Syntax:**
```bash
nself whitelabel email preview <template-name> [language]
```

**Parameters:**
- `template-name` (required): Template to preview
- `language` (optional): Language code (default: `en`)

**Example:**
```bash
# Preview welcome email
nself whitelabel email preview welcome
```

**Output:**
```
Preview: welcome
============================================================

<!DOCTYPE html>
<html>
<head>
  <title>Welcome to nself</title>
</head>
<body>
  <h1>Welcome to nself!</h1>
  <p>Hi John Doe,</p>
  <p>Welcome aboard! We're excited to have you.</p>
  <a href="https://app.example.com" class="button">Get Started</a>
  <p>&copy; 2026 nself. All rights reserved.</p>
</body>
</html>
```

**Sample Data Used:**
- `{{BRAND_NAME}}` → "nself"
- `{{USER_NAME}}` → "John Doe"
- `{{APP_URL}}` → "https://app.example.com"
- `{{CURRENT_YEAR}}` → Current year

**Function Reference:**
- Implementation: `src/lib/whitelabel/email-templates.sh::preview_email_template()`
- Variable Replacement: Bash string substitution

---

### `nself whitelabel email test <template> <email>`

Send test email.

**Syntax:**
```bash
nself whitelabel email test <template-name> <recipient-email>
```

**Parameters:**
- `template-name` (required): Template to send
- `recipient-email` (required): Email address to send to

**Prerequisites:**
- Mail service configured (SMTP, SendGrid, MailPit, etc.)
- See [Mail Configuration](../../configuration/README.md)

**Example:**
```bash
# Send test welcome email
nself whitelabel email test welcome john@example.com

# Send test password reset
nself whitelabel email test password-reset admin@mycompany.com
```

**Output:**
```
Sending test email to: john@example.com
Note: Email sending requires mail service configuration
✓ Test email queued for: welcome
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/email-templates.sh::test_email_template()`
- Integration: Requires mail service (MailPit, SMTP, etc.)

**Note:**
This command queues the email. Actual sending depends on your mail service configuration. For development, use MailPit to view test emails.

---

### `nself whitelabel email set-language <lang>`

Set email template language.

**Syntax:**
```bash
nself whitelabel email set-language <language-code>
```

**Parameters:**
- `language-code` (required): ISO 639-1 language code

**Supported Languages:**
- `en`: English (default)
- `es`: Spanish
- `fr`: French
- `de`: German
- `ja`: Japanese
- `zh`: Chinese
- Custom: Any language code

**Example:**
```bash
# Set Spanish language
nself whitelabel email set-language es

# Set French language
nself whitelabel email set-language fr
```

**Output:**
```
Setting email language to: es
✓ Email language set to: es
```

**Side Effects:**
- Creates language directory: `branding/email-templates/languages/{lang}/`
- Copies default English templates as starting point
- You can then edit templates for the new language

**Function Reference:**
- Implementation: `src/lib/whitelabel/email-templates.sh::set_email_language()`
- Creates: All template types for new language

**Multi-Language Workflow:**
```bash
# 1. Add Spanish language
nself whitelabel email set-language es

# 2. Edit Spanish templates
nself whitelabel email edit welcome es
nself whitelabel email edit password-reset es

# 3. Preview Spanish template
nself whitelabel email preview welcome es
```

---

## Theme System

### `nself whitelabel theme create <name>`

Create a new theme.

**Syntax:**
```bash
nself whitelabel theme create <theme-name>
```

**Parameters:**
- `theme-name` (required): Theme name (lowercase, hyphens only)

**Theme Name Rules:**
- Lowercase letters only
- Numbers allowed
- Hyphens for word separation
- Examples: `dark-mode`, `high-contrast`, `corporate-2024`

**Example:**
```bash
# Create custom theme
nself whitelabel theme create corporate

# Create dark mode variant
nself whitelabel theme create dark-blue
```

**Output:**
```
Creating theme: corporate
✓ Theme created: corporate

Next steps:
  1. Edit theme: nself whitelabel theme edit corporate
  2. Preview theme: nself whitelabel theme preview corporate
  3. Activate theme: nself whitelabel theme activate corporate
```

**Generated Files:**
```
branding/themes/corporate/
├── theme.json      # Theme configuration
└── theme.css       # Generated CSS
```

**Default Theme Structure:**
```json
{
  "name": "corporate",
  "displayName": "corporate",
  "description": "Custom theme",
  "version": "1.0.0",
  "author": "Custom",
  "mode": "light",
  "variables": {
    "colors": {
      "primary": "#0066cc",
      "secondary": "#6c757d",
      "accent": "#00cc66",
      "background": "#ffffff",
      "text": "#212529"
    },
    "typography": {
      "fontFamily": "-apple-system, sans-serif",
      "fontSize": "16px",
      "fontWeight": "400"
    },
    "spacing": {
      "xs": "4px",
      "sm": "8px",
      "md": "16px",
      "lg": "24px"
    },
    "borders": {
      "radius": "4px",
      "width": "1px"
    },
    "shadows": {
      "sm": "0 1px 3px rgba(0,0,0,0.12)",
      "md": "0 4px 6px rgba(0,0,0,0.1)"
    }
  }
}
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/themes.sh::create_theme()`
- Template: `create_custom_theme_template()`
- CSS Generator: `generate_theme_css()`

---

### `nself whitelabel theme edit <name>`

Edit theme configuration.

**Syntax:**
```bash
nself whitelabel theme edit <theme-name>
```

**Parameters:**
- `theme-name` (required): Theme to edit

**Example:**
```bash
# Edit theme
nself whitelabel theme edit corporate
```

**Edits:** `branding/themes/{name}/theme.json`

**Output:**
```
✓ Theme updated: corporate
```

**Side Effects:**
- Opens theme.json in `$EDITOR`
- Regenerates theme.css after save

**Function Reference:**
- Implementation: `src/lib/whitelabel/themes.sh::edit_theme()`
- CSS Regeneration: `generate_theme_css()`

**Theme Variables Reference:**

```json
{
  "variables": {
    "colors": {
      "primary": "#hex",           // Main brand color
      "primaryHover": "#hex",      // Hover state
      "secondary": "#hex",         // Secondary color
      "accent": "#hex",            // Accent/highlight
      "background": "#hex",        // Page background
      "backgroundAlt": "#hex",     // Alt background
      "surface": "#hex",           // Card/panel surface
      "surfaceAlt": "#hex",        // Alt surface
      "text": "#hex",              // Primary text
      "textSecondary": "#hex",     // Secondary text
      "textMuted": "#hex",         // Muted text
      "border": "#hex",            // Border color
      "borderLight": "#hex",       // Light border
      "success": "#hex",           // Success state
      "warning": "#hex",           // Warning state
      "error": "#hex",             // Error state
      "info": "#hex"               // Info state
    },
    "typography": {
      "fontFamily": "font-stack",
      "fontFamilyMono": "mono-stack",
      "fontSize": "16px",
      "fontWeight": "400",
      "lineHeight": "1.5"
    },
    "spacing": {
      "xs": "4px",
      "sm": "8px",
      "md": "16px",
      "lg": "24px",
      "xl": "32px",
      "xxl": "48px"
    },
    "borders": {
      "radius": "4px",
      "radiusLg": "8px",
      "width": "1px"
    },
    "shadows": {
      "sm": "box-shadow-value",
      "md": "box-shadow-value",
      "lg": "box-shadow-value"
    }
  }
}
```

---

### `nself whitelabel theme activate <name>`

Activate a theme.

**Syntax:**
```bash
nself whitelabel theme activate <theme-name>
```

**Parameters:**
- `theme-name` (required): Theme to activate

**Built-in Themes:**
- `light`: Default light theme
- `dark`: Dark mode theme
- `high-contrast`: Accessibility theme

**Example:**
```bash
# Activate dark theme
nself whitelabel theme activate dark

# Activate custom theme
nself whitelabel theme activate corporate
```

**Output:**
```
Activating theme: dark
✓ Theme activated: dark
```

**Side Effects:**
- Updates `branding/themes/.active` file
- Application will use new theme on next load

**Function Reference:**
- Implementation: `src/lib/whitelabel/themes.sh::activate_theme()`
- Active Marker: `branding/themes/.active`

---

### `nself whitelabel theme preview <name>`

Preview theme configuration.

**Syntax:**
```bash
nself whitelabel theme preview <theme-name>
```

**Parameters:**
- `theme-name` (required): Theme to preview

**Example:**
```bash
nself whitelabel theme preview dark
```

**Output:**
```
Theme Preview: dark
============================================================

Name: Dark Theme
Description: Easy on the eyes dark theme
Mode: dark

Colors:
  primary: #4a9eff
  primaryHover: #6bb0ff
  secondary: #8b949e
  accent: #3fb950
  background: #0d1117
  backgroundAlt: #161b22
  surface: #161b22
  text: #c9d1d9
  textSecondary: #8b949e

Typography:
  fontFamily: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto
  fontSize: 16px
  fontWeight: 400
  lineHeight: 1.5
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/themes.sh::preview_theme()`

---

### `nself whitelabel theme export <name>`

Export theme configuration.

**Syntax:**
```bash
nself whitelabel theme export <theme-name>
```

**Parameters:**
- `theme-name` (required): Theme to export

**Example:**
```bash
# Export to file
nself whitelabel theme export corporate > corporate-theme.json

# Export to stdout
nself whitelabel theme export dark
```

**Output:** JSON theme configuration (stdout)

**Function Reference:**
- Implementation: `src/lib/whitelabel/themes.sh::export_theme()`

**Use Cases:**
- Backup themes
- Share themes between installations
- Version control theme configurations

---

### `nself whitelabel theme import <file>`

Import theme from JSON file.

**Syntax:**
```bash
nself whitelabel theme import <path-to-theme.json>
```

**Parameters:**
- `path-to-theme.json` (required): Path to theme JSON file

**Example:**
```bash
# Import theme
nself whitelabel theme import corporate-theme.json
```

**Output:**
```
Importing theme: corporate
Generating CSS for theme...
✓ CSS generated: /path/to/branding/themes/corporate/theme.css
✓ Theme imported: corporate
```

**Validation:**
- Validates JSON syntax
- Requires `name` field in JSON
- Creates theme directory and CSS

**Function Reference:**
- Implementation: `src/lib/whitelabel/themes.sh::import_theme()`
- Validator: `jq` for JSON validation
- CSS Generator: `generate_theme_css()`

---

### Built-in Themes

#### Light Theme

Clean and bright default theme.

**Colors:**
- Primary: `#0066cc` (Blue)
- Secondary: `#6c757d` (Gray)
- Accent: `#00cc66` (Green)
- Background: `#ffffff` (White)
- Text: `#212529` (Dark gray)

**Use Cases:**
- Default application theme
- Clean, professional interface
- High readability

---

#### Dark Theme

Easy on the eyes, GitHub-inspired dark theme.

**Colors:**
- Primary: `#4a9eff` (Light blue)
- Secondary: `#8b949e` (Gray)
- Accent: `#3fb950` (Green)
- Background: `#0d1117` (Dark blue-black)
- Text: `#c9d1d9` (Light gray)

**Use Cases:**
- Night mode
- Reduced eye strain
- Developer preference

---

#### High Contrast Theme

Maximum contrast for accessibility.

**Colors:**
- Primary: `#ffff00` (Yellow)
- Secondary: `#ffffff` (White)
- Accent: `#00ff00` (Green)
- Background: `#000000` (Black)
- Text: `#ffffff` (White)

**Accessibility Features:**
- Bold borders (2px)
- No rounded corners
- Maximum color contrast
- Larger font sizes
- Heavier font weights

**Use Cases:**
- Visually impaired users
- WCAG AAA compliance
- Maximum readability

---

## Configuration Management

### `nself whitelabel init`

Initialize white-label system.

**Syntax:**
```bash
nself whitelabel init
```

**Example:**
```bash
nself whitelabel init
```

**Output:**
```
Initializing branding system...
✓ Created default branding configuration
✓ Branding system initialized
Initializing email templates system...
✓ Email templates initialized
Initializing custom domains system...
✓ Custom domains system initialized
Initializing theme system...
✓ Theme system initialized
✓ White-label system initialized
```

**Created Structure:**
```
branding/
├── config.json
├── logos/
├── css/
│   └── variables.css
├── fonts/
├── assets/
├── domains/
│   ├── domains.json
│   ├── ssl/
│   └── dns-challenges/
├── email-templates/
│   ├── languages/
│   │   └── en/
│   │       ├── welcome.html
│   │       ├── welcome.txt
│   │       ├── welcome.json
│   │       └── [other templates...]
│   ├── previews/
│   └── VARIABLES.md
└── themes/
    ├── .active
    ├── light/
    ├── dark/
    └── high-contrast/
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::initialize_whitelabel_system()`
- Calls:
  - `initialize_branding_system()`
  - `initialize_email_templates()`
  - `initialize_domains_system()`
  - `initialize_themes_system()`

---

### `nself whitelabel list`

List all white-label resources.

**Syntax:**
```bash
nself whitelabel list
```

**Example:**
```bash
nself whitelabel list
```

**Output:**
```
White-Label Resources
============================================================

Brands:

Branding Preview
============================================================

Brand: My Company
Primary Color: #0066cc
Secondary Color: #ff6600
Primary Font: Inter, system-ui, sans-serif

Logos:
  main: logo-main.png
  icon: logo-icon.png
  email: not set
  favicon: not set

Updated: 2026-01-30T12:34:56Z

Logos:

Configured Logos:

main: logo-main.png
icon: logo-icon.png
email: logo-email.png
favicon: not set
```

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::list_whitelabel_resources()`

---

### `nself whitelabel export`

Export complete white-label configuration.

**Syntax:**
```bash
nself whitelabel export [--format <json|yaml>]
```

**Options:**
- `--format <format>`: Output format (default: `json`)
  - `json`: JSON format
  - `yaml`: YAML format (if supported)

**Example:**
```bash
# Export to file
nself whitelabel export > branding-config.json

# Export with format
nself whitelabel export --format json > config.json
```

**Output:** Complete branding configuration as JSON

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::export_whitelabel_config()`
- Source: `branding/config.json`

**Use Cases:**
- Backup configurations
- Migrate branding between environments
- Version control branding
- Clone branding for new tenants

---

### `nself whitelabel import <file>`

Import white-label configuration.

**Syntax:**
```bash
nself whitelabel import <config-file>
```

**Parameters:**
- `config-file` (required): Path to configuration JSON file

**Example:**
```bash
# Import configuration
nself whitelabel import branding-config.json
```

**Output:**
```
Importing white-label configuration...
Backed up existing config to /path/to/config.json.backup
Generating CSS variables...
✓ CSS variables generated: /path/to/variables.css
✓ Configuration imported successfully
```

**Safety:**
- Creates backup of existing config before import
- Validates JSON format before import
- Regenerates CSS variables after import

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::import_whitelabel_config()`
- Validator: `jq` for JSON validation
- Backup: `config.json.backup`

---

### `nself whitelabel settings`

View current white-label settings.

**Syntax:**
```bash
nself whitelabel settings
```

**Example:**
```bash
nself whitelabel settings
```

**Output:** Same as `nself whitelabel branding preview`

**Function Reference:**
- Implementation: `src/lib/whitelabel/branding.sh::view_whitelabel_settings()`
- Alias for: `preview_branding()`

---

## Error Handling

### Common Error Codes

| Code | Meaning | Common Causes |
|------|---------|---------------|
| `0` | Success | Operation completed successfully |
| `1` | General error | Invalid input, missing file, operation failed |

### Error Messages

**Format:**
```
Error: <description>
```

**Examples:**
```bash
# Invalid domain
Error: Invalid domain format: localhost

# Missing file
Error: Logo file not found: /path/to/logo.png

# Uninitialized system
Error: Branding not initialized. Run 'nself whitelabel init' first.

# Invalid color
Error: Invalid primary color format. Use #RRGGBB

# Missing dependency
Error: jq required for theme import
```

### Error Colors

```bash
RED='\033[0;31m'      # Errors
YELLOW='\033[1;33m'   # Warnings
GREEN='\033[0;32m'    # Success
CYAN='\033[0;36m'     # Info
BLUE='\033[0;34m'     # Headers
```

### Validation Functions

**Domain Validation:**
```bash
validate_domain_format() {
  local domain="$1"
  # Basic domain validation regex
  if [[ "$domain" =~ ^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?\.[a-zA-Z]{2,}$ ]]; then
    return 0
  fi
  return 1
}
```

**Color Validation:**
```bash
validate_hex_color() {
  local color="$1"
  # Match #RGB or #RRGGBB format
  if [[ "$color" =~ ^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$ ]]; then
    return 0
  fi
  return 1
}
```

### Error Handling Best Practices

**1. Check Prerequisites:**
```bash
# Check if system is initialized
if [[ ! -f "$config_file" ]]; then
  printf "${RED}Error: Branding not initialized.${NC}\n" >&2
  printf "Run 'nself whitelabel init' first.\n" >&2
  return 1
fi
```

**2. Validate Input:**
```bash
# Validate required parameters
if [[ $# -eq 0 ]]; then
  printf "${RED}Error: Domain name required${NC}\n" >&2
  return 1
fi
```

**3. Check Dependencies:**
```bash
# Check if jq is available
if ! command -v jq >/dev/null 2>&1; then
  printf "${RED}Error: jq required for this operation${NC}\n" >&2
  return 1
fi
```

**4. Graceful Fallbacks:**
```bash
# Try multiple DNS tools
if command -v dig >/dev/null 2>&1; then
  dig +short "$domain" A
elif command -v nslookup >/dev/null 2>&1; then
  nslookup "$domain"
elif command -v host >/dev/null 2>&1; then
  host "$domain"
else
  printf "${YELLOW}Warning: No DNS tools available${NC}\n" >&2
fi
```

---

## Function Reference

### Core Functions

**Branding System:**

| Function | File | Description |
|----------|------|-------------|
| `initialize_branding_system()` | `branding.sh` | Initialize branding directories and config |
| `create_brand()` | `branding.sh` | Create new brand configuration |
| `set_brand_colors()` | `branding.sh` | Configure brand color palette |
| `set_brand_fonts()` | `branding.sh` | Configure brand typography |
| `upload_brand_logo()` | `branding.sh` | Upload logo file |
| `set_custom_css()` | `branding.sh` | Set custom CSS file |
| `generate_css_variables()` | `branding.sh` | Generate CSS from config |
| `preview_branding()` | `branding.sh` | Display branding preview |

**Domain System:**

| Function | File | Description |
|----------|------|-------------|
| `initialize_domains_system()` | `domains.sh` | Initialize domain system |
| `add_custom_domain()` | `domains.sh` | Add domain to configuration |
| `remove_custom_domain()` | `domains.sh` | Remove domain |
| `verify_domain()` | `domains.sh` | Verify domain ownership |
| `provision_ssl()` | `domains.sh` | Provision SSL certificate |
| `provision_letsencrypt_ssl()` | `domains.sh` | Let's Encrypt SSL |
| `provision_selfsigned_ssl()` | `domains.sh` | Self-signed SSL |
| `check_domain_health()` | `domains.sh` | Health check domain |
| `check_dns_propagation()` | `domains.sh` | Check DNS resolution |

**Email Templates:**

| Function | File | Description |
|----------|------|-------------|
| `initialize_email_templates()` | `email-templates.sh` | Initialize template system |
| `create_default_template()` | `email-templates.sh` | Create template files |
| `list_email_templates()` | `email-templates.sh` | List available templates |
| `edit_email_template()` | `email-templates.sh` | Edit template in editor |
| `preview_email_template()` | `email-templates.sh` | Preview with sample data |
| `test_email_template()` | `email-templates.sh` | Send test email |
| `set_email_language()` | `email-templates.sh` | Set template language |

**Theme System:**

| Function | File | Description |
|----------|------|-------------|
| `initialize_themes_system()` | `themes.sh` | Initialize theme system |
| `create_theme()` | `themes.sh` | Create new theme |
| `create_default_theme()` | `themes.sh` | Create built-in theme |
| `edit_theme()` | `themes.sh` | Edit theme configuration |
| `activate_theme()` | `themes.sh` | Set active theme |
| `preview_theme()` | `themes.sh` | Display theme preview |
| `export_theme()` | `themes.sh` | Export theme JSON |
| `import_theme()` | `themes.sh` | Import theme from file |
| `generate_theme_css()` | `themes.sh` | Generate CSS from theme |

### Helper Functions

**Validation:**

| Function | File | Returns |
|----------|------|---------|
| `validate_domain_format()` | `domains.sh` | `0` if valid domain |
| `validate_hex_color()` | `branding.sh` | `0` if valid hex color |
| `domain_exists()` | `domains.sh` | `0` if domain in config |
| `is_domain_verified()` | `domains.sh` | `0` if domain verified |
| `has_ssl_certificate()` | `domains.sh` | `0` if SSL cert exists |

**Utilities:**

| Function | File | Purpose |
|----------|------|---------|
| `generate_verification_token()` | `domains.sh` | Generate DNS token |
| `update_domain_status()` | `domains.sh` | Update domain field |
| `update_domain_ssl_status()` | `domains.sh` | Update SSL info |
| `check_http_response()` | `domains.sh` | Test HTTP connectivity |
| `get_active_theme()` | `themes.sh` | Get active theme name |

---

## Multi-Tenant Support

### Tenant-Specific Branding

**Create tenant brand:**
```bash
nself whitelabel branding create "Tenant A" tenant-a
nself whitelabel branding create "Tenant B" tenant-b
```

**Tenant configuration structure:**
```json
{
  "brand": {
    "name": "Tenant A",
    "tenantId": "tenant-a"
  },
  "colors": { ... },
  "logos": { ... }
}
```

**Multi-tenant workflow:**
```bash
# 1. Create base branding
nself whitelabel init

# 2. Create tenant-specific brands
nself whitelabel branding create "Enterprise Corp" enterprise-1
nself whitelabel branding set-colors --primary #ff0000 --tenant enterprise-1

# 3. Configure domains per tenant
nself whitelabel domain add enterprise.example.com --tenant enterprise-1

# 4. Tenant-specific themes
nself whitelabel theme create enterprise-dark
nself whitelabel theme activate enterprise-dark --tenant enterprise-1
```

**Future Enhancement:**
Full multi-tenant support with `--tenant` flag across all commands is planned for v0.10.0.

---

## Integration Examples

### Complete Branding Setup

```bash
#!/bin/bash
# Complete white-label setup script

# 1. Initialize system
nself whitelabel init

# 2. Configure brand
nself whitelabel branding create "Acme Corporation"

# 3. Set colors
nself whitelabel branding set-colors \
  --primary #e63946 \
  --secondary #457b9d \
  --accent #2a9d8f \
  --background #f8f9fa \
  --text #212529

# 4. Set fonts
nself whitelabel branding set-fonts \
  --primary "Roboto, system-ui, sans-serif" \
  --secondary "Merriweather, Georgia, serif" \
  --code "Fira Code, monospace"

# 5. Upload logos
nself whitelabel logo upload ./assets/logo-main.svg --type main
nself whitelabel logo upload ./assets/icon-64.png --type icon
nself whitelabel logo upload ./assets/logo-email.png --type email

# 6. Add custom CSS
nself whitelabel branding set-css ./custom-styles.css

# 7. Configure domain
nself whitelabel domain add app.acmecorp.com
nself whitelabel domain verify app.acmecorp.com
nself whitelabel domain ssl app.acmecorp.com --auto-renew

# 8. Customize email templates
nself whitelabel email edit welcome
nself whitelabel email edit password-reset

# 9. Create custom theme
nself whitelabel theme create acme-corporate
nself whitelabel theme edit acme-corporate
nself whitelabel theme activate acme-corporate

# 10. Export configuration for backup
nself whitelabel export > branding-backup.json

echo "✓ White-label configuration complete"
```

### Multi-Language Email Setup

```bash
#!/bin/bash
# Setup multi-language email templates

# English (default)
nself whitelabel email edit welcome en
nself whitelabel email edit password-reset en

# Spanish
nself whitelabel email set-language es
nself whitelabel email edit welcome es
nself whitelabel email edit password-reset es

# French
nself whitelabel email set-language fr
nself whitelabel email edit welcome fr
nself whitelabel email edit password-reset fr

# Preview templates
nself whitelabel email preview welcome en
nself whitelabel email preview welcome es
nself whitelabel email preview welcome fr
```

### Domain Health Monitoring

```bash
#!/bin/bash
# Domain health check script

domains=(
  "app.example.com"
  "api.example.com"
  "admin.example.com"
)

for domain in "${domains[@]}"; do
  echo "Checking $domain..."
  nself whitelabel domain health "$domain"
  echo ""
done
```

---

## Changelog

### Version 0.9.0 (Sprint 14)

**Added:**
- Complete white-label system
- Branding customization (colors, fonts, logos)
- Custom domain management
- SSL certificate provisioning (Let's Encrypt, self-signed)
- Email template system with variables
- Multi-language email support
- Theme system (light, dark, high-contrast)
- Custom theme creation
- Configuration import/export
- Domain health monitoring
- DNS verification system

**Files:**
- `src/cli/whitelabel.sh` - CLI interface
- `src/lib/whitelabel/branding.sh` - Branding system
- `src/lib/whitelabel/domains.sh` - Domain management
- `src/lib/whitelabel/email-templates.sh` - Email templates
- `src/lib/whitelabel/themes.sh` - Theme system

---

## See Also

- [White-Label Configuration Guide](../../guides/WHITE-LABEL-CUSTOMIZATION.md)
- [Email Templates Guide](../../guides/EMAIL-TEMPLATES.md)
- [Theme Customization](../../guides/THEMES.md)
- [SSL Configuration](../../configuration/SSL.md)
- [DNS Setup](../../guides/domain-selection-guide.md)

---

**Last Updated:** January 30, 2026
**nself Version:** 0.9.0
**Sprint:** 14 - White-Label & Customization
