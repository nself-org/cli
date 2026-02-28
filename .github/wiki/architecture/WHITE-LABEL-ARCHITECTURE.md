# White-Label System Architecture

**nself v0.9.0** - Technical Architecture Documentation

> Complete technical architecture for nself's white-label and branding customization system, supporting multi-tenant SaaS platforms, agency reselling, and enterprise deployments.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Database Schema](#2-database-schema)
3. [Brand Customization System](#3-brand-customization-system)
4. [Custom Domain System](#4-custom-domain-system)
5. [Email Template System](#5-email-template-system)
6. [Theme System](#6-theme-system)
7. [Multi-Tenant Branding](#7-multi-tenant-branding)
8. [Asset Management](#8-asset-management)
9. [Security](#9-security)
10. [Performance](#10-performance)
11. [API Integration](#11-api-integration)
12. [Deployment Architecture](#12-deployment-architecture)

---

## 1. System Overview

### 1.1 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                     White-Label System Architecture                  │
└─────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────┐
│ Request Layer (Domain → Tenant Resolution)                          │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  User Request                                                        │
│       ↓                                                              │
│  Nginx (Port 443)                                                    │
│    ├─ SSL Termination                                               │
│    ├─ Domain Detection (Host header)                                │
│    └─ Lua Tenant Resolver                                           │
│           ↓                                                          │
│           ├─ Query: whitelabel_domains (PostgreSQL)                 │
│           ├─ Cache: Redis (domain → tenant_id)                      │
│           └─ Inject Headers: X-Tenant-ID, X-Brand-ID                │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────────────┐
│ Application Layer (Brand Context)                                   │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Brand Loader Middleware                                            │
│    ├─ Load: whitelabel_brands (PostgreSQL)                          │
│    ├─ Cache: Redis (brand_id → branding config)                     │
│    └─ Context: Inject brand into request                            │
│                                                                      │
│  Theme Compiler                                                      │
│    ├─ Load: whitelabel_themes (PostgreSQL)                          │
│    ├─ Compile: CSS variables → compiled_css                         │
│    └─ Serve: CDN or MinIO                                           │
│                                                                      │
│  Email Renderer                                                      │
│    ├─ Load: whitelabel_email_templates (PostgreSQL)                 │
│    ├─ Parse: Replace variables ({{USER_NAME}}, etc.)                │
│    └─ Send: SMTP with brand context                                 │
│                                                                      │
│  Asset Manager                                                       │
│    ├─ Load: whitelabel_assets (PostgreSQL)                          │
│    ├─ Store: MinIO (S3-compatible)                                  │
│    └─ Serve: CDN (cached)                                           │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────────────┐
│ Data Layer (PostgreSQL + MinIO + Redis)                             │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  PostgreSQL (Brand Configurations)                                   │
│    ├─ whitelabel_brands                                             │
│    ├─ whitelabel_domains                                            │
│    ├─ whitelabel_themes                                             │
│    ├─ whitelabel_email_templates                                    │
│    └─ whitelabel_assets (metadata)                                  │
│                                                                      │
│  MinIO (Asset Storage)                                               │
│    └─ Buckets: brand-{brand_id}/                                    │
│         ├─ logos/                                                    │
│         ├─ fonts/                                                    │
│         ├─ css/                                                      │
│         └─ ssl/                                                      │
│                                                                      │
│  Redis (Caching)                                                     │
│    ├─ domain:{domain} → tenant_id                                   │
│    ├─ brand:{brand_id} → branding config                            │
│    ├─ theme:{theme_id} → compiled CSS                               │
│    └─ email:{template_name}:{brand_id} → rendered HTML              │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 1.2 Component Interactions

```
┌────────────────────────────────────────────────────────────────────┐
│ Component Interaction Flow                                         │
└────────────────────────────────────────────────────────────────────┘

User Visits: https://acme.yoursaas.com
      ↓
1. Nginx receives request
      ↓
2. Lua tenant_resolver.lua executes:
      ├─ Extract domain: "acme.yoursaas.com"
      ├─ Check Redis cache: domain:acme.yoursaas.com
      │    ├─ HIT: Return cached tenant_id
      │    └─ MISS: Query PostgreSQL
      │         SELECT tenant_id FROM whitelabel_domains
      │         WHERE domain = 'acme.yoursaas.com'
      └─ Cache result in Redis (TTL: 3600s)
      ↓
3. Inject headers:
      ├─ X-Tenant-ID: 550e8400-...
      ├─ X-Brand-ID: 660f9511-...
      └─ X-Domain: acme.yoursaas.com
      ↓
4. Proxy to nself-admin:8080
      ↓
5. Brand Loader Middleware (nself-admin):
      ├─ Read X-Brand-ID header
      ├─ Check Redis: brand:660f9511-...
      │    ├─ HIT: Use cached config
      │    └─ MISS: Load from PostgreSQL
      │         SELECT * FROM whitelabel_brands WHERE id = '660f9511-...'
      ├─ Load related data:
      │    ├─ Active theme
      │    ├─ Logo assets
      │    └─ Custom CSS
      └─ Inject into app context
      ↓
6. Render page with brand context:
      ├─ Logo: <img src="{{logo_url}}">
      ├─ Theme: <link href="{{theme_css_url}}">
      ├─ Colors: CSS variables from theme
      └─ Domain: canonical URL
```

### 1.3 Multi-Tenant Isolation

Each tenant gets complete branding isolation:

```
Platform: yoursaas.com
│
├─ Tenant A: acme
│    ├─ Domain: acme.yoursaas.com
│    ├─ Brand: Acme Corp
│    ├─ Theme: acme-light (custom colors)
│    ├─ Logo: acme-logo.svg (MinIO: brand-{id}/logos/)
│    ├─ Email: Acme branded templates
│    └─ SSL: Let's Encrypt (*.yoursaas.com)
│
├─ Tenant B: techstart
│    ├─ Domain: techstart.yoursaas.com
│    ├─ Brand: TechStart Inc
│    ├─ Theme: techstart-dark (custom theme)
│    ├─ Logo: techstart-logo.png
│    ├─ Email: TechStart branded templates
│    └─ SSL: Let's Encrypt (*.yoursaas.com)
│
└─ Tenant C: financeapp (custom domain)
     ├─ Domain: app.financeapp.io
     ├─ Brand: FinanceApp
     ├─ Theme: finance-light (custom)
     ├─ Logo: financeapp-logo.svg
     ├─ Email: FinanceApp branded
     └─ SSL: Let's Encrypt (app.financeapp.io)
```

---

## 2. Database Schema

### 2.1 Schema Overview

```sql
-- White-Label Schema Namespace
-- All tables prefixed with whitelabel_

┌──────────────────────────────────────────────────────────────────┐
│ Database: nself_db                                                │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│ Core Tables:                                                     │
│  ├─ whitelabel_brands         (Brand configurations)            │
│  ├─ whitelabel_domains        (Custom domains + SSL)            │
│  ├─ whitelabel_themes         (UI themes)                       │
│  ├─ whitelabel_email_templates (Email customization)            │
│  └─ whitelabel_assets         (Logos, files, certs)             │
│                                                                  │
│ Relationships:                                                   │
│  whitelabel_brands (1) ──→ (N) whitelabel_domains               │
│  whitelabel_brands (1) ──→ (N) whitelabel_themes                │
│  whitelabel_brands (1) ──→ (N) whitelabel_email_templates       │
│  whitelabel_brands (1) ──→ (N) whitelabel_assets                │
│  whitelabel_brands (1) ──→ (1) whitelabel_themes (active)       │
│                                                                  │
│ Indexes:                                                         │
│  ├─ idx_whitelabel_brands_tenant (tenant_id)                    │
│  ├─ idx_whitelabel_domains_domain (domain) UNIQUE               │
│  ├─ idx_whitelabel_domains_ssl_expiry (ssl_expiry_date)         │
│  ├─ idx_whitelabel_themes_active (is_active)                    │
│  └─ idx_whitelabel_email_templates_name (template_name)         │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 2.2 Table: whitelabel_brands

**Purpose**: Stores brand identity configurations for each tenant.

```sql
CREATE TABLE whitelabel_brands (
  -- Identity
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id VARCHAR(255) UNIQUE NOT NULL DEFAULT 'default',
  brand_name VARCHAR(255) NOT NULL,
  tagline TEXT,
  description TEXT,

  -- Contact Information
  company_address TEXT,
  support_email VARCHAR(255),
  support_url TEXT,

  -- Branding Configuration
  primary_color VARCHAR(7) DEFAULT '#0066cc',
  secondary_color VARCHAR(7) DEFAULT '#ff6600',
  accent_color VARCHAR(7) DEFAULT '#00cc66',
  background_color VARCHAR(7) DEFAULT '#ffffff',
  text_color VARCHAR(7) DEFAULT '#333333',

  -- Typography
  primary_font VARCHAR(255) DEFAULT 'Inter, system-ui, sans-serif',
  secondary_font VARCHAR(255) DEFAULT 'Georgia, serif',
  code_font VARCHAR(255) DEFAULT 'Fira Code, Consolas, monospace',

  -- Asset References (FK to whitelabel_assets)
  logo_main_id UUID,
  logo_icon_id UUID,
  logo_email_id UUID,
  logo_favicon_id UUID,
  custom_css_id UUID,

  -- Theme Reference
  active_theme_id UUID,

  -- Status
  is_active BOOLEAN DEFAULT true,
  is_primary BOOLEAN DEFAULT false,

  -- Audit
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  created_by UUID,
  updated_by UUID,

  -- Constraints
  CONSTRAINT valid_primary_color CHECK (primary_color ~ '^#[0-9A-Fa-f]{6}$'),
  CONSTRAINT valid_secondary_color CHECK (secondary_color ~ '^#[0-9A-Fa-f]{6}$'),
  CONSTRAINT valid_accent_color CHECK (accent_color ~ '^#[0-9A-Fa-f]{6}$')
);

-- Indexes
CREATE INDEX idx_whitelabel_brands_tenant ON whitelabel_brands(tenant_id);
CREATE INDEX idx_whitelabel_brands_active ON whitelabel_brands(is_active);
```

**Key Fields**:

| Field | Type | Purpose |
|-------|------|---------|
| `tenant_id` | VARCHAR(255) | Unique tenant identifier (links to `tenants.tenants`) |
| `brand_name` | VARCHAR(255) | Display name (e.g., "Acme Corp") |
| `primary_color` | VARCHAR(7) | Hex color for buttons, links, CTAs |
| `secondary_color` | VARCHAR(7) | Hex color for secondary actions |
| `accent_color` | VARCHAR(7) | Hex color for highlights |
| `logo_main_id` | UUID | Reference to main logo asset |
| `active_theme_id` | UUID | Reference to active theme |

**Example Row**:

```sql
INSERT INTO whitelabel_brands (
  tenant_id,
  brand_name,
  tagline,
  primary_color,
  secondary_color,
  accent_color,
  support_email
) VALUES (
  'acme-corp',
  'Acme Corporation',
  'Innovation at Scale',
  '#E02424',
  '#DC2626',
  '#F59E0B',
  'support@acmecorp.com'
);
```

### 2.3 Table: whitelabel_domains

**Purpose**: Manages custom domains with DNS verification and SSL certificates.

```sql
CREATE TABLE whitelabel_domains (
  -- Identity
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  brand_id UUID REFERENCES whitelabel_brands(id) ON DELETE CASCADE,

  -- Domain Configuration
  domain VARCHAR(255) UNIQUE NOT NULL,
  is_primary BOOLEAN DEFAULT false,

  -- DNS Configuration
  dns_verified BOOLEAN DEFAULT false,
  dns_verification_token VARCHAR(255),
  dns_verification_method VARCHAR(50) DEFAULT 'txt',
  dns_verified_at TIMESTAMP WITH TIME ZONE,

  -- SSL Configuration
  ssl_enabled BOOLEAN DEFAULT false,
  ssl_provider VARCHAR(50) DEFAULT 'letsencrypt',
  ssl_issuer VARCHAR(255),
  ssl_issued_at TIMESTAMP WITH TIME ZONE,
  ssl_expiry_date TIMESTAMP WITH TIME ZONE,
  ssl_auto_renew BOOLEAN DEFAULT true,
  ssl_last_renewed_at TIMESTAMP WITH TIME ZONE,

  -- Certificate Storage (FK to whitelabel_assets)
  ssl_cert_id UUID,
  ssl_key_id UUID,
  ssl_chain_id UUID,

  -- Health Monitoring
  health_status VARCHAR(50) DEFAULT 'unknown',
  last_health_check_at TIMESTAMP WITH TIME ZONE,
  health_check_interval INTEGER DEFAULT 300,

  -- HTTP Configuration
  redirect_to_https BOOLEAN DEFAULT true,
  redirect_www_to_apex BOOLEAN DEFAULT false,

  -- Status
  status VARCHAR(50) DEFAULT 'pending',
  is_active BOOLEAN DEFAULT true,

  -- Audit
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  -- Constraints
  CONSTRAINT valid_ssl_provider CHECK (ssl_provider IN ('letsencrypt', 'selfsigned', 'custom')),
  CONSTRAINT valid_health_status CHECK (health_status IN ('healthy', 'degraded', 'unhealthy', 'unknown')),
  CONSTRAINT valid_status CHECK (status IN ('pending', 'verified', 'active', 'suspended', 'failed'))
);

-- Indexes
CREATE INDEX idx_whitelabel_domains_brand ON whitelabel_domains(brand_id);
CREATE INDEX idx_whitelabel_domains_domain ON whitelabel_domains(domain);
CREATE INDEX idx_whitelabel_domains_ssl_expiry ON whitelabel_domains(ssl_expiry_date)
  WHERE ssl_enabled = true;
```

**Domain Lifecycle**:

```
pending → verified → active → (suspended) → (deleted)
   ↓         ↓          ↓
 failed    failed     failed
```

**SSL Auto-Renewal Logic**:

```sql
-- Cron job to check SSL expiry
SELECT id, domain, ssl_expiry_date
FROM whitelabel_domains
WHERE ssl_enabled = true
  AND ssl_auto_renew = true
  AND ssl_expiry_date <= NOW() + INTERVAL '30 days';

-- Trigger renewal for domains expiring in 30 days
```

### 2.4 Table: whitelabel_themes

**Purpose**: Stores UI themes with CSS variables for dark/light modes and custom styling.

```sql
CREATE TABLE whitelabel_themes (
  -- Identity
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  brand_id UUID REFERENCES whitelabel_brands(id) ON DELETE CASCADE,

  -- Theme Information
  theme_name VARCHAR(255) NOT NULL,
  display_name VARCHAR(255) NOT NULL,
  description TEXT,
  version VARCHAR(50) DEFAULT '1.0.0',
  author VARCHAR(255),

  -- Theme Mode
  mode VARCHAR(50) DEFAULT 'light',

  -- CSS Variables (JSONB)
  colors JSONB DEFAULT '{
    "primary": "#0066cc",
    "secondary": "#6c757d",
    "accent": "#00cc66",
    "background": "#ffffff",
    "surface": "#ffffff",
    "text": "#212529",
    "border": "#dee2e6"
  }'::jsonb,

  typography JSONB DEFAULT '{
    "fontFamily": "-apple-system, BlinkMacSystemFont, sans-serif",
    "fontSize": "16px",
    "fontWeight": "400",
    "lineHeight": "1.5"
  }'::jsonb,

  spacing JSONB DEFAULT '{
    "xs": "4px",
    "sm": "8px",
    "md": "16px",
    "lg": "24px",
    "xl": "32px"
  }'::jsonb,

  borders JSONB DEFAULT '{
    "radius": "4px",
    "width": "1px"
  }'::jsonb,

  shadows JSONB DEFAULT '{
    "sm": "0 1px 3px rgba(0,0,0,0.12)",
    "md": "0 4px 6px rgba(0,0,0,0.1)",
    "lg": "0 10px 20px rgba(0,0,0,0.15)"
  }'::jsonb,

  -- Custom CSS
  custom_css TEXT,

  -- Compiled CSS (generated from variables + custom_css)
  compiled_css TEXT,

  -- Status
  is_active BOOLEAN DEFAULT false,
  is_default BOOLEAN DEFAULT false,
  is_system BOOLEAN DEFAULT false,

  -- Audit
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  -- Constraints
  CONSTRAINT unique_theme_per_brand UNIQUE(brand_id, theme_name),
  CONSTRAINT valid_theme_mode CHECK (mode IN ('light', 'dark', 'auto'))
);
```

**Theme Compilation**:

```javascript
// Pseudo-code for theme compilation
function compileTheme(theme) {
  const cssVariables = generateCSSVariables(theme.colors, theme.typography);
  const compiledCSS = `
    [data-theme="${theme.theme_name}"] {
      ${cssVariables}
    }
    ${theme.custom_css || ''}
  `;

  return minifyCSS(compiledCSS);
}

// Store in compiled_css column
UPDATE whitelabel_themes
SET compiled_css = compileTheme(theme)
WHERE id = theme.id;
```

### 2.5 Table: whitelabel_email_templates

**Purpose**: Manages customizable email templates with multi-language support.

```sql
CREATE TABLE whitelabel_email_templates (
  -- Identity
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  brand_id UUID REFERENCES whitelabel_brands(id) ON DELETE CASCADE,

  -- Template Information
  template_name VARCHAR(255) NOT NULL,
  display_name VARCHAR(255) NOT NULL,
  description TEXT,
  category VARCHAR(100),

  -- Language Support
  language_code VARCHAR(10) DEFAULT 'en',

  -- Email Configuration
  subject VARCHAR(500) NOT NULL,
  from_name VARCHAR(255),
  from_email VARCHAR(255),
  reply_to VARCHAR(255),

  -- Template Content
  html_content TEXT NOT NULL,
  text_content TEXT NOT NULL,

  -- Variables
  variables JSONB DEFAULT '[]'::jsonb,
  sample_data JSONB,

  -- Compiled Templates
  compiled_html TEXT,
  compiled_text TEXT,

  -- Status
  is_active BOOLEAN DEFAULT true,
  is_default BOOLEAN DEFAULT false,
  is_system BOOLEAN DEFAULT false,

  -- Usage Statistics
  sent_count INTEGER DEFAULT 0,
  last_sent_at TIMESTAMP WITH TIME ZONE,

  -- Audit
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  -- Constraints
  CONSTRAINT unique_template_per_brand_language
    UNIQUE(brand_id, template_name, language_code)
);
```

**Variable Replacement Engine**:

```javascript
function renderEmailTemplate(template, data) {
  let html = template.html_content;
  let text = template.text_content;

  // Replace variables
  Object.keys(data).forEach(key => {
    const regex = new RegExp(`{{${key}}}`, 'g');
    html = html.replace(regex, data[key]);
    text = text.replace(regex, data[key]);
  });

  return { html, text };
}

// Usage
const renderedEmail = renderEmailTemplate(template, {
  'USER_NAME': 'John Doe',
  'BRAND_NAME': 'Acme Corp',
  'ACTION_URL': 'https://acme.yoursaas.com/verify?token=...',
  'LOGO_URL': 'https://cdn.yoursaas.com/brands/acme/logo.png'
});
```

### 2.6 Table: whitelabel_assets

**Purpose**: Manages all brand assets (logos, fonts, certificates, custom CSS).

```sql
CREATE TABLE whitelabel_assets (
  -- Identity
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  brand_id UUID REFERENCES whitelabel_brands(id) ON DELETE CASCADE,

  -- Asset Information
  asset_name VARCHAR(255) NOT NULL,
  asset_type VARCHAR(100) NOT NULL,
  asset_category VARCHAR(100),

  -- File Information
  file_name VARCHAR(255) NOT NULL,
  file_path TEXT NOT NULL,
  file_size BIGINT,
  mime_type VARCHAR(255),
  file_extension VARCHAR(50),

  -- Image Metadata
  image_width INTEGER,
  image_height INTEGER,
  image_format VARCHAR(50),

  -- Storage Information
  storage_provider VARCHAR(100) DEFAULT 'local',
  storage_bucket VARCHAR(255),
  storage_key TEXT,

  -- CDN Information
  cdn_url TEXT,
  cdn_enabled BOOLEAN DEFAULT false,

  -- Access Control
  is_public BOOLEAN DEFAULT true,
  access_url TEXT,

  -- Version Control
  version INTEGER DEFAULT 1,
  previous_version_id UUID,

  -- Status
  is_active BOOLEAN DEFAULT true,

  -- Audit
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  -- Constraints
  CONSTRAINT valid_asset_type CHECK (
    asset_type IN ('logo', 'image', 'font', 'css', 'certificate', 'key', 'other')
  )
);
```

**Asset Storage Strategy**:

```
MinIO Bucket Structure:
brand-{brand_id}/
├── logos/
│   ├── main-v1.svg
│   ├── main-v2.svg (versioned)
│   ├── icon.png
│   ├── email.png
│   └── favicon.ico
├── fonts/
│   ├── CustomFont-Regular.woff2
│   └── CustomFont-Bold.woff2
├── css/
│   └── custom.css
└── ssl/
    ├── cert.pem
    ├── key.pem (encrypted)
    └── chain.pem
```

### 2.7 Views

#### whitelabel_brands_full

Complete brand information with related data:

```sql
CREATE OR REPLACE VIEW whitelabel_brands_full AS
SELECT
  b.*,
  t.theme_name,
  t.display_name as theme_display_name,
  t.mode as theme_mode,
  t.compiled_css as theme_css,
  array_agg(DISTINCT d.domain) FILTER (WHERE d.domain IS NOT NULL) as domains,
  COUNT(DISTINCT d.id) as domain_count,
  logo_main.access_url as logo_main_url,
  logo_icon.access_url as logo_icon_url,
  logo_email.access_url as logo_email_url
FROM whitelabel_brands b
LEFT JOIN whitelabel_themes t ON b.active_theme_id = t.id
LEFT JOIN whitelabel_domains d ON b.id = d.brand_id AND d.is_active = true
LEFT JOIN whitelabel_assets logo_main ON b.logo_main_id = logo_main.id
LEFT JOIN whitelabel_assets logo_icon ON b.logo_icon_id = logo_icon.id
LEFT JOIN whitelabel_assets logo_email ON b.logo_email_id = logo_email.id
WHERE b.is_active = true
GROUP BY b.id, t.id, logo_main.id, logo_icon.id, logo_email.id;
```

**Usage**:

```sql
-- Get complete brand info with theme and logos
SELECT * FROM whitelabel_brands_full
WHERE tenant_id = 'acme-corp';
```

---

## 3. Brand Customization System

### 3.1 Brand Configuration Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Brand Customization Hierarchy                               │
└─────────────────────────────────────────────────────────────┘

Platform Default Brand
  ├── Logo (platform logo)
  ├── Colors (platform color scheme)
  ├── Fonts (platform typography)
  └── Theme (light/dark)
        ↓ inherits
Tenant Brand (per customer)
  ├── Override Logo? (optional)
  ├── Override Colors? (optional)
  ├── Override Fonts? (optional)
  ├── Override Theme? (optional)
  ├── Custom Domain (required)
  └── Email Templates (override or inherit)
```

### 3.2 Logo Management

**Logo Types**:

1. **Main Logo** - Header, navigation (240×60px recommended)
2. **Icon** - Compact views, mobile (64×64px)
3. **Email Logo** - Email headers (600×150px, PNG for compatibility)
4. **Favicon** - Browser tab (32×32px, ICO format)

**Storage Flow**:

```javascript
// Upload logo flow
async function uploadLogo(brandId, file, logoType) {
  // 1. Validate file
  const validation = validateLogoFile(file);
  if (!validation.valid) {
    throw new Error(validation.error);
  }

  // 2. Optimize image
  const optimized = await optimizeImage(file, {
    format: logoType === 'email' ? 'png' : 'svg',
    maxWidth: logoType === 'main' ? 240 : 64,
    quality: 90
  });

  // 3. Upload to MinIO
  const bucketName = `brand-${brandId}`;
  const fileName = `logos/${logoType}-v${Date.now()}.${optimized.ext}`;
  const uploadResult = await minioClient.putObject(
    bucketName,
    fileName,
    optimized.buffer,
    optimized.size,
    {
      'Content-Type': optimized.mimeType,
      'Cache-Control': 'public, max-age=31536000'
    }
  );

  // 4. Create asset record
  const asset = await db.query(`
    INSERT INTO whitelabel_assets (
      brand_id, asset_name, asset_type, asset_category,
      file_name, file_path, file_size, mime_type,
      storage_provider, storage_bucket, storage_key,
      access_url, is_public
    ) VALUES (
      $1, $2, 'logo', $3, $4, $5, $6, $7, 'minio', $8, $9, $10, true
    )
    RETURNING id, access_url
  `, [
    brandId,
    `${logoType}_logo`,
    logoType,
    fileName,
    `/${bucketName}/${fileName}`,
    optimized.size,
    optimized.mimeType,
    bucketName,
    fileName,
    `https://cdn.yoursaas.com/${bucketName}/${fileName}`
  ]);

  // 5. Update brand record
  await db.query(`
    UPDATE whitelabel_brands
    SET logo_${logoType}_id = $1,
        updated_at = NOW()
    WHERE id = $2
  `, [asset.id, brandId]);

  // 6. Invalidate cache
  await redis.del(`brand:${brandId}`);

  return asset;
}
```

### 3.3 Color Scheme System

**CSS Variable Architecture**:

```css
/* Base colors from database */
:root {
  /* Primary Colors */
  --color-primary-base: #0066cc;
  --color-primary-light: color-mix(in srgb, var(--color-primary-base) 80%, white 20%);
  --color-primary-dark: color-mix(in srgb, var(--color-primary-base) 80%, black 20%);
  --color-primary-contrast: #ffffff;

  /* Secondary Colors */
  --color-secondary-base: #ff6600;
  --color-secondary-light: color-mix(in srgb, var(--color-secondary-base) 80%, white 20%);
  --color-secondary-dark: color-mix(in srgb, var(--color-secondary-base) 80%, black 20%);

  /* Semantic Colors */
  --color-success: #10b981;
  --color-warning: #f59e0b;
  --color-error: #ef4444;
  --color-info: #3b82f6;

  /* Backgrounds */
  --bg-main: #ffffff;
  --bg-secondary: #f3f4f6;
  --bg-tertiary: #e5e7eb;

  /* Text */
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --text-tertiary: #9ca3af;
}
```

**Dynamic Color Generation**:

```javascript
// Generate color variants from base color
function generateColorVariants(baseColor) {
  const base = parseColor(baseColor); // { r, g, b }

  return {
    base: baseColor,
    light: lighten(base, 0.2),   // 20% lighter
    dark: darken(base, 0.2),     // 20% darker
    contrast: getContrastColor(base) // Auto-detect black or white
  };
}

// Example
const primaryVariants = generateColorVariants('#0066cc');
// {
//   base: '#0066cc',
//   light: '#3385d6',
//   dark: '#0052a3',
//   contrast: '#ffffff'
// }
```

### 3.4 Typography System

**Font Loading**:

```javascript
// Load fonts from Google Fonts or self-hosted
async function loadFonts(brand) {
  const fonts = [];

  // Primary font
  if (brand.primary_font) {
    fonts.push({
      family: brand.primary_font,
      weights: [400, 500, 600, 700],
      display: 'swap'
    });
  }

  // Generate font-face CSS
  const fontFacesCSS = fonts.map(font => {
    if (font.source === 'google') {
      return `@import url('https://fonts.googleapis.com/css2?family=${font.family.replace(' ', '+')}:wght@${font.weights.join(';')}&display=${font.display}');`;
    } else {
      // Self-hosted fonts
      return generateFontFaceCSS(font);
    }
  }).join('\n');

  return fontFacesCSS;
}
```

**Typography Scale**:

```css
:root {
  --font-primary: 'Inter', system-ui, sans-serif;
  --font-heading: 'Poppins', var(--font-primary);
  --font-monospace: 'Fira Code', Consolas, monospace;

  /* Modular scale (1.2 ratio) */
  --text-xs: 0.694rem;   /* 11px */
  --text-sm: 0.833rem;   /* 13px */
  --text-base: 1rem;     /* 16px */
  --text-lg: 1.2rem;     /* 19px */
  --text-xl: 1.44rem;    /* 23px */
  --text-2xl: 1.728rem;  /* 28px */
  --text-3xl: 2.074rem;  /* 33px */
  --text-4xl: 2.488rem;  /* 40px */

  /* Line heights */
  --leading-tight: 1.25;
  --leading-normal: 1.5;
  --leading-relaxed: 1.75;

  /* Font weights */
  --weight-normal: 400;
  --weight-medium: 500;
  --weight-semibold: 600;
  --weight-bold: 700;
}
```

### 3.5 Custom CSS Injection

**Safe CSS Sandboxing**:

```javascript
// Sanitize custom CSS to prevent XSS
function sanitizeCustomCSS(css) {
  // Remove dangerous patterns
  const dangerous = [
    /javascript:/gi,
    /<script/gi,
    /expression\(/gi,
    /import\s+/gi,
    /@import/gi,
    /url\(['"]?javascript:/gi
  ];

  let sanitized = css;
  dangerous.forEach(pattern => {
    sanitized = sanitized.replace(pattern, '/* blocked */');
  });

  // Scope all selectors to prevent global pollution
  sanitized = scopeCSS(sanitized, `.brand-${brandId}`);

  return sanitized;
}

function scopeCSS(css, scope) {
  // Prefix all selectors with scope
  return css.replace(/([^\r\n,{}]+)(,(?=[^}]*{)|\s*{)/g, (match, selector, separator) => {
    // Don't scope @-rules
    if (selector.trim().startsWith('@')) {
      return match;
    }
    return `${scope} ${selector.trim()}${separator}`;
  });
}

// Example
const customCSS = `.button { color: red; }`;
const scoped = scopeCSS(customCSS, '.brand-acme');
// Result: .brand-acme .button { color: red; }
```

**CSS Minification**:

```javascript
// Minify CSS for production
function minifyCSS(css) {
  return css
    .replace(/\/\*[\s\S]*?\*\//g, '') // Remove comments
    .replace(/\s+/g, ' ')              // Collapse whitespace
    .replace(/\s*([{}:;,])\s*/g, '$1') // Remove spaces around punctuation
    .replace(/;}/g, '}')               // Remove last semicolon
    .trim();
}
```

---

## 4. Custom Domain System

### 4.1 Domain Addition Flow

```
┌──────────────────────────────────────────────────────────────┐
│ Custom Domain Addition Flow                                  │
└──────────────────────────────────────────────────────────────┘

1. User adds domain via CLI/API
   └─> nself whitelabel domain add app.customer.com
        ↓
2. Create domain record (status: pending)
   └─> INSERT INTO whitelabel_domains (domain, brand_id, status)
        ↓
3. Generate verification token
   └─> UPDATE whitelabel_domains SET dns_verification_token = uuid_generate_v4()
        ↓
4. Return DNS instructions
   └─> "Add TXT record: _nself-verification.app.customer.com = {token}"
        ↓
5. User configures DNS
   └─> Wait for DNS propagation (up to 48 hours)
        ↓
6. Verification check (manual or cron)
   └─> nself whitelabel domain verify app.customer.com
        ├─> Query DNS for TXT record
        ├─> Match token
        └─> Update status: verified
             ↓
7. SSL provisioning
   └─> nself whitelabel ssl provision app.customer.com
        ├─> Certbot (Let's Encrypt)
        ├─> Store certificates in whitelabel_assets
        └─> Update status: active
             ↓
8. Nginx configuration
   └─> Generate server block for app.customer.com
        ├─> SSL cert paths
        ├─> Proxy to application
        └─> Reload Nginx
```

### 4.2 DNS Verification

**DNS Record Requirements**:

```
Type: A
Name: @ (or subdomain)
Value: {SERVER_IP}
TTL: 3600

Type: TXT
Name: _nself-verification
Value: {VERIFICATION_TOKEN}
TTL: 3600
```

**Verification Implementation**:

```javascript
const dns = require('dns').promises;

async function verifyDomain(domain) {
  // 1. Get verification token from database
  const domainRecord = await db.query(`
    SELECT dns_verification_token, status
    FROM whitelabel_domains
    WHERE domain = $1
  `, [domain]);

  if (!domainRecord) {
    throw new Error('Domain not found');
  }

  const expectedToken = domainRecord.dns_verification_token;

  // 2. Query DNS for TXT record
  try {
    const txtRecords = await dns.resolveTxt(`_nself-verification.${domain}`);

    // 3. Find matching token
    const verified = txtRecords.some(record => {
      const value = Array.isArray(record) ? record.join('') : record;
      return value === expectedToken;
    });

    if (verified) {
      // 4. Update database
      await db.query(`
        UPDATE whitelabel_domains
        SET dns_verified = true,
            dns_verified_at = NOW(),
            status = 'verified',
            updated_at = NOW()
        WHERE domain = $1
      `, [domain]);

      return { success: true, verified: true };
    } else {
      return { success: false, error: 'Token mismatch' };
    }
  } catch (error) {
    return { success: false, error: error.message };
  }
}
```

### 4.3 SSL Certificate Management

**Let's Encrypt Integration**:

```bash
#!/bin/bash
# SSL provisioning script

DOMAIN=$1
EMAIL=$2

# 1. Verify domain is DNS-verified
STATUS=$(psql -t -c "
  SELECT status FROM whitelabel_domains WHERE domain='$DOMAIN'
" | tr -d ' ')

if [ "$STATUS" != "verified" ]; then
  echo "Error: Domain must be DNS-verified first"
  exit 1
fi

# 2. Request certificate from Let's Encrypt
certbot certonly \
  --nginx \
  --non-interactive \
  --agree-tos \
  --email "$EMAIL" \
  --domains "$DOMAIN" \
  --deploy-hook "/usr/local/bin/nself-ssl-deploy-hook.sh $DOMAIN"

# 3. Check if successful
if [ $? -eq 0 ]; then
  # 4. Store certificates in database
  CERT_PATH="/etc/letsencrypt/live/$DOMAIN/fullchain.pem"
  KEY_PATH="/etc/letsencrypt/live/$DOMAIN/privkey.pem"
  CHAIN_PATH="/etc/letsencrypt/live/$DOMAIN/chain.pem"

  # Upload to MinIO and create asset records
  nself whitelabel ssl store "$DOMAIN" \
    --cert "$CERT_PATH" \
    --key "$KEY_PATH" \
    --chain "$CHAIN_PATH"

  echo "SSL certificate provisioned successfully"
else
  echo "SSL provisioning failed"
  exit 1
fi
```

**Certificate Storage**:

```javascript
async function storeSSLCertificates(domain, certPath, keyPath, chainPath) {
  const brand = await getBrandByDomain(domain);
  const bucketName = `brand-${brand.id}`;

  // 1. Upload to MinIO (encrypted storage)
  const certAsset = await uploadAsset(bucketName, 'ssl/cert.pem', certPath, {
    encrypt: false, // Cert is public
    category: 'certificate'
  });

  const keyAsset = await uploadAsset(bucketName, 'ssl/key.pem', keyPath, {
    encrypt: true,  // Key must be encrypted
    category: 'key'
  });

  const chainAsset = await uploadAsset(bucketName, 'ssl/chain.pem', chainPath, {
    encrypt: false,
    category: 'certificate'
  });

  // 2. Update domain record
  await db.query(`
    UPDATE whitelabel_domains
    SET ssl_enabled = true,
        ssl_provider = 'letsencrypt',
        ssl_cert_id = $1,
        ssl_key_id = $2,
        ssl_chain_id = $3,
        ssl_issued_at = NOW(),
        ssl_expiry_date = NOW() + INTERVAL '90 days',
        status = 'active',
        updated_at = NOW()
    WHERE domain = $4
  `, [certAsset.id, keyAsset.id, chainAsset.id, domain]);
}
```

**Auto-Renewal**:

```javascript
// Cron job: Check SSL certificates daily
async function checkSSLRenewal() {
  const expiringDomains = await db.query(`
    SELECT id, domain, ssl_expiry_date
    FROM whitelabel_domains
    WHERE ssl_enabled = true
      AND ssl_auto_renew = true
      AND ssl_expiry_date <= NOW() + INTERVAL '30 days'
      AND status = 'active'
  `);

  for (const domain of expiringDomains) {
    try {
      await renewSSLCertificate(domain.domain);
      console.log(`Renewed SSL for ${domain.domain}`);
    } catch (error) {
      console.error(`Failed to renew SSL for ${domain.domain}:`, error);
      // Send alert to admin
      await sendAlert(`SSL renewal failed for ${domain.domain}`);
    }
  }
}

// Run daily at 2 AM
cron.schedule('0 2 * * *', checkSSLRenewal);
```

### 4.4 Health Monitoring

```javascript
// Health check for custom domains
async function checkDomainHealth(domain) {
  const checks = {
    dns: false,
    http: false,
    https: false,
    ssl_valid: false,
    ssl_expiry: null
  };

  try {
    // 1. DNS resolution
    const addresses = await dns.resolve4(domain);
    checks.dns = addresses.length > 0;

    // 2. HTTP accessibility
    try {
      const httpResponse = await fetch(`http://${domain}`, {
        method: 'HEAD',
        timeout: 5000
      });
      checks.http = httpResponse.ok;
    } catch (e) {
      checks.http = false;
    }

    // 3. HTTPS accessibility
    try {
      const httpsResponse = await fetch(`https://${domain}`, {
        method: 'HEAD',
        timeout: 5000
      });
      checks.https = httpsResponse.ok;
    } catch (e) {
      checks.https = false;
    }

    // 4. SSL certificate validity
    if (checks.https) {
      const cert = await getSSLCertificate(domain);
      checks.ssl_valid = new Date(cert.valid_to) > new Date();
      checks.ssl_expiry = cert.valid_to;
    }

    // 5. Determine health status
    let healthStatus = 'healthy';
    if (!checks.dns || !checks.https) {
      healthStatus = 'unhealthy';
    } else if (!checks.ssl_valid) {
      healthStatus = 'degraded';
    }

    // 6. Update database
    await db.query(`
      UPDATE whitelabel_domains
      SET health_status = $1,
          last_health_check_at = NOW(),
          updated_at = NOW()
      WHERE domain = $2
    `, [healthStatus, domain]);

    return { domain, healthStatus, checks };
  } catch (error) {
    await db.query(`
      UPDATE whitelabel_domains
      SET health_status = 'unknown',
          last_health_check_at = NOW()
      WHERE domain = $1
    `, [domain]);

    return { domain, healthStatus: 'unknown', error: error.message };
  }
}
```

### 4.5 Nginx Configuration Generation

```javascript
// Generate Nginx server block for custom domain
function generateNginxConfig(domain, brandId) {
  const sslCertPath = `/var/lib/nself/ssl/${domain}/cert.pem`;
  const sslKeyPath = `/var/lib/nself/ssl/${domain}/key.pem`;

  return `
# Custom domain: ${domain}
# Brand ID: ${brandId}
# Generated: ${new Date().toISOString()}

server {
  listen 80;
  server_name ${domain};

  # Redirect to HTTPS
  return 301 https://$host$request_uri;
}

server {
  listen 443 ssl http2;
  server_name ${domain};

  # SSL Configuration
  ssl_certificate ${sslCertPath};
  ssl_certificate_key ${sslKeyPath};
  ssl_protocols TLSv1.2 TLSv1.3;
  ssl_ciphers HIGH:!aNULL:!MD5;
  ssl_prefer_server_ciphers on;

  # Security headers
  add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
  add_header X-Frame-Options "SAMEORIGIN" always;
  add_header X-Content-Type-Options "nosniff" always;

  # Brand context injection
  set $brand_id "${brandId}";

  location / {
    # Pass brand context to backend
    proxy_set_header X-Brand-ID $brand_id;
    proxy_set_header X-Domain ${domain};
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # Proxy to application
    proxy_pass http://nself-admin:8080;
  }

  # Static assets (logos, CSS, etc.)
  location /branding/ {
    proxy_pass http://minio:9000/brand-${brandId}/;
    proxy_cache_valid 200 1d;
    add_header Cache-Control "public, max-age=86400";
  }
}
`;
}
```

---

## 5. Email Template System

### 5.1 Template Storage Structure

```
whitelabel_email_templates
├── brand_id: UUID
├── template_name: 'welcome', 'password-reset', etc.
├── language_code: 'en', 'es', 'fr'
├── html_content: Full HTML template
├── text_content: Plain text version
├── variables: JSONB array ['USER_NAME', 'ACTION_URL', ...]
└── sample_data: JSONB for preview
```

### 5.2 Variable Injection Engine

**Template Variables**:

```javascript
const GLOBAL_VARIABLES = {
  // Brand variables (from whitelabel_brands)
  'BRAND_NAME': (brand) => brand.brand_name,
  'LOGO_URL': (brand) => brand.logo_main_url,
  'SUPPORT_EMAIL': (brand) => brand.support_email,
  'COMPANY_ADDRESS': (brand) => brand.company_address,

  // System variables
  'CURRENT_YEAR': () => new Date().getFullYear(),
  'CURRENT_DATE': () => new Date().toISOString().split('T')[0],
  'APP_URL': (brand, domain) => `https://${domain}`,

  // User variables (passed at render time)
  'USER_NAME': (brand, domain, user) => user.name,
  'USER_EMAIL': (brand, domain, user) => user.email,

  // Action variables (template-specific)
  'ACTION_URL': (brand, domain, user, data) => data.action_url,
  'VERIFICATION_CODE': (brand, domain, user, data) => data.verification_code,
  'RESET_TOKEN': (brand, domain, user, data) => data.reset_token
};
```

**Rendering Engine**:

```javascript
async function renderEmailTemplate(templateName, brandId, userData, actionData = {}) {
  // 1. Load template
  const template = await db.query(`
    SELECT html_content, text_content, variables, from_name, from_email, subject
    FROM whitelabel_email_templates
    WHERE brand_id = $1
      AND template_name = $2
      AND language_code = $3
      AND is_active = true
  `, [brandId, templateName, userData.language || 'en']);

  if (!template) {
    throw new Error(`Template not found: ${templateName}`);
  }

  // 2. Load brand
  const brand = await db.query(`
    SELECT * FROM whitelabel_brands_full WHERE id = $1
  `, [brandId]);

  // 3. Get domain
  const domain = await db.query(`
    SELECT domain FROM whitelabel_domains
    WHERE brand_id = $1 AND is_primary = true
  `, [brandId]);

  // 4. Build variable context
  const context = {
    brand,
    domain: domain.domain,
    user: userData,
    data: actionData
  };

  // 5. Replace variables in HTML
  let html = template.html_content;
  let text = template.text_content;
  let subject = template.subject;

  template.variables.forEach(varName => {
    const resolver = GLOBAL_VARIABLES[varName];
    if (resolver) {
      const value = resolver(brand, domain.domain, userData, actionData);
      const regex = new RegExp(`{{${varName}}}`, 'g');

      html = html.replace(regex, value || '');
      text = text.replace(regex, value || '');
      subject = subject.replace(regex, value || '');
    }
  });

  // 6. Return rendered email
  return {
    from: {
      name: template.from_name || brand.brand_name,
      email: template.from_email || brand.support_email
    },
    to: userData.email,
    subject,
    html,
    text
  };
}
```

**Usage Example**:

```javascript
// Send welcome email
const email = await renderEmailTemplate('welcome', brandId, {
  name: 'John Doe',
  email: 'john@example.com',
  language: 'en'
}, {
  action_url: 'https://acme.yoursaas.com/verify?token=abc123'
});

await sendEmail(email);
```

### 5.3 Multi-Language Support

**Language Detection Flow**:

```
1. Check user's language preference (user.language_code)
    ↓
2. Fall back to account language (account.default_language)
    ↓
3. Fall back to brand default (brand.default_language || 'en')
    ↓
4. Query template:
   SELECT * FROM whitelabel_email_templates
   WHERE brand_id = ? AND template_name = ? AND language_code = ?
    ↓
5. If not found, fall back to 'en'
```

**Template Fallback**:

```javascript
async function getEmailTemplate(brandId, templateName, languageCode) {
  // Try requested language
  let template = await db.query(`
    SELECT * FROM whitelabel_email_templates
    WHERE brand_id = $1
      AND template_name = $2
      AND language_code = $3
      AND is_active = true
  `, [brandId, templateName, languageCode]);

  if (template) {
    return template;
  }

  // Fall back to English
  template = await db.query(`
    SELECT * FROM whitelabel_email_templates
    WHERE brand_id = $1
      AND template_name = $2
      AND language_code = 'en'
      AND is_active = true
  `, [brandId, templateName]);

  if (template) {
    return template;
  }

  // Fall back to system default
  template = await db.query(`
    SELECT * FROM whitelabel_email_templates
    WHERE brand_id IS NULL
      AND template_name = $1
      AND language_code = 'en'
      AND is_system = true
  `, [templateName]);

  return template;
}
```

### 5.4 Email Preview System

```javascript
// Generate preview with sample data
async function previewEmailTemplate(brandId, templateName, languageCode = 'en') {
  const template = await db.query(`
    SELECT *, sample_data
    FROM whitelabel_email_templates
    WHERE brand_id = $1
      AND template_name = $2
      AND language_code = $3
  `, [brandId, templateName, languageCode]);

  if (!template.sample_data) {
    throw new Error('Template has no sample data');
  }

  // Render with sample data
  const rendered = await renderEmailTemplate(
    templateName,
    brandId,
    template.sample_data.user || {
      name: 'John Doe',
      email: 'john@example.com'
    },
    template.sample_data.action || {}
  );

  return {
    subject: rendered.subject,
    html: rendered.html,
    text: rendered.text,
    preview_url: `/email-preview/${template.id}`
  };
}
```

### 5.5 HTML Rendering Best Practices

**Email-Safe HTML**:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <!--[if !mso]><!-->
  <meta http-equiv="X-UA-Compatible" content="IE=edge">
  <!--<![endif]-->
  <title>{{SUBJECT}}</title>
  <style>
    /* Inline styles for email compatibility */
    body {
      margin: 0;
      padding: 0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif;
      background-color: #f3f4f6;
    }
    .container {
      max-width: 600px;
      margin: 0 auto;
      background: #ffffff;
    }
    .button {
      display: inline-block;
      background-color: {{PRIMARY_COLOR}};
      color: #ffffff;
      padding: 12px 24px;
      text-decoration: none;
      border-radius: 6px;
    }
    /* Mobile responsive */
    @media only screen and (max-width: 600px) {
      .container {
        width: 100% !important;
      }
    }
  </style>
</head>
<body>
  <table width="100%" cellpadding="0" cellspacing="0" border="0">
    <tr>
      <td align="center" style="padding: 40px 20px;">
        <table class="container" cellpadding="0" cellspacing="0" border="0">
          <!-- Header -->
          <tr>
            <td style="background: {{PRIMARY_COLOR}}; padding: 30px; text-align: center;">
              <img src="{{LOGO_URL}}" alt="{{BRAND_NAME}}" width="200" style="max-width: 100%; height: auto;">
            </td>
          </tr>
          <!-- Content -->
          <tr>
            <td style="padding: 40px 30px;">
              <h1 style="margin: 0 0 20px; font-size: 24px; color: #111827;">
                {{EMAIL_TITLE}}
              </h1>
              <p style="margin: 0 0 20px; font-size: 16px; line-height: 1.5; color: #374151;">
                {{EMAIL_BODY}}
              </p>
              <!-- CTA Button -->
              <table cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td style="padding: 20px 0;">
                    <a href="{{ACTION_URL}}" class="button" style="background-color: {{PRIMARY_COLOR}}; color: #ffffff; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">
                      {{BUTTON_TEXT}}
                    </a>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <!-- Footer -->
          <tr>
            <td style="background: #f3f4f6; padding: 30px; text-align: center; font-size: 14px; color: #6b7280;">
              <p style="margin: 0 0 10px;">&copy; {{CURRENT_YEAR}} {{BRAND_NAME}}. All rights reserved.</p>
              <p style="margin: 0;">
                {{COMPANY_ADDRESS}}
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>
```

---

## 6. Theme System

### 6.1 Theme Architecture

```
┌────────────────────────────────────────────────────────────┐
│ CSS Variable-Based Theme System                            │
└────────────────────────────────────────────────────────────┘

Base Theme (CSS Variables)
  ├── Colors (JSONB → CSS vars)
  ├── Typography (JSONB → CSS vars)
  ├── Spacing (JSONB → CSS vars)
  ├── Borders (JSONB → CSS vars)
  └── Shadows (JSONB → CSS vars)
        ↓ compile
Generated CSS File
  └── Served via CDN or MinIO
        ↓ apply
User's Browser
  └── <link href="theme.css">
        ↓ override
Custom CSS (optional)
  └── Brand-specific overrides
```

### 6.2 Theme Compilation

```javascript
// Compile theme from JSONB to CSS
function compileTheme(theme) {
  const cssVariables = [];

  // Colors
  Object.entries(theme.colors).forEach(([key, value]) => {
    cssVariables.push(`  --color-${key}: ${value};`);
  });

  // Typography
  Object.entries(theme.typography).forEach(([key, value]) => {
    const cssKey = key.replace(/([A-Z])/g, '-$1').toLowerCase();
    cssVariables.push(`  --${cssKey}: ${value};`);
  });

  // Spacing
  Object.entries(theme.spacing).forEach(([key, value]) => {
    cssVariables.push(`  --spacing-${key}: ${value};`);
  });

  // Borders
  Object.entries(theme.borders).forEach(([key, value]) => {
    cssVariables.push(`  --border-${key}: ${value};`);
  });

  // Shadows
  Object.entries(theme.shadows).forEach(([key, value]) => {
    cssVariables.push(`  --shadow-${key}: ${value};`);
  });

  // Build CSS
  const css = `
/* Theme: ${theme.theme_name} */
/* Mode: ${theme.mode} */
/* Generated: ${new Date().toISOString()} */

[data-theme="${theme.theme_name}"],
.theme-${theme.theme_name} {
${cssVariables.join('\n')}
}

${theme.custom_css || ''}
`;

  return css;
}
```

**Store Compiled CSS**:

```javascript
async function saveCompiledTheme(themeId) {
  // 1. Load theme
  const theme = await db.query(`
    SELECT * FROM whitelabel_themes WHERE id = $1
  `, [themeId]);

  // 2. Compile
  const compiledCSS = compileTheme(theme);
  const minifiedCSS = minifyCSS(compiledCSS);

  // 3. Save to database
  await db.query(`
    UPDATE whitelabel_themes
    SET compiled_css = $1,
        updated_at = NOW()
    WHERE id = $2
  `, [minifiedCSS, themeId]);

  // 4. Upload to MinIO for CDN serving
  const brandId = theme.brand_id;
  const fileName = `themes/${theme.theme_name}.css`;
  await uploadAsset(`brand-${brandId}`, fileName, Buffer.from(minifiedCSS), {
    mimeType: 'text/css',
    cacheControl: 'public, max-age=31536000'
  });

  // 5. Invalidate cache
  await redis.del(`theme:${themeId}`);

  return { themeId, cssUrl: `https://cdn.yoursaas.com/brand-${brandId}/${fileName}` };
}
```

### 6.3 Dark Mode Support

**Automatic Dark Mode Generation**:

```javascript
// Generate dark mode variant from light theme
function generateDarkMode(lightTheme) {
  const darkColors = {
    // Invert backgrounds
    background: invertColor(lightTheme.colors.background),
    surface: invertColor(lightTheme.colors.surface),

    // Invert text
    text: invertColor(lightTheme.colors.text),

    // Keep brand colors but adjust brightness
    primary: adjustBrightness(lightTheme.colors.primary, 1.2),
    secondary: adjustBrightness(lightTheme.colors.secondary, 1.2),

    // Adjust borders
    border: adjustBrightness(lightTheme.colors.border, 0.6)
  };

  return {
    ...lightTheme,
    theme_name: `${lightTheme.theme_name}-dark`,
    mode: 'dark',
    colors: darkColors
  };
}
```

### 6.4 Component-Level Theming

**Button Component**:

```css
/* Buttons use theme variables */
.btn {
  font-family: var(--font-primary);
  font-size: var(--text-base);
  font-weight: var(--weight-semibold);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--border-radius);
  transition: all 150ms ease-in-out;
}

.btn-primary {
  background-color: var(--color-primary);
  color: var(--color-primary-contrast);
  border: 1px solid var(--color-primary-dark);
}

.btn-primary:hover {
  background-color: var(--color-primary-dark);
  box-shadow: var(--shadow-md);
}

.btn-secondary {
  background-color: var(--color-secondary);
  color: var(--color-secondary-contrast);
  border: 1px solid var(--color-secondary-dark);
}
```

### 6.5 Theme Preview Without Applying

```javascript
// Live theme preview in admin UI
function previewTheme(themeId) {
  const iframe = document.createElement('iframe');
  iframe.src = `/theme-preview?theme=${themeId}`;
  iframe.style.width = '100%';
  iframe.style.height = '600px';
  iframe.style.border = '1px solid #e5e7eb';

  document.getElementById('theme-preview-container').appendChild(iframe);
}

// Server-side theme preview route
app.get('/theme-preview', async (req, res) => {
  const theme = await loadTheme(req.query.theme);

  res.send(`
    <!DOCTYPE html>
    <html data-theme="${theme.theme_name}">
    <head>
      <style>${theme.compiled_css}</style>
    </head>
    <body>
      <div class="preview-container">
        <!-- Theme preview components -->
        <h1>Heading 1</h1>
        <p>Body text with <a href="#">link</a></p>
        <button class="btn-primary">Primary Button</button>
        <button class="btn-secondary">Secondary Button</button>
        <!-- More preview components -->
      </div>
    </body>
    </html>
  `);
});
```

### 6.6 Performance Considerations

**CSS Caching Strategy**:

```nginx
# Nginx configuration for theme CSS
location ~ ^/themes/(.+)\.css$ {
  proxy_pass http://minio:9000/themes/$1.css;

  # Cache for 1 year (immutable)
  add_header Cache-Control "public, max-age=31536000, immutable";

  # Gzip compression
  gzip on;
  gzip_types text/css;
  gzip_min_length 256;
}
```

**Minification**:

```javascript
// CSS minification for production
const CleanCSS = require('clean-css');

function minifyCSS(css) {
  const output = new CleanCSS({
    level: 2,
    returnPromise: false
  }).minify(css);

  return output.styles;
}
```

---

## 7. Multi-Tenant Branding

### 7.1 Tenant → Brand Relationship

```
tenants.tenants (Multi-Tenancy System)
  ├── id: tenant_id
  ├── slug: 'acme-corp'
  └── ...
        ↓ 1:1 relationship
whitelabel_brands
  ├── tenant_id: 'acme-corp'
  ├── brand_name: 'Acme Corp'
  ├── primary_color: '#E02424'
  └── ...
        ↓ 1:N relationships
whitelabel_domains, whitelabel_themes, whitelabel_email_templates, whitelabel_assets
```

### 7.2 Brand Inheritance Model

```javascript
// Load brand with inheritance from platform default
async function loadBrandWithInheritance(tenantId) {
  // 1. Load tenant-specific brand
  const tenantBrand = await db.query(`
    SELECT * FROM whitelabel_brands
    WHERE tenant_id = $1
  `, [tenantId]);

  // 2. Load platform default brand
  const platformBrand = await db.query(`
    SELECT * FROM whitelabel_brands
    WHERE tenant_id = 'default'
  `);

  // 3. Merge (tenant overrides platform)
  const mergedBrand = {
    ...platformBrand,
    ...tenantBrand,

    // Specific merge logic for complex fields
    colors: {
      ...platformBrand.colors,
      ...tenantBrand.colors
    },

    // Use tenant logo if set, otherwise platform logo
    logo_main_url: tenantBrand.logo_main_url || platformBrand.logo_main_url,
    logo_icon_url: tenantBrand.logo_icon_url || platformBrand.logo_icon_url,

    // Use tenant theme if active, otherwise platform theme
    active_theme_id: tenantBrand.active_theme_id || platformBrand.active_theme_id
  };

  return mergedBrand;
}
```

### 7.3 Domain-Based Tenant Resolution

**Nginx Lua Resolver**:

```lua
-- /etc/nginx/lua/tenant_resolver.lua

local redis = require "resty.redis"
local pgmoon = require "pgmoon"

local function resolve_tenant(domain)
  -- 1. Check Redis cache
  local red = redis:new()
  red:set_timeout(1000)
  local ok, err = red:connect("redis", 6379)

  if ok then
    local cached_tenant_id = red:get("domain:" .. domain)
    if cached_tenant_id and cached_tenant_id ~= ngx.null then
      red:close()
      return cached_tenant_id
    end
  end

  -- 2. Query PostgreSQL
  local pg = pgmoon.new({
    host = "postgres",
    port = 5432,
    database = "nself_db",
    user = "postgres",
    password = os.getenv("POSTGRES_PASSWORD")
  })

  pg:connect()

  local result = pg:query([[
    SELECT tenant_id
    FROM whitelabel_domains
    WHERE domain = ']] .. domain .. [['
      AND is_active = true
      AND status = 'active'
    LIMIT 1
  ]])

  pg:keepalive()

  if result and result[1] then
    local tenant_id = result[1].tenant_id

    -- 3. Cache in Redis (TTL: 1 hour)
    if red then
      red:setex("domain:" .. domain, 3600, tenant_id)
      red:close()
    end

    return tenant_id
  end

  return nil
end

-- Export function
return { resolve = resolve_tenant }
```

**Nginx Usage**:

```nginx
server {
  listen 443 ssl;
  server_name *.yoursaas.com;

  # Resolve tenant from domain
  set $tenant_id '';
  access_by_lua_block {
    local tenant_resolver = require "tenant_resolver"
    local domain = ngx.var.host

    local tenant_id = tenant_resolver.resolve(domain)

    if tenant_id then
      ngx.var.tenant_id = tenant_id
    else
      ngx.exit(404)
    end
  }

  # Pass tenant context to backend
  location / {
    proxy_set_header X-Tenant-ID $tenant_id;
    proxy_set_header X-Domain $host;
    proxy_pass http://nself-admin:8080;
  }
}
```

### 7.4 Tenant Isolation in Branding

**Row-Level Security**:

```sql
-- Enable RLS on whitelabel tables
ALTER TABLE whitelabel_brands ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only access their tenant's brand
CREATE POLICY brand_tenant_isolation ON whitelabel_brands
  FOR ALL
  USING (
    tenant_id = current_setting('hasura.user.x-hasura-tenant-id', true)
    OR
    current_setting('hasura.user.x-hasura-role', true) = 'admin'
  );

-- Apply to all whitelabel tables
ALTER TABLE whitelabel_domains ENABLE ROW LEVEL SECURITY;
CREATE POLICY domain_tenant_isolation ON whitelabel_domains
  FOR ALL
  USING (
    brand_id IN (
      SELECT id FROM whitelabel_brands
      WHERE tenant_id = current_setting('hasura.user.x-hasura-tenant-id', true)
    )
  );

-- Repeat for themes, email templates, assets
```

### 7.5 Subdomain vs Custom Domain Routing

```
┌──────────────────────────────────────────────────────────┐
│ Routing Strategy                                          │
└──────────────────────────────────────────────────────────┘

Subdomain (*.yoursaas.com):
  acme.yoursaas.com
    ├─ Extract: "acme" (subdomain)
    ├─ Query: SELECT id FROM tenants WHERE slug = 'acme'
    ├─ Query: SELECT * FROM whitelabel_brands WHERE tenant_id = {id}
    └─ Apply: Acme branding

Custom Domain (app.customer.com):
  app.customer.com
    ├─ Query: SELECT tenant_id FROM whitelabel_domains WHERE domain = 'app.customer.com'
    ├─ Query: SELECT * FROM whitelabel_brands WHERE tenant_id = {tenant_id}
    └─ Apply: Customer branding

Wildcard SSL:
  *.yoursaas.com → Single wildcard certificate
  app.customer.com → Individual certificate per domain
```

---

## 8. Asset Management

### 8.1 Asset Storage Architecture

```
┌──────────────────────────────────────────────────────────┐
│ Asset Storage & Delivery                                 │
└──────────────────────────────────────────────────────────┘

Asset Upload
  ├─ Validate (file type, size)
  ├─ Optimize (images: WebP conversion, resize)
  ├─ Store in MinIO (S3-compatible)
  ├─ Create metadata record (whitelabel_assets)
  └─ Cache on CDN (optional)

Asset Retrieval
  ├─ Query metadata (whitelabel_assets)
  ├─ Check CDN cache (HIT → serve, MISS → fetch)
  ├─ Fetch from MinIO
  ├─ Stream to client
  └─ Update CDN cache
```

### 8.2 MinIO Bucket Strategy

```javascript
// Bucket naming convention
const bucketName = `brand-${brandId}`;

// Folder structure within bucket
const assetPaths = {
  logos: 'logos/',
  fonts: 'fonts/',
  css: 'css/',
  themes: 'themes/',
  ssl: 'ssl/',          // SSL certificates
  email: 'email/',      // Email attachments
  uploads: 'uploads/'   // User uploads
};

// Example storage paths
// brand-550e8400.../logos/main-v1.svg
// brand-550e8400.../themes/acme-light.css
// brand-550e8400.../ssl/cert.pem
```

### 8.3 Asset Upload Flow

```javascript
async function uploadAsset(brandId, file, category) {
  // 1. Validate file
  const validation = validateFile(file, {
    maxSize: 10 * 1024 * 1024, // 10 MB
    allowedTypes: ['image/png', 'image/jpeg', 'image/svg+xml', 'text/css']
  });

  if (!validation.valid) {
    throw new Error(validation.error);
  }

  // 2. Optimize if image
  let processedFile = file;
  if (file.mimetype.startsWith('image/')) {
    processedFile = await optimizeImage(file, {
      maxWidth: category === 'logo' ? 600 : 1200,
      format: 'webp',
      quality: 90
    });
  }

  // 3. Generate unique filename
  const timestamp = Date.now();
  const ext = processedFile.originalname.split('.').pop();
  const fileName = `${category}/${timestamp}.${ext}`;

  // 4. Upload to MinIO
  const bucketName = `brand-${brandId}`;
  await minioClient.putObject(
    bucketName,
    fileName,
    processedFile.buffer,
    processedFile.size,
    {
      'Content-Type': processedFile.mimetype,
      'Cache-Control': 'public, max-age=31536000',
      'X-Brand-ID': brandId
    }
  );

  // 5. Generate access URL
  const accessUrl = `https://cdn.yoursaas.com/${bucketName}/${fileName}`;

  // 6. Create asset record
  const asset = await db.query(`
    INSERT INTO whitelabel_assets (
      brand_id, asset_name, asset_type, asset_category,
      file_name, file_path, file_size, mime_type,
      storage_provider, storage_bucket, storage_key,
      cdn_url, access_url, is_public
    ) VALUES (
      $1, $2, $3, $4, $5, $6, $7, $8, 'minio', $9, $10, $11, $12, true
    )
    RETURNING id, access_url
  `, [
    brandId,
    file.originalname,
    getAssetType(file.mimetype),
    category,
    file.originalname,
    `/${bucketName}/${fileName}`,
    processedFile.size,
    processedFile.mimetype,
    bucketName,
    fileName,
    accessUrl,
    accessUrl
  ]);

  return asset;
}
```

### 8.4 Image Optimization

```javascript
const sharp = require('sharp');

async function optimizeImage(file, options = {}) {
  const {
    maxWidth = 1200,
    maxHeight = null,
    format = 'webp',
    quality = 85
  } = options;

  let image = sharp(file.buffer);

  // Get metadata
  const metadata = await image.metadata();

  // Resize if needed
  if (metadata.width > maxWidth) {
    image = image.resize(maxWidth, maxHeight, {
      fit: 'inside',
      withoutEnlargement: true
    });
  }

  // Convert format
  if (format === 'webp') {
    image = image.webp({ quality });
  } else if (format === 'jpeg') {
    image = image.jpeg({ quality, progressive: true });
  } else if (format === 'png') {
    image = image.png({ compressionLevel: 9 });
  }

  // Get optimized buffer
  const buffer = await image.toBuffer();

  return {
    buffer,
    size: buffer.length,
    mimetype: `image/${format}`,
    originalname: file.originalname.replace(/\.[^.]+$/, `.${format}`)
  };
}
```

### 8.5 CDN Integration

**CloudFront / CloudFlare Configuration**:

```javascript
// CDN URL generation
function getCDNUrl(assetPath, brandId) {
  const cdnDomain = process.env.CDN_DOMAIN || 'cdn.yoursaas.com';
  const bucketName = `brand-${brandId}`;

  return `https://${cdnDomain}/${bucketName}/${assetPath}`;
}

// Cache invalidation
async function invalidateCDNCache(assetPath, brandId) {
  const cdnUrl = getCDNUrl(assetPath, brandId);

  // CloudFlare example
  if (process.env.CDN_PROVIDER === 'cloudflare') {
    await fetch(`https://api.cloudflare.com/client/v4/zones/${process.env.CLOUDFLARE_ZONE_ID}/purge_cache`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${process.env.CLOUDFLARE_API_TOKEN}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        files: [cdnUrl]
      })
    });
  }
}
```

### 8.6 Asset Versioning

```javascript
// Version assets to prevent cache issues
async function versionAsset(assetId) {
  // 1. Load current asset
  const current = await db.query(`
    SELECT * FROM whitelabel_assets WHERE id = $1
  `, [assetId]);

  // 2. Create new version
  const newAsset = await db.query(`
    INSERT INTO whitelabel_assets (
      brand_id, asset_name, asset_type, asset_category,
      file_name, file_path, file_size, mime_type,
      storage_provider, storage_bucket, storage_key,
      version, previous_version_id
    )
    SELECT
      brand_id, asset_name, asset_type, asset_category,
      file_name, file_path, file_size, mime_type,
      storage_provider, storage_bucket, storage_key,
      version + 1, id
    FROM whitelabel_assets
    WHERE id = $1
    RETURNING id, version
  `, [assetId]);

  // 3. Update references to point to new version
  // (e.g., update whitelabel_brands.logo_main_id)

  return newAsset;
}
```

---

## 9. Security

### 9.1 CSS Sanitization

**Prevent XSS via Custom CSS**:

```javascript
const cssTree = require('css-tree');

function sanitizeCSS(css) {
  try {
    // Parse CSS
    const ast = cssTree.parse(css);

    // Remove dangerous patterns
    cssTree.walk(ast, {
      visit: 'Declaration',
      enter(node) {
        // Block javascript: URLs
        if (node.value && node.value.type === 'Url') {
          const url = cssTree.generate(node.value);
          if (url.match(/javascript:|data:/i)) {
            this.remove();
          }
        }

        // Block expression()
        if (cssTree.generate(node).match(/expression\(/i)) {
          this.remove();
        }

        // Block imports
        if (node.property === 'import' || node.property === '@import') {
          this.remove();
        }
      }
    });

    // Generate sanitized CSS
    return cssTree.generate(ast);
  } catch (error) {
    throw new Error('Invalid CSS syntax');
  }
}
```

### 9.2 Domain Verification Security

**Prevent Domain Hijacking**:

```javascript
// Verification token generation
function generateVerificationToken() {
  const crypto = require('crypto');
  return crypto.randomBytes(32).toString('hex');
}

// Token expiry (24 hours)
async function checkVerificationTokenExpiry(domain) {
  const result = await db.query(`
    SELECT created_at
    FROM whitelabel_domains
    WHERE domain = $1
      AND dns_verified = false
  `, [domain]);

  if (!result) return false;

  const createdAt = new Date(result.created_at);
  const now = new Date();
  const hoursSinceCreation = (now - createdAt) / (1000 * 60 * 60);

  return hoursSinceCreation <= 24;
}

// Verification with rate limiting
const verificationAttempts = new Map();

async function verifyDomainWithRateLimit(domain) {
  const attempts = verificationAttempts.get(domain) || 0;

  if (attempts >= 5) {
    throw new Error('Too many verification attempts. Please wait 1 hour.');
  }

  const result = await verifyDomain(domain);

  if (!result.success) {
    verificationAttempts.set(domain, attempts + 1);

    // Clear after 1 hour
    setTimeout(() => {
      verificationAttempts.delete(domain);
    }, 60 * 60 * 1000);
  } else {
    verificationAttempts.delete(domain);
  }

  return result;
}
```

### 9.3 SSL Private Key Security

**Encrypt Private Keys**:

```javascript
const crypto = require('crypto');

// Encryption key from environment
const ENCRYPTION_KEY = Buffer.from(process.env.SSL_KEY_ENCRYPTION_KEY, 'hex');

function encryptPrivateKey(privateKey) {
  const iv = crypto.randomBytes(16);
  const cipher = crypto.createCipheriv('aes-256-gcm', ENCRYPTION_KEY, iv);

  let encrypted = cipher.update(privateKey, 'utf8', 'hex');
  encrypted += cipher.final('hex');

  const authTag = cipher.getAuthTag();

  return {
    encrypted,
    iv: iv.toString('hex'),
    authTag: authTag.toString('hex')
  };
}

function decryptPrivateKey(encrypted, iv, authTag) {
  const decipher = crypto.createDecipheriv(
    'aes-256-gcm',
    ENCRYPTION_KEY,
    Buffer.from(iv, 'hex')
  );

  decipher.setAuthTag(Buffer.from(authTag, 'hex'));

  let decrypted = decipher.update(encrypted, 'hex', 'utf8');
  decrypted += decipher.final('utf8');

  return decrypted;
}

// Store encrypted in database
async function storeSSLKey(domain, privateKey) {
  const { encrypted, iv, authTag } = encryptPrivateKey(privateKey);

  await db.query(`
    INSERT INTO whitelabel_assets (
      brand_id, asset_type, asset_category,
      file_name, storage_provider,
      encrypted_content, encryption_iv, encryption_auth_tag
    ) VALUES ($1, 'key', 'ssl', $2, 'database', $3, $4, $5)
  `, [brandId, `${domain}.key`, encrypted, iv, authTag]);
}
```

### 9.4 Asset Access Control

**Signed URLs for Private Assets**:

```javascript
function generateSignedURL(assetId, expiresIn = 3600) {
  const crypto = require('crypto');

  const payload = {
    asset_id: assetId,
    expires: Date.now() + (expiresIn * 1000)
  };

  const signature = crypto
    .createHmac('sha256', process.env.ASSET_SIGNING_KEY)
    .update(JSON.stringify(payload))
    .digest('hex');

  const token = Buffer.from(JSON.stringify({ ...payload, signature })).toString('base64');

  return `/assets/${assetId}?token=${token}`;
}

// Verify signed URL
function verifySignedURL(assetId, token) {
  try {
    const payload = JSON.parse(Buffer.from(token, 'base64').toString());

    // Check expiry
    if (Date.now() > payload.expires) {
      return false;
    }

    // Verify signature
    const expectedSignature = crypto
      .createHmac('sha256', process.env.ASSET_SIGNING_KEY)
      .update(JSON.stringify({ asset_id: payload.asset_id, expires: payload.expires }))
      .digest('hex');

    return payload.signature === expectedSignature && payload.asset_id === assetId;
  } catch (error) {
    return false;
  }
}
```

### 9.5 Audit Logging

```sql
-- Audit log table
CREATE TABLE whitelabel_audit_log (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  brand_id UUID REFERENCES whitelabel_brands(id),
  action VARCHAR(100) NOT NULL,
  resource_type VARCHAR(100),
  resource_id UUID,
  changes JSONB,
  user_id UUID,
  ip_address INET,
  user_agent TEXT,
  timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_audit_brand ON whitelabel_audit_log(brand_id);
CREATE INDEX idx_audit_timestamp ON whitelabel_audit_log(timestamp);
```

**Audit Trigger**:

```sql
CREATE OR REPLACE FUNCTION log_whitelabel_changes()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO whitelabel_audit_log (
    brand_id, action, resource_type, resource_id, changes, user_id
  ) VALUES (
    COALESCE(NEW.brand_id, OLD.brand_id),
    TG_OP,
    TG_TABLE_NAME,
    COALESCE(NEW.id, OLD.id),
    jsonb_build_object(
      'old', row_to_json(OLD),
      'new', row_to_json(NEW)
    ),
    current_setting('hasura.user.x-hasura-user-id', true)::uuid
  );

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply to all whitelabel tables
CREATE TRIGGER audit_whitelabel_brands
  AFTER INSERT OR UPDATE OR DELETE ON whitelabel_brands
  FOR EACH ROW EXECUTE FUNCTION log_whitelabel_changes();

-- Repeat for other tables
```

---

## 10. Performance

### 10.1 Caching Strategy

**Redis Cache Layers**:

```javascript
// Cache TTLs
const CACHE_TTL = {
  brand: 3600,          // 1 hour
  domain: 3600,         // 1 hour
  theme: 86400,         // 24 hours
  email_template: 3600, // 1 hour
  asset_metadata: 86400 // 24 hours
};

// Cache keys
const CACHE_KEYS = {
  brand: (brandId) => `brand:${brandId}`,
  domain: (domain) => `domain:${domain}`,
  theme: (themeId) => `theme:${themeId}`,
  emailTemplate: (brandId, name, lang) => `email:${brandId}:${name}:${lang}`,
  asset: (assetId) => `asset:${assetId}`
};

// Cache with fallback
async function getCachedBrand(brandId) {
  const cacheKey = CACHE_KEYS.brand(brandId);

  // 1. Try cache
  const cached = await redis.get(cacheKey);
  if (cached) {
    return JSON.parse(cached);
  }

  // 2. Query database
  const brand = await db.query(`
    SELECT * FROM whitelabel_brands_full WHERE id = $1
  `, [brandId]);

  if (!brand) {
    return null;
  }

  // 3. Cache result
  await redis.setex(cacheKey, CACHE_TTL.brand, JSON.stringify(brand));

  return brand;
}
```

### 10.2 Database Query Optimization

**Indexes**:

```sql
-- Critical indexes for performance
CREATE INDEX CONCURRENTLY idx_whitelabel_domains_lookup
  ON whitelabel_domains(domain, is_active, status)
  WHERE is_active = true AND status = 'active';

CREATE INDEX CONCURRENTLY idx_whitelabel_brands_tenant_active
  ON whitelabel_brands(tenant_id, is_active)
  WHERE is_active = true;

CREATE INDEX CONCURRENTLY idx_whitelabel_email_templates_lookup
  ON whitelabel_email_templates(brand_id, template_name, language_code, is_active)
  WHERE is_active = true;

-- Partial indexes for common queries
CREATE INDEX CONCURRENTLY idx_whitelabel_domains_ssl_renewal
  ON whitelabel_domains(ssl_expiry_date)
  WHERE ssl_enabled = true
    AND ssl_auto_renew = true
    AND status = 'active';
```

**Query Optimization**:

```sql
-- Use JOIN instead of subqueries
-- ❌ Slow
SELECT * FROM whitelabel_brands
WHERE id IN (
  SELECT brand_id FROM whitelabel_domains
  WHERE domain = 'acme.yoursaas.com'
);

-- ✅ Fast
SELECT b.*
FROM whitelabel_brands b
JOIN whitelabel_domains d ON b.id = d.brand_id
WHERE d.domain = 'acme.yoursaas.com'
  AND d.is_active = true
LIMIT 1;
```

### 10.3 CSS Minification and Compression

```javascript
// Minify and gzip CSS
const zlib = require('zlib');

async function compileAndCompressTheme(themeId) {
  const theme = await loadTheme(themeId);

  // 1. Compile
  const compiledCSS = compileTheme(theme);

  // 2. Minify
  const minified = minifyCSS(compiledCSS);

  // 3. Gzip
  const gzipped = await new Promise((resolve, reject) => {
    zlib.gzip(minified, { level: 9 }, (err, result) => {
      if (err) reject(err);
      else resolve(result);
    });
  });

  // 4. Store both versions
  await db.query(`
    UPDATE whitelabel_themes
    SET compiled_css = $1,
        compiled_css_size = $2,
        compiled_css_gzip_size = $3
    WHERE id = $4
  `, [minified, Buffer.byteLength(minified), gzipped.length, themeId]);

  return { minified, gzipped };
}
```

### 10.4 CDN Configuration

**Nginx CDN Headers**:

```nginx
location /branding/ {
  proxy_pass http://minio:9000/;

  # Cache for 1 year
  add_header Cache-Control "public, max-age=31536000, immutable";

  # Gzip compression
  gzip on;
  gzip_types text/css image/svg+xml;
  gzip_min_length 256;

  # Vary header for proper caching
  add_header Vary "Accept-Encoding";

  # CORS headers
  add_header Access-Control-Allow-Origin "*";
}
```

### 10.5 Asset Loading Strategy

**Lazy Loading**:

```html
<!-- Preload critical assets -->
<link rel="preload" href="{{logo_url}}" as="image">
<link rel="preload" href="{{theme_css_url}}" as="style">

<!-- Load theme CSS -->
<link rel="stylesheet" href="{{theme_css_url}}">

<!-- Lazy load non-critical assets -->
<img src="{{logo_url}}" loading="lazy" alt="{{brand_name}}">
```

**Progressive Enhancement**:

```html
<!-- Inline critical CSS -->
<style>
  /* Critical above-the-fold styles */
  :root {
    --color-primary: {{primary_color}};
  }
  .header {
    background: var(--color-primary);
  }
</style>

<!-- Async load full theme -->
<link rel="stylesheet" href="{{theme_css_url}}" media="print" onload="this.media='all'">
```

---

## 11. API Integration

### 11.1 GraphQL API

**Hasura Metadata for White-Label Tables**:

```yaml
# tables.yaml
- table:
    schema: public
    name: whitelabel_brands
  select_permissions:
    - role: user
      permission:
        columns:
          - id
          - brand_name
          - tagline
          - primary_color
          - secondary_color
          - accent_color
          - logo_main_url
          - logo_icon_url
        filter:
          _or:
            - tenant_id:
                _eq: X-Hasura-Tenant-Id
            - is_public:
                _eq: true
```

**GraphQL Queries**:

```graphql
# Get brand for current tenant
query GetBrand {
  whitelabel_brands(limit: 1) {
    id
    brand_name
    tagline
    primary_color
    secondary_color
    accent_color
    logo_main_url
    logo_icon_url
    active_theme {
      theme_name
      compiled_css
      mode
    }
    domains(where: { is_active: { _eq: true } }) {
      domain
      is_primary
      ssl_enabled
    }
  }
}

# Get email template
query GetEmailTemplate($template_name: String!, $language: String = "en") {
  whitelabel_email_templates(
    where: {
      template_name: { _eq: $template_name }
      language_code: { _eq: $language }
      is_active: { _eq: true }
    }
    limit: 1
  ) {
    subject
    html_content
    text_content
    variables
  }
}
```

### 11.2 REST API Endpoints

```javascript
// Express.js routes
const express = require('express');
const router = express.Router();

// Get brand by domain
router.get('/api/brand/by-domain/:domain', async (req, res) => {
  const { domain } = req.params;

  const brand = await getBrandByDomain(domain);

  if (!brand) {
    return res.status(404).json({ error: 'Brand not found' });
  }

  res.json(brand);
});

// Upload logo
router.post('/api/brand/:id/logo', upload.single('file'), async (req, res) => {
  const { id } = req.params;
  const { file } = req;
  const logoType = req.body.type || 'main';

  const asset = await uploadLogo(id, file, logoType);

  res.json(asset);
});

// Update theme
router.put('/api/brand/:id/theme/:themeId', async (req, res) => {
  const { id, themeId } = req.params;
  const { colors, typography, custom_css } = req.body;

  const updated = await updateTheme(id, themeId, {
    colors,
    typography,
    custom_css
  });

  res.json(updated);
});
```

### 11.3 Webhook Notifications

```javascript
// Webhook for domain verification
async function notifyDomainVerified(domain) {
  const webhookUrl = process.env.WEBHOOK_URL;

  if (!webhookUrl) return;

  await fetch(webhookUrl, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      event: 'domain.verified',
      data: {
        domain,
        verified_at: new Date().toISOString()
      }
    })
  });
}

// Webhook for SSL expiry warning
async function notifySSLExpiringSoon(domain, expiryDate) {
  const webhookUrl = process.env.WEBHOOK_URL;

  if (!webhookUrl) return;

  await fetch(webhookUrl, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      event: 'ssl.expiring_soon',
      data: {
        domain,
        expiry_date: expiryDate,
        days_remaining: Math.floor((new Date(expiryDate) - new Date()) / (1000 * 60 * 60 * 24))
      }
    })
  });
}
```

---

## 12. Deployment Architecture

### 12.1 Production Deployment Diagram

```
┌─────────────────────────────────────────────────────────────┐
│ Production White-Label Deployment                            │
└─────────────────────────────────────────────────────────────┘

