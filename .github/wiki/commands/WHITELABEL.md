# nself tenant - White-Label & Customization Management

**Version 0.9.0** | Complete white-label solution for branding, customization, and multi-tenant configurations

---

## Overview

> **Note:** All white-label commands are now part of the `nself tenant` command family. These commands enable multi-tenant branding and customization.

The white-label command system enables you to completely rebrand the nself platform with your own company identity. Whether you're building a white-label SaaS product, reselling the platform, or running a multi-tenant system, the white-label tools provide comprehensive control over:

- **Branding**: Logo, colors, fonts, and custom styling
- **Domains**: Custom domains with automatic SSL provisioning
- **Email Templates**: Branded email communications with multi-language support
- **Themes**: Pre-built and custom themes for consistent UI
- **Multi-Tenant**: Complete brand isolation per tenant/customer

---

## Quick Start

### Minimal White-Label (5 minutes)

```bash
# 1. Enable white-label mode
echo "WHITELABEL_ENABLED=true" >> .env
echo "WHITELABEL_TIER=basic" >> .env

# 2. Set your brand name and logo
nself tenant branding create "My Company"
nself tenant logo upload ./logo.png --type main

# 3. Apply brand colors
nself tenant branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600"

# 4. Rebuild and restart
nself build && nself start
```

### Professional Setup (15 minutes)

```bash
# 1. Initialize white-label system
nself tenant init

# 2. Create brand with all assets
nself tenant branding create "Acme Platform"
nself tenant logo upload ./logo.svg --type main
nself tenant logo upload ./icon.png --type icon
nself tenant logo upload ./email-logo.png --type email

# 3. Add custom domain
nself tenant domains add app.acmeplatform.com

# 4. Verify domain ownership
nself tenant domains verify app.acmeplatform.com

# 5. Provision SSL certificate
nself tenant domains ssl app.acmeplatform.com --auto-renew

# 6. Customize emails
nself tenant email edit welcome
nself tenant email test welcome admin@example.com

# 7. Create custom theme
nself tenant theme create "corporate"
nself tenant theme activate "corporate"

# 8. Deploy
nself build && nself start
```

---

## Command Reference

### Branding Commands

Commands for managing logos, colors, fonts, and visual branding.

#### `nself tenant branding create <brand-name>`

Create a new brand configuration.

```bash
# Create basic brand
nself tenant branding create "My Company"

# View result
nself tenant settings
```

**Parameters:**
- `<brand-name>`: Brand display name (alphanumeric, spaces, hyphens allowed)

**Outputs:**
- Creates brand database entry
- Initializes default colors and fonts
- Ready for customization

---

#### `nself tenant branding set-colors`

Set primary and secondary brand colors.

```bash
# Set colors
nself tenant branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600"

# Set with accent colors
nself tenant branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600" \
  --accent "#00cc66" \
  --background "#ffffff" \
  --text "#333333"

# For multi-tenant setup
nself tenant branding set-colors \
  --primary "#ff0000" \
  --tenant "tenant-a-id"
```

