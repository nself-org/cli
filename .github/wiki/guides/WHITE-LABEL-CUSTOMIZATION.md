# nself White-Label & Customization Guide

**Version**: nself v0.8.0
**Last Updated**: January 29, 2026

Complete guide to white-labeling and customizing nself for agencies, resellers, and B2B SaaS providers.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Core Concepts](#2-core-concepts)
3. [Getting Started](#3-getting-started)
4. [Brand Customization](#4-brand-customization)
5. [Custom Domains](#5-custom-domains)
6. [Email Templates](#6-email-templates)
7. [Theme System](#7-theme-system)
8. [Multi-Tenant Branding](#8-multi-tenant-branding)
9. [Admin UI Customization](#9-admin-ui-customization)
10. [Best Practices](#10-best-practices)
11. [Advanced Customization](#11-advanced-customization)
12. [Troubleshooting](#12-troubleshooting)

---

## 1. Overview

### What is White-Labeling in nself?

**White-labeling** allows you to rebrand nself as your own product, completely removing or replacing nself branding with your own company identity.

This enables:
- **Agencies**: Offer branded backend-as-a-service to clients
- **Resellers**: Sell nself under your brand
- **B2B SaaS**: Provide whitelabeled infrastructure to customers
- **Enterprise**: Internal branding for corporate deployments

### Use Cases

#### 1. Agency Use Case
```
Agency: "DevStudio"
Client: "Acme Corp"

DevStudio branding:
- Logo: devstudio-logo.svg
- Colors: #FF6B35 (orange)
- Domain: platform.devstudio.io

Client view sees:
- "Powered by DevStudio"
- DevStudio support links
- DevStudio documentation
```

#### 2. B2B SaaS Use Case
```
SaaS Provider: "CloudStack"
Customer: "TechStartup Inc"

Per-customer branding:
- Customer 1: TechStartup branding + techstartup.cloudstack.io
- Customer 2: FinanceApp branding + financeapp.cloudstack.io
- Each customer sees their own brand, not CloudStack
```

#### 3. Enterprise Use Case
```
Company: "MegaCorp"
Divisions: Sales, Marketing, Engineering

Shared platform with division branding:
- sales.internal.megacorp.com (Sales branding)
- marketing.internal.megacorp.com (Marketing branding)
- eng.internal.megacorp.com (Engineering branding)
```

### Complete Brand Customization

Every visual and textual element can be customized:
- Logos (header, login, favicon)
- Colors (primary, secondary, accent, backgrounds)
- Typography (fonts, sizes, weights)
- Email templates (transactional, marketing)
- Authentication pages (login, signup, password reset)
- Admin dashboard
- Documentation links
- Support contact information
- Legal pages (terms, privacy)

---

## 2. Core Concepts

### Brand Identity Components

```
┌─────────────────────────────────────────────────────────┐
│               Brand Identity Hierarchy                   │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Platform Brand (Top Level)                             │
│  ├── Logo (SVG, PNG)                                    │
│  ├── Color Palette                                      │
│  │   ├── Primary                                        │
│  │   ├── Secondary                                      │
│  │   ├── Accent                                         │
│  │   └── Backgrounds                                    │
│  ├── Typography                                         │
│  │   ├── Font Family                                    │
│  │   ├── Heading Styles                                 │
│  │   └── Body Text Styles                               │
│  └── Themes                                             │
│      ├── Light Mode                                     │
│      └── Dark Mode                                      │
│                                                          │
│  Tenant Brand (Per-Customer)                            │
│  ├── Inherits Platform Brand                            │
│  ├── Override Logo                                      │
│  ├── Override Colors                                    │
│  ├── Custom Domain                                      │
│  └── Custom Email Templates                             │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Branding Levels

| Level | Scope | Use Case |
|-------|-------|----------|
| **Platform** | Global default | Single-brand deployment |
| **Tenant** | Per customer | Multi-tenant SaaS |
| **Organization** | Per org | Enterprise divisions |
| **Theme** | User preference | Dark/light mode |

### Custom Domains and SSL

```
┌─────────────────────────────────────────────────────────┐
│              Domain & SSL Architecture                   │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Custom Domain Setup                                    │
│  ├── Add domain to nself                                │
│  ├── Configure DNS records                              │
│  │   ├── A record → Server IP                          │
│  │   ├── CNAME for subdomains                          │
│  │   └── TXT for verification                           │
│  ├── SSL provisioning                                   │
│  │   ├── Let's Encrypt (automatic)                      │
│  │   └── Custom certificate (manual)                    │
│  └── Nginx routing                                      │
│      └── Route to correct tenant                        │
│                                                          │
│  Wildcard Support                                       │
│  *.yourdomain.com → tenant-based routing                │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Email Template System

```
Email Template Structure:
├── transactional/
│   ├── welcome.html
│   ├── password-reset.html
│   ├── email-verification.html
│   └── invite.html
├── marketing/
│   ├── newsletter.html
│   └── announcement.html
└── system/
    ├── error-notification.html
    └── maintenance.html

Variables Available:
- {{user.name}}
- {{user.email}}
- {{company.name}}
- {{company.logo}}
- {{action_url}}
- {{support_email}}
```

### Theme System

nself uses a **CSS variable-based theming system** for easy customization:

```css
:root {
  /* Primary colors */
  --color-primary: #3B82F6;
  --color-primary-light: #60A5FA;
  --color-primary-dark: #2563EB;

  /* Secondary colors */
  --color-secondary: #8B5CF6;
  --color-accent: #F59E0B;

  /* Backgrounds */
  --bg-main: #FFFFFF;
  --bg-secondary: #F3F4F6;

  /* Text */
  --text-primary: #111827;
  --text-secondary: #6B7280;

  /* Fonts */
  --font-primary: 'Inter', sans-serif;
  --font-heading: 'Poppins', sans-serif;
}
```

---

## 3. Getting Started

### Prerequisites

- nself v0.5.0+ (white-label features)
- Admin access to nself instance
- Brand assets ready (logos, colors)
- Custom domain(s) available

### Quick Start

```bash
# Initialize white-label configuration
nself whitelabel init

# Set basic brand information
nself whitelabel branding set \
  --name "YourBrand" \
  --primary-color "#FF6B35" \
  --logo "./assets/logo.svg"

# Add custom domain
nself whitelabel domain add yourbrand.com

# Apply changes
nself whitelabel apply
```

### White-Label Configuration File

When you run `nself whitelabel init`, it creates `.whitelabel/config.json`:

```json
{
  "version": "1.0",
  "platform": {
    "name": "YourBrand",
    "company": "Your Company Inc",
    "website": "https://yourbrand.com",
    "support_email": "support@yourbrand.com",
    "branding": {
      "logo": {
        "main": "./assets/logo.svg",
        "icon": "./assets/icon.svg",
        "favicon": "./assets/favicon.ico"
      },
      "colors": {
        "primary": "#FF6B35",
        "secondary": "#4ECDC4",
        "accent": "#FFE66D"
      },
      "fonts": {
        "primary": "Inter",
        "heading": "Poppins"
      }
    },
    "domains": [
      {
        "domain": "yourbrand.com",
        "primary": true,
        "ssl": "letsencrypt"
      }
    ]
  },
  "features": {
    "hide_nself_branding": true,
    "custom_login_page": true,
    "custom_email_templates": true,
    "multi_tenant_branding": false
  }
}
```

### Directory Structure

```
project/
├── .whitelabel/
│   ├── config.json           # Main configuration
│   ├── themes/
│   │   ├── light.css
│   │   └── dark.css
│   ├── emails/
│   │   └── templates/
│   ├── assets/
│   │   ├── logo.svg
│   │   ├── icon.svg
│   │   └── favicon.ico
│   └── pages/
│       ├── login.html
│       └── terms.html
├── branding/                 # Generated files (gitignored)
│   ├── css/
│   ├── images/
│   └── templates/
```

### Initial Setup Workflow

```bash
# 1. Create white-label directory
nself whitelabel init

# 2. Add your brand assets
cp ~/my-brand/logo.svg .whitelabel/assets/
cp ~/my-brand/favicon.ico .whitelabel/assets/

# 3. Configure branding
nself whitelabel branding set \
  --name "MyBrand" \
  --primary-color "#FF6B35" \
  --secondary-color "#4ECDC4" \
  --logo ".whitelabel/assets/logo.svg" \
  --favicon ".whitelabel/assets/favicon.ico"

# 4. Add custom domain
nself whitelabel domain add mybrand.com \
  --ssl letsencrypt \
  --primary

# 5. Generate branding files
nself whitelabel generate

# 6. Rebuild with new branding
nself build
nself restart nginx

# 7. Verify branding
nself whitelabel status
```

---

## 4. Brand Customization

### Logo Customization

#### Logo Requirements

| Type | Format | Size | Usage |
|------|--------|------|-------|
| **Main Logo** | SVG, PNG | 240×60px | Header, navigation |
| **Icon** | SVG, PNG | 64×64px | Small spaces |
| **Favicon** | ICO, PNG | 32×32px | Browser tab |
| **Email Logo** | PNG, JPG | 600×150px | Email headers |

#### Setting Logos

```bash
# Set main logo
nself whitelabel logo set main ./assets/logo.svg

# Set icon (for compact views)
nself whitelabel logo set icon ./assets/icon.svg

# Set favicon
nself whitelabel logo set favicon ./assets/favicon.ico

# Set email logo (optimized for email clients)
nself whitelabel logo set email ./assets/email-logo.png

# View current logos
nself whitelabel logo list
```

#### Logo Configuration in JSON

```json
{
  "branding": {
    "logo": {
      "main": {
        "path": "./assets/logo.svg",
        "alt": "YourBrand Logo",
        "height": "60px"
      },
      "icon": {
        "path": "./assets/icon.svg",
        "size": "64px"
      },
      "favicon": {
        "path": "./assets/favicon.ico"
      },
      "email": {
        "path": "./assets/email-logo.png",
        "width": "600px"
      }
    }
  }
}
```

### Color Scheme Customization

#### Color Palette Structure

```bash
# Set primary color (buttons, links, highlights)
nself whitelabel colors set primary "#FF6B35"

# Set secondary color (secondary actions)
nself whitelabel colors set secondary "#4ECDC4"

# Set accent color (call-to-action, highlights)
nself whitelabel colors set accent "#FFE66D"

# Set background colors
nself whitelabel colors set bg-main "#FFFFFF"
nself whitelabel colors set bg-secondary "#F3F4F6"

# Set text colors
nself whitelabel colors set text-primary "#111827"
nself whitelabel colors set text-secondary "#6B7280"

# Preview colors
nself whitelabel colors preview
```

#### Full Color Palette Definition

```json
{
  "branding": {
    "colors": {
      "primary": {
        "base": "#FF6B35",
        "light": "#FF8C5A",
        "dark": "#CC5629",
        "contrast": "#FFFFFF"
      },
      "secondary": {
        "base": "#4ECDC4",
        "light": "#71D7CF",
        "dark": "#3BA39C",
        "contrast": "#FFFFFF"
      },
      "accent": {
        "base": "#FFE66D",
        "light": "#FFF088",
        "dark": "#CCB857",
        "contrast": "#111827"
      },
      "backgrounds": {
        "main": "#FFFFFF",
        "secondary": "#F3F4F6",
        "tertiary": "#E5E7EB",
        "dark": "#111827"
      },
      "text": {
        "primary": "#111827",
        "secondary": "#6B7280",
        "tertiary": "#9CA3AF",
        "inverse": "#FFFFFF"
      },
      "states": {
        "success": "#10B981",
        "warning": "#F59E0B",
        "error": "#EF4444",
        "info": "#3B82F6"
      }
    }
  }
}
```

#### Generated CSS Variables

After running `nself whitelabel generate`, this generates:

```css
/* .whitelabel/themes/light.css */
:root {
  /* Primary Colors */
  --color-primary: #FF6B35;
  --color-primary-light: #FF8C5A;
  --color-primary-dark: #CC5629;
  --color-primary-contrast: #FFFFFF;

  /* Secondary Colors */
  --color-secondary: #4ECDC4;
  --color-secondary-light: #71D7CF;
  --color-secondary-dark: #3BA39C;
  --color-secondary-contrast: #FFFFFF;

  /* Accent Colors */
  --color-accent: #FFE66D;
  --color-accent-light: #FFF088;
  --color-accent-dark: #CCB857;
  --color-accent-contrast: #111827;

  /* Backgrounds */
  --bg-main: #FFFFFF;
  --bg-secondary: #F3F4F6;
  --bg-tertiary: #E5E7EB;
  --bg-dark: #111827;

  /* Text */
  --text-primary: #111827;
  --text-secondary: #6B7280;
  --text-tertiary: #9CA3AF;
  --text-inverse: #FFFFFF;

  /* States */
  --color-success: #10B981;
  --color-warning: #F59E0B;
  --color-error: #EF4444;
  --color-info: #3B82F6;
}
```

### Typography Customization

#### Font Configuration

```bash
# Set primary font (body text)
nself whitelabel fonts set primary "Inter" \
  --weights "400,500,600,700" \
  --source "google"

# Set heading font
nself whitelabel fonts set heading "Poppins" \
  --weights "600,700,800" \
  --source "google"

# Set monospace font (code blocks)
nself whitelabel fonts set monospace "Fira Code" \
  --weights "400,500" \
  --source "google"

# Use custom fonts (self-hosted)
nself whitelabel fonts set primary "CustomFont" \
  --source "local" \
  --files "./fonts/CustomFont-*.woff2"
```

#### Font Configuration in JSON

```json
{
  "branding": {
    "fonts": {
      "primary": {
        "family": "Inter",
        "source": "google",
        "weights": [400, 500, 600, 700],
        "fallback": "system-ui, sans-serif"
      },
      "heading": {
        "family": "Poppins",
        "source": "google",
        "weights": [600, 700, 800],
        "fallback": "system-ui, sans-serif"
      },
      "monospace": {
        "family": "Fira Code",
        "source": "google",
        "weights": [400, 500],
        "fallback": "Consolas, Monaco, monospace"
      }
    },
    "typography": {
      "scale": "1.2",
      "base_size": "16px",
      "line_height": "1.5"
    }
  }
}
```

#### Generated Typography CSS

```css
/* Font imports */
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');
@import url('https://fonts.googleapis.com/css2?family=Poppins:wght@600;700;800&display=swap');

:root {
  /* Font Families */
  --font-primary: 'Inter', system-ui, sans-serif;
  --font-heading: 'Poppins', system-ui, sans-serif;
  --font-monospace: 'Fira Code', Consolas, Monaco, monospace;

  /* Font Sizes (Modular Scale 1.2) */
  --text-xs: 0.694rem;    /* 11px */
  --text-sm: 0.833rem;    /* 13px */
  --text-base: 1rem;      /* 16px */
  --text-lg: 1.2rem;      /* 19px */
  --text-xl: 1.44rem;     /* 23px */
  --text-2xl: 1.728rem;   /* 28px */
  --text-3xl: 2.074rem;   /* 33px */
  --text-4xl: 2.488rem;   /* 40px */

  /* Line Heights */
  --leading-tight: 1.25;
  --leading-normal: 1.5;
  --leading-relaxed: 1.75;

  /* Font Weights */
  --weight-normal: 400;
  --weight-medium: 500;
  --weight-semibold: 600;
  --weight-bold: 700;
  --weight-extrabold: 800;
}
```

### Custom CSS Injection

For advanced customization beyond variables:

```bash
# Add custom CSS file
nself whitelabel css add ./custom-styles.css

# Add inline CSS
nself whitelabel css inline ".custom-class { color: red; }"

# List custom CSS
nself whitelabel css list

# Remove custom CSS
nself whitelabel css remove custom-styles.css
```

Custom CSS is injected after theme CSS, allowing full override capability.

---

## 5. Custom Domains

### Adding Custom Domains

#### Single Domain Setup

```bash
# Add primary domain
nself whitelabel domain add yourbrand.com \
  --ssl letsencrypt \
  --primary

# View domain status
nself whitelabel domain status yourbrand.com
```

#### Multiple Domains Setup

```bash
# Add multiple domains
nself whitelabel domain add app.yourbrand.com --ssl letsencrypt
nself whitelabel domain add api.yourbrand.com --ssl letsencrypt
nself whitelabel domain add admin.yourbrand.com --ssl letsencrypt

# List all domains
nself whitelabel domain list
```

#### Wildcard Domain Setup

```bash
# Add wildcard domain (for multi-tenant)
nself whitelabel domain add "*.yourbrand.com" \
  --ssl letsencrypt \
  --wildcard

# This enables:
# - customer1.yourbrand.com
# - customer2.yourbrand.com
# - *.yourbrand.com
```

### DNS Configuration

After adding a domain, configure DNS:

#### A Record (Apex Domain)

```
Type: A
Name: @
Value: YOUR_SERVER_IP
TTL: 3600
```

#### CNAME Records (Subdomains)

```
Type: CNAME
Name: app
Value: yourbrand.com
TTL: 3600

Type: CNAME
Name: api
Value: yourbrand.com
TTL: 3600
```

#### TXT Record (Domain Verification)

```
Type: TXT
Name: _nself-verification
Value: VERIFICATION_TOKEN
TTL: 3600
```

Get verification token:
```bash
nself whitelabel domain verify-token yourbrand.com
```

### SSL Certificate Provisioning

#### Let's Encrypt (Automatic)

```bash
# Provision SSL certificate (automatic renewal)
nself whitelabel ssl provision yourbrand.com

# This will:
# 1. Verify domain ownership
# 2. Request certificate from Let's Encrypt
# 3. Install certificate
# 4. Configure Nginx
# 5. Set up auto-renewal
```

#### Custom Certificate (Manual)

```bash
# Use your own certificate
nself whitelabel ssl custom yourbrand.com \
  --cert ./certs/yourbrand.crt \
  --key ./certs/yourbrand.key \
  --chain ./certs/chain.crt
```

### Domain Verification

Before SSL provisioning, verify domain points to your server:

```bash
# Check domain DNS
nself whitelabel domain check yourbrand.com

# Output:
# ✓ DNS A record points to 203.0.113.42 (correct)
# ✓ HTTP reachable on port 80
# ✓ Domain verification TXT record found
# ✓ Ready for SSL provisioning
```

### Domain Configuration in JSON

```json
{
  "domains": [
    {
      "domain": "yourbrand.com",
      "primary": true,
      "ssl": {
        "provider": "letsencrypt",
        "auto_renew": true,
        "status": "active",
        "expires": "2026-04-29"
      },
      "verification": {
        "method": "txt",
        "token": "nself-verify-abc123",
        "verified": true
      }
    },
    {
      "domain": "*.yourbrand.com",
      "wildcard": true,
      "ssl": {
        "provider": "letsencrypt",
        "auto_renew": true,
        "status": "active"
      }
    }
  ]
}
```

### Multi-Domain Routing

Configure how domains route to services:

```bash
# Route domain to specific service
nself whitelabel route add yourbrand.com / frontend
nself whitelabel route add api.yourbrand.com / hasura
nself whitelabel route add admin.yourbrand.com / nself-admin

# View routing table
nself whitelabel route list
```

---

## 6. Email Templates

### Email Template Structure

```
.whitelabel/emails/
├── templates/
│   ├── transactional/
│   │   ├── welcome.html
│   │   ├── password-reset.html
│   │   ├── email-verification.html
│   │   ├── invite.html
│   │   └── notification.html
│   ├── marketing/
│   │   ├── newsletter.html
│   │   └── announcement.html
│   └── system/
│       ├── error-notification.html
│       └── maintenance.html
├── layouts/
│   ├── base.html
│   └── plain.html
├── partials/
│   ├── header.html
│   ├── footer.html
│   └── button.html
└── config.json
```

### Creating Email Templates

#### Initialize Email Templates

```bash
# Generate default templates
nself whitelabel email init

# This creates all default templates in .whitelabel/emails/
```

#### Customize Welcome Email

Create `.whitelabel/emails/templates/transactional/welcome.html`:

```html
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Welcome to {{company.name}}</title>
  <style>
    body {
      font-family: {{fonts.primary}}, sans-serif;
      background-color: #F3F4F6;
      margin: 0;
      padding: 0;
    }
    .container {
      max-width: 600px;
      margin: 40px auto;
      background: #FFFFFF;
      border-radius: 8px;
      overflow: hidden;
    }
    .header {
      background: {{colors.primary}};
      padding: 40px 20px;
      text-align: center;
    }
    .logo {
      max-width: 200px;
      height: auto;
    }
    .content {
      padding: 40px 30px;
    }
    .button {
      display: inline-block;
      background: {{colors.primary}};
      color: #FFFFFF;
      padding: 12px 30px;
      text-decoration: none;
      border-radius: 6px;
      font-weight: 600;
    }
    .footer {
      background: #F3F4F6;
      padding: 20px;
      text-align: center;
      font-size: 14px;
      color: #6B7280;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <img src="{{company.email_logo}}" alt="{{company.name}}" class="logo">
    </div>
    <div class="content">
      <h1>Welcome, {{user.name}}!</h1>
      <p>We're excited to have you on board. Your account has been successfully created.</p>

      <p>Get started by verifying your email address:</p>

      <p style="text-align: center; margin: 30px 0;">
        <a href="{{action_url}}" class="button">Verify Email</a>
      </p>

      <p>Or copy this link into your browser:</p>
      <p style="word-break: break-all; color: #6B7280; font-size: 14px;">
        {{action_url}}
      </p>

      <p>Need help? Contact us at <a href="mailto:{{support_email}}">{{support_email}}</a></p>
    </div>
    <div class="footer">
      <p>&copy; {{current_year}} {{company.name}}. All rights reserved.</p>
      <p>
        <a href="{{company.website}}/privacy">Privacy Policy</a> |
        <a href="{{company.website}}/terms">Terms of Service</a>
      </p>
    </div>
  </div>
</body>
</html>
```

### Template Variables

All templates have access to these variables:

#### User Variables
```
{{user.id}}
{{user.email}}
{{user.name}}
{{user.first_name}}
{{user.last_name}}
{{user.created_at}}
```

#### Company Variables
```
{{company.name}}
{{company.website}}
{{company.logo}}
{{company.email_logo}}
{{support_email}}
{{company.address}}
```

#### Branding Variables
```
{{colors.primary}}
{{colors.secondary}}
{{colors.accent}}
{{fonts.primary}}
{{fonts.heading}}
```

#### Action Variables
```
{{action_url}}       # Primary CTA link
{{action_text}}      # CTA button text
{{verification_code}}
{{reset_token}}
```

#### System Variables
```
{{current_year}}
{{current_date}}
{{app_name}}
{{app_url}}
```

### Multi-Language Email Templates

Support multiple languages:

```bash
# Create language-specific templates
.whitelabel/emails/templates/transactional/
├── welcome.en.html
├── welcome.es.html
├── welcome.fr.html
└── welcome.de.html

# Configure language detection
nself whitelabel email config \
  --default-language en \
  --detect-language user_preference
```

Template selection logic:
1. Use user's language preference
2. Fall back to account language
3. Fall back to default language (en)

### Email Template Testing

```bash
# Preview email template
nself whitelabel email preview welcome \
  --user-email "test@example.com" \
  --user-name "Test User"

# Send test email
nself whitelabel email test welcome \
  --to "your-email@example.com" \
  --variables '{"user.name":"John Doe"}'

# Validate all templates
nself whitelabel email validate
```

### Email Configuration

Configure email sending in `.whitelabel/emails/config.json`:

```json
{
  "from": {
    "name": "{{company.name}}",
    "email": "noreply@yourbrand.com"
  },
  "reply_to": "{{support_email}}",
  "templates": {
    "base_url": "/emails/templates",
    "default_language": "en",
    "languages": ["en", "es", "fr", "de"]
  },
  "branding": {
    "logo": {
      "url": "{{company.email_logo}}",
      "width": "600px"
    },
    "colors": {
      "primary": "{{colors.primary}}",
      "background": "#F3F4F6",
      "text": "#111827"
    }
  },
  "footer": {
    "show_unsubscribe": true,
    "show_privacy_link": true,
    "show_address": true,
    "custom_links": [
      {"text": "Help Center", "url": "{{company.website}}/help"},
      {"text": "Contact Us", "url": "{{company.website}}/contact"}
    ]
  }
}
```

---

## 7. Theme System

### Theme Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Theme System                           │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  CSS Variables (Base)                                   │
│  └── Defined in :root                                   │
│                                                          │
│  Light Theme                                            │
│  └── Override variables for light mode                  │
│                                                          │
│  Dark Theme                                             │
│  └── Override variables for dark mode                   │
│                                                          │
│  Custom Theme                                           │
│  └── User-defined theme extensions                      │
│                                                          │
│  Component Styles                                       │
│  └── Use CSS variables (theme-agnostic)                 │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Creating Custom Themes

#### Initialize Theme

```bash
# Create new theme
nself whitelabel theme create mybrand-light

# This creates:
# .whitelabel/themes/mybrand-light.css
```

#### Theme Template

`.whitelabel/themes/mybrand-light.css`:

```css
/* MyBrand Light Theme */
[data-theme="mybrand-light"] {
  /* Primary Colors */
  --color-primary: #FF6B35;
  --color-primary-light: #FF8C5A;
  --color-primary-dark: #CC5629;

  /* Secondary Colors */
  --color-secondary: #4ECDC4;
  --color-secondary-light: #71D7CF;
  --color-secondary-dark: #3BA39C;

  /* Backgrounds */
  --bg-main: #FFFFFF;
  --bg-secondary: #F9FAFB;
  --bg-tertiary: #F3F4F6;
  --bg-elevated: #FFFFFF;

  /* Text */
  --text-primary: #111827;
  --text-secondary: #6B7280;
  --text-tertiary: #9CA3AF;

  /* Borders */
  --border-color: #E5E7EB;
  --border-light: #F3F4F6;

  /* Shadows */
  --shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
  --shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  --shadow-lg: 0 10px 15px -3px rgba(0, 0, 0, 0.1);

  /* Spacing */
  --spacing-unit: 8px;

  /* Border Radius */
  --radius-sm: 4px;
  --radius-md: 6px;
  --radius-lg: 8px;

  /* Transitions */
  --transition-fast: 150ms ease-in-out;
  --transition-normal: 250ms ease-in-out;
  --transition-slow: 350ms ease-in-out;
}
```

#### Dark Theme

`.whitelabel/themes/mybrand-dark.css`:

```css
/* MyBrand Dark Theme */
[data-theme="mybrand-dark"] {
  /* Primary Colors (slightly adjusted for dark bg) */
  --color-primary: #FF8C5A;
  --color-primary-light: #FFAD7F;
  --color-primary-dark: #FF6B35;

  /* Backgrounds */
  --bg-main: #111827;
  --bg-secondary: #1F2937;
  --bg-tertiary: #374151;
  --bg-elevated: #1F2937;

  /* Text */
  --text-primary: #F9FAFB;
  --text-secondary: #D1D5DB;
  --text-tertiary: #9CA3AF;

  /* Borders */
  --border-color: #374151;
  --border-light: #1F2937;

  /* Shadows (more subtle in dark mode) */
  --shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.3);
  --shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.4);
  --shadow-lg: 0 10px 15px -3px rgba(0, 0, 0, 0.5);
}
```

### Theme Management

```bash
# List available themes
nself whitelabel theme list

# Set default theme
nself whitelabel theme default mybrand-light

# Enable theme switching
nself whitelabel theme toggle enable

# Preview theme
nself whitelabel theme preview mybrand-dark

# Export theme as CSS
nself whitelabel theme export mybrand-light ./output.css
```

### Component-Level Customization

Override specific components:

```css
/* .whitelabel/themes/components.css */

/* Buttons */
.button-primary {
  background: var(--color-primary);
  color: var(--color-primary-contrast);
  border-radius: var(--radius-md);
  padding: 12px 24px;
  font-weight: var(--weight-semibold);
  transition: all var(--transition-fast);
}

.button-primary:hover {
  background: var(--color-primary-dark);
  transform: translateY(-1px);
  box-shadow: var(--shadow-md);
}

/* Cards */
.card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: calc(var(--spacing-unit) * 3);
  box-shadow: var(--shadow-sm);
}

/* Forms */
.input {
  background: var(--bg-main);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 10px 14px;
  color: var(--text-primary);
  transition: border-color var(--transition-fast);
}

.input:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(var(--color-primary-rgb), 0.1);
}

/* Navigation */
.navbar {
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-color);
  box-shadow: var(--shadow-sm);
}

.nav-link {
  color: var(--text-secondary);
  transition: color var(--transition-fast);
}

.nav-link:hover,
.nav-link.active {
  color: var(--color-primary);
}
```

### Theme Preview

Generate a preview page to see your theme:

```bash
# Generate theme preview
nself whitelabel theme preview-generate

# Opens browser at http://localhost:1337/theme-preview
```

The preview shows:
- Color palette
- Typography samples
- Button variations
- Form elements
- Cards and containers
- Navigation components

---

## 8. Multi-Tenant Branding

### Multi-Tenant Architecture

```
┌─────────────────────────────────────────────────────────┐
│            Multi-Tenant Branding System                  │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Platform Level (Default)                               │
│  ├── YourBrand Logo                                     │
│  ├── YourBrand Colors                                   │
│  └── YourBrand Domain                                   │
│                                                          │
│  Tenant 1: "Acme Corp"                                  │
│  ├── Inherits: YourBrand base                           │
│  ├── Override: Logo → acme-logo.svg                     │
│  ├── Override: Primary Color → #E02424                  │
│  ├── Custom Domain: acme.yourbrand.com                  │
│  └── Custom Emails: Acme branding                       │
│                                                          │
│  Tenant 2: "TechStart Inc"                              │
│  ├── Inherits: YourBrand base                           │
│  ├── Override: Logo → techstart-logo.svg                │
│  ├── Override: Colors → #3B82F6                         │
│  ├── Custom Domain: techstart.yourbrand.com             │
│  └── Custom Emails: TechStart branding                  │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Enable Multi-Tenant Branding

```bash
# Enable multi-tenant branding feature
nself whitelabel multi-tenant enable

# This adds tenant branding tables to database
nself whitelabel multi-tenant migrate
```

### Creating Tenant Brands

```bash
# Create tenant brand
nself whitelabel tenant create acme-corp \
  --name "Acme Corp" \
  --logo "./tenants/acme/logo.svg" \
  --primary-color "#E02424" \
  --domain "acme.yourbrand.com"

# View tenant
nself whitelabel tenant show acme-corp

# List all tenants
nself whitelabel tenant list
```

### Tenant Configuration in JSON

`.whitelabel/tenants/acme-corp.json`:

```json
{
  "tenant_id": "acme-corp",
  "name": "Acme Corp",
  "slug": "acme",
  "branding": {
    "inherit_platform": true,
    "overrides": {
      "logo": {
        "main": "./tenants/acme/logo.svg",
        "icon": "./tenants/acme/icon.svg"
      },
      "colors": {
        "primary": "#E02424",
        "secondary": "#DC2626"
      }
    }
  },
  "domain": {
    "primary": "acme.yourbrand.com",
    "aliases": ["acme-corp.yourbrand.com"]
  },
  "emails": {
    "from_name": "Acme Corp",
    "from_email": "noreply@acme.yourbrand.com",
    "reply_to": "support@acmecorp.com",
    "custom_templates": [
      "welcome",
      "password-reset"
    ]
  },
  "settings": {
    "show_platform_branding": false,
    "allow_theme_switching": true,
    "default_theme": "acme-light"
  }
}
```

### Domain-Based Tenant Detection

When a user visits `acme.yourbrand.com`:

1. **Nginx detects domain** and passes to nself
2. **nself looks up tenant** by domain
3. **Loads tenant branding** from config
4. **Renders page** with tenant-specific branding

Configuration in `nginx/sites/wildcard-tenant.conf`:

```nginx
server {
  listen 443 ssl;
  server_name *.yourbrand.com;

  # Pass tenant domain to backend
  location / {
    proxy_pass http://nself-admin:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Tenant-Domain $host;
    proxy_set_header X-Forwarded-For $remote_addr;
  }
}
```

Backend tenant detection (pseudo-code):

```javascript
// In nself-admin or custom service
function getTenantFromRequest(req) {
  const domain = req.headers['x-tenant-domain'];
  const tenant = db.tenants.findByDomain(domain);
  return tenant;
}

function renderWithBranding(tenant) {
  const branding = {
    ...platformBranding,
    ...tenant.branding.overrides
  };
  return render(branding);
}
```

### Tenant-Specific Email Templates

Create tenant-specific email templates:

```
.whitelabel/tenants/acme-corp/emails/
├── welcome.html
├── password-reset.html
└── notification.html
```

These override platform templates for this tenant only.

```bash
# Create tenant email template
nself whitelabel tenant email create acme-corp welcome \
  --template ./tenants/acme/emails/welcome.html

# Test tenant email
nself whitelabel tenant email test acme-corp welcome \
  --to "test@example.com"
```

### Tenant Branding Inheritance

Tenants can inherit from platform or override:

| Property | Inherited | Overridable |
|----------|-----------|-------------|
| Logo | ✅ Yes | ✅ Yes |
| Colors | ✅ Yes | ✅ Yes |
| Fonts | ✅ Yes | ✅ Yes |
| Themes | ✅ Yes | ⚠️ Partial |
| Email Templates | ✅ Yes | ✅ Yes |
| Domain | ❌ No | ✅ Required |
| SSL | ✅ Yes | ✅ Yes |

Example inheritance:

```json
{
  "branding": {
    "inherit_platform": true,
    "overrides": {
      "logo": {
        "main": "./tenant-logo.svg"
      },
      "colors": {
        "primary": "#E02424"
        // secondary, accent, etc. inherited from platform
      }
      // fonts inherited from platform
    }
  }
}
```

### Tenant Management Commands

```bash
# Create tenant
nself whitelabel tenant create TENANT_ID --name "Name"

# Update tenant
nself whitelabel tenant update TENANT_ID --logo ./logo.svg

# Delete tenant
nself whitelabel tenant delete TENANT_ID

# List tenants
nself whitelabel tenant list

# Show tenant details
nself whitelabel tenant show TENANT_ID

# Export tenant config
nself whitelabel tenant export TENANT_ID > tenant.json

# Import tenant config
nself whitelabel tenant import tenant.json
```

---

## 9. Admin UI Customization

### Admin Dashboard Branding

The nself-admin UI can be fully customized.

#### Logo and Header

```bash
# Set admin logo
nself whitelabel admin logo ./admin-logo.svg

# Set header color
nself whitelabel admin header-color "#FF6B35"

# Set admin title
nself whitelabel admin title "YourBrand Admin"
```

#### Login Page Customization

Create custom login page at `.whitelabel/pages/login.html`:

```html
<!DOCTYPE html>
<html data-theme="mybrand-light">
<head>
  <meta charset="UTF-8">
  <title>Login - {{company.name}}</title>
  <link rel="stylesheet" href="/branding/themes/mybrand-light.css">
  <style>
    .login-container {
      display: flex;
      min-height: 100vh;
      background: linear-gradient(135deg, {{colors.primary}} 0%, {{colors.secondary}} 100%);
    }
    .login-box {
      margin: auto;
      background: var(--bg-main);
      padding: 40px;
      border-radius: var(--radius-lg);
      box-shadow: var(--shadow-lg);
      width: 100%;
      max-width: 400px;
    }
    .logo {
      text-align: center;
      margin-bottom: 30px;
    }
    .logo img {
      max-width: 200px;
      height: auto;
    }
  </style>
</head>
<body>
  <div class="login-container">
    <div class="login-box">
      <div class="logo">
        <img src="{{company.logo}}" alt="{{company.name}}">
      </div>
      <h1 style="text-align: center; margin-bottom: 30px;">Welcome Back</h1>

      <form action="/auth/login" method="POST">
        <div class="form-group">
          <label for="email">Email</label>
          <input type="email" id="email" name="email" class="input" required>
        </div>

        <div class="form-group">
          <label for="password">Password</label>
          <input type="password" id="password" name="password" class="input" required>
        </div>

        <button type="submit" class="button-primary" style="width: 100%; margin-top: 20px;">
          Sign In
        </button>
      </form>

      <div style="text-align: center; margin-top: 20px;">
        <a href="/auth/forgot-password">Forgot password?</a>
      </div>

      <div style="text-align: center; margin-top: 30px; color: var(--text-tertiary); font-size: 14px;">
        &copy; {{current_year}} {{company.name}}
      </div>
    </div>
  </div>
</body>
</html>
```

Apply custom login page:

```bash
nself whitelabel admin page login .whitelabel/pages/login.html
```

#### Navigation Customization

Customize admin navigation menu:

```bash
# Add custom navigation items
nself whitelabel admin nav add "Help Center" \
  --url "https://help.yourbrand.com" \
  --icon "help-circle" \
  --position 5

# Remove navigation items
nself whitelabel admin nav remove "Documentation"

# Reorder navigation
nself whitelabel admin nav order \
  "Dashboard,Projects,Users,Settings,Help Center"
```

Navigation configuration in `.whitelabel/admin/nav.json`:

```json
{
  "navigation": [
    {
      "id": "dashboard",
      "label": "Dashboard",
      "icon": "home",
      "url": "/admin",
      "order": 1
    },
    {
      "id": "projects",
      "label": "Projects",
      "icon": "folder",
      "url": "/admin/projects",
      "order": 2
    },
    {
      "id": "help",
      "label": "Help Center",
      "icon": "help-circle",
      "url": "https://help.yourbrand.com",
      "external": true,
      "order": 5
    }
  ],
  "footer": [
    {
      "label": "Support",
      "url": "mailto:{{support_email}}"
    },
    {
      "label": "Privacy",
      "url": "{{company.website}}/privacy"
    }
  ]
}
```

#### Help Documentation Links

Replace nself documentation links with your own:

```bash
# Set custom documentation URL
nself whitelabel admin docs-url "https://docs.yourbrand.com"

# Set custom support URL
nself whitelabel admin support-url "https://support.yourbrand.com"

# Set custom status page
nself whitelabel admin status-url "https://status.yourbrand.com"
```

This updates all "Learn more" and "Get help" links throughout the admin UI.

#### Dashboard Widgets

Customize admin dashboard:

```bash
# Remove default widgets
nself whitelabel admin dashboard remove-widget "nself-news"

# Add custom widget
nself whitelabel admin dashboard add-widget \
  --type "iframe" \
  --title "Company Announcements" \
  --url "https://yourbrand.com/announcements/embed" \
  --width "full" \
  --order 1
```

---

## 10. Best Practices

### Brand Consistency

#### Use Design Tokens

Define all design values in one place:

```json
{
  "design_tokens": {
    "colors": {
      "primary": "#FF6B35",
      "secondary": "#4ECDC4"
    },
    "spacing": {
      "unit": 8,
      "scale": [0, 8, 16, 24, 32, 40, 48, 64, 80]
    },
    "typography": {
      "scale": 1.2,
      "base_size": 16
    }
  }
}
```

Then reference these tokens everywhere:
- CSS variables
- Email templates
- Email signatures
- PDF exports
- Mobile apps

#### Brand Guidelines Document

Create `.whitelabel/BRAND-GUIDELINES.md`:

```markdown
# YourBrand Brand Guidelines

## Logo Usage
- Minimum size: 120px wide
- Clear space: 24px on all sides
- Do not stretch or distort
- Use primary logo on light backgrounds
- Use white logo on dark backgrounds

## Color Palette
Primary: #FF6B35 - Use for CTAs, links, primary actions
Secondary: #4ECDC4 - Use for secondary actions, highlights
Accent: #FFE66D - Use sparingly for emphasis

## Typography
Headings: Poppins, 600-800 weight
Body: Inter, 400-600 weight
Never use more than 2 font families

## Voice & Tone
- Professional but approachable
- Clear and concise
- Action-oriented
- Avoid jargon
```

### Performance Considerations

#### Optimize Logo Files

```bash
# Use SVG for logos (scales perfectly, small file size)
# If using PNG:
# - Use transparent background
# - Optimize with tools like ImageOptim or TinyPNG
# - Provide @2x versions for retina displays

# Example sizes:
logo.svg         # Vector, any size
logo.png         # 240x60px
logo@2x.png      # 480x120px
logo-email.png   # 600x150px (PNG for email compatibility)
```

#### Minimize Custom CSS

Only override what's necessary:

```css
/* ❌ Bad: Overriding everything */
.button {
  background: #FF6B35;
  color: white;
  padding: 12px 24px;
  border: none;
  border-radius: 6px;
  /* ...50 more lines */
}

/* ✅ Good: Only override specific values */
.button-primary {
  background: var(--color-primary);
  border-radius: var(--radius-md);
}
```

#### Use CSS Variables

This allows runtime theme switching without loading additional CSS:

```css
/* ✅ Good: Uses variables */
.card {
  background: var(--bg-elevated);
  border: 1px solid var(--border-color);
}

/* ❌ Bad: Hardcoded colors */
.card {
  background: #FFFFFF;
  border: 1px solid #E5E7EB;
}
```

#### Lazy Load Custom Fonts

```html
<!-- Load fonts with display=swap to prevent blocking -->
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600&display=swap" rel="stylesheet">
```

### Mobile Responsiveness

All custom branding must work on mobile:

#### Responsive Logo

```css
.logo {
  max-width: 200px;
  height: auto;
}

@media (max-width: 768px) {
  .logo {
    max-width: 140px;
  }
}

/* For very small screens, use icon only */
@media (max-width: 480px) {
  .logo-full {
    display: none;
  }
  .logo-icon {
    display: block;
    max-width: 40px;
  }
}
```

#### Mobile-Friendly Emails

```html
<!-- Use responsive email template -->
<meta name="viewport" content="width=device-width, initial-scale=1.0">

<style>
  /* Mobile styles */
  @media only screen and (max-width: 600px) {
    .container {
      width: 100% !important;
      padding: 20px !important;
    }
    .button {
      width: 100% !important;
      display: block !important;
    }
  }
</style>
```

#### Test on Real Devices

```bash
# Use nself testing tools
nself whitelabel test mobile

# Opens preview with device simulators:
# - iPhone 13
# - Samsung Galaxy S21
# - iPad
```

### Accessibility

#### Color Contrast

Ensure sufficient contrast (WCAG AA minimum):

```bash
# Check color contrast
nself whitelabel accessibility check-contrast

# Output:
# ✓ Primary color on white: 4.52:1 (WCAG AA Pass)
# ✗ Secondary text on primary: 2.1:1 (WCAG AA Fail)
# Suggestion: Darken text to #1F2937 for 4.5:1 ratio
```

#### Alt Text for Logos

```html
<!-- ✅ Good: Descriptive alt text -->
<img src="/logo.svg" alt="YourBrand - Backend as a Service">

<!-- ❌ Bad: Generic or missing alt text -->
<img src="/logo.svg" alt="Logo">
```

#### Focus States

Ensure interactive elements have visible focus states:

```css
.button:focus {
  outline: 3px solid var(--color-primary);
  outline-offset: 2px;
}

.input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(var(--color-primary-rgb), 0.1);
}
```

### Customer Onboarding

#### Branded Onboarding Flow

Create tenant-specific onboarding:

```bash
# Create onboarding template
nself whitelabel onboarding create \
  --template "./onboarding/welcome-flow.json"

# Show branded tutorial on first login
nself whitelabel onboarding enable --for-new-users
```

Example onboarding flow:

```json
{
  "steps": [
    {
      "id": "welcome",
      "title": "Welcome to {{company.name}}!",
      "content": "Let's get you started.",
      "image": "{{company.logo}}"
    },
    {
      "id": "setup-profile",
      "title": "Complete Your Profile",
      "action": "redirect:/profile/edit"
    },
    {
      "id": "first-project",
      "title": "Create Your First Project",
      "action": "redirect:/projects/new"
    }
  ]
}
```

#### Welcome Email Automation

Send branded welcome email on tenant creation:

```bash
nself whitelabel tenant create new-customer \
  --name "New Customer" \
  --send-welcome-email

# Uses custom welcome template with tenant branding
```

---

## 11. Advanced Customization

### Custom Authentication Pages

Override all auth pages:

```bash
# Create custom auth pages
.whitelabel/pages/
├── login.html
├── signup.html
├── forgot-password.html
├── reset-password.html
└── verify-email.html

# Apply to auth service
nself whitelabel auth pages-dir .whitelabel/pages/
```

### PDF Branding

Brand PDF exports (invoices, reports):

```bash
# Configure PDF templates
nself whitelabel pdf template invoice \
  --header "./pdf/header.html" \
  --footer "./pdf/footer.html" \
  --css "./pdf/styles.css"
```

PDF header template:

```html
<!-- .whitelabel/pdf/header.html -->
<div style="display: flex; justify-content: space-between; padding: 20px; border-bottom: 2px solid {{colors.primary}};">
  <img src="{{company.logo}}" style="height: 40px;" alt="{{company.name}}">
  <div style="text-align: right;">
    <strong>{{document.type}}</strong><br>
    {{document.number}}<br>
    {{document.date}}
  </div>
</div>
```

### API Response Branding

Even API responses can be branded:

```bash
# Configure API metadata
nself whitelabel api metadata \
  --name "{{company.name}} API" \
  --docs-url "https://docs.yourbrand.com/api" \
  --support-email "{{support_email}}"
```

API responses include branding:

```json
{
  "data": {...},
  "meta": {
    "powered_by": "YourBrand API",
    "docs": "https://docs.yourbrand.com/api",
    "support": "support@yourbrand.com"
  }
}
```

### White-Label Mobile Apps

For complete white-labeling, configure mobile apps:

```bash
# Generate mobile app config
nself whitelabel mobile init

# Configure iOS
nself whitelabel mobile ios \
  --app-name "YourBrand" \
  --bundle-id "com.yourbrand.app" \
  --icon "./mobile/icon-ios.png"

# Configure Android
nself whitelabel mobile android \
  --app-name "YourBrand" \
  --package-name "com.yourbrand.app" \
  --icon "./mobile/icon-android.png"

# Export configuration
nself whitelabel mobile export ./mobile-config.json
```

### Subdomain Per Feature

Route different features to different subdomains:

```bash
# Configure feature-based subdomains
nself whitelabel subdomain add app.yourbrand.com \
  --routes "/, /dashboard, /projects"

nself whitelabel subdomain add api.yourbrand.com \
  --service hasura

nself whitelabel subdomain add admin.yourbrand.com \
  --service nself-admin

nself whitelabel subdomain add docs.yourbrand.com \
  --proxy "https://your-docs-site.com"
```

---

## 12. Troubleshooting

### Common Issues

#### Logo Not Displaying

**Problem**: Logo doesn't show up after setting

**Solutions**:

```bash
# 1. Check file path is correct
nself whitelabel logo verify

# 2. Ensure file is accessible
ls -la .whitelabel/assets/logo.svg

# 3. Regenerate branding files
nself whitelabel generate --force

# 4. Clear browser cache
# CMD+Shift+R (Mac) or CTRL+Shift+R (Windows)
```

#### Custom Domain SSL Issues

**Problem**: SSL certificate provisioning fails

**Solutions**:

```bash
# 1. Verify DNS is correct
nself whitelabel domain check yourbrand.com

# 2. Check domain verification
nself whitelabel domain verify yourbrand.com

# 3. Retry SSL provisioning
nself whitelabel ssl provision yourbrand.com --force

# 4. Check Let's Encrypt rate limits
# (50 certs per domain per week)

# 5. Use manual certificate as fallback
nself whitelabel ssl custom yourbrand.com \
  --cert ./cert.pem \
  --key ./key.pem
```

#### Email Templates Not Working

**Problem**: Emails still show default template

**Solutions**:

```bash
# 1. Validate template syntax
nself whitelabel email validate welcome

# 2. Check variables are correct
nself whitelabel email variables welcome

# 3. Clear email template cache
nself whitelabel email cache clear

# 4. Send test email
nself whitelabel email test welcome \
  --to "your-email@example.com" \
  --debug
```

#### Theme Not Applying

**Problem**: Custom theme doesn't take effect

**Solutions**:

```bash
# 1. Check theme is registered
nself whitelabel theme list

# 2. Set as default
nself whitelabel theme default mybrand-light

# 3. Regenerate theme CSS
nself whitelabel theme generate mybrand-light --force

# 4. Clear CSS cache
nself whitelabel cache clear css

# 5. Check browser isn't caching
# Force refresh: CMD+Shift+R
```

#### Multi-Tenant Domain Routing Issues

**Problem**: Tenant domain doesn't route correctly

**Solutions**:

```bash
# 1. Check tenant configuration
nself whitelabel tenant show tenant-id

# 2. Verify domain is added
nself whitelabel domain list | grep tenant-domain

# 3. Check nginx configuration
nself logs nginx | grep "tenant-domain"

# 4. Regenerate nginx configs
nself build --services nginx
nself restart nginx

# 5. Test routing manually
curl -H "Host: tenant.yourbrand.com" http://localhost
```

### Debugging Tools

```bash
# Check all white-label configuration
nself whitelabel status --verbose

# Validate configuration
nself whitelabel validate

# Test specific tenant
nself whitelabel test tenant acme-corp

# View generated files
nself whitelabel files list

# Export all branding
nself whitelabel export ./backup.tar.gz

# Import branding backup
nself whitelabel import ./backup.tar.gz
```

### Getting Help

If you encounter issues:

1. Check logs: `nself logs --service nginx,nself-admin`
2. Run diagnostics: `nself whitelabel diagnose`
3. Check documentation: `nself whitelabel docs`
4. Community forum: https://community.nself.org
5. File issue: https://github.com/nself-org/cli/issues

---

## Conclusion

White-labeling nself enables complete brand customization for:
- **Agencies** offering branded backend services
- **Resellers** selling under their own brand
- **B2B SaaS** with per-customer branding
- **Enterprise** internal divisions

### Key Takeaways

1. **Start simple**: Logo + colors + domain
2. **Expand gradually**: Email templates, themes, multi-tenant
3. **Test thoroughly**: Mobile, accessibility, email clients
4. **Document branding**: Create brand guidelines
5. **Performance matters**: Optimize assets, use CSS variables
6. **Consistency is key**: Use design tokens everywhere

### Next Steps

```bash
# 1. Initialize white-label
nself whitelabel init

# 2. Set basic branding
nself whitelabel branding set --name "YourBrand"

# 3. Add custom domain
nself whitelabel domain add yourbrand.com

# 4. Customize theme
nself whitelabel theme create mybrand-light

# 5. Apply and test
nself whitelabel apply
nself whitelabel test
```

For additional examples and templates, see:
- GitHub: https://github.com/nself-org/cli-examples/whitelabel
- Templates: https://templates.nself.org/whitelabel

---

**Version**: nself v0.8.0
**Last Updated**: January 29, 2026
**License**: MIT

For more information, visit [nself.org](https://nself.org) or join our community at [community.nself.org](https://community.nself.org).