Internet
  ↓
CloudFlare (CDN + DDoS Protection)
  ↓
Load Balancer (HAProxy or AWS ALB)
  ├─ Health checks
  ├─ SSL termination
  └─ Round-robin to Nginx instances
      ↓
Nginx Cluster (3+ instances)
  ├─ Tenant resolution (Lua)
  ├─ SSL certificate serving
  ├─ Static asset serving
  └─ Proxy to application
      ↓
nself Application Cluster
  ├─ nself-admin (React frontend)
  ├─ Hasura GraphQL Engine
  ├─ Auth service
  └─ Custom services
      ↓
Data Layer
  ├─ PostgreSQL (Primary + Read Replicas)
  ├─ Redis Cluster (Caching)
  └─ MinIO Cluster (S3-compatible storage)
```

### 12.2 High Availability Configuration

**PostgreSQL Replication**:

```
Primary (Read/Write)
  ├─ Streaming replication →
  ├─ Replica 1 (Read-only)
  └─ Replica 2 (Read-only)

Automatic failover with patroni/stolon
```

**Redis Sentinel**:

```
Redis Master
  ├─ Replication →
  ├─ Replica 1
  └─ Replica 2

Sentinel Cluster (3 nodes)
  └─ Automatic master election
```

**MinIO Distributed Mode**:

```
MinIO Cluster (4+ nodes)
  ├─ Node 1: /data1, /data2, /data3, /data4
  ├─ Node 2: /data1, /data2, /data3, /data4
  ├─ Node 3: /data1, /data2, /data3, /data4
  └─ Node 4: /data1, /data2, /data3, /data4

