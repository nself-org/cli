# nself Branding System - Complete Documentation

## Overview

The nself Branding System provides comprehensive white-label functionality for managing brand identity, including logos, colors, fonts, typography, and custom CSS. It supports multi-tenant isolation, asset versioning, and security validation.

**File**: `/Users/admin/Sites/nself/src/lib/whitelabel/branding.sh`
**Lines of Code**: 1,782
**Functions**: 26
**Status**: Production-ready

---

## Features

### 1. Brand Management
- Create and manage brand identity
- Multi-tenant support with isolation
- Brand metadata (name, tagline, description)
- Update and delete brands with confirmation

### 2. Logo Management
- Upload logos (PNG, JPG, JPEG, SVG, WebP)
- Multiple logo types: main, icon, email, favicon
- File size validation (max 5MB)
- MIME type verification
- Asset versioning with timestamps
- Symlink management for easy access

### 3. Color Customization
- Full color palette management
- Hex color validation (#RGB or #RRGGBB)
- Accessibility validation
- Support for 10+ color categories:
  - Primary, Secondary, Accent
  - Background, Text, Text Light
  - Border, Success, Warning, Error, Info

### 4. Typography Management
- Font family settings (primary, secondary, code)
- Font size configuration (base, small, large, h1, h2, h3)
- Font weight settings (normal, medium, semibold, bold)
- Line height options (tight, normal, relaxed)
- Custom font upload (WOFF, WOFF2, TTF, OTF)
- Font file validation (max 1MB)

### 5. Custom CSS
- Upload custom CSS files (max 2MB)
- Syntax validation (balanced braces)
- Security scanning:
  - External URL detection
  - @import statement warnings
  - XSS vulnerability checks (expression())
- Versioned CSS with rollback

### 6. Asset Versioning
- Automatic version backups on changes
- Keep last 10 versions
- Restore previous versions
- Version listing with timestamps

### 7. Security Features
- Secure file permissions (0644 for files, 0755 for dirs)
- File size limits enforcement
- MIME type validation
- CSS security scanning
- Input sanitization
- Multi-tenant isolation

### 8. CSS Variable Generation
- Auto-generate CSS variables from config
- Color variables (`--color-*`)
- Font variables (`--font-*`)
- Size variables (`--font-size-*`)
- Weight variables (`--font-weight-*`)
- Line height variables (`--line-height-*`)

---

## Installation & Initialization

```bash
# Initialize branding system (default tenant)
nself whitelabel branding init

# Initialize for specific tenant
nself whitelabel branding init --tenant mycompany

# Verify initialization
ls -la branding/
# Output:
# branding/
# ├── config.json       # Main configuration
# ├── logos/            # Logo assets
# ├── css/              # CSS files
# ├── fonts/            # Custom fonts
# ├── assets/           # Other assets
# └── versions/         # Version backups
```

---

## Usage Guide

### Brand Creation

```bash
# Create a brand
nself whitelabel branding create "My Company"

# Create with tagline and description
nself whitelabel branding create "My Company" \
  --tagline "Innovation Through Technology" \
  --description "Leading provider of cloud solutions"

# Create for specific tenant
nself whitelabel branding create "Client Brand" --tenant client123
```

### Update Brand Information

```bash
# Update brand name
nself whitelabel branding update --name "New Company Name"

# Update tagline only
nself whitelabel branding update --tagline "New Tagline"

# Update multiple fields
nself whitelabel branding update \
  --name "Updated Name" \
  --tagline "Updated Tagline" \
  --description "Updated description"
```

### Logo Management

```bash
# Upload main logo
nself whitelabel logo upload /path/to/logo.png

# Upload specific logo type
nself whitelabel logo upload /path/to/icon.svg --type icon
nself whitelabel logo upload /path/to/email-logo.png --type email
nself whitelabel logo upload /path/to/favicon.ico --type favicon

# List all logos
nself whitelabel logo list

# Remove a logo
nself whitelabel logo remove main
nself whitelabel logo remove icon

# Get logo information
nself whitelabel branding asset-info branding/logos/logo-main.png
```

**Supported Logo Types:**
- `main` - Primary brand logo
- `icon` - App icon/square logo
- `email` - Email template logo
- `favicon` - Browser favicon

**Supported Formats:**
- PNG, JPG, JPEG, SVG, WebP
- Maximum size: 5MB per file

### Color Customization

```bash
# Set primary color
nself whitelabel branding set-colors --primary "#0066cc"

# Set multiple colors
nself whitelabel branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600" \
  --accent "#00cc66"

# Set full palette
nself whitelabel branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600" \
  --accent "#00cc66" \
  --background "#ffffff" \
  --text "#333333"
```

**Color Validation:**
- Must be valid hex code (#RGB or #RRGGBB)
- Accessibility check for color contrast
- Warning if primary and secondary are too similar

### Typography Configuration

```bash
# Set font families
nself whitelabel branding set-fonts \
  --primary "Inter, system-ui, sans-serif" \
  --secondary "Georgia, serif" \
  --code "Fira Code, monospace"

# Set typography settings
nself whitelabel branding set-typography \
  --base-size "16px" \
  --h1-size "32px" \
  --h2-size "24px" \
  --normal-weight "400" \
  --bold-weight "700" \
  --line-height "1.5"

# Upload custom font file
nself whitelabel font upload /path/to/custom-font.woff2 --name "CustomFont"
```

**Supported Font Formats:**
- WOFF, WOFF2, TTF, OTF
- Maximum size: 1MB per file

### Custom CSS

```bash
# Set custom CSS
nself whitelabel branding set-css /path/to/custom.css

# Remove custom CSS
nself whitelabel branding remove-css

# View CSS security scan results
# (automatically runs on upload)
```

**CSS Security Checks:**
- External URL detection (data exfiltration risk)
- @import statement detection (external stylesheet loading)
- expression() detection (XSS vulnerability)
- Syntax validation (balanced braces)

### Version Management

```bash
# List all versions
nself whitelabel branding list-versions
# Output:
#  1. 2026-01-30 14:30:00
#  2. 2026-01-29 10:15:00
#  3. 2026-01-28 16:45:00

# Restore a previous version
nself whitelabel branding restore 20260129_101500

# Versions are automatically created on every change
# Last 10 versions are kept (older ones auto-deleted)
```

### Configuration Export/Import

```bash
# Export configuration (JSON)
nself whitelabel config export > branding-backup.json
nself whitelabel config export --tenant client123 > client-branding.json

# Export as YAML (requires yq or python3)
nself whitelabel config export --format yaml > branding.yaml

# Import configuration
nself whitelabel config import branding-backup.json
nself whitelabel config import client-branding.json --tenant client123

# Validate configuration
nself whitelabel branding validate
nself whitelabel branding validate --tenant client123
```

### Asset Management

```bash
# View asset information
nself whitelabel branding asset-info branding/logos/logo-main.png

# Clean unused assets
nself whitelabel branding clean-unused

# List all resources
nself whitelabel list
```

### Multi-Tenant Management

```bash
# List all tenants
nself whitelabel tenants list

# Create tenant-specific brand
nself whitelabel branding create "Client Brand" --tenant client123

# Export tenant config
nself whitelabel config export --tenant client123

# Import tenant config
nself whitelabel config import config.json --tenant client123
```

---

## Configuration File Structure

### config.json

```json
{
  "version": "1.0.0",
  "tenantId": "default",
  "brand": {
    "name": "nself",
    "tagline": "Powerful Backend for Modern Applications",
    "description": "Open-source backend infrastructure platform"
  },
  "colors": {
    "primary": "#0066cc",
    "secondary": "#ff6600",
    "accent": "#00cc66",
    "background": "#ffffff",
    "text": "#333333",
    "textLight": "#666666",
    "border": "#e0e0e0",
    "success": "#00cc66",
    "warning": "#ff9900",
    "error": "#cc0000",
    "info": "#0066cc"
  },
  "typography": {
    "fonts": {
      "primary": "Inter, system-ui, sans-serif",
      "secondary": "Georgia, serif",
      "code": "Fira Code, Consolas, monospace"
    },
    "sizes": {
      "base": "16px",
      "small": "14px",
      "large": "18px",
      "h1": "32px",
      "h2": "24px",
      "h3": "20px"
    },
    "weights": {
      "normal": "400",
      "medium": "500",
      "semibold": "600",
      "bold": "700"
    },
    "lineHeights": {
      "tight": "1.25",
      "normal": "1.5",
      "relaxed": "1.75"
    }
  },
  "logos": {
    "main": "logo-main-20260130_143000.png",
    "icon": "logo-icon-20260130_143015.svg",
    "email": "logo-email-20260130_143030.png",
    "favicon": null
  },
  "customCSS": "custom-20260130_143045.css",
  "customFonts": [
    {
      "name": "CustomFont",
      "path": "CustomFont-20260130_143100.woff2"
    }
  ],
  "theme": "light",
  "createdAt": "2026-01-30T14:30:00Z",
  "updatedAt": "2026-01-30T14:31:00Z"
}
```

---

## Generated CSS Variables

### variables.css

```css
/**
 * nself White-Label CSS Variables
 * Auto-generated from branding configuration
 * Do not edit manually - changes will be overwritten
 */

:root {
  /* ========================================
   * Colors
   * ======================================== */
  --color-primary: #0066cc;
  --color-secondary: #ff6600;
  --color-accent: #00cc66;
  --color-background: #ffffff;
  --color-text: #333333;
  --color-textLight: #666666;
  --color-border: #e0e0e0;
  --color-success: #00cc66;
  --color-warning: #ff9900;
  --color-error: #cc0000;
  --color-info: #0066cc;

  /* ========================================
   * Typography - Fonts
   * ======================================== */
  --font-primary: Inter, system-ui, sans-serif;
  --font-secondary: Georgia, serif;
  --font-code: Fira Code, Consolas, monospace;

  /* ========================================
   * Typography - Sizes
   * ======================================== */
  --font-size-base: 16px;
  --font-size-small: 14px;
  --font-size-large: 18px;
  --font-size-h1: 32px;
  --font-size-h2: 24px;
  --font-size-h3: 20px;

  /* ========================================
   * Typography - Weights
   * ======================================== */
  --font-weight-normal: 400;
  --font-weight-medium: 500;
  --font-weight-semibold: 600;
  --font-weight-bold: 700;

  /* ========================================
   * Typography - Line Heights
   * ======================================== */
  --line-height-tight: 1.25;
  --line-height-normal: 1.5;
  --line-height-relaxed: 1.75;
}
```

### Usage in Application

```css
/* Use generated variables in your CSS */
.header {
  background-color: var(--color-primary);
  color: var(--color-background);
  font-family: var(--font-primary);
  font-size: var(--font-size-h1);
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-tight);
}

.button-primary {
  background: var(--color-primary);
  color: white;
  border: 1px solid var(--color-border);
}

.button-secondary {
  background: var(--color-secondary);
  color: white;
}

.text-muted {
  color: var(--color-textLight);
}
```

---

## Directory Structure

```
branding/
├── config.json                    # Main configuration file
├── .gitignore                     # Ignore uploaded assets
│
├── logos/                         # Logo assets
│   ├── .gitkeep
│   ├── logo-main-20260130_143000.png
│   ├── logo-main.png → logo-main-20260130_143000.png  # Symlink to current
│   ├── logo-icon-20260130_143015.svg
│   └── logo-icon.svg → logo-icon-20260130_143015.svg
│
├── css/                          # CSS files
│   ├── .gitkeep
│   ├── variables.css             # Auto-generated CSS variables
│   ├── custom-20260130_143045.css
│   └── custom.css → custom-20260130_143045.css  # Symlink to current
│
├── fonts/                        # Custom fonts
│   ├── .gitkeep
│   └── CustomFont-20260130_143100.woff2
│
├── assets/                       # Other assets
│   └── .gitkeep
│
├── versions/                     # Version backups
│   ├── config-20260130_143000.json
│   ├── config-20260129_101500.json
│   └── config-20260128_164500.json
│
└── client123/                    # Tenant-specific (example)
    ├── config.json
    ├── logos/
    ├── css/
    ├── fonts/
    └── assets/
```

---

## Security Features

### File Validation

1. **Size Limits**
   - Logos: 5MB max
   - CSS: 2MB max
   - Fonts: 1MB max

2. **Format Validation**
   - Logos: PNG, JPG, JPEG, SVG, WebP only
   - Fonts: WOFF, WOFF2, TTF, OTF only
   - CSS: .css extension required

3. **MIME Type Verification**
   - Uses `file` command for real type detection
   - Prevents extension spoofing attacks

### File Permissions

- **Directories**: 0755 (rwxr-xr-x)
- **Files**: 0644 (rw-r--r--)
- Prevents unauthorized modification

### CSS Security Scanning

```bash
# Automatically scans uploaded CSS for:

1. External URLs (potential data exfiltration)
   - Pattern: url(http://... or url(https://...

2. @import statements (external stylesheet loading)
   - Can load malicious external code

3. expression() usage (IE-specific XSS)
   - Known XSS vulnerability vector

4. Syntax validation
   - Ensures balanced braces
   - Prevents broken CSS injection
```

### Multi-Tenant Isolation

- Each tenant has separate directory structure
- Configuration files are tenant-specific
- No cross-tenant asset access
- Tenant ID validation on all operations

### Input Sanitization

- Brand names sanitized (alphanumeric + spaces, hyphens, underscores)
- Hex color validation with regex
- File path resolution to prevent directory traversal
- No shell command injection vulnerabilities

---

## API Reference

### Core Functions

#### Brand Management

```bash
create_brand <name> [tenant_id] [tagline] [description]
update_brand --name <name> [--tagline <tagline>] [--description <desc>]
delete_brand [tenant_id]
```

#### Logo Management

```bash
upload_brand_logo <path> [type]
upload_logo <path> --type <type>
list_logos
remove_logo <type>
```

#### Color Management

```bash
set_brand_colors --primary <hex> [--secondary <hex>] [--accent <hex>] [--background <hex>] [--text <hex>]
validate_hex_color <color>
validate_color_palette <primary> <secondary> [background]
```

#### Typography Management

```bash
set_brand_fonts --primary <font> [--secondary <font>] [--code <font>]
set_typography --base-size <size> [--h1-size <size>] [--normal-weight <weight>] [--line-height <height>]
upload_font <path> [name]
```

#### CSS Management

```bash
set_custom_css <path>
remove_custom_css
generate_css_variables
```

#### Version Management

```bash
branding::create_version_backup
branding::list_versions
branding::restore_version <timestamp>
branding::cleanup_old_versions
```

#### Asset Management

```bash
branding::get_asset_info <path>
clean_unused_assets
```

#### Tenant Management

```bash
list_tenants
branding::get_tenant_config_path <tenant_id>
branding::ensure_tenant_isolation <tenant_id>
```

#### Utility Functions

```bash
initialize_branding_system [tenant_id]
preview_branding
validate_branding_config [tenant_id]
export_whitelabel_config [format] [tenant_id]
import_whitelabel_config <path> [tenant_id]
list_whitelabel_resources
```

---

## Error Handling

### Common Errors

```bash
# File not found
Error: Logo file not found: /path/to/logo.png
# Solution: Check file path is correct

# Unsupported format
Error: Unsupported logo format 'gif'. Supported: png jpg jpeg svg webp
# Solution: Convert to supported format

# File too large
Error: Logo file too large (6.50 MB). Maximum: 5 MB
# Solution: Compress or resize the file

# Invalid color
Error: Invalid primary color format. Use #RRGGBB
# Solution: Use hex format: #0066cc

# Branding not initialized
Error: Branding not initialized. Run 'nself whitelabel init' first.
# Solution: Initialize branding system first

# Invalid JSON
Error: Invalid JSON in config file
# Solution: Validate JSON syntax

# Missing required field
Error: Config missing required 'brand' section
# Solution: Use proper config structure

# CSS syntax error
Error: CSS syntax error - mismatched braces
# Solution: Fix CSS syntax

# Security issue
Error: CSS contains expression() - potential XSS vulnerability
# Solution: Remove dangerous CSS patterns
```

---

## Best Practices

### 1. Version Control

```bash
# Always create backups before major changes
nself whitelabel config export > backup-$(date +%Y%m%d).json

# Use version restore for rollbacks
nself whitelabel branding list-versions
nself whitelabel branding restore 20260130_143000
```

### 2. Asset Optimization

```bash
# Optimize images before upload
# PNG: Use pngcrush or optipng
pngcrush -brute logo.png logo-optimized.png

# JPG: Use jpegoptim or imagemagick
jpegoptim --max=85 logo.jpg

# SVG: Use svgo
svgo logo.svg -o logo-optimized.svg

# Fonts: Use woff2 for best compression
# Convert TTF to WOFF2 with fonttools
```

### 3. Security

```bash
# Always validate uploaded files
# Use CSS security scanning
# Set proper file permissions
# Isolate tenant data

# Regular security audits
nself whitelabel branding validate
nself security audit
```

### 4. Multi-Tenant Setup

```bash
# Use consistent tenant naming
# tenant_id format: lowercase-alphanumeric-dashes
# Examples: client-abc, company-123, org-xyz

# Keep tenant configs separate
# Export/import for backup
nself whitelabel config export --tenant client-abc > client-abc-backup.json
```

### 5. CSS Variables

```bash
# Always use generated CSS variables
# Don't hardcode colors or fonts in application
# Regenerate after changes
nself whitelabel branding set-colors --primary "#newcolor"
# CSS variables automatically regenerated

# Include variables.css in your HTML
<link rel="stylesheet" href="/branding/css/variables.css">
```

---

## Integration Examples

### React/Next.js

```jsx
// Import CSS variables
import '/path/to/branding/css/variables.css';

// Use in styled-components
const Button = styled.button`
  background: var(--color-primary);
  color: var(--color-background);
  font-family: var(--font-primary);
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-medium);
`;

// Or with inline styles
<div style={{
  backgroundColor: 'var(--color-primary)',
  color: 'var(--color-text)',
  fontFamily: 'var(--font-primary)'
}}>
  Content
</div>
```

### Vue.js

```vue
<template>
  <div class="branded-component">
    <h1>{{ title }}</h1>
    <p>{{ description }}</p>
  </div>
</template>

<style scoped>
@import '/path/to/branding/css/variables.css';

.branded-component {
  background: var(--color-background);
  color: var(--color-text);
  font-family: var(--font-primary);
}

h1 {
  color: var(--color-primary);
  font-size: var(--font-size-h1);
  font-weight: var(--font-weight-bold);
}
</style>
```

### Plain HTML/CSS

```html
<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="/branding/css/variables.css">
  <link rel="stylesheet" href="/branding/css/custom.css">
  <style>
    body {
      font-family: var(--font-primary);
      color: var(--color-text);
      background: var(--color-background);
    }
    .header {
      background: var(--color-primary);
      color: white;
    }
  </style>
</head>
<body>
  <header class="header">
    <img src="/branding/logos/logo-main.png" alt="Logo">
    <h1>Welcome</h1>
  </header>
</body>
</html>
```

---

## Troubleshooting

### Issue: CSS variables not applying

```bash
# Check CSS file exists
ls -la branding/css/variables.css

# Regenerate CSS variables
nself whitelabel branding set-colors --primary "#0066cc"

# Verify config is valid
nself whitelabel branding validate

# Check browser console for errors
# Ensure CSS file is loaded in HTML
```

### Issue: Logo not displaying

```bash
# Check logo exists
ls -la branding/logos/

# Verify logo in config
cat branding/config.json | jq '.logos'

# Check symlink
ls -la branding/logos/logo-main.png

# Re-upload logo if needed
nself whitelabel logo upload /path/to/logo.png --type main
```

### Issue: Version restore failed

```bash
# List available versions
nself whitelabel branding list-versions

# Check version file exists
ls -la branding/versions/

# Try manual restore
cp branding/versions/config-TIMESTAMP.json branding/config.json
nself whitelabel branding validate
```

### Issue: Custom CSS security warnings

```bash
# Review CSS file
cat branding/css/custom.css

# Remove external URLs
# Replace: url(https://example.com/font.woff)
# With: url(/branding/fonts/font.woff)

# Remove @import statements
# Replace: @import url('https://fonts.googleapis.com/...');
# With: Local font files

# Re-upload cleaned CSS
nself whitelabel branding set-css /path/to/cleaned.css
```

---

## Performance Considerations

### Asset Loading

1. **Optimize file sizes** before upload
2. **Use CDN** for branding assets in production
3. **Enable browser caching** for logos and CSS
4. **Lazy load** non-critical assets

### CSS Variables

- Minimal performance impact
- Better than inline styles
- Cacheable by browser
- Consistent theming

### Version Management

- Versions stored as JSON (small files)
- Auto-cleanup keeps only 10 versions
- No performance impact on runtime

---

## Future Enhancements

### Planned Features

1. **Dark Mode Support**
   - Automatic dark theme generation
   - Color palette inversion
   - User preference detection

2. **Advanced Typography**
   - Letter spacing configuration
   - Text transform settings
   - Custom font pairings

3. **Theme Previews**
   - Live preview in browser
   - Before/after comparison
   - Mobile responsive preview

4. **Asset CDN Integration**
   - Upload to S3/MinIO
   - CloudFlare integration
   - Automatic image optimization

5. **Brand Kit Export**
   - PDF style guide generation
   - Logo pack download
   - Color palette swatches

6. **API Endpoints**
   - REST API for brand management
   - GraphQL mutations
   - Webhook notifications on changes

---

## Support

### Documentation
- Main docs: `/docs/whitelabel/`
- API reference: This file
- Examples: `/src/examples/whitelabel/`

### Community
- GitHub Issues: Report bugs and feature requests
- Discussions: Ask questions and share tips
- Wiki: Community-contributed guides

### Professional Support
- Email: support@nself.org
- Enterprise: Custom branding solutions
- Training: White-label implementation workshops

---

## License

MIT License - See LICENSE file for details

---

**Last Updated**: 2026-01-30
**Version**: 1.0.0
**Author**: nself Development Team
