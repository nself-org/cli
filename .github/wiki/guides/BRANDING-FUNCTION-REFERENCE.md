# Branding System - Function Reference

Complete reference of all 36 functions implemented in the branding system.

**File**: `/Users/admin/Sites/nself/src/lib/whitelabel/branding.sh`
**Total Functions**: 36

---

## Validation Functions (10)

### 1. `validate_brand_name()`
Validates brand name format
- **Parameters**: `$1` - Brand name
- **Returns**: 0 if valid, 1 if invalid
- **Validation**: Alphanumeric + spaces, hyphens, underscores

### 2. `validate_tenant_id()`
Validates tenant ID format
- **Parameters**: `$1` - Tenant ID
- **Returns**: 0 if valid, 1 if invalid
- **Validation**: Lowercase alphanumeric + hyphens

### 3. `validate_logo_type()`
Validates logo type is supported
- **Parameters**: `$1` - Logo type (main, icon, email, favicon)
- **Returns**: 0 if valid, 1 if invalid

### 4. `validate_file_size()`
Validates file size is within limits
- **Parameters**: `$1` - File path, `$2` - Max size in MB
- **Returns**: 0 if valid, 1 if too large

### 5. `validate_file_extension()`
Validates file extension is allowed
- **Parameters**: `$1` - File path, `$2` - Allowed extensions (space-separated)
- **Returns**: 0 if valid, 1 if invalid

### 6. `validate_file_magic_bytes()`
Validates file MIME type matches extension
- **Parameters**: `$1` - File path
- **Returns**: 0 if valid, 1 if mismatch
- **Uses**: `file` command for detection

### 7. `validate_string_length()`
Validates string length is within bounds
- **Parameters**: `$1` - String, `$2` - Min length, `$3` - Max length
- **Returns**: 0 if valid, 1 if invalid

### 8. `validate_css_security()`
Scans CSS for security vulnerabilities
- **Parameters**: `$1` - CSS file path
- **Returns**: 0 (always, but prints warnings)
- **Checks**: External URLs, @import, expression()

### 9. `validate_hex_color()`
Validates hex color format
- **Parameters**: `$1` - Color code
- **Returns**: 0 if valid, 1 if invalid
- **Format**: #RGB or #RRGGBB

### 10. `validate_color_palette()`
Validates color palette for accessibility
- **Parameters**: `$1` - Primary color, `$2` - Secondary color, `$3` - Background color (optional)
- **Returns**: 0 if valid, 1 if invalid
- **Checks**: Hex format, color similarity

---

## Utility Functions (2)

### 11. `escape_html()`
Escapes HTML special characters
- **Parameters**: `$1` - String to escape
- **Returns**: Escaped string via stdout
- **Escapes**: <, >, &, ", '

### 12. `escape_json_string()`
Escapes JSON special characters
- **Parameters**: `$1` - String to escape
- **Returns**: Escaped string via stdout
- **Escapes**: \, ", newlines

---

## Brand Management Functions (4)

### 13. `initialize_branding_system()`
Initializes the branding system
- **Parameters**: `$1` - Tenant ID (optional, default: "default")
- **Returns**: 0 on success
- **Creates**: Directory structure, default config, .gitignore

### 14. `create_brand()`
Creates a new brand
- **Parameters**: `$1` - Brand name, `$2` - Tenant ID (optional), `$3` - Tagline (optional), `$4` - Description (optional)
- **Returns**: 0 on success, 1 on error
- **Actions**: Updates config, creates version backup

### 15. `update_brand()`
Updates existing brand information
- **Parameters**: `--name`, `--tagline`, `--description` (at least one required)
- **Returns**: 0 on success, 1 on error
- **Actions**: Updates specified fields, creates version backup

### 16. `delete_brand()`
Deletes brand with confirmation
- **Parameters**: `$1` - Tenant ID (optional)
- **Returns**: 0 on success
- **Actions**: Prompts for confirmation, creates final backup, removes directory

---

## Color Management Functions (2)

### 17. `set_brand_colors()`
Sets brand color palette
- **Parameters**: `--primary`, `--secondary`, `--accent`, `--background`, `--text` (at least one required)
- **Returns**: 0 on success, 1 on error
- **Actions**: Validates colors, updates config, generates CSS variables

---

## Typography Functions (3)

### 18. `set_brand_fonts()`
Sets brand font families
- **Parameters**: `--primary`, `--secondary`, `--code` (at least one required)
- **Returns**: 0 on success, 1 on error
- **Actions**: Updates config, generates CSS variables

