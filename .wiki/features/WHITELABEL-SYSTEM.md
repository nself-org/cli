# White-Label & Customization System

**Sprint 14: White-Label & Customization (60 story points) - v0.9.0**

Complete white-label branding and customization system for multi-tenant applications with custom domains, themes, and email templates.

---

## Table of Contents

1. [Overview](#overview)
2. [Features](#features)
3. [Architecture](#architecture)
4. [CLI Commands](#cli-commands)
5. [Database Schema](#database-schema)
6. [Usage Examples](#usage-examples)
7. [API Integration](#api-integration)
8. [Best Practices](#best-practices)

---

## Overview

The nself White-Label System provides comprehensive branding customization capabilities for SaaS platforms, allowing you to:

- **Create multiple brands** for multi-tenant applications
- **Configure custom domains** with automatic SSL provisioning
- **Customize themes** with CSS variables and dark/light modes
- **Personalize email templates** with multi-language support
- **Manage brand assets** (logos, images, fonts, custom CSS)

### Key Benefits

- ✅ **Multi-tenant support** - Separate branding per tenant
- ✅ **Zero-downtime domain addition** - Add domains without restarting
- ✅ **Automatic SSL** - Let's Encrypt integration with auto-renewal
- ✅ **Theme system** - Pre-built themes + custom theme creation
- ✅ **Email customization** - Full control over email appearance
- ✅ **Asset management** - Centralized logo and image storage

---

## Features

### 1. Branding Management

**Create and manage brand identities:**

```bash
# Create a new brand
nself whitelabel branding create "My Company"

# Set brand colors
nself whitelabel branding set-colors \
  --primary #0066cc \
  --secondary #ff6600 \
  --accent #00cc66

# Set brand fonts
nself whitelabel branding set-fonts \
  --primary "Inter, system-ui, sans-serif" \
  --secondary "Georgia, serif"

# Upload logo
nself whitelabel logo upload ./logo.png --type main

# Set custom CSS
nself whitelabel branding set-css ./custom.css

# Preview current branding
nself whitelabel branding preview
```

**Features:**
- Primary, secondary, and accent colors
- Custom fonts (primary, secondary, monospace)
- Multiple logo types (main, icon, email, favicon)
- Custom CSS injection
- Color validation (hex format)
- CSS variable auto-generation

### 2. Custom Domains

**Configure custom domains with SSL:**

```bash
# Add custom domain
nself whitelabel domain add app.mycompany.com

# Verify DNS configuration
nself whitelabel domain verify app.mycompany.com

# Provision SSL certificate
nself whitelabel domain ssl app.mycompany.com --auto-renew

# Check domain health
nself whitelabel domain health app.mycompany.com

# Remove domain
nself whitelabel domain remove app.mycompany.com
```

**Features:**
- DNS verification (TXT, CNAME, A records)
- SSL providers: Let's Encrypt, self-signed, custom
- Automatic SSL renewal
- Health monitoring
- Multiple domains per brand
- HTTPS redirect configuration

### 3. Theme System

**Create and manage UI themes:**

```bash
# Create custom theme
nself whitelabel theme create my-theme

# Edit theme configuration
nself whitelabel theme edit my-theme

# Preview theme
nself whitelabel theme preview my-theme

# Activate theme
nself whitelabel theme activate my-theme

# Export theme
nself whitelabel theme export my-theme > theme.json

# Import theme
nself whitelabel theme import theme.json
```

**Built-in Themes:**
- **Light** - Clean and bright (default)
- **Dark** - Easy on the eyes
- **High Contrast** - Maximum accessibility

**Theme Variables:**
- Colors (primary, secondary, accent, background, text, etc.)
- Typography (fonts, sizes, weights, line heights)
- Spacing (xs, sm, md, lg, xl, xxl)
- Borders (radius, width)
- Shadows (sm, md, lg)

### 4. Email Templates

**Customize email templates:**

```bash
# List available templates
nself whitelabel email list

# Edit template
nself whitelabel email edit welcome

# Preview template with sample data
nself whitelabel email preview welcome

# Send test email
nself whitelabel email test welcome user@example.com

# Set language
nself whitelabel email set-language es
```

**Default Templates:**
- `welcome` - Welcome new users
- `password-reset` - Password reset with secure link
- `verify-email` - Email verification
- `invite` - User invitations
- `password-change` - Password change confirmation
- `account-update` - Account update notifications
- `notification` - Generic notifications
- `alert` - Alert/warning messages

**Template Variables:**

Global variables available in all templates:
- `{{BRAND_NAME}}` - Brand/company name
- `{{LOGO_URL}}` - URL to brand logo
- `{{APP_URL}}` - Main application URL
- `{{CURRENT_YEAR}}` - Current year
- `{{USER_NAME}}` - User's display name
- `{{SUPPORT_EMAIL}}` - Support email

Template-specific variables:
- `{{RESET_URL}}` - Password reset link
- `{{VERIFY_URL}}` - Email verification link
- `{{NOTIFICATION_TITLE}}` - Notification headline
- And more...

### 5. Asset Management

**Upload and manage assets:**

```bash
# Upload logo
nself whitelabel logo upload ./logo.png --type main
nself whitelabel logo upload ./icon.png --type icon
nself whitelabel logo upload ./email-logo.png --type email

# List logos
nself whitelabel logo list

# Remove logo
nself whitelabel logo remove <logo-id>
```

**Supported Asset Types:**
- **Logos**: PNG, JPG, JPEG, SVG, WebP
- **Fonts**: TTF, OTF, WOFF, WOFF2
- **CSS**: Custom stylesheets
- **SSL Certificates**: PEM format

---

## Architecture

### Component Structure

```
src/
├── cli/
│   └── whitelabel.sh              # Main CLI entry point
├── lib/
│   └── whitelabel/
│       ├── branding.sh             # Brand management
│       ├── email-templates.sh      # Email customization
│       ├── domains.sh              # Custom domains & SSL
│       └── themes.sh               # Theme system
└── postgres/
    └── migrations/
        └── 016_create_whitelabel_system.sql
```

### Database Tables

1. **whitelabel_brands** - Brand configurations
2. **whitelabel_domains** - Custom domains with SSL
3. **whitelabel_themes** - UI themes and CSS variables
4. **whitelabel_email_templates** - Email templates
5. **whitelabel_assets** - Logos, images, fonts, files

### File System Structure

```
branding/
├── config.json                    # Main branding config
├── logos/                         # Logo files
│   ├── logo-main.png
│   ├── logo-icon.png
│   └── logo-email.png
├── css/                           # CSS files
│   ├── variables.css              # Auto-generated from config
│   └── custom.css                 # User custom CSS
├── fonts/                         # Font files
├── assets/                        # Other assets
├── themes/                        # Theme configurations
│   ├── light/
│   │   ├── theme.json
│   │   └── theme.css
│   ├── dark/
│   │   ├── theme.json
│   │   └── theme.css
│   └── .active                    # Active theme name
├── email-templates/               # Email templates
│   └── languages/
│       ├── en/                    # English templates
│       │   ├── welcome.html
│       │   ├── welcome.txt
│       │   └── welcome.json
│       └── es/                    # Spanish templates
└── domains/                       # Domain configurations
    ├── domains.json
    ├── ssl/                       # SSL certificates
    │   └── app.mycompany.com/
    │       ├── cert.pem
    │       ├── key.pem
    │       └── chain.pem
    └── dns-challenges/            # DNS verification tokens
```

---

## CLI Commands

### Branding Commands

```bash
# Create brand
nself whitelabel branding create <brand-name>

# Set colors
nself whitelabel branding set-colors --primary <color> --secondary <color>

# Set fonts
nself whitelabel branding set-fonts --primary <font> --secondary <font>

# Upload logo
nself whitelabel branding upload-logo <path>

# Set custom CSS
nself whitelabel branding set-css <path>

# Preview branding
nself whitelabel branding preview
```

### Domain Commands

```bash
# Add domain
nself whitelabel domain add <domain>

# Verify domain
nself whitelabel domain verify <domain>

# Provision SSL
nself whitelabel domain ssl <domain> [--auto-renew]

# Check health
nself whitelabel domain health <domain>

# Remove domain
nself whitelabel domain remove <domain>
```

### Email Commands

```bash
# List templates
nself whitelabel email list

# Edit template
nself whitelabel email edit <template-name>

# Preview template
nself whitelabel email preview <template-name>

# Test template
nself whitelabel email test <template-name> <email>

# Set language
nself whitelabel email set-language <lang-code>
```

### Theme Commands

```bash
# Create theme
nself whitelabel theme create <theme-name>

# Edit theme
nself whitelabel theme edit <theme-name>

# Activate theme
nself whitelabel theme activate <theme-name>

# Preview theme
nself whitelabel theme preview <theme-name>

# Export theme
nself whitelabel theme export <theme-name>

# Import theme
nself whitelabel theme import <path>
```

### Logo Commands

```bash
# Upload logo
nself whitelabel logo upload <path> [--type main|icon|email]

# List logos
nself whitelabel logo list

# Remove logo
nself whitelabel logo remove <logo-id>
```

### System Commands

```bash
# Initialize system
nself whitelabel init

# View settings
nself whitelabel settings

# List all resources
nself whitelabel list

# Export configuration
nself whitelabel export --format json

# Import configuration
nself whitelabel import <config-file>
```

---

## Database Schema

### whitelabel_brands

Primary table for brand configurations.

**Columns:**
- `id` - UUID primary key
- `tenant_id` - Unique tenant identifier
- `brand_name` - Brand display name
- `tagline`, `description` - Brand information
- `primary_color`, `secondary_color`, `accent_color` - Brand colors
- `primary_font`, `secondary_font`, `code_font` - Typography
- `logo_main_id`, `logo_icon_id`, etc. - Asset references
- `active_theme_id` - Current theme reference
- `is_active`, `is_primary` - Status flags
- Timestamps and audit fields

### whitelabel_domains

Custom domain management with SSL.

**Columns:**
- `id` - UUID primary key
- `brand_id` - Brand reference
- `domain` - Domain name (unique)
- `dns_verified`, `dns_verification_token` - DNS status
- `ssl_enabled`, `ssl_provider`, `ssl_expiry_date` - SSL config
- `ssl_cert_id`, `ssl_key_id`, `ssl_chain_id` - Certificate references
- `health_status`, `last_health_check_at` - Health monitoring
- `status` - pending, verified, active, suspended, failed
- Timestamps and audit fields

### whitelabel_themes

Theme configurations with CSS variables.

**Columns:**
- `id` - UUID primary key
- `brand_id` - Brand reference
- `theme_name`, `display_name`, `description` - Theme info
- `mode` - light, dark, auto
- `colors` - JSONB color variables
- `typography` - JSONB typography variables
- `spacing` - JSONB spacing variables
- `borders` - JSONB border variables
- `shadows` - JSONB shadow variables
- `custom_css`, `compiled_css` - CSS content
- `is_active`, `is_default`, `is_system` - Status flags
- Timestamps and audit fields

### whitelabel_email_templates

Email template customization.

**Columns:**
- `id` - UUID primary key
- `brand_id` - Brand reference
- `template_name`, `display_name`, `category` - Template info
- `language_code` - ISO 639-1 language code
- `subject`, `from_name`, `from_email` - Email headers
- `html_content`, `text_content` - Template content
- `variables` - JSONB array of variable names
- `sample_data` - JSONB sample data for preview
- `sent_count`, `last_sent_at` - Usage stats
- Timestamps and audit fields

### whitelabel_assets

Asset storage and management.

**Columns:**
- `id` - UUID primary key
- `brand_id` - Brand reference
- `asset_name`, `asset_type`, `asset_category` - Asset info
- `file_name`, `file_path`, `file_size`, `mime_type` - File info
- `image_width`, `image_height` - Image metadata
- `storage_provider`, `storage_bucket`, `storage_key` - Storage config
- `cdn_url`, `cdn_enabled` - CDN configuration
- `is_public`, `access_url` - Access control
- `version`, `previous_version_id` - Version control
- Timestamps and audit fields

---

## Usage Examples

### Example 1: Complete Brand Setup

```bash
# 1. Initialize white-label system
nself whitelabel init

# 2. Create brand
nself whitelabel branding create "TechCorp"

# 3. Configure colors
nself whitelabel branding set-colors \
  --primary #1a73e8 \
  --secondary #34a853 \
  --accent #fbbc04

# 4. Upload logos
nself whitelabel logo upload ./assets/logo.png --type main
nself whitelabel logo upload ./assets/icon.png --type icon

# 5. Configure custom domain
nself whitelabel domain add app.techcorp.com
nself whitelabel domain verify app.techcorp.com
nself whitelabel domain ssl app.techcorp.com --auto-renew

# 6. Customize email template
nself whitelabel email edit welcome

# 7. Create custom theme
nself whitelabel theme create techcorp-theme
nself whitelabel theme edit techcorp-theme
nself whitelabel theme activate techcorp-theme

# 8. Preview everything
nself whitelabel branding preview
```

### Example 2: Multi-Tenant Setup

```bash
# Tenant 1
nself whitelabel branding create "Company A" --tenant tenant-a
nself whitelabel domain add app.companya.com --tenant tenant-a

# Tenant 2
nself whitelabel branding create "Company B" --tenant tenant-b
nself whitelabel domain add app.companyb.com --tenant tenant-b

# Each tenant has independent branding
```

### Example 3: Export/Import Configuration

```bash
# Export complete branding
nself whitelabel export --format json > branding-backup.json

# Import on different server
nself whitelabel import branding-backup.json
```

### Example 4: Theme Development

```bash
# Create custom theme
nself whitelabel theme create my-custom-theme

# Edit theme.json to customize
nself whitelabel theme edit my-custom-theme

# Preview changes
nself whitelabel theme preview my-custom-theme

# Export for sharing
nself whitelabel theme export my-custom-theme > my-theme.json

# Import on another instance
nself whitelabel theme import my-theme.json

# Activate
nself whitelabel theme activate my-custom-theme
```

---

## API Integration

### GraphQL Queries

```graphql
# Get brand information
query GetBrand($tenantId: String!) {
  whitelabel_brands(where: {tenant_id: {_eq: $tenantId}}) {
    id
    brand_name
    tagline
    primary_color
    secondary_color
    accent_color
    logo_main {
      access_url
    }
    active_theme {
      theme_name
      colors
      typography
    }
  }
}

# Get custom domains
query GetDomains($brandId: uuid!) {
  whitelabel_domains(where: {brand_id: {_eq: $brandId}, is_active: {_eq: true}}) {
    domain
    ssl_enabled
    ssl_expiry_date
    health_status
  }
}

# Get email template
query GetEmailTemplate($brandId: uuid!, $templateName: String!, $language: String!) {
  whitelabel_email_templates(
    where: {
      brand_id: {_eq: $brandId}
      template_name: {_eq: $templateName}
      language_code: {_eq: $language}
    }
  ) {
    subject
    html_content
    text_content
    variables
  }
}
```

### REST API Integration

The white-label system integrates with your application through:

1. **Brand Context Injection** - Automatically inject brand info based on domain/tenant
2. **Theme CSS Serving** - Serve compiled theme CSS via CDN
3. **Email Rendering** - Replace template variables before sending
4. **Asset URLs** - Generate signed URLs for private assets

---

## Best Practices

### 1. Branding

✅ **DO:**
- Use consistent color schemes (primary, secondary, accent)
- Validate hex colors before setting
- Use web-safe fonts with fallbacks
- Optimize logo images (WebP, SVG preferred)
- Test branding on dark/light backgrounds

❌ **DON'T:**
- Use too many custom colors (stick to theme variables)
- Upload unoptimized large images
- Use custom fonts without fallbacks
- Hardcode colors in custom CSS

### 2. Custom Domains

✅ **DO:**
- Verify DNS before provisioning SSL
- Enable auto-renewal for Let's Encrypt
- Set up health monitoring
- Use HTTPS redirects
- Document DNS requirements for users

❌ **DON'T:**
- Skip DNS verification
- Use self-signed certificates in production
- Forget to renew certificates
- Allow HTTP in production

### 3. Themes

✅ **DO:**
- Use CSS variables for all colors/spacing
- Test themes for accessibility (WCAG 2.1 AA)
- Provide both light and dark variants
- Document custom theme variables
- Version theme changes

❌ **DON'T:**
- Hardcode values in theme CSS
- Break accessibility with custom themes
- Create themes without dark mode support
- Forget to test on different screen sizes

### 4. Email Templates

✅ **DO:**
- Provide both HTML and plain text versions
- Test templates with sample data
- Use semantic HTML
- Keep templates mobile-responsive
- Document all template variables

❌ **DON'T:**
- Use only HTML (always include plain text)
- Hardcode brand information
- Use complex CSS (email clients strip it)
- Forget to test on multiple email clients

### 5. Asset Management

✅ **DO:**
- Optimize images before upload
- Use version control for assets
- Enable CDN for public assets
- Document asset dimensions/formats
- Clean up unused assets

❌ **DON'T:**
- Upload raw, unoptimized files
- Store sensitive data in public assets
- Use inconsistent file naming
- Forget to backup assets

---

## Security Considerations

### Asset Access Control

- Public assets: Served directly via CDN
- Private assets: Require signed URLs
- SSL certificates: Stored with restricted permissions
- Version control: Previous versions accessible only to admins

### Domain Verification

- DNS verification required before SSL
- TXT records for ownership proof
- Challenge tokens expire after 24 hours
- Only verified domains can be activated

### Multi-Tenant Isolation

- Each tenant has separate brand configuration
- Assets are scoped by brand_id
- Themes cannot be shared across tenants without explicit import
- Domains must be unique across all brands

---

## Troubleshooting

### Common Issues

**DNS verification fails:**
```bash
# Check DNS propagation
dig TXT _nself-verification.yourdomain.com

# Verify A/CNAME record
dig yourdomain.com A

# Force re-verification
nself whitelabel domain verify yourdomain.com
```

**SSL provisioning fails:**
```bash
# Check certbot installation
which certbot

# Check domain accessibility
curl -I http://yourdomain.com

# Use self-signed for testing
SSL_PROVIDER=selfsigned nself whitelabel domain ssl yourdomain.com
```

**Theme not applying:**
```bash
# Check active theme
cat branding/themes/.active

# Regenerate CSS
nself whitelabel theme edit <theme-name>

# Verify CSS file exists
ls -la branding/themes/<theme-name>/theme.css
```

**Email template variables not replaced:**
```bash
# Check template variables
nself whitelabel email preview <template-name>

# Verify variable names match
cat branding/email-templates/languages/en/<template>.json
```

---

## Performance Optimization

### CDN Integration

Enable CDN for assets to improve load times:

```bash
# Configure CDN in brand settings
cdn_enabled=true
cdn_url=https://cdn.yourdomain.com
```

### CSS Minification

Minify compiled theme CSS for production:

```bash
# Install cssmin
npm install -g clean-css-cli

# Minify theme CSS
cleancss -o theme.min.css theme.css
```

### Asset Optimization

Optimize images before upload:

```bash
# PNG optimization
optipng logo.png

# JPEG optimization
jpegoptim --max=85 photo.jpg

# WebP conversion
cwebp logo.png -o logo.webp
```

---

## Migration Guide

### From Manual Branding

If you're currently using manual branding configuration:

1. **Export existing configuration**
2. **Initialize white-label system**
3. **Import configuration**
4. **Verify branding**
5. **Update application to use white-label API**

### From Other Platforms

Import from common platforms:

```bash
# Export from other platform (example format)
{
  "brand": {
    "name": "Company",
    "colors": {...},
    "fonts": {...}
  }
}

# Convert to nself format
nself whitelabel import converted-config.json
```

---

## Future Enhancements

Planned features for future releases:

- [ ] Multi-language theme support
- [ ] Theme marketplace
- [ ] Advanced email template editor (WYSIWYG)
- [ ] A/B testing for email templates
- [ ] Automated domain provisioning (API integration)
- [ ] Theme inheritance system
- [ ] Brand analytics and usage tracking
- [ ] Custom domain proxy configuration
- [ ] Advanced asset management (transformations, CDN sync)
- [ ] Webhook notifications for domain/SSL events

---

## Support

For issues, questions, or contributions:

- **Documentation**: https://docs.nself.org/whitelabel
- **GitHub Issues**: https://github.com/nself-org/cli/issues
- **Discord**: https://discord.gg/nself
- **Email**: support@nself.org

---

## License

The nself White-Label System is part of the nself project and is licensed under the MIT License.

---

**Last Updated**: January 29, 2026
**Version**: 0.9.0
**Sprint**: 14 - White-Label & Customization (60 story points)