Erasure coding: N/2 data, N/2 parity
```

### 12.3 Scaling Considerations

**Horizontal Scaling**:

- **Nginx**: Add more instances behind load balancer
- **Application**: Stateless containers, scale based on CPU/memory
- **PostgreSQL**: Read replicas for read-heavy workloads
- **Redis**: Redis Cluster for distributed caching
- **MinIO**: Add more nodes to cluster

**Vertical Scaling**:

- **PostgreSQL**: Increase resources for write-heavy workloads
- **Redis**: Increase memory for larger caches
- **Nginx**: Increase worker processes

### 12.4 Monitoring and Observability

**Metrics to Track**:

```yaml
White-Label Metrics:
  - brand_active_count: Number of active brands
  - domain_verified_count: Verified custom domains
  - domain_health_check_failures: Failed health checks
  - ssl_expiring_soon_count: Certificates expiring in 30 days
  - theme_compilation_duration: Time to compile themes
  - email_template_render_duration: Email render time
  - asset_upload_count: Asset uploads per day
  - cdn_cache_hit_rate: CDN cache efficiency

Application Metrics:
  - request_duration: P50, P95, P99
  - request_count: By endpoint
  - error_rate: 4xx, 5xx errors
  - database_query_duration: Slow query tracking
```

**Prometheus Configuration**:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'whitelabel'
    static_configs:
      - targets: ['nself-admin:9090']
    metrics_path: '/metrics'
    relabel_configs:
      - source_labels: [__address__]
        target_label: instance
```