### 19. `set_typography()`
Sets typography settings (sizes, weights, line heights)
- **Parameters**: `--base-size`, `--h1-size`, `--h2-size`, `--normal-weight`, `--bold-weight`, `--line-height`
- **Returns**: 0 on success, 1 on error
- **Actions**: Updates typography config, generates CSS variables

### 20. `upload_font()`
Uploads custom font file
- **Parameters**: `$1` - Font file path, `$2` - Font name (optional, default: "custom")
- **Returns**: 0 on success, 1 on error
- **Validation**: Format (WOFF, WOFF2, TTF, OTF), Size (1MB max)
- **Actions**: Validates, copies with versioning, updates config

---

## Logo Management Functions (4)

### 21. `upload_brand_logo()`
Uploads logo file (internal implementation)
- **Parameters**: `$1` - Logo file path, `$2` - Logo type (main, icon, email, favicon)
- **Returns**: 0 on success, 1 on error
- **Validation**: Format, Size (5MB max), MIME type
- **Actions**: Validates, copies with versioning, creates symlink, updates config

### 22. `upload_logo()`
Uploads logo file (user-facing wrapper)
- **Parameters**: `$1` - Logo file path, `--type <type>`
- **Returns**: 0 on success, 1 on error
- **Calls**: `upload_brand_logo()`

### 23. `list_logos()`
Lists all configured logos
- **Returns**: 0
- **Output**: Logo types and filenames

### 24. `remove_logo()`
Removes a logo
- **Parameters**: `$1` - Logo type
- **Returns**: 0 on success
- **Actions**: Deletes file, removes symlink, updates config

---

## CSS Management Functions (3)

### 25. `set_custom_css()`
Sets custom CSS file
- **Parameters**: `$1` - CSS file path
- **Returns**: 0 on success, 1 on error
- **Validation**: Extension (.css), Size (2MB max), Syntax (balanced braces)
- **Security**: Scans for vulnerabilities
- **Actions**: Validates, copies with versioning, creates symlink, updates config

### 26. `remove_custom_css()`
Removes custom CSS
- **Returns**: 0 on success
- **Actions**: Deletes file, removes symlink, updates config

### 27. `generate_css_variables()`
Generates CSS variables from configuration
- **Returns**: 0 on success, 1 on error
- **Output**: `branding/css/variables.css`
- **Variables**: Colors, fonts, sizes, weights, line heights

---

## Display Functions (2)

### 28. `preview_branding()`
Displays branding preview
- **Returns**: 0
- **Output**: Brand name, colors, fonts, logos, update timestamp

### 29. `list_tenants()`
Lists all configured tenants
- **Returns**: 0
- **Output**: Tenant IDs from directory structure

---

## Asset Management Functions (1)

### 30. `clean_unused_assets()`
Removes unused asset files
- **Returns**: 0
- **Actions**: Compares files with config references, deletes unreferenced files

---

## Helper/Wrapper Functions (6)

### 31. `view_whitelabel_settings()`
Wrapper for `preview_branding()`
- **Parameters**: Same as `preview_branding()`
- **Returns**: 0
- **Calls**: `preview_branding()`

### 32. `initialize_whitelabel_system()`
Wrapper for branding system initialization
- **Parameters**: `$1` - Tenant ID (optional)
- **Returns**: 0
- **Calls**: `initialize_branding_system()`

### 33. `list_whitelabel_resources()`
Lists all white-label resources
- **Returns**: 0
- **Output**: Brands, logos, tenants

### 34. `export_whitelabel_config()`
Exports branding configuration
- **Parameters**: `$1` - Format (json or yaml), `$2` - Tenant ID (optional)
- **Returns**: 0 on success, 1 on error
- **Output**: Configuration in specified format to stdout

### 35. `import_whitelabel_config()`
Imports branding configuration
- **Parameters**: `$1` - Config file path, `$2` - Tenant ID (optional)
- **Returns**: 0 on success, 1 on error
- **Validation**: JSON structure, required fields
- **Actions**: Validates, backs up existing, imports new, regenerates CSS

### 36. `validate_branding_config()`
Validates branding configuration
- **Parameters**: `$1` - Tenant ID (optional)
- **Returns**: 0 if valid, 1 if errors
- **Checks**: JSON structure, required fields, color formats, file references
- **Output**: Detailed validation report

---

## Internal/Helper Functions (Not User-Facing)

Additional helper functions used internally:

### `branding::create_secure_directory()`
Creates directory with secure permissions
- **Parameters**: `$1` - Directory path
- **Actions**: Creates dir, sets permissions to 0755

### `branding::create_default_config()`
Creates default configuration file
- **Parameters**: `$1` - Config file path, `$2` - Tenant ID
- **Actions**: Generates default JSON config

