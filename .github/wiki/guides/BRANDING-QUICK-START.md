# Branding System - Quick Start Guide

## 5-Minute Setup

### Step 1: Initialize
```bash
nself whitelabel branding init
```

### Step 2: Set Brand Identity
```bash
nself whitelabel branding create "My Company" \
  --tagline "Innovation Through Technology" \
  --description "Leading cloud solutions provider"
```

### Step 3: Configure Colors
```bash
nself whitelabel branding set-colors \
  --primary "#0066cc" \
  --secondary "#ff6600" \
  --accent "#00cc66"
```

### Step 4: Upload Logo
```bash
nself whitelabel logo upload ./logo.png --type main
nself whitelabel logo upload ./icon.svg --type icon
```

### Step 5: Set Fonts
```bash
nself whitelabel branding set-fonts \
  --primary "Inter, system-ui, sans-serif" \
  --secondary "Georgia, serif"
```

### Step 6: Generate CSS
```bash
# Automatically generated after each change
# Include in your HTML:
<link rel="stylesheet" href="/branding/css/variables.css">
```

## Usage in Application

### CSS
```css
.header {
  background: var(--color-primary);
  color: var(--color-background);
  font-family: var(--font-primary);
}
```

### React
```jsx
<Button style={{
  backgroundColor: 'var(--color-primary)',
  fontFamily: 'var(--font-primary)'
}}>
  Click Me
</Button>
```

### Vue
```vue
<div class="card" :style="{
  backgroundColor: 'var(--color-background)',
  borderColor: 'var(--color-border)'
}">
  Content
</div>
```

## Common Commands

```bash
# View current branding
nself whitelabel branding preview

# Update colors
nself whitelabel branding set-colors --primary "#new-color"

# List all logos
nself whitelabel logo list

# Export configuration
nself whitelabel config export > backup.json

# Import configuration
nself whitelabel config import backup.json

# List versions
nself whitelabel branding list-versions

# Restore version
nself whitelabel branding restore 20260130_143000

# Validate configuration
nself whitelabel branding validate
```

## File Size Limits

- **Logos**: 5MB max (PNG, JPG, JPEG, SVG, WebP)
- **Fonts**: 1MB max (WOFF, WOFF2, TTF, OTF)
- **CSS**: 2MB max (.css files)

## Directory Structure

```
branding/
├── config.json          # Configuration
├── logos/               # Logo files
├── css/
│   ├── variables.css    # Auto-generated
│   └── custom.css       # Your custom CSS
├── fonts/               # Custom fonts
└── versions/            # Backups (last 10)
```

## Multi-Tenant Setup

```bash
# Create tenant
nself whitelabel branding create "Client Brand" --tenant client123

# Configure tenant
nself whitelabel branding set-colors --primary "#client-color" --tenant client123

# Export tenant config
nself whitelabel config export --tenant client123 > client123.json
```

## Troubleshooting

### CSS Variables Not Working
```bash
# Regenerate CSS
nself whitelabel branding set-colors --primary "#0066cc"

# Check file exists
ls -la branding/css/variables.css
```

### Logo Not Displaying
```bash
# Check logo was uploaded
nself whitelabel logo list

# Re-upload if needed
nself whitelabel logo upload /path/to/logo.png --type main
```

### Version Restore
```bash
# List available versions
nself whitelabel branding list-versions

# Restore specific version
nself whitelabel branding restore TIMESTAMP
```

## Next Steps

1. Read full documentation: `docs/whitelabel/BRANDING-SYSTEM.md`
2. Explore custom CSS: Upload your own styles
3. Add custom fonts: Upload WOFF2 files
4. Set up multiple tenants: For white-label clients
5. Integrate with your app: Use CSS variables

## Support

- Documentation: `/docs/whitelabel/`
- Examples: `/src/examples/whitelabel/`
- GitHub Issues: Report bugs
- Community: Discussions and tips
