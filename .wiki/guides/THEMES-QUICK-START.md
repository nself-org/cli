# nself Themes - Quick Start Guide

## 5-Minute Setup

### Prerequisites

```bash
# Ensure nself is running
nself start

# Verify database is running
docker ps | grep postgres
```

### Step 1: Initialize Theme System

```bash
nself whitelabel theme init
```

This creates:
- Light theme (active by default)
- Dark theme
- High contrast theme

### Step 2: List Available Themes

```bash
nself whitelabel theme list
```

Output:
```
Available Themes
============================================================

 theme_name     | display_name   | mode  | is_active | version
----------------+----------------+-------+-----------+---------
 light          | Light Theme    | light | t         | 1.0.0
 dark           | Dark Theme     | dark  | f         | 1.0.0
 high-contrast  | High Contrast  | dark  | f         | 1.0.0
```

### Step 3: Switch Themes

```bash
# Activate dark theme
nself whitelabel theme activate dark

# Check active theme
nself whitelabel theme active
# Output: dark
```

### Step 4: Create Custom Theme

```bash
# Create new theme from template
nself whitelabel theme create my-brand-theme

# Edit theme
nself whitelabel theme edit my-brand-theme
```

Edit the JSON in your default editor:

```json
{
  "name": "my-brand-theme",
  "displayName": "My Brand",
  "mode": "light",
  "variables": {
    "colors": {
      "primary": "#ff6600",    // Change to brand color
      "accent": "#0066cc"
    }
  }
}
```

Save and exit. CSS is automatically generated.

### Step 5: Preview and Activate

```bash
# Preview your theme
nself whitelabel theme preview my-brand-theme

# Activate it
nself whitelabel theme activate my-brand-theme
```

## Common Tasks

### Export Theme

```bash
# Export to file
nself whitelabel theme export my-brand-theme > my-theme.json

# Share the JSON file with your team
```

### Import Theme

```bash
# Import from file
nself whitelabel theme import my-theme.json
```

### Delete Theme

```bash
# Delete custom theme (cannot delete system themes)
nself whitelabel theme delete my-old-theme
```

## Theme JSON Structure

### Minimal Theme

```json
{
  "name": "minimal",
  "displayName": "Minimal Theme",
  "mode": "light",
  "variables": {
    "colors": {
      "primary": "#0066cc",
      "background": "#ffffff",
      "text": "#212529"
    },
    "typography": {
      "fontFamily": "Arial, sans-serif"
    },
    "spacing": {
      "md": "16px"
    },
    "borders": {
      "radius": "4px"
    },
    "shadows": {
      "sm": "0 1px 3px rgba(0,0,0,0.12)"
    }
  }
}
```

### Full Theme

See `docs/whitelabel/THEMES.md` for complete structure.

## Color Palette Quick Reference

### Light Theme Colors

```
Primary:     #0066cc (Blue)
Secondary:   #6c757d (Gray)
Accent:      #00cc66 (Green)
Background:  #ffffff (White)
Text:        #212529 (Dark Gray)
Success:     #28a745 (Green)
Warning:     #ffc107 (Yellow)
Error:       #dc3545 (Red)
```

### Dark Theme Colors

```
Primary:     #4a9eff (Light Blue)
Secondary:   #8b949e (Light Gray)
Accent:      #3fb950 (Green)
Background:  #0d1117 (Very Dark)
Text:        #c9d1d9 (Light Gray)
Success:     #3fb950 (Green)
Warning:     #d29922 (Yellow)
Error:       #f85149 (Red)
```

## Multi-Tenant Usage

### Create Tenant-Specific Themes

```bash
# Create theme for tenant A
nself whitelabel theme create startup-theme --tenant=startup-inc

# Create theme for tenant B
nself whitelabel theme create corp-theme --tenant=big-corp

# Activate for specific tenant
nself whitelabel theme activate startup-theme --tenant=startup-inc
```

### List Tenant Themes

```bash
# List themes for specific tenant
nself whitelabel theme list --tenant=startup-inc
```

## CSS Variables Generated

After activating a theme, use these CSS variables:

```css
/* Colors */
var(--color-primary)
var(--color-secondary)
var(--color-accent)
var(--color-background)
var(--color-text)

/* Typography */
var(--typography-fontFamily)
var(--typography-fontSize)
var(--typography-lineHeight)

/* Spacing */
var(--spacing-xs)    /* 4px  */
var(--spacing-sm)    /* 8px  */
var(--spacing-md)    /* 16px */
var(--spacing-lg)    /* 24px */
var(--spacing-xl)    /* 32px */

/* Borders */
var(--border-radius)
var(--border-width)

/* Shadows */
var(--shadow-sm)
var(--shadow-md)
var(--shadow-lg)
```