**Parameters:**
- `--primary`: Primary brand color (hex format: #RRGGBB)
- `--secondary`: Secondary brand color (hex format: #RRGGBB)
- `--accent`: Accent color (optional)
- `--background`: Background color (optional)
- `--text`: Text color (optional)
- `--tenant`: Tenant ID for multi-tenant setup (optional)

**Color Usage:**
- **Primary**: Buttons, links, highlights, CTAs
- **Secondary**: Secondary buttons, accents
- **Accent**: Alert boxes, badges, highlights
- **Background**: Main page background
- **Text**: Primary text color

**Best Practices:**
- Use high contrast between text and background (WCAG AA minimum)
- Test colors in both light and dark modes
- Consider colorblind accessibility (avoid red/green only combinations)
- Use tools like WebAIM Contrast Checker to verify

---

#### `nself tenant branding set-fonts`

Configure typography for your brand.

```bash
# Set fonts
nself tenant branding set-fonts \
  --primary "Inter" \
  --secondary "Roboto"

# Set with multiple fonts
nself tenant branding set-fonts \
  --primary "Inter" \
  --secondary "Georgia" \
  --monospace "Fira Code"

# For specific tenant
nself tenant branding set-fonts \
  --primary "Helvetica" \
  --tenant "tenant-b-id"
```

**Parameters:**
- `--primary`: Primary font family (body text, general UI)
- `--secondary`: Secondary font family (headings, emphasis)
- `--monospace`: Monospace font (code blocks, technical content)
- `--tenant`: Tenant ID for multi-tenant setup (optional)

**Popular Font Choices:**
- **Modern**: Inter, Poppins, Roboto Flex
- **Corporate**: Helvetica, Proxima Nova, Montserrat
- **Elegant**: Georgia, Playfair Display, Libre Baskerville
- **Technical**: Fira Code, Source Code Pro, IBM Plex Mono

**Google Fonts Integration:**
Fonts are automatically loaded from Google Fonts if available. Custom font URLs can be configured in environment variables.

---

#### `nself tenant branding upload-logo <path>`

Upload brand logo. Alias for `nself tenant logo upload`.

```bash
nself tenant branding upload-logo ./logo.png
```

See [Logo Commands](#logo-commands) section for full details.

---

#### `nself tenant branding set-css <path-to-custom.css>`

Apply custom CSS stylesheet to override default styles.

```bash
# Apply custom CSS
nself tenant branding set-css ./custom-brand.css

# View current CSS
nself tenant branding preview
```

**CSS Best Practices:**

```css
/* Use CSS variables for maintainability */
:root {
  --primary-color: #0066cc;
  --secondary-color: #ff6600;
  --font-primary: 'Inter', sans-serif;
  --font-secondary: 'Roboto', sans-serif;
  --border-radius: 8px;
  --shadow-sm: 0 1px 3px rgba(0,0,0,0.1);
  --shadow-lg: 0 10px 25px rgba(0,0,0,0.15);
}

/* Override component styles */
.nself-button {
  background-color: var(--primary-color);
  border-radius: var(--border-radius);
  box-shadow: var(--shadow-sm);
  transition: all 0.3s ease;
}

.nself-button:hover {
  box-shadow: var(--shadow-lg);
}

.nself-card {
  border-radius: var(--border-radius);
  box-shadow: var(--shadow-sm);
}

/* Dark mode support */
@media (prefers-color-scheme: dark) {
  :root {
    --background-color: #1a1a1a;
    --text-color: #f8f9fa;
  }
}
```

**File Requirements:**
- Maximum 2MB
- Valid CSS syntax
- No JavaScript or embedded images
- Scoped selectors to avoid breaking core functionality

**Testing:**
- Preview before deploying: `nself tenant branding preview`
- Test in light and dark modes
- Verify on mobile devices
- Check browser compatibility

---

#### `nself tenant branding preview`

Preview branding changes in the browser.

```bash
# Open preview in browser
nself tenant branding preview

# Show preview URL only
nself tenant branding preview --url-only
```

**What's Shown:**
- Brand colors applied to components
- Logo rendering in different contexts
- Font rendering and readability
- Custom CSS applied
- Light and dark mode preview

---

### Domain Commands

Configure custom domains with automatic DNS verification and SSL provisioning.

#### `nself tenant domains add <domain>`

Add a custom domain for your application.

```bash
# Add primary domain
nself tenant domains add app.acmeplatform.com

# Add subdomain
nself tenant domains add api.acmeplatform.com

# Add multiple domains
nself tenant domains add app.acmeplatform.com
nself tenant domains add api.acmeplatform.com
nself tenant domains add admin.acmeplatform.com

# For multi-tenant
nself tenant domains add tenant-a.app.com --tenant "tenant-a-id"
```

**Parameters:**
- `<domain>`: Fully qualified domain name (FQDN)
- `--tenant`: Tenant ID for multi-tenant setup (optional)
- `--primary`: Set as primary domain (optional)

**Validation:**
- Domain must be valid FQDN format
- Cannot contain spaces or special characters (except hyphens and dots)
- Must not already exist in system

**Next Steps:**
1. Verify domain ownership: `nself tenant domains verify <domain>`
2. Provision SSL: `nself tenant domains ssl <domain>`

---

#### `nself tenant domains verify <domain>`

Verify domain ownership via DNS validation.

```bash
# Start verification process
nself tenant domains verify app.acmeplatform.com

# This returns a verification token and instructions

# After adding DNS record, verify:
nself tenant domains verify app.acmeplatform.com --force
```

**DNS Verification Process:**

1. **Get verification token:**
   ```bash
   nself tenant domains verify app.acmeplatform.com
   # Output: Add TXT record to DNS:
   # Name: _nself-verify.app.acmeplatform.com
   # Value: nself-verification-token-abc123xyz
   ```

2. **Add to DNS provider** (GoDaddy, Namecheap, Route 53, etc.):
   - Log into DNS provider
   - Add TXT record with name `_nself-verify.app.acmeplatform.com`
   - Set value to verification token
   - Save changes

3. **Verify completion:**
   ```bash
   nself tenant domains verify app.acmeplatform.com
   # May take up to 48 hours for DNS to propagate
   ```

**Troubleshooting:**
```bash
# Check DNS propagation
dig _nself-verify.app.acmeplatform.com TXT

# Or using online tool
nslookup _nself-verify.app.acmeplatform.com

# If verification fails after DNS is updated
nself tenant domains verify app.acmeplatform.com --force
```

---

#### `nself tenant domains ssl <domain> [--auto-renew]`

Provision and manage SSL certificates via Let's Encrypt.

```bash
# Provision SSL certificate
nself tenant domains ssl app.acmeplatform.com

# With auto-renewal enabled
nself tenant domains ssl app.acmeplatform.com --auto-renew

# Check certificate status
nself tenant domains ssl app.acmeplatform.com --status

# Renew certificate manually
nself tenant domains ssl app.acmeplatform.com --renew

# Use custom certificate instead
nself tenant domains ssl app.acmeplatform.com --cert-path /path/to/cert.pem --key-path /path/to/key.pem
```

**Parameters:**
- `<domain>`: Domain to provision SSL for
- `--auto-renew`: Enable automatic renewal (recommended)
- `--status`: Check certificate status
- `--renew`: Manually renew certificate
- `--cert-path`: Path to custom certificate file
- `--key-path`: Path to custom private key
- `--provider`: SSL provider (letsencrypt, custom)

**Requirements:**
- Domain must be verified first (`nself tenant domains verify`)
- Domain must point to your server (DNS A/CNAME record)
- Ports 80 and 443 must be accessible from internet

**Auto-Renewal:**
- Enabled by default with `--auto-renew`
- Certificates renewed 30 days before expiration
- Renewal checks run daily automatically
- Recommended for production environments

**Custom Certificates:**
```bash
# Upload existing certificate
nself tenant domains ssl app.acmeplatform.com \
  --cert-path /path/to/cert.pem \
  --key-path /path/to/key.pem
```

---

#### `nself tenant domains health <domain>`

Check custom domain health and SSL status.

```bash
# Full health check
nself tenant domains health app.acmeplatform.com

# Output includes:
# - DNS resolution status
# - IP address verification
# - SSL certificate validity
# - Certificate expiration date
# - HTTP/HTTPS accessibility
# - Redirect status
```

**Health Check Details:**

| Check | What It Verifies |
|-------|------------------|
| DNS Resolution | Domain resolves to correct IP |
| A/CNAME Records | DNS records point to server |
| SSL Certificate | Certificate is valid and non-expired |
| Certificate Chain | Full certificate chain is valid |
| HTTP Redirect | HTTP redirects to HTTPS |
| HTTPS Connection | HTTPS connection works |
| TLS Version | Modern TLS version (1.2+) |
| Certificate Age | Days until expiration |

**Output Example:**
```
Domain Health Report: app.acmeplatform.com
============================================
DNS Resolution:      ✓ PASS (1.2.3.4)
A Record:            ✓ PASS
SSL Certificate:     ✓ PASS (expires in 89 days)
Certificate Chain:   ✓ PASS
HTTP Redirect:       ✓ PASS
HTTPS Connection:    ✓ PASS
TLS Version:         ✓ PASS (1.3)
Overall Status:      ✓ HEALTHY
```

**Scheduled Health Checks:**
- Automatically run daily for all domains
- Alerts generated 30 days before certificate expiration
- Critical alerts 7 days before expiration

---

#### `nself tenant domains remove <domain>`

Remove a custom domain from the system.

```bash
# Remove domain
nself tenant domains remove app.acmeplatform.com

# Confirm removal when prompted
# Domain will be removed from routing after next deployment
```

**Warning:**
- Removes domain from all services
- Existing SSL certificates become invalid
- Requires redeployment to take effect: `nself build && nself start`
- Cannot be undone - DNS will be unreachable

---

### Email Commands

Customize email templates with brand identity, multi-language support, and testing.

#### `nself tenant email list`

List all available email templates.

```bash
# List templates
nself tenant email list

# Output shows:
# Template Name      Description             Default Language
# ─────────────────────────────────────────────────────────
# welcome           Welcome new users       en
# verify-email      Email verification      en
# password-reset    Password reset          en
# invitation        Team invitation         en
# notification      General notifications  en
# receipt           Payment receipts        en
# report            Periodic reports        en
```

**Available Templates:**

| Template | Purpose | Variables |
|----------|---------|-----------|
| `welcome` | New user welcome email | user_name, brand_name, action_url |
| `verify-email` | Email verification | user_email, verification_url, expires_in |
| `password-reset` | Password reset instructions | user_name, reset_url, expires_in |
| `invitation` | Team/tenant invitation | user_name, inviter_name, action_url |
| `notification` | General notifications | title, message, action_url |
| `receipt` | Payment/order receipts | user_name, amount, order_id |
| `report` | Periodic reports | report_type, period, data |

---

#### `nself tenant email edit <template-name>`

Edit an email template with your brand messaging.

```bash
# Open template in editor
nself tenant email edit welcome

# After saving, preview the changes
nself tenant email preview welcome

# Test with real email
nself tenant email test welcome admin@example.com
```

**Template Variables:**

Templates support the following variables that get replaced at send time:

```html
<!-- User Information -->
{{user_name}}           <!-- User's display name -->
{{user_email}}          <!-- User's email address -->
{{user_id}}             <!-- User's unique ID -->

<!-- Brand Information -->
{{brand_name}}          <!-- Your brand name -->
{{brand_logo}}          <!-- Brand logo URL -->
{{brand_color}}         <!-- Primary brand color -->
{{support_email}}       <!-- Support email address -->
{{company_address}}     <!-- Company physical address -->

<!-- Action URLs -->
{{action_url}}          <!-- Main call-to-action URL -->
{{confirmation_url}}    <!-- Confirmation/verification URL -->
{{reset_url}}           <!-- Password reset URL -->
{{unsubscribe_url}}     <!-- Email unsubscribe link -->

<!-- Dynamic Content -->
{{title}}               <!-- Email title/subject -->
{{message}}             <!-- Email body content -->
{{footer_text}}         <!-- Email footer -->
{{current_year}}        <!-- Current year (for copyright) -->

<!-- Transaction-specific -->
{{order_id}}            <!-- Order/transaction ID -->
{{amount}}              <!-- Transaction amount -->
{{currency}}            <!-- Currency code (USD, EUR, etc.) -->
```

**Template Structure:**

Email templates use HTML and support inline CSS:

```html
<!DOCTYPE html>
<html>
<head>
  <style>
    body {
      font-family: {{font_family}};
      color: {{text_color}};
      background-color: {{background_color}};
    }
    a { color: {{brand_color}}; }
  </style>
</head>
<body>
  <div style="max-width: 600px; margin: 0 auto;">
    <!-- Header -->
    <div style="background-color: {{brand_color}}; padding: 20px; text-align: center;">
      <img src="{{brand_logo}}" alt="{{brand_name}}" style="height: 50px;" />
    </div>

    <!-- Content -->
    <div style="padding: 20px;">
      <h1>Welcome, {{user_name}}!</h1>
      <p>Welcome to {{brand_name}}.</p>
      <a href="{{action_url}}" style="background-color: {{brand_color}}; color: white; padding: 10px 20px; text-decoration: none;">
        Get Started
      </a>
    </div>

    <!-- Footer -->
    <div style="border-top: 1px solid #ddd; padding: 20px; font-size: 12px; color: #999;">
      <p>{{footer_text}}</p>
      <p>
        <a href="https://example.com/unsubscribe">Unsubscribe</a> |
        <a href="https://example.com/preferences">Preferences</a>
      </p>
    </div>
  </div>
</body>
</html>
```

**Best Practices:**
- Use responsive design (tested on mobile)
- Ensure sufficient color contrast
- Include unsubscribe link for legal compliance
- Test rendering in major email clients
- Keep CSS inline (not all clients support stylesheets)
- Use background images sparingly (many clients block them)

---

#### `nself tenant email preview <template-name>`

Preview email template in browser before sending.

```bash
# Preview template
nself tenant email preview welcome

# Open in browser with sample data
nself tenant email preview welcome --browser

# Export HTML for external testing
nself tenant email preview welcome --export > welcome.html
```

**Preview Features:**
- Light and dark mode preview
- Mobile responsive preview
- Variable substitution with sample data
- Email client compatibility warnings
- Accessibility checks

---

#### `nself tenant email test <template-name> <email>`

Send a test email to verify template rendering and branding.

```bash
# Send test welcome email
nself tenant email test welcome admin@example.com

# Test with custom data
nself tenant email test welcome admin@example.com \
  --name "John Doe" \
  --company "Acme Corp"

# Send to multiple addresses
nself tenant email test welcome admin@example.com support@example.com

# Test all templates at once
nself tenant email test all admin@example.com
```

**What Gets Tested:**
- Email delivery (SMTP connectivity)
- Template rendering (HTML/text)
- Variable substitution
- Image loading
- Link validity
- Email client compatibility

**Troubleshooting Test Failures:**
```bash
# Check email service status
nself status mailpit

# Verify SMTP configuration
nself email check

# Check email logs
nself logs mailpit
```

---

#### `nself tenant email set-language <lang-code>`

Set default language for email templates.

```bash
# Set to Spanish
nself tenant email set-language es

# Set to French
nself tenant email set-language fr

# View available languages
nself tenant email set-language --list
```

**Supported Languages:**

| Code | Language |
|------|----------|
| `en` | English (default) |
| `es` | Spanish (Español) |
| `fr` | French (Français) |
| `de` | German (Deutsch) |
| `ja` | Japanese (日本語) |
| `zh` | Chinese Simplified (简体中文) |
| `pt` | Portuguese (Português) |
| `ru` | Russian (Русский) |

**Multi-Language Support:**
- Each template can have versions in multiple languages
- Language determined by user preference or locale
- Falls back to English if language not available
- Administrators can enable language selection in UI

---

### Theme Commands

Create and manage visual themes for consistent branding across the platform.

#### `nself tenant theme create <theme-name>`

Create a new theme from scratch or template.

```bash
# Create blank theme
nself tenant theme create "my-theme"

# Create from template
nself tenant theme create "dark-mode" --template "dark"

# Create for specific tenant
nself tenant theme create "tenant-theme" --tenant "tenant-id"
```

**Parameters:**
- `<theme-name>`: Unique theme name (lowercase, hyphens allowed)
- `--template`: Base template (default, dark, minimal, corporate)
- `--tenant`: Tenant ID for multi-tenant setup
- `--description`: Theme description

**Theme Structure:**

Themes define the visual design system:

```json
{
  "name": "corporate",
  "version": "1.0.0",
  "description": "Professional corporate theme",

  "colors": {
    "primary": "#0066cc",
    "secondary": "#ff6600",
    "accent": "#00cc66",
    "background": "#ffffff",
    "surface": "#f5f5f5",
    "text": "#333333",
    "textLight": "#666666",
    "border": "#e0e0e0",
    "success": "#28a745",
    "warning": "#ffc107",
    "error": "#dc3545",
    "info": "#17a2b8"
  },

  "typography": {
    "fontFamily": {
      "primary": "Inter",
      "secondary": "Roboto",
      "monospace": "Fira Code"
    },
    "fontSize": {
      "xs": "0.75rem",
      "sm": "0.875rem",
      "base": "1rem",
      "lg": "1.125rem",
      "xl": "1.25rem",
      "2xl": "1.5rem"
    },
    "fontWeight": {
      "light": 300,
      "normal": 400,
      "medium": 500,
      "semibold": 600,
      "bold": 700
    }
  },

  "spacing": {
    "unit": "8px",
    "scale": [0, 4, 8, 12, 16, 24, 32, 48, 64]
  },

  "borders": {
    "radius": {
      "sm": "4px",
      "md": "8px",
      "lg": "12px",
      "full": "9999px"
    },
    "width": {
      "thin": "1px",
      "normal": "2px",
      "thick": "4px"
    }
  },

  "shadows": {
    "sm": "0 1px 3px rgba(0,0,0,0.1)",
    "md": "0 4px 12px rgba(0,0,0,0.15)",
    "lg": "0 10px 25px rgba(0,0,0,0.2)"
  }
}
```

---

#### `nself tenant theme edit <theme-name>`

Edit theme configuration and design tokens.

```bash
# Open theme editor
nself tenant theme edit "corporate"

# Edit specific property
nself tenant theme edit "corporate" \
  --colors.primary="#ff0000" \
  --typography.fontSize.base="16px"

# Edit via JSON file
nself tenant theme edit "corporate" --file theme.json
```

**Editable Properties:**
- Colors (primary, secondary, accent, states)
- Typography (fonts, sizes, weights)
- Spacing (unit, scale)
- Borders (radius, widths)
- Shadows (elevation system)
- Animations (transitions, durations)

---

#### `nself tenant theme activate <theme-name>`

Activate a theme for the application.

```bash
# Activate theme
nself tenant theme activate "corporate"

# Activate for specific brand
nself tenant theme activate "corporate" --brand "Acme"

# Activate for specific tenant
nself tenant theme activate "corporate" --tenant "tenant-id"

# View current active theme
nself tenant theme activate --current
```

**Deployment:**
- Takes effect immediately (no restart needed)
- Theme changes apply to all active users
- Previous theme settings preserved for rollback

---

#### `nself tenant theme preview <theme-name>`

Preview theme before activating.

```bash
# Preview theme in browser
nself tenant theme preview "corporate"

# Preview specific components
nself tenant theme preview "corporate" --components buttons,cards,forms

# Export preview HTML
nself tenant theme preview "corporate" --export > preview.html
```

**Preview Components:**
- Buttons and states
- Cards and containers
- Forms and inputs
- Navigation and menus
- Alerts and notifications
- Data tables
- Modals and dialogs
- Typography scales
- Color palette
- Spacing grid

---

#### `nself tenant theme export <theme-name>`

Export theme configuration as JSON file.

```bash
# Export theme
nself tenant theme export "corporate" > corporate.json

# Export for backup
nself tenant theme export "corporate" \
  --output ./backups/corporate-v1.json

# Export all themes
nself tenant theme export all --output ./themes/
```

**Export Contents:**
- Complete theme configuration
- All design tokens
- Metadata and versioning
- Creation/modification timestamps

---

#### `nself tenant theme import <path-to-theme.json>`

Import theme from file (created by export or external source).

```bash
# Import theme
nself tenant theme import ./corporate.json

# Import and activate immediately
nself tenant theme import ./corporate.json --activate

# Import for specific tenant
nself tenant theme import ./theme.json --tenant "tenant-id"
```

**Import Validation:**
- Validates JSON schema
- Checks color formats
- Verifies typography properties
- Prevents name conflicts

---

### Logo Commands

Upload and manage logos for different use cases and contexts.

#### `nself tenant logo upload <path> [--type main|icon|email]`

Upload a logo file for your brand.

```bash
# Upload main logo (horizontal)
nself tenant logo upload ./logo.svg --type main

# Upload icon logo (square)
nself tenant logo upload ./icon.png --type icon

# Upload email logo (optimized for email clients)
nself tenant logo upload ./email-logo.png --type email

# Upload dark mode variant
nself tenant logo upload ./logo-dark.svg --type dark

# Upload favicon
nself tenant logo upload ./favicon.ico --type favicon
```

**Parameters:**
- `<path>`: Path to logo file
- `--type`: Logo type - determines usage context
  - `main`: Primary logo (header, dashboard)
  - `dark`: Dark mode variant
  - `icon`: Square icon (favicon, mobile)
  - `email`: Email header logo
  - `favicon`: Browser favicon

**Logo Types and Recommendations:**

| Type | Purpose | Format | Size | Notes |
|------|---------|--------|------|-------|
| main | Header/primary branding | SVG, PNG | 300x100px+ | Horizontal orientation |
| dark | Dark background use | SVG, PNG | 300x100px+ | Transparent background |
| icon | Icon/favicon | PNG, ICO | 512x512px | Square format |
| email | Email headers | PNG, JPG | 600x200px | No SVG (email limitations) |
| favicon | Browser tab | ICO, PNG | 32x32, 64x64 | Multiple sizes recommended |

**File Requirements:**
- Maximum 5MB
- Supported formats: PNG, JPG, JPEG, SVG, WebP, ICO
- Recommended: SVG for scalability (all except email)
- Email logos: PNG/JPG only (many email clients block SVG)

**Best Practices:**
- Ensure logo is recognizable at small sizes (favicon)
- Use transparent backgrounds for flexibility
- Create dark variant for light-on-dark usage
- Optimize file size (SVGO for SVG, compression for PNG)
- Test across different backgrounds

---

#### `nself tenant logo list`

List all uploaded logos with metadata.

```bash
# List all logos
nself tenant logo list

# Output shows:
# ID                    Type      File Size  Uploaded      Status
# ──────────────────────────────────────────────────────────────
# logo-001              main      15.2KB     2025-01-30    Active
# logo-002              icon      28.5KB     2025-01-30    Active
# logo-003              email     42.1KB     2025-01-28    Active

# Export as JSON
nself tenant logo list --format json
```

**Information Shown:**
- Logo ID (unique identifier)
- Logo type
- File size
- Upload date/time
- Current usage status
- CDN URL

---

#### `nself tenant logo remove <logo-id>`

Remove a logo from the system.

```bash
# Remove specific logo
nself tenant logo remove "logo-001"

# Remove all logos (dangerous!)
nself tenant logo remove all --force
```

**Warning:**
- Cannot be undone
- Deletes from storage and CDN
- May break branding if still in use
- Check usage before removing: `nself tenant settings`

---

### Settings Commands

View and manage white-label system configuration.

#### `nself tenant settings`

View current white-label settings and configuration.

```bash
# View all settings
nself tenant settings

# View as JSON
nself tenant settings --format json

# View specific section
nself tenant settings --section branding
nself tenant settings --section domains
nself tenant settings --section email
nself tenant settings --section themes

# For specific tenant
nself tenant settings --tenant "tenant-id"
```

**Settings Sections:**

**Branding:**
- Brand name, tagline, description
- Colors (primary, secondary, accent)
- Typography (fonts)
- Logos (all types)
- Custom CSS
- Active theme

**Domains:**
- Primary domain
- Custom domains list
- SSL status for each domain
- Verification status
- Auto-renewal settings

**Email:**
- Sender name and email
- Default language
- Available templates
- Template customizations
- Footer text

**Themes:**
- Active theme name
- All available themes
- Theme versions
- Last modified date

**Social & Links:**
- Support URL
- Documentation URL
- Terms of service
- Privacy policy
- Social media profiles

---

### List Commands

List white-label resources (brands, themes, domains).

#### `nself tenant list`

List all white-label resources in the system.

```bash
# List all resources
nself tenant list

# List specific resource type
nself tenant list --type brands
nself tenant list --type themes
nself tenant list --type domains
nself tenant list --type email-templates

# List for specific tenant
nself tenant list --tenant "tenant-id"

# List with details
nself tenant list --verbose

# Export as JSON
nself tenant list --format json > resources.json
```

**Resource Types:**
- **brands**: All brand configurations
- **themes**: All available themes
- **domains**: All custom domains
- **email-templates**: All customized email templates
- **logos**: All uploaded logos
- **assets**: All branding assets

---

### System Commands

#### `nself tenant init`

Initialize white-label system (first-time setup).

```bash
# Initialize white-label system
nself tenant init

# Interactive setup wizard
nself tenant init --interactive

# Initialize with demo data
nself tenant init --demo
```

**What Gets Initialized:**
- Database schema for white-label tables
- Directory structure for assets
- Default brand configuration
- Sample themes
- Email templates
- Default colors and fonts

**First Time Setup:**
1. Run `nself tenant init`
2. Create your brand: `nself tenant branding create "Your Brand"`
3. Upload logos: `nself tenant logo upload ./logo.png`
4. Set colors: `nself tenant branding set-colors --primary ... --secondary ...`
5. Add domain: `nself tenant domains add your-domain.com`
6. Verify domain: `nself tenant domains verify your-domain.com`
7. Provision SSL: `nself tenant domains ssl your-domain.com --auto-renew`

---

#### `nself tenant export [--format json|yaml]`

Export entire branding configuration for backup or migration.

```bash
# Export as JSON
nself tenant export --format json > branding-backup.json

# Export as YAML
nself tenant export --format yaml > branding-backup.yaml

# Export specific brand
nself tenant export --brand "Acme" --format json

# Export for specific tenant
nself tenant export --tenant "tenant-id" > tenant-branding.json
```

**Export Contents:**
- All brands and configurations
- All themes and theme versions
- All domains and SSL certificates
- Email template customizations
- Logo references and metadata
- Settings and preferences

**Use Cases:**
- Backup before making changes
- Migrate branding to another environment
- Share configuration with team
- Version control branding
- Reuse configuration as template

---

#### `nself tenant import <path>`

Import branding configuration from backup or template.

```bash
# Import from JSON file
nself tenant import ./branding-backup.json

# Import with conflict handling
nself tenant import ./branding-backup.json --on-conflict merge

# Import for specific tenant
nself tenant import ./tenant-branding.json --tenant "tenant-id"

# Validate before importing
nself tenant import ./branding-backup.json --validate
```

**Parameters:**
- `<path>`: Path to export file (JSON or YAML)
- `--on-conflict`: How to handle conflicts (overwrite, merge, skip)
- `--tenant`: Tenant ID for multi-tenant import
- `--validate`: Validate file without importing

**Import Behavior:**
- Validates file format and schema
- Creates missing resources
- Updates existing resources by default
- Preserves timestamps and metadata
- Logs all changes

---

## Environment Variables Reference

Configure white-label system behavior via environment variables.

### Feature Toggle

```bash
# Enable/disable white-label mode
WHITELABEL_ENABLED=true

# White-label tier (basic, professional, enterprise)
WHITELABEL_TIER=professional
```

### Brand Identity

```bash
# Brand display name
WHITELABEL_BRAND_NAME=Acme Platform

# Company legal name
WHITELABEL_COMPANY_NAME="Acme Corporation Inc."

# Brand tagline/slogan
WHITELABEL_TAGLINE="The Modern Application Platform"

# Support contacts
WHITELABEL_SUPPORT_EMAIL=support@acmeplatform.com
WHITELABEL_SALES_EMAIL=sales@acmeplatform.com

# Company website
WHITELABEL_WEBSITE_URL=https://acmeplatform.com
```

### Logos & Assets

```bash
# Logo URLs (must be publicly accessible)
WHITELABEL_LOGO_URL=https://cdn.acmeplatform.com/logo.svg
WHITELABEL_LOGO_DARK_URL=https://cdn.acmeplatform.com/logo-dark.svg
WHITELABEL_ICON_URL=https://cdn.acmeplatform.com/icon.svg
WHITELABEL_FAVICON_URL=https://cdn.acmeplatform.com/favicon.ico
WHITELABEL_EMAIL_LOGO_URL=https://cdn.acmeplatform.com/email-logo.png

# Social preview image
WHITELABEL_OG_IMAGE_URL=https://cdn.acmeplatform.com/og-image.jpg
```

### Colors

```bash
# Primary and secondary colors
WHITELABEL_PRIMARY_COLOR=#007bff
WHITELABEL_SECONDARY_COLOR=#6c757d

# Semantic colors
WHITELABEL_SUCCESS_COLOR=#28a745
WHITELABEL_WARNING_COLOR=#ffc107
WHITELABEL_ERROR_COLOR=#dc3545
WHITELABEL_INFO_COLOR=#17a2b8

# Theme colors
WHITELABEL_BACKGROUND_COLOR=#ffffff
WHITELABEL_BACKGROUND_DARK_COLOR=#1a1a1a
WHITELABEL_TEXT_COLOR=#212529
WHITELABEL_TEXT_DARK_COLOR=#f8f9fa
WHITELABEL_BORDER_COLOR=#dee2e6
```

### Typography

```bash
# Font families
WHITELABEL_FONT_FAMILY=Inter
WHITELABEL_HEADING_FONT_FAMILY=Montserrat

# Google Fonts (if using)
WHITELABEL_GOOGLE_FONTS_API_KEY=your-api-key

# Custom font URL
WHITELABEL_CUSTOM_FONT_URL=https://cdn.acmeplatform.com/fonts.css
```

### Custom Domains

```bash
# Primary domain
WHITELABEL_DOMAIN=app.acmeplatform.com

# Service-specific domains
WHITELABEL_API_DOMAIN=api.acmeplatform.com
WHITELABEL_AUTH_DOMAIN=auth.acmeplatform.com
WHITELABEL_ADMIN_DOMAIN=admin.acmeplatform.com
WHITELABEL_STORAGE_DOMAIN=cdn.acmeplatform.com

# SSL configuration
WHITELABEL_SSL_MODE=auto  # auto, manual, letsencrypt, custom
WHITELABEL_SSL_CERT_PATH=/path/to/cert.pem
WHITELABEL_SSL_KEY_PATH=/path/to/key.pem
```

### Email Configuration

```bash
# Enable custom email templates
WHITELABEL_CUSTOM_EMAIL_TEMPLATES=true
WHITELABEL_EMAIL_TEMPLATE_DIR=/path/to/email-templates

# Email sender info
WHITELABEL_EMAIL_FROM_NAME="Acme Platform"
WHITELABEL_EMAIL_FROM_ADDRESS=noreply@acmeplatform.com
WHITELABEL_EMAIL_FOOTER="© 2025 Acme Corporation Inc."

# Email styling
WHITELABEL_EMAIL_HEADER_BG_COLOR=#007bff
WHITELABEL_EMAIL_HEADER_TEXT_COLOR=#ffffff
WHITELABEL_EMAIL_BUTTON_COLOR=#007bff

# Social media in emails
WHITELABEL_EMAIL_SOCIAL_TWITTER=https://twitter.com/acmeplatform
WHITELABEL_EMAIL_SOCIAL_LINKEDIN=https://linkedin.com/company/acmeplatform
```

### Themes & Styling

```bash
# Custom theme
WHITELABEL_THEME=corporate

# Custom CSS
WHITELABEL_CUSTOM_CSS_ENABLED=true
WHITELABEL_CUSTOM_CSS_URL=https://cdn.acmeplatform.com/custom.css

# User theme control
WHITELABEL_ALLOW_USER_THEMING=false
WHITELABEL_DEFAULT_THEME_MODE=auto  # light, dark, auto
WHITELABEL_ALLOW_THEME_TOGGLE=true
```

### Branding Control

```bash
# Show/hide nself attribution
WHITELABEL_SHOW_NSELF_BRANDING=false

# Custom messaging
WHITELABEL_WELCOME_MESSAGE="Welcome to Acme Platform"
WHITELABEL_FOOTER_TEXT="© 2025 Acme Corporation Inc."

# Custom URLs
WHITELABEL_DOCS_URL=https://docs.acmeplatform.com
WHITELABEL_API_DOCS_URL=https://docs.acmeplatform.com/api
WHITELABEL_HELP_URL=https://support.acmeplatform.com
```

### Localization

```bash
# Languages
WHITELABEL_DEFAULT_LANGUAGE=en
WHITELABEL_SUPPORTED_LANGUAGES=en,es,fr,de
WHITELABEL_ALLOW_LANGUAGE_CHANGE=false

# Formats
WHITELABEL_DATE_FORMAT=us    # us, eu, iso
WHITELABEL_TIME_FORMAT=12h   # 12h, 24h
WHITELABEL_DEFAULT_TIMEZONE=America/New_York
WHITELABEL_CURRENCY=USD
```

### Multi-Tenant Configuration

```bash
# Enable multi-brand/multi-tenant
WHITELABEL_MULTI_BRAND_MODE=true

# Brand configuration source
WHITELABEL_BRAND_CONFIG_SOURCE=database  # database, config-files, api

# Default brand fallback
WHITELABEL_DEFAULT_BRAND_ID=default
```

### Integration Settings

```bash
# Sync to database
WHITELABEL_SYNC_TO_HASURA=true
WHITELABEL_DB_SCHEMA=whitelabel

# GraphQL API
WHITELABEL_GRAPHQL_ENABLED=false

# Caching
WHITELABEL_CACHE_ENABLED=true
WHITELABEL_CACHE_TTL=3600
```

---

## Complete Workflows

### Workflow 1: Basic White-Label Setup (30 minutes)

Perfect for startups and small businesses wanting basic branding.

**Prerequisites:**
- Domain purchased and accessible
- Logo file (PNG, SVG, or JPG)
- Brand colors (hex codes)

**Steps:**

```bash
# 1. Enable white-label
echo "WHITELABEL_ENABLED=true" >> .env
echo "WHITELABEL_TIER=basic" >> .env

# 2. Create brand
nself tenant branding create "My Startup"

# 3. Upload logo
nself tenant logo upload ./logo.svg --type main
nself tenant logo upload ./icon.png --type icon

# 4. Set brand colors
nself tenant branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600"

# 5. Set fonts
nself tenant branding set-fonts \
  --primary "Inter" \
  --secondary "Roboto"

# 6. Build and deploy
nself build
nself start

# 7. Verify
nself tenant settings
nself urls
```

**Result:**
- ✅ Custom brand name and logo
- ✅ Brand colors applied
- ✅ Custom fonts
- ✅ Default domain

---

### Workflow 2: Professional Setup with Custom Domain (45 minutes)

For SaaS platforms and resellers wanting custom domains and emails.

**Prerequisites:**
- Domain purchased
- Email provider configured
- Logo and branding assets
- Custom email templates (optional)

**Steps:**

```bash
# 1. Initialize system
nself tenant init

# 2. Create brand with all details
nself tenant branding create "Acme Platform" \
  --tagline "Enterprise Application Platform"

# 3. Upload all logos
nself tenant logo upload ./logo.svg --type main
nself tenant logo upload ./logo-dark.svg --type dark
nself tenant logo upload ./icon.png --type icon
nself tenant logo upload ./email-logo.png --type email

# 4. Configure branding
nself tenant branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600" \
  --accent "#00cc66"

nself tenant branding set-fonts \
  --primary "Inter" \
  --secondary "Roboto"

# 5. Set environment variables for email
cat >> .env << EOF
WHITELABEL_EMAIL_FROM_NAME="Acme Platform"
WHITELABEL_EMAIL_FROM_ADDRESS="noreply@acmeplatform.com"
WHITELABEL_CUSTOM_EMAIL_TEMPLATES=true
EOF

# 6. Add custom domains
nself tenant domains add app.acmeplatform.com
nself tenant domains add api.acmeplatform.com
nself tenant domains add auth.acmeplatform.com

# 7. Verify domains
nself tenant domains verify app.acmeplatform.com
nself tenant domains verify api.acmeplatform.com
nself tenant domains verify auth.acmeplatform.com

# Wait for DNS propagation (up to 48 hours)
# Monitor with: nself tenant domains health <domain>

# 8. Provision SSL certificates
nself tenant domains ssl app.acmeplatform.com --auto-renew
nself tenant domains ssl api.acmeplatform.com --auto-renew
nself tenant domains ssl auth.acmeplatform.com --auto-renew

# 9. Customize email templates
nself tenant email edit welcome
nself tenant email edit password-reset
nself tenant email edit verify-email

# 10. Test emails
nself tenant email test welcome admin@example.com
nself tenant email test password-reset admin@example.com

# 11. Create custom theme
nself tenant theme create "corporate"
nself tenant theme activate "corporate"

# 12. Configure via environment
cat >> .env << EOF
WHITELABEL_DOMAIN=app.acmeplatform.com
WHITELABEL_API_DOMAIN=api.acmeplatform.com
WHITELABEL_AUTH_DOMAIN=auth.acmeplatform.com
WHITELABEL_SHOW_NSELF_BRANDING=false
EOF

# 13. Build and deploy
nself build
nself start

# 14. Verify everything
nself tenant settings
nself tenant list
nself urls
```

**Result:**
- ✅ Complete brand identity
- ✅ Custom domains (all services)
- ✅ Auto-renewing SSL certificates
- ✅ Branded email templates
- ✅ Custom theme applied
- ✅ No nself branding visible

---

### Workflow 3: Multi-Tenant Setup (Enterprise)

For agencies and resellers managing multiple branded instances.

**Prerequisites:**
- Enterprise tier license
- Multiple domains
- Brand configuration templates
- Multi-tenant database schema

**Steps:**

```bash
# 1. Enable multi-brand mode
cat >> .env << EOF
WHITELABEL_ENABLED=true
WHITELABEL_TIER=enterprise
WHITELABEL_MULTI_BRAND_MODE=true
WHITELABEL_BRAND_CONFIG_SOURCE=database
EOF

# 2. Initialize system
nself tenant init

# 3. Create first brand (Tenant A)
nself tenant branding create "Client A Corp" \
  --tenant "client-a-id"

nself tenant logo upload ./client-a-logo.svg \
  --type main --tenant "client-a-id"

nself tenant branding set-colors \
  --primary "#ff0000" \
  --secondary "#0000ff" \
  --tenant "client-a-id"

nself tenant domains add a.app.io --tenant "client-a-id"
nself tenant domains verify a.app.io --tenant "client-a-id"
nself tenant domains ssl a.app.io --auto-renew --tenant "client-a-id"

# 4. Create second brand (Tenant B)
nself tenant branding create "Client B Ltd" \
  --tenant "client-b-id"

nself tenant logo upload ./client-b-logo.svg \
  --type main --tenant "client-b-id"

nself tenant branding set-colors \
  --primary "#00aa00" \
  --secondary "#ffaa00" \
  --tenant "client-b-id"

nself tenant domains add b.app.io --tenant "client-b-id"
nself tenant domains verify b.app.io --tenant "client-b-id"
nself tenant domains ssl b.app.io --auto-renew --tenant "client-b-id"

# 5. Customize email templates per tenant
nself tenant email edit welcome --tenant "client-a-id"
nself tenant email edit welcome --tenant "client-b-id"

# 6. Export configurations for backup
nself tenant export --tenant "client-a-id" > client-a-config.json
nself tenant export --tenant "client-b-id" > client-b-config.json

# 7. Configure nginx routing
# (Automatically handled by nself build)

# 8. Deploy
nself build
nself start

# 9. Verify multi-tenant setup
nself tenant list --tenant "client-a-id"
nself tenant list --tenant "client-b-id"
nself tenant settings --tenant "client-a-id"
nself tenant settings --tenant "client-b-id"

# 10. Monitor both instances
nself tenant domains health a.app.io --tenant "client-a-id"
nself tenant domains health b.app.io --tenant "client-b-id"
```

**Result:**
- ✅ Two independent branded instances
- ✅ Separate domains and SSL certificates
- ✅ Isolated branding per tenant
- ✅ Unique email templates
- ✅ Complete multi-tenant isolation

---

### Workflow 4: Migration from Existing Setup

Migrating white-label configuration to a new environment.

```bash
# 1. Export current configuration
nself tenant export --format json > branding-export.json

# 2. Copy logos and assets
mkdir -p ./branding-assets
cp -r ./branding/* ./branding-assets/

# 3. On new environment, initialize
nself tenant init

# 4. Import configuration
nself tenant import ./branding-export.json

# 5. Verify everything imported
nself tenant list
nself tenant settings

# 6. Update environment variables if needed
# (Domain, email provider, etc.)
vi .env

# 7. Re-verify domains if moving to new IP
nself tenant domains health <domain>

# 8. Build and deploy
nself build
nself start
```

---

## Multi-Tenant Considerations

### Tenant Isolation

Each tenant gets complete branding isolation:

```bash
# Tenant A - Blue theme
nself tenant branding create "Client A" --tenant "a"
nself tenant branding set-colors --primary "#0066cc" --tenant "a"

# Tenant B - Green theme
nself tenant branding create "Client B" --tenant "b"
nself tenant branding set-colors --primary "#00cc66" --tenant "b"

# Each tenant sees only their branding
nself tenant settings --tenant "a"
nself tenant settings --tenant "b"
```

### Domain Routing

Each tenant gets its own domain routing:

```bash
# Client A uses: a.app.io
# Client B uses: b.app.io
# Admin uses: admin.app.io

# Nginx automatically routes based on domain
# Brand configuration determined by incoming domain
```

### Email Per-Tenant

Each tenant can have customized email templates:

```bash
# Edit welcome email for Tenant A
nself tenant email edit welcome --tenant "a"

# Edit welcome email for Tenant B
nself tenant email edit welcome --tenant "b"

# System automatically uses correct template based on tenant
```

### Fallback Behavior

If tenant brand is not configured:

```bash
# 1. Check for tenant-specific brand
# 2. Fall back to default brand
# 3. Use nself branding as last resort

# Configure fallback brand
nself tenant branding create "Default" --tenant "default"
```

---

## Security Best Practices

### Asset Security

1. **Always use HTTPS for logos and assets**
   ```bash
   WHITELABEL_LOGO_URL=https://cdn.acmeplatform.com/logo.svg  # ✅
   WHITELABEL_LOGO_URL=http://cdn.acmeplatform.com/logo.svg   # ❌
   ```

2. **Use a CDN for logo delivery**
   ```bash
   # Cloudflare, AWS CloudFront, Fastly, etc.
   WHITELABEL_CDN_URL=https://cdn.acmeplatform.com
   WHITELABEL_CDN_PROVIDER=cloudflare
   ```

3. **Validate logo file uploads**
   ```bash
   # Only PNG, SVG, JPG, WebP allowed
   # Maximum 5MB per file
   # Scanned for malicious content
   ```

### Domain Security

1. **Verify domain ownership before SSL**
   ```bash
   nself tenant domains verify <domain>      # Required
   nself tenant domains ssl <domain>         # Only after verification
   ```

2. **Enable auto-renewal to prevent expiration**
   ```bash
   nself tenant domains ssl <domain> --auto-renew
   ```

3. **Monitor domain health regularly**
   ```bash
   nself tenant domains health <domain>
   ```

### Email Security

1. **Use authenticated SMTP**
   ```bash
   AUTH_SMTP_SECURE=true
   AUTH_SMTP_HOST=smtp.sendgrid.net
   ```

2. **Never commit SMTP credentials**
   ```bash
   # ❌ DON'T do this
   AUTH_SMTP_PASS=actual-password  # in git

   # ✅ DO this
   AUTH_SMTP_PASS=${SMTP_PASSWORD}  # environment variable
   ```

3. **Test email delivery regularly**
   ```bash
   nself tenant email test welcome admin@example.com
   ```

### CSS Security

1. **Avoid `!important` abuse**
   ```css
   /* ❌ Fragile and hard to maintain */
   .button { color: red !important; }

   /* ✅ Use specificity properly */
   .brand-button { color: red; }
   ```

2. **Don't include JavaScript in CSS**
   ```css
   /* ❌ Not allowed */
   .button { behavior: url(malicious.htc); }

   /* ✅ Pure CSS only */
   .button { color: var(--brand-color); }
   ```

3. **Use CSS variables for maintainability**
   ```css
   :root {
     --brand-color: #0066cc;
     --brand-accent: #ff6600;
   }

   .button { background: var(--brand-color); }
   ```

### Database Security

1. **Encrypt sensitive brand data**
   - API keys (CDN access, etc.)
   - DKIM/SPF records
   - SSL private keys

2. **Restrict database access**
   - White-label tables should only be accessible to admin
   - Use Hasura permissions for multi-tenant isolation

3. **Audit configuration changes**
   ```bash
   # Enable audit logging
   WHITELABEL_AUDIT_LOG=true
   ```

---

## Troubleshooting

### Branding Issues

**Problem: Custom logo not showing**
```bash
# 1. Verify logo URL is publicly accessible
curl -I https://cdn.acmeplatform.com/logo.svg

# 2. Check CORS headers if loading from different domain
curl -i -H "Origin: https://app.acmeplatform.com" \
     -H "Access-Control-Request-Method: GET" \
     https://cdn.acmeplatform.com/logo.svg

# 3. Clear browser cache
# Ctrl+Shift+Delete (Windows/Linux) or Cmd+Shift+Delete (Mac)

# 4. Check CDN cache
# Clear CDN cache for the URL

# 5. Verify logo is still active
nself tenant logo list | grep logo.svg
```

**Problem: Colors not applying**
```bash
# 1. Verify hex codes are valid
nself tenant settings | grep COLOR

# 2. Check if custom CSS is overriding
nself tenant branding preview

# 3. Clear browser cache
# Press F12 → Network → Disable cache → Refresh

# 4. Restart services
nself restart
```

**Problem: Font not loading**
```bash
# 1. Verify font exists in Google Fonts or custom URL
nself tenant settings | grep FONT

# 2. Check browser console for font loading errors
# Press F12 → Console tab → Look for font errors

# 3. Test font URL directly
curl -I https://fonts.googleapis.com/css?family=Inter

# 4. Add font fallback in CSS
# If main font fails, secondary font should display
```

### Domain Issues

**Problem: Domain verification fails**
```bash
# 1. Check DNS propagation
dig _nself-verify.example.com TXT
# or
nslookup _nself-verify.example.com

# 2. Verify TXT record value exactly matches
nself tenant domains verify example.com
# Compare the token with your DNS provider

# 3. Wait for DNS propagation
# Can take up to 48 hours

# 4. Check DNS provider UI shows record correctly
# Log into your DNS provider and verify

# 5. Try forcing re-verification
nself tenant domains verify example.com --force

# 6. Check domain is not using too many TXT records
# DNS providers have limits on TXT record size
```

**Problem: SSL certificate provisioning fails**
```bash
# 1. Ensure domain is verified first
nself tenant domains verify example.com

# 2. Check domain points to correct IP
dig example.com
# Should return your server's IP

# 3. Verify ports 80 and 443 are accessible
curl -v http://example.com:80
curl -v https://example.com:443

# 4. Check Let's Encrypt rate limits
# (50 certificates per domain per week)
nself logs | grep letsencrypt

# 5. Verify domain DNS is propagated
# Some providers need full propagation before cert

# 6. Try manual certificate upload
nself tenant domains ssl example.com \
  --cert-path /path/to/cert.pem \
  --key-path /path/to/key.pem
```

**Problem: Domain health check shows errors**
```bash
# 1. Check current certificate status
nself tenant domains health example.com

# 2. Review detailed health info
nself tenant domains ssl example.com --status

# 3. If cert expires soon, renew manually
nself tenant domains ssl example.com --renew

# 4. Check HTTPS redirect working
curl -I http://example.com
# Should show 301 redirect to https

# 5. Verify TLS version is modern (1.2+)
openssl s_client -connect example.com:443 -tls1_2
```

### Email Issues

**Problem: Email template changes not applied**
```bash
# 1. Verify template was saved
nself tenant email list
# Check if your template shows as modified

# 2. Rebuild template cache
nself tenant email list --refresh

# 3. Check template syntax
nself tenant email preview welcome
# Look for errors in preview

# 4. Verify variables are correctly formatted
# Variables must be {{variable_name}} format

# 5. Restart email service
nself restart mailpit

# 6. Test with fresh email
nself tenant email test welcome admin@example.com
```

**Problem: Test email not arriving**
```bash
# 1. Verify email service is running
nself status mailpit

# 2. Check SMTP configuration
nself email check

# 3. Verify SMTP credentials are correct
cat .env | grep AUTH_SMTP

# 4. Check firewall/network connectivity
nself email check  # Runs connectivity test

# 5. Review email service logs
nself logs mailpit

# 6. Try with different email provider
nself email setup

# 7. If using external provider, check:
#    - API key is valid
#    - Sender email is verified
#    - Account not rate limited
#    - No IP restrictions
```

**Problem: Email images not loading**
```bash
# 1. Use absolute URLs for images
<!-- ❌ WRONG -->
<img src="/assets/logo.png" />

<!-- ✅ RIGHT -->
<img src="https://cdn.acmeplatform.com/logo.png" />

# 2. Ensure image URLs are HTTPS
# Many email clients block HTTP images

# 3. Verify image URLs are publicly accessible
curl -I https://cdn.acmeplatform.com/logo.png

# 4. Keep image sizes reasonable (< 100KB per image)
# Some email clients block large images

# 5. Test rendering in multiple email clients
# Different clients have different image support
```

### Theme Issues

**Problem: Theme not activating**
```bash
# 1. Verify theme exists
nself tenant list --type themes

# 2. Check theme syntax is valid
nself tenant theme preview theme-name

# 3. Verify theme is for correct tenant (multi-tenant)
nself tenant theme activate theme-name --tenant "tenant-id"

# 4. Clear application cache
WHITELABEL_CACHE_ENABLED=false nself start

# 5. Restart services
nself restart

# 6. Check browser cache
# Press Ctrl+Shift+Delete to clear
```

**Problem: Theme preview shows errors**
```bash
# 1. Export theme to check JSON validity
nself tenant theme export theme-name > theme.json

# 2. Validate JSON format
jq . theme.json  # or use online JSON validator

# 3. Check all required color properties exist
# Required: primary, secondary, background, text, border

# 4. Verify color hex codes are valid format (#RRGGBB)

# 5. Check font families are available
# Verify font names match Google Fonts or custom fonts

# 6. Re-import to validate
nself tenant theme import theme.json --validate
```

### Multi-Tenant Issues

**Problem: Tenant branding not showing**
```bash
# 1. Verify tenant ID is correct
nself tenant list --tenant "tenant-id"

# 2. Check brand exists for tenant
nself tenant settings --tenant "tenant-id"

# 3. Verify domain is mapped to tenant
# Check nginx configuration

# 4. Check tenant routing in application
# May need to configure domain-to-tenant mapping

# 5. Verify multi-tenant mode is enabled
cat .env | grep WHITELABEL_MULTI_BRAND_MODE

# 6. Check application detects tenant from domain
# May need custom code in application
```

**Problem: Domains conflicting between tenants**
```bash
# 1. Each domain must be unique
# Tenant A: a.app.io
# Tenant B: b.app.io
# Cannot use same domain for multiple tenants

# 2. Verify domain-to-tenant mapping
nself tenant domains verify domain.io --tenant "tenant-id"

# 3. Check nginx routing rules
nself status | grep nginx
nself logs nginx | grep domain.io

# 4. Use subdomains if sharing main domain
# a.app.io (Tenant A)
# b.app.io (Tenant B)
# Not: app.io for both
```

---

## Performance Optimization

### Logo CDN Caching

```bash
# Configure CDN caching
WHITELABEL_CDN_URL=https://cdn.acmeplatform.com
WHITELABEL_CDN_CACHE_TTL=86400  # 24 hours

# Enable asset versioning for cache busting
WHITELABEL_ASSET_VERSIONING=true
# URLs become: /assets/v123/logo.svg (changes when asset updates)
```

### Settings Caching

```bash
# Cache white-label settings in memory
WHITELABEL_CACHE_ENABLED=true
WHITELABEL_CACHE_TTL=3600  # 1 hour

# Invalidate cache manually if needed
nself tenant settings --clear-cache

# Disable for development
WHITELABEL_CACHE_ENABLED=false
```

### Database Optimization

```sql
-- Indexes are created automatically, but verify:
CREATE INDEX idx_whitelabel_brands_tenant ON whitelabel_brands(tenant_id);
CREATE INDEX idx_whitelabel_domains_brand ON whitelabel_domains(brand_id);
CREATE INDEX idx_whitelabel_domains_verified ON whitelabel_domains(dns_verified);
```

### Email Template Optimization

```bash
# Keep email templates minimal
# - Use CSS variables instead of inline styles
# - Minimize CSS complexity
# - Use responsive design (single column recommended for email)
# - Test rendering size (keep under 100KB total)
```

---

## Related Commands

- [nself admin](ADMIN.md) - Admin dashboard
- [nself ssl](SSL.md) - SSL certificate management
- [nself email](EMAIL.md) - Email service configuration
- [nself tenant](TENANT.md) - Multi-tenant management
- [nself build](BUILD.md) - Build Docker images
- [nself start](START.md) - Start services
- [nself urls](URLS.md) - Show service URLs

---

## Tips & Best Practices

1. **Always preview before deploying**
   ```bash
   nself tenant branding preview
   nself tenant theme preview "theme-name"
   nself tenant email preview "template"
   ```

2. **Keep backup of configurations**
   ```bash
   nself tenant export --format json > backup-$(date +%Y%m%d).json
   ```

3. **Test email templates with real data**
   ```bash
   nself tenant email test welcome admin@example.com
   ```

4. **Verify domains before pointing DNS**
   ```bash
   nself tenant domains verify <domain>
   nself tenant domains health <domain>
   ```

5. **Use version control for custom CSS**
   ```bash
   git add custom-brand.css
   git commit -m "Update brand styles"
   ```

6. **Test themes in light and dark modes**
   - Ensure sufficient contrast in both
   - Test on actual devices, not just browser

7. **Monitor domain certificate expiration**
   ```bash
   nself tenant domains health <domain>
   # Shows days until certificate expires
   ```

8. **Use SVG logos when possible**
   - Scalable to any size
   - Smaller file size than PNG
   - Better quality on high-DPI displays
   - Email exception: use PNG for email logos

9. **Document brand guidelines for team**
   - Brand colors and usage rules
   - Logo variations and clearance
   - Typography guidelines
   - Approved imagery styles

10. **Test on multiple devices and browsers**
    - Mobile devices (iOS, Android)
    - Tablets
    - Desktop browsers (Chrome, Firefox, Safari, Edge)
    - Email clients (Outlook, Gmail, Apple Mail)

---

## Resources

- **Documentation**: https://docs.nself.org/whitelabel
- **Theme Gallery**: https://themes.nself.org
- **Brand Examples**: https://docs.nself.org/whitelabel/examples
- **CSS Variables Reference**: https://docs.nself.org/whitelabel/css-reference
- **Email Template Guide**: https://docs.nself.org/whitelabel/email-templates
- **Support**: support@nself.org

---

**Last Updated**: January 30, 2025
**Version**: 0.9.0
**Status**: Production Ready