**Grafana Dashboard**:

```json
{
  "dashboard": {
    "title": "White-Label System",
    "panels": [
      {
        "title": "Active Brands",
        "targets": [
          {
            "expr": "sum(whitelabel_brands_active)"
          }
        ]
      },
      {
        "title": "SSL Expiry Warnings",
        "targets": [
          {
            "expr": "sum(whitelabel_ssl_expiring_soon)"
          }
        ]
      }
    ]
  }
}
```

---

## Conclusion

The nself white-label system provides a complete, production-ready architecture for multi-tenant SaaS platforms with comprehensive branding customization.

### Key Features

1. **Complete Brand Isolation** - Per-tenant branding with inheritance
2. **Custom Domains & SSL** - Automated provisioning and renewal
3. **Email Templates** - Multi-language with variable injection
4. **Theme System** - CSS variables with dark/light modes
5. **Asset Management** - S3-compatible storage with CDN
6. **High Performance** - Redis caching, CDN integration, optimized queries
7. **Enterprise Security** - RLS, encryption, audit logging
8. **Production Ready** - HA, scaling, monitoring

### Implementation Checklist

- [x] Database schema (5 tables, views, indexes)
- [x] Brand customization (logos, colors, fonts, CSS)
- [x] Custom domain system (DNS verification, SSL provisioning)
- [x] Email template engine (multi-language, variable injection)
- [x] Theme compiler (CSS variables, dark mode)
- [x] Multi-tenant branding (inheritance, isolation)
- [x] Asset management (MinIO, CDN, optimization)
- [x] Security (sanitization, encryption, RLS)
- [x] Performance (caching, minification, compression)
- [x] API integration (GraphQL, REST, webhooks)
- [x] Deployment architecture (HA, scaling, monitoring)

---

**Version**: nself v0.9.0
**Last Updated**: January 30, 2026
**Sprint**: 14 - White-Label & Customization (60 story points)
**Status**: Production Ready