### Usage Example

```css
.card {
  background: var(--color-surface);
  color: var(--color-text);
  padding: var(--spacing-lg);
  border-radius: var(--border-radius);
  box-shadow: var(--shadow-md);
}

.button-primary {
  background: var(--color-primary);
  color: white;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--border-radius);
}
```

## Validation

### Validate Theme Before Import

```bash
nself whitelabel theme validate my-theme.json
```

Output:
```
Validating theme configuration...
✓ Valid JSON syntax
✓ All required fields present
✓ Valid mode: light
✓ Color variables defined
✓ Theme configuration valid
```

## Troubleshooting

### Theme Not Activating

```bash
# Check if theme exists
nself whitelabel theme list

# Check database connection
docker ps | grep postgres

# Check for errors
docker logs nself_postgres | tail
```

### CSS Not Generated

```bash
# Install jq if missing
brew install jq  # macOS
apt-get install jq  # Ubuntu/Debian

# Manually regenerate CSS
nself whitelabel theme edit my-theme  # Save without changes
```

### Invalid JSON

```bash
# Validate JSON syntax
jq empty my-theme.json

# Pretty print JSON
jq '.' my-theme.json
```

## File Locations

```
project/
├── branding/
│   └── themes/
│       ├── .active              # Active theme name
│       ├── light/
│       │   ├── theme.json       # Theme config
│       │   └── theme.css        # Generated CSS
│       ├── dark/
│       │   ├── theme.json
│       │   └── theme.css
│       └── my-brand-theme/
│           ├── theme.json
│           └── theme.css
```

## API Access

### GraphQL Query

```graphql
query GetActiveTheme($brandId: uuid!) {
  whitelabel_themes(
    where: {
      brand_id: { _eq: $brandId }
      is_active: { _eq: true }
    }
  ) {
    theme_name
    display_name
    mode
    colors
    typography
    compiled_css
  }
}
```

### REST API (if configured)

```bash
# Get active theme
curl http://localhost:8080/api/theme/active

# Get theme by name
curl http://localhost:8080/api/theme/light

# Update theme
curl -X PUT http://localhost:8080/api/theme/my-theme \
  -H "Content-Type: application/json" \
  -d @my-theme.json
```

## Next Steps

1. **Read Full Documentation**
   - `docs/whitelabel/THEMES.md`

2. **Review Examples**
   - See built-in themes in `branding/themes/`

3. **Customize for Your Brand**
   - Create theme with brand colors
   - Add custom fonts
   - Adjust spacing/borders

4. **Integrate with Frontend**
   - Load theme via GraphQL
   - Apply CSS variables
   - Support theme switching

5. **Set Up Multi-Tenant**
   - Create themes per tenant
   - Implement tenant detection
   - Load appropriate theme

## Quick Commands Reference

```bash
# Theme Management
nself whitelabel theme init                          # Initialize system
nself whitelabel theme list                          # List themes
nself whitelabel theme create <name>                 # Create theme
nself whitelabel theme edit <name>                   # Edit theme
nself whitelabel theme preview <name>                # Preview theme
nself whitelabel theme activate <name>               # Activate theme
nself whitelabel theme delete <name>                 # Delete theme
nself whitelabel theme active                        # Get active theme

# Import/Export
nself whitelabel theme export <name>                 # Export to stdout
nself whitelabel theme export <name> > file.json     # Export to file
nself whitelabel theme import <file>                 # Import from file

# Validation
nself whitelabel theme validate <file>               # Validate JSON

# Multi-Tenant
nself whitelabel theme list --tenant=<id>            # Tenant themes
nself whitelabel theme activate <name> --tenant=<id> # Activate for tenant
```

## Tips

1. **Start with a built-in theme** - Modify rather than create from scratch
2. **Use semantic color names** - `primary` not `blue`
3. **Test contrast ratios** - Ensure text is readable
4. **Version your themes** - Increment version on changes
5. **Export themes regularly** - Backup your customizations
6. **Document customizations** - Add description to theme JSON
7. **Test on multiple screens** - Different sizes and resolutions
8. **Use relative units** - `em` or `rem` for typography
9. **Maintain consistency** - Use spacing variables consistently
10. **Keep it simple** - Don't over-customize

## Support

- **Documentation**: `docs/whitelabel/THEMES.md`
- **Tests**: `src/tests/unit/test-whitelabel-themes.sh`
- **Issues**: https://github.com/nself-org/cli/issues
- **Community**: https://community.nself.org

## License

Part of nself project. See LICENSE file.