### `branding::validate_logo_file()`
Internal logo validation
- **Parameters**: `$1` - Logo file path
- **Returns**: 0 if valid, 1 if invalid

### `branding::validate_css_file()`
Internal CSS validation
- **Parameters**: `$1` - CSS file path
- **Returns**: 0 if valid, 1 if invalid

### `branding::validate_font_file()`
Internal font validation
- **Parameters**: `$1` - Font file path
- **Returns**: 0 if valid, 1 if invalid

### `branding::get_file_size_mb()`
Gets file size in MB (cross-platform)
- **Parameters**: `$1` - File path
- **Returns**: File size via stdout

### `branding::get_absolute_path()`
Resolves to absolute path
- **Parameters**: `$1` - File path
- **Returns**: Absolute path via stdout

### `branding::scan_css_security()`
Scans CSS for security issues
- **Parameters**: `$1` - CSS file path
- **Returns**: 0 (prints warnings)

### `branding::create_version_backup()`
Creates version backup of config
- **Actions**: Copies config to versions/ with timestamp

### `branding::cleanup_old_versions()`
Cleans up old version backups
- **Actions**: Keeps only last 10 versions

### `branding::list_versions()`
Lists all version backups
- **Returns**: 0
- **Output**: Formatted list of versions with timestamps

### `branding::restore_version()`
Restores a previous version
- **Parameters**: `$1` - Version timestamp
- **Returns**: 0 on success, 1 on error
- **Actions**: Backs up current, restores specified version, regenerates CSS

### `branding::get_tenant_config_path()`
Gets config file path for tenant
- **Parameters**: `$1` - Tenant ID
- **Returns**: Config file path via stdout

### `branding::ensure_tenant_isolation()`
Ensures tenant has isolated directory structure
- **Parameters**: `$1` - Tenant ID
- **Actions**: Creates tenant-specific directories

### `branding::get_asset_info()`
Gets detailed asset information
- **Parameters**: `$1` - Asset file path
- **Returns**: 0 on success, 1 if not found
- **Output**: Size, MIME type, permissions, modified date

---

## Function Categories

### By Purpose

**Validation (10)**
- validate_brand_name, validate_tenant_id, validate_logo_type
- validate_file_size, validate_file_extension, validate_file_magic_bytes
- validate_string_length, validate_css_security
- validate_hex_color, validate_color_palette

**Brand Management (4)**
- initialize_branding_system, create_brand, update_brand, delete_brand

**Color Management (1)**
- set_brand_colors

**Typography (3)**
- set_brand_fonts, set_typography, upload_font

**Logo Management (4)**
- upload_brand_logo, upload_logo, list_logos, remove_logo

**CSS Management (3)**
- set_custom_css, remove_custom_css, generate_css_variables

**Display (2)**
- preview_branding, list_tenants

**Asset Management (1)**
- clean_unused_assets

**Config Management (3)**
- export_whitelabel_config, import_whitelabel_config, validate_branding_config

**Utilities (2)**
- escape_html, escape_json_string

**Wrappers (3)**
- view_whitelabel_settings, initialize_whitelabel_system, list_whitelabel_resources

---

## Usage Patterns

### Common Function Chains

**Initial Setup**
```bash
initialize_branding_system → create_brand → set_brand_colors → upload_logo → generate_css_variables
```

**Update Colors**
```bash
set_brand_colors → generate_css_variables
```

**Update Typography**
```bash
set_brand_fonts → generate_css_variables
set_typography → generate_css_variables
```

**Upload Assets**
```bash
upload_logo → (validates) → (creates version backup) → (updates config)
upload_font → (validates) → (creates version backup) → (updates config)
set_custom_css → (validates) → (scans security) → (creates version backup) → (updates config)
```

**Version Management**
```bash
(any change) → branding::create_version_backup → (operation) → branding::cleanup_old_versions
```

---

## Dependencies

### External Commands Used
- `jq` - JSON manipulation (required)
- `file` - MIME type detection (recommended)
- `yq` or `python3` - YAML export (optional)
- `stat` - File information (cross-platform handled)
- `find`, `grep`, `awk`, `sed` - Standard POSIX utilities

### Internal Dependencies
- `src/lib/utils/platform-compat.sh` - Cross-platform utilities
- `src/lib/utils/validation.sh` - Additional validation functions

---

## Error Handling

All functions follow consistent error handling:
- Return 0 on success
- Return 1 on error
- Print error messages to stderr
- Use color-coded output (RED for errors, YELLOW for warnings, GREEN for success)
- Provide helpful error messages with suggested solutions

---

**Last Updated**: January 30, 2026
**Total Functions**: 36 (user-facing) + ~14 (internal helpers)
**Status**: Production-ready
