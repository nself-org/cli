# nself Theme System Documentation

Complete documentation for the nself white-label theme system with database-backed configuration and CSS generation.

## Overview

The nself theme system provides:

- **JSONB Configuration Storage** - Themes stored in PostgreSQL with JSONB for flexibility
- **CSS Variable Generation** - Automatic CSS generation from theme configuration
- **Theme Inheritance** - Base themes with custom overrides
- **Multi-tenant Isolation** - Themes scoped to brands/tenants
- **Built-in Themes** - Light, Dark, and High Contrast themes included
- **Theme Preview** - Preview themes without activating them
- **Import/Export** - Share themes across projects

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
├─────────────────────────────────────────────────────────────┤
│  nself CLI (whitelabel theme commands)                      │
├─────────────────────────────────────────────────────────────┤
│  Theme System (src/lib/whitelabel/themes.sh)               │
│  - Theme CRUD operations                                    │
│  - CSS generation                                           │
│  - Validation                                               │
│  - Import/Export                                            │
├─────────────────────────────────────────────────────────────┤
│  PostgreSQL Database                                        │
│  - whitelabel_brands (tenant isolation)                     │
│  - whitelabel_themes (JSONB storage)                        │
│  - whitelabel_assets (CSS/fonts)                            │
├─────────────────────────────────────────────────────────────┤
│  File System Cache                                          │
│  - branding/themes/{theme-name}/                            │
│    - theme.json (configuration)                             │
│    - theme.css (compiled CSS)                               │
└─────────────────────────────────────────────────────────────┘
```

## Database Schema

### whitelabel_themes Table

```sql
CREATE TABLE whitelabel_themes (
  id UUID PRIMARY KEY,
  brand_id UUID REFERENCES whitelabel_brands(id),
  theme_name VARCHAR(255) NOT NULL,
  display_name VARCHAR(255) NOT NULL,
  description TEXT,
  version VARCHAR(50) DEFAULT '1.0.0',
  author VARCHAR(255),
  mode VARCHAR(50) DEFAULT 'light', -- light, dark, auto

  -- JSONB configuration
  colors JSONB,
  typography JSONB,
  spacing JSONB,
  borders JSONB,
  shadows JSONB,

  -- Generated CSS
  custom_css TEXT,
  compiled_css TEXT,

  -- Status flags
  is_active BOOLEAN DEFAULT false,
  is_default BOOLEAN DEFAULT false,
  is_system BOOLEAN DEFAULT false,

  -- Timestamps
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  CONSTRAINT unique_theme_per_brand UNIQUE(brand_id, theme_name),
  CONSTRAINT valid_theme_mode CHECK (mode IN ('light', 'dark', 'auto'))
);
```

## Theme Configuration Format

### Complete Theme JSON

```json
{
  "name": "my-theme",
  "displayName": "My Custom Theme",
  "description": "A beautiful custom theme",
  "version": "1.0.0",
  "author": "Your Name",
  "mode": "light",
  "variables": {
    "colors": {
      "primary": "#0066cc",
      "primaryHover": "#0052a3",
      "secondary": "#6c757d",
      "accent": "#00cc66",
      "background": "#ffffff",
      "backgroundAlt": "#f8f9fa",
      "surface": "#ffffff",
      "surfaceAlt": "#f1f3f5",
      "text": "#212529",
      "textSecondary": "#6c757d",
      "textMuted": "#adb5bd",
      "border": "#dee2e6",
      "borderLight": "#e9ecef",
      "success": "#28a745",
      "warning": "#ffc107",
      "error": "#dc3545",
      "info": "#17a2b8"
    },
    "typography": {
      "fontFamily": "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
      "fontFamilyMono": "'Fira Code', 'Courier New', monospace",
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
      "sm": "0 1px 3px rgba(0,0,0,0.12)",
      "md": "0 4px 6px rgba(0,0,0,0.1)",
      "lg": "0 10px 20px rgba(0,0,0,0.15)"
    }
  }
}
```

### Minimal Theme JSON

```json
{
  "name": "minimal-theme",
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

## Usage Examples

### Initialize Theme System

```bash
# Initialize theme system (creates default themes)
nself whitelabel theme init

# Check initialization status
nself whitelabel theme list
```

### List Available Themes

```bash
# List all themes for default tenant
nself whitelabel theme list

# List themes for specific tenant
nself whitelabel theme list --tenant=customer-123

# Output:
# Available Themes
# ============================================================
#
#  theme_name     | display_name   | mode  | is_active | is_system | version
# ----------------+----------------+-------+-----------+-----------+---------
#  light          | Light Theme    | light | t         | t         | 1.0.0
#  dark           | Dark Theme     | dark  | f         | t         | 1.0.0
#  high-contrast  | High Contrast  | dark  | f         | t         | 1.0.0
```

### Create Custom Theme

```bash
# Create new theme (uses light theme as template)
nself whitelabel theme create my-brand-theme

# Create theme for specific tenant
nself whitelabel theme create corporate-theme --tenant=acme-corp

# Output:
# Creating theme: my-brand-theme
# ✓ Theme created: my-brand-theme
#
# Next steps:
#   1. Edit theme: nself whitelabel theme edit my-brand-theme
#   2. Preview theme: nself whitelabel theme preview my-brand-theme
#   3. Activate theme: nself whitelabel theme activate my-brand-theme
```

### Edit Theme

```bash
# Edit theme in default editor ($EDITOR)
nself whitelabel theme edit my-brand-theme

# Edit with specific editor
EDITOR=nano nself whitelabel theme edit my-brand-theme

# After editing:
# - Theme is validated (JSON syntax check)
# - Theme is saved to database
# - CSS is regenerated
# - Local cache is updated
```

### Preview Theme

```bash
# Preview theme configuration
nself whitelabel theme preview my-brand-theme

# Output:
# Theme Preview: my-brand-theme
# ============================================================
#
# Name: My Brand Theme
# Description: Custom theme for our brand
# Mode: light
# Status: Inactive
#
# Colors:
#   {
#     "primary": "#0066cc",
#     "background": "#ffffff",
#     "text": "#212529",
#     ...
#   }
#
# Typography:
#   {
#     "fontFamily": "-apple-system, sans-serif",
#     "fontSize": "16px",
#     ...
#   }
```

### Activate Theme

```bash
# Activate theme (deactivates others)
nself whitelabel theme activate my-brand-theme

# Activate for specific tenant
nself whitelabel theme activate corporate-theme --tenant=acme-corp

# Output:
# Activating theme: my-brand-theme
# ✓ Theme activated: my-brand-theme
```

### Get Active Theme

```bash
# Get currently active theme
nself whitelabel theme active

# Get active theme for specific tenant
nself whitelabel theme active --tenant=acme-corp

# Output: light
```

### Export Theme

```bash
# Export to stdout (JSON)
nself whitelabel theme export my-brand-theme

# Export to file
nself whitelabel theme export my-brand-theme > my-theme.json

# Export with explicit output file
nself whitelabel theme export my-brand-theme --output=my-theme.json

# Output:
# ✓ Theme exported to: my-theme.json
```

### Import Theme

```bash
# Import theme from file
nself whitelabel theme import my-theme.json

# Import for specific tenant
nself whitelabel theme import corporate-theme.json --tenant=acme-corp

# Output:
# Importing theme: my-brand-theme
# ✓ Theme imported: my-brand-theme
```

### Validate Theme

```bash
# Validate theme configuration
nself whitelabel theme validate my-theme.json

# Output:
# Validating theme configuration...
# ✓ Valid JSON syntax
# ✓ All required fields present
# ✓ Valid mode: light
# ✓ Color variables defined
# ✓ Theme configuration valid
```

### Delete Theme

```bash
# Delete custom theme
nself whitelabel theme delete my-old-theme

# Cannot delete:
# - System themes (light, dark, high-contrast)
# - Active themes (deactivate first)

# Output:
# Deleting theme: my-old-theme
# ✓ Theme deleted: my-old-theme
```

## Built-in Themes

### Light Theme

Clean and bright theme suitable for most applications.

**Colors:**
- Primary: `#0066cc` (Blue)
- Background: `#ffffff` (White)
- Text: `#212529` (Dark gray)
- Success: `#28a745` (Green)
- Error: `#dc3545` (Red)

**Use cases:**
- Default application theme
- Productivity apps
- Professional dashboards

### Dark Theme

Easy on the eyes, inspired by GitHub dark mode.

**Colors:**
- Primary: `#4a9eff` (Light blue)
- Background: `#0d1117` (Dark gray)
- Text: `#c9d1d9` (Light gray)
- Success: `#3fb950` (Green)
- Error: `#f85149` (Red)

**Use cases:**
- Night mode
- Developer tools
- Media applications

### High Contrast Theme

Maximum contrast for accessibility (WCAG AAA compliant).

**Colors:**
- Primary: `#ffff00` (Yellow)
- Background: `#000000` (Black)
- Text: `#ffffff` (White)
- Success: `#00ff00` (Bright green)
- Error: `#ff0000` (Bright red)

**Features:**
- Larger font sizes (18px base)
- Bold font weight (600)
- Thicker borders (2px)
- No rounded corners
- High contrast shadows

**Use cases:**
- Accessibility compliance
- Users with visual impairments
- High ambient light environments

## CSS Variable Output

### Generated CSS Structure

When a theme is compiled, it generates CSS with custom properties (CSS variables):

```css
:root {
  /* Colors */
  --color-primary: #0066cc;
  --color-primaryHover: #0052a3;
  --color-background: #ffffff;
  --color-text: #212529;
  /* ... more colors */

  /* Typography */
  --typography-fontFamily: -apple-system, BlinkMacSystemFont, sans-serif;
  --typography-fontSize: 16px;
  --typography-fontWeight: 400;
  --typography-lineHeight: 1.5;

  /* Spacing */
  --spacing-xs: 4px;
  --spacing-sm: 8px;
  --spacing-md: 16px;
  --spacing-lg: 24px;
  --spacing-xl: 32px;

  /* Borders */
  --border-radius: 4px;
  --border-radiusLg: 8px;
  --border-width: 1px;

  /* Shadows */
  --shadow-sm: 0 1px 3px rgba(0,0,0,0.12);
  --shadow-md: 0 4px 6px rgba(0,0,0,0.1);
  --shadow-lg: 0 10px 20px rgba(0,0,0,0.15);
}

/* Base styles */
body {
  font-family: var(--typography-fontFamily);
  font-size: var(--typography-fontSize);
  color: var(--color-text);
  background-color: var(--color-background);
}

/* Utility classes */
.bg-primary { background-color: var(--color-primary); }
.text-primary { color: var(--color-primary); }
.shadow-sm { box-shadow: var(--shadow-sm); }
.p-md { padding: var(--spacing-md); }
/* ... more utilities */
```

### Using CSS Variables in Your Application

```html
<!-- HTML -->
<div class="card">
  <h2 class="card-title">Welcome</h2>
  <p class="card-text">This uses theme variables</p>
</div>

<!-- CSS -->
<style>
.card {
  background: var(--color-surface);
  border: var(--border-width) solid var(--color-border);
  border-radius: var(--border-radius);
  padding: var(--spacing-lg);
  box-shadow: var(--shadow-md);
}

.card-title {
  color: var(--color-primary);
  font-family: var(--typography-fontFamily);
  margin-bottom: var(--spacing-sm);
}

.card-text {
  color: var(--color-text);
  font-size: var(--typography-fontSize);
  line-height: var(--typography-lineHeight);
}
</style>
```

## Multi-Tenant Theme Management

### Tenant Isolation

Each brand/tenant can have its own set of themes:

```bash
# Create themes for different tenants
nself whitelabel theme create startup-theme --tenant=startup-inc
nself whitelabel theme create enterprise-theme --tenant=big-corp
nself whitelabel theme create agency-theme --tenant=creative-agency

# Activate different themes per tenant
nself whitelabel theme activate startup-theme --tenant=startup-inc
nself whitelabel theme activate enterprise-theme --tenant=big-corp

# List themes for specific tenant
nself whitelabel theme list --tenant=startup-inc
```

### Theme Inheritance

Themes can inherit from system themes and override specific values:

```json
{
  "name": "brand-theme",
  "displayName": "Brand Theme",
  "description": "Based on light theme with brand colors",
  "mode": "light",
  "variables": {
    "colors": {
      "primary": "#ff6600",  // Brand orange
      "accent": "#0066cc"     // Brand blue
      // Other colors inherited from light theme
    }
  }
}
```

## Integration with nself Services

### Nginx Configuration

Themes can be served via nginx:

```nginx
location /themes/ {
    alias /var/www/branding/themes/;
    expires 1y;
    add_header Cache-Control "public, immutable";
}
```

### Hasura Integration

Theme data is accessible via GraphQL:

```graphql
query GetActiveTheme($brandId: uuid!) {
  whitelabel_themes(
    where: {
      brand_id: { _eq: $brandId }
      is_active: { _eq: true }
    }
  ) {
    id
    theme_name
    display_name
    mode
    colors
    typography
    spacing
    borders
    shadows
    compiled_css
  }
}
```

### React/Frontend Integration

```typescript
// Load theme from nself API
const useTheme = () => {
  const [theme, setTheme] = useState(null);

  useEffect(() => {
    fetch('/api/theme/active')
      .then(res => res.json())
      .then(data => {
        setTheme(data);
        applyTheme(data);
      });
  }, []);

  return theme;
};

// Apply theme to document
const applyTheme = (theme) => {
  const root = document.documentElement;

  // Apply color variables
  Object.entries(theme.colors).forEach(([key, value]) => {
    root.style.setProperty(`--color-${key}`, value);
  });

  // Apply typography variables
  Object.entries(theme.typography).forEach(([key, value]) => {
    root.style.setProperty(`--typography-${key}`, value);
  });

  // ... apply other variables
};
```

## Advanced Features

### Dynamic Theme Switching

```bash
# Switch themes at runtime
nself whitelabel theme activate dark
nself whitelabel theme activate light

# Get current theme
ACTIVE_THEME=$(nself whitelabel theme active)
echo "Current theme: $ACTIVE_THEME"
```

### Theme Versioning

```json
{
  "name": "my-theme",
  "version": "2.0.0",  // Increment when making changes
  "description": "v2.0: Added new color palette"
}
```

### Custom CSS Injection

Add custom CSS to themes:

```sql
UPDATE whitelabel_themes
SET custom_css = '
  .custom-button {
    background: linear-gradient(45deg, var(--color-primary), var(--color-accent));
    border: none;
    padding: var(--spacing-md);
  }
'
WHERE theme_name = 'my-theme';
```

## Troubleshooting

### Theme Not Activating

```bash
# Check if theme exists
nself whitelabel theme list

# Check database connection
docker ps | grep postgres

# Verify theme in database
docker exec -it nself_postgres psql -U postgres -d nself_db \
  -c "SELECT theme_name, is_active FROM whitelabel_themes;"
```

### CSS Not Generating

```bash
# Manually regenerate CSS
nself whitelabel theme edit my-theme  # Save without changes

# Check jq availability (required for CSS generation)
which jq
brew install jq  # macOS
apt-get install jq  # Ubuntu/Debian
```

### Invalid JSON

```bash
# Validate theme file
nself whitelabel theme validate my-theme.json

# Use jq to check JSON syntax
jq empty my-theme.json

# Pretty print JSON
jq '.' my-theme.json
```

## Best Practices

### 1. Use Semantic Color Names

```json
{
  "colors": {
    "primary": "#0066cc",      // ✓ Good: semantic
    "brandBlue": "#0066cc"     // ✗ Bad: specific color name
  }
}
```

### 2. Maintain Contrast Ratios

Ensure text is readable (WCAG AA: 4.5:1 for normal text):

```json
{
  "colors": {
    "background": "#ffffff",
    "text": "#212529"  // ✓ High contrast (16:1)
  }
}
```

### 3. Use Relative Font Sizes

```json
{
  "typography": {
    "fontSize": "16px",        // Base size
    "fontSizeSmall": "0.875em", // Relative to base
    "fontSizeLarge": "1.25em"   // Relative to base
  }
}
```

### 4. Version Your Themes

Always increment version when making changes:

```json
{
  "version": "1.0.0",  // Initial release
  "version": "1.1.0",  // Minor update (new colors)
  "version": "2.0.0"   // Major update (breaking changes)
}
```

### 5. Document Custom Properties

```json
{
  "description": "Corporate theme v2.0",
  "changelog": [
    "v2.0.0: Redesigned color palette",
    "v1.5.0: Added dark mode support",
    "v1.0.0: Initial release"
  ]
}
```

## API Reference

### Functions

#### `initialize_themes_system()`

Initializes the theme system, creates default themes, and sets up directories.

**Returns:** `0` on success, `1` on error

#### `create_theme(theme_name, tenant_id)`

Creates a new custom theme.

**Parameters:**
- `theme_name`: Theme identifier (lowercase, alphanumeric, hyphens)
- `tenant_id`: Tenant identifier (default: "default")

**Returns:** `0` on success, `1` on error

#### `activate_theme(theme_name, tenant_id)`

Activates a theme for a tenant.

**Parameters:**
- `theme_name`: Theme to activate
- `tenant_id`: Tenant identifier (default: "default")

**Returns:** `0` on success, `1` on error

#### `preview_theme(theme_name, tenant_id)`

Displays theme configuration without activating.

**Parameters:**
- `theme_name`: Theme to preview
- `tenant_id`: Tenant identifier (default: "default")

**Returns:** `0` on success, `1` on error

#### `export_theme(theme_name, output_file, tenant_id)`

Exports theme to JSON file.

**Parameters:**
- `theme_name`: Theme to export
- `output_file`: Output file path (optional, stdout if not specified)
- `tenant_id`: Tenant identifier (default: "default")

**Returns:** `0` on success, `1` on error

#### `import_theme(theme_file, tenant_id)`

Imports theme from JSON file.

**Parameters:**
- `theme_file`: Path to theme JSON file
- `tenant_id`: Tenant identifier (default: "default")

**Returns:** `0` on success, `1` on error

#### `list_themes(tenant_id)`

Lists all themes for a tenant.

**Parameters:**
- `tenant_id`: Tenant identifier (default: "default")

**Returns:** `0` on success, `1` on error

#### `delete_theme(theme_name, tenant_id)`

Deletes a custom theme (cannot delete system themes or active themes).

**Parameters:**
- `theme_name`: Theme to delete
- `tenant_id`: Tenant identifier (default: "default")

**Returns:** `0` on success, `1` on error

#### `validate_theme_config(theme_file)`

Validates theme JSON configuration.

**Parameters:**
- `theme_file`: Path to theme JSON file

**Returns:** `0` if valid, `1` if invalid

#### `generate_theme_css(config_file, css_file)`

Generates CSS from theme JSON configuration.

**Parameters:**
- `config_file`: Path to theme JSON file
- `css_file`: Output CSS file path

**Returns:** `0` on success, `1` on error

## File Structure

```
project/
├── branding/
│   └── themes/
│       ├── .active                    # Active theme name
│       ├── light/
│       │   ├── theme.json            # Theme configuration
│       │   └── theme.css             # Generated CSS
│       ├── dark/
│       │   ├── theme.json
│       │   └── theme.css
│       ├── high-contrast/
│       │   ├── theme.json
│       │   └── theme.css
│       └── custom-theme/
│           ├── theme.json
│           └── theme.css
└── src/
    └── lib/
        └── whitelabel/
            └── themes.sh             # Theme system implementation
```

## Related Documentation

- [White-Label Branding](./BRANDING-SYSTEM.md)
- [Custom Domains](../tutorials/CUSTOM-DOMAINS.md)
- [Email Templates](./EMAIL-TEMPLATES.md)
- Database Schema

## Support

For issues or questions:
- GitHub Issues: https://github.com/nself-org/cli/issues
- Documentation: https://docs.nself.org
- Community: https://community.nself.org
