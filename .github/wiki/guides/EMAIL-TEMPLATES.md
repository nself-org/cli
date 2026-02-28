# Email Templates System

Complete email template management with variable substitution, multi-language support, and tenant isolation.

## Features

### 1. Security
- **HTML Escaping**: All user-provided variables are HTML-escaped to prevent XSS attacks
- **Command Injection Prevention**: Variable names are sanitized to allow only A-Z, 0-9, and underscore
- **Template Validation**: Templates are validated before saving to prevent dangerous code
- **Tenant Isolation**: Each tenant has completely isolated templates

### 2. Template Management
- **8 Default Templates**: Welcome, password reset, verify email, invite, password change, account update, notification, and alert
- **HTML + Plain Text**: Each template has both HTML and plain text versions
- **Custom Templates**: Upload and manage custom templates
- **Template Preview**: Preview templates with sample data before sending
- **Backup System**: Automatic backups when templates are edited or deleted

### 3. Multi-Language Support
- **Multiple Languages**: Support for unlimited languages (en, es, fr, de, etc.)
- **Language Fallback**: Automatically falls back to default language if translation is missing
- **Easy Translation**: Copy templates from one language to another and translate

### 4. Variable Substitution
- **Global Variables**: Brand name, app URL, logo URL, current year, etc.
- **User Variables**: User name, email, ID
- **Template-Specific Variables**: Reset URL, verify URL, invite URL, etc.
- **Safe Substitution**: All values are HTML-escaped before insertion

### 5. Email Sending Integration
- **SMTP Support**: Send emails via configured SMTP server
- **Docker Integration**: Uses swaks in Docker for email testing
- **Multi-Format**: Sends both HTML and plain text versions
- **Subject Line Rendering**: Dynamic subject lines with variable substitution

### 6. Multi-Tenant Support
- **Isolated Templates**: Each tenant has their own template directory
- **Tenant Variables**: Tenant ID automatically included in emails
- **Custom SMTP**: Tenants can have their own SMTP configuration (future)
- **Fallback to Default**: Uses default templates if tenant templates don't exist

## Usage

### Initialize Email Templates

```bash
# Initialize with default templates
source src/lib/whitelabel/email-templates.sh
initialize_email_templates
```

### List Available Templates

```bash
list_email_templates "en"
```

Output:
```
Email Templates (Language: en)
============================================================

welcome              Welcome email sent to new users upon registration
  Subject: Welcome to {{BRAND_NAME}}!
  Category: authentication

password-reset       Password reset email with secure reset link
  Subject: Reset Your Password
  Category: security

verify-email         Email verification with confirmation link
  Subject: Verify Your Email Address
  Category: authentication

...
```

### Render a Template

```bash
# Render with custom variables
render_template "welcome" "en" "html" \
  "USER_NAME=John Doe" \
  "USER_EMAIL=john@example.com" \
  "APP_URL=https://myapp.com"
```

### Send an Email

```bash
# Send email using template
send_email_from_template "welcome" "john@example.com" "en" \
  "USER_NAME=John Doe" \
  "APP_URL=https://myapp.com"
```

### Preview a Template

```bash
# Preview with sample data
preview_email_template "password-reset" "en" "html"
```

### Edit a Template

```bash
# Edit template in default editor
edit_email_template "welcome" "en" "html"

# Edit plain text version
edit_email_template "welcome" "en" "txt"
```

### Multi-Language Management

```bash
# Add a new language
set_email_language "es"

# Copy templates from English to Spanish
copy_templates_to_language "en" "es"

# List available languages
list_available_languages
```

### Custom Templates

```bash
# Upload custom HTML template
upload_custom_template "newsletter" "/path/to/newsletter.html" "/path/to/newsletter.txt" "en"

# Delete custom template
delete_custom_template "newsletter" "en"
```

### Template Validation

```bash
# Validate all templates in a language
validate_all_templates "en"
```

### Export/Import Templates

```bash
# Export all templates
export_all_templates "en" "./my-templates"

# Import templates
import_all_templates "./my-templates" "en"
```

### Multi-Tenant Operations

```bash
# Initialize templates for a tenant
initialize_tenant_templates "tenant123" "en"

# List tenant templates
list_tenant_templates "tenant123" "en"

# Send email using tenant templates
send_tenant_email "tenant123" "welcome" "user@example.com" "en" \
  "USER_NAME=John Doe"

# Render tenant template
render_tenant_template "tenant123" "welcome" "en" "html" \
  "USER_NAME=John Doe"
```

### Statistics

```bash
# Show template system statistics
show_template_stats
```

Output:
```
Email Template System Statistics
============================================================

Languages: 3
  • en: 8 templates
  • es: 8 templates
  • fr: 8 templates

Tenant Templates: 5 tenants

Preview Files: 12
Backups: 3
```

## Available Templates

### 1. Welcome (`welcome`)
Sent to new users upon registration.

**Variables:**
- `USER_NAME` - User's display name
- `BRAND_NAME` - Brand/company name
- `LOGO_URL` - URL to brand logo
- `APP_URL` - Main application URL
- `CURRENT_YEAR` - Current year
- `COMPANY_ADDRESS` - Company address

### 2. Password Reset (`password-reset`)
Sent when user requests password reset.

**Variables:**
- `USER_NAME` - User's display name
- `BRAND_NAME` - Brand/company name
- `RESET_URL` - Password reset URL with token
- `EXPIRY_TIME` - Link expiration time (e.g., "1 hour")
- `CURRENT_YEAR` - Current year

### 3. Verify Email (`verify-email`)
Sent to verify user's email address.

**Variables:**
- `USER_NAME` - User's display name
- `VERIFY_URL` - Email verification URL with token
- `EXPIRY_TIME` - Link expiration time

### 4. Invite (`invite`)
Sent when a user invites someone.

**Variables:**
- `RECIPIENT_NAME` - Name of invited person
- `SENDER_NAME` - Name of user sending invite
- `BRAND_NAME` - Brand/company name
- `INVITE_URL` - Invitation acceptance URL

### 5. Password Change (`password-change`)
Confirmation that password was changed.

**Variables:**
- `USER_NAME` - User's display name
- `CHANGE_DATE` - Date of password change

### 6. Account Update (`account-update`)
Notification of account information changes.

**Variables:**
- `USER_NAME` - User's display name
- `UPDATE_DESCRIPTION` - Description of what changed

### 7. Notification (`notification`)
Generic notification template.

**Variables:**
- `NOTIFICATION_TITLE` - Notification headline
- `NOTIFICATION_MESSAGE` - Notification body
- `ACTION_URL` - Call-to-action URL
- `ACTION_TEXT` - Button text

### 8. Alert (`alert`)
Alert/warning notifications.

**Variables:**
- `ALERT_TITLE` - Alert headline
- `ALERT_MESSAGE` - Alert message
- `ACTION_REQUIRED` - Required action description

## File Structure

```
branding/
├── email-templates/
│   ├── VARIABLES.md                    # Variable reference documentation
│   ├── languages/
│   │   ├── en/
│   │   │   ├── welcome.html            # HTML version
│   │   │   ├── welcome.txt             # Plain text version
│   │   │   ├── welcome.json            # Metadata (subject, variables, etc.)
│   │   │   ├── password-reset.html
│   │   │   ├── password-reset.txt
│   │   │   ├── password-reset.json
│   │   │   └── ...
│   │   ├── es/
│   │   │   └── ...
│   │   └── fr/
│   │       └── ...
│   ├── previews/                       # Preview files
│   │   ├── welcome-en.html
│   │   └── ...
│   └── backups/                        # Automatic backups
│       ├── 20260130_120000/
│       └── ...
└── tenants/                            # Multi-tenant isolation
    ├── tenant123/
    │   └── email-templates/
    │       ├── VARIABLES.md
    │       └── languages/
    │           └── en/
    │               └── ...
    └── tenant456/
        └── ...
```

## Security Considerations

### 1. XSS Prevention
All user-provided variables are HTML-escaped before insertion:

```bash
# Input: <script>alert('xss')</script>
# Output: &lt;script&gt;alert('xss')&lt;/script&gt;
```

### 2. Command Injection Prevention
Variable names are sanitized to prevent command injection:

```bash
# Input: USER$(whoami)_NAME
# Output: USER_NAME  # $(whoami) removed
```

### 3. Template Validation
Templates are validated before saving to prevent dangerous code:

```bash
# Rejected patterns: $(), `, eval, exec, source, bash, sh
```

### 4. Tenant Isolation
Each tenant has isolated templates in their own directory:

```bash
branding/tenants/tenant123/email-templates/  # Tenant 123
branding/tenants/tenant456/email-templates/  # Tenant 456
```

### 5. Directory Traversal Protection
Tenant IDs are sanitized to prevent directory traversal:

```bash
# Input: ../../../etc/passwd
# Output: etcpasswd  # ../ removed
```

## Integration with nself CLI

```bash
# Via nself whitelabel command
nself whitelabel email-templates list
nself whitelabel email-templates preview welcome
nself whitelabel email-templates edit welcome
nself whitelabel email-templates test welcome user@example.com
nself whitelabel email-templates validate
nself whitelabel email-templates stats
```

## Environment Variables

```bash
# SMTP Configuration (used for sending emails)
AUTH_SMTP_HOST=smtp.example.com
AUTH_SMTP_PORT=587
AUTH_SMTP_USER=noreply@example.com
AUTH_SMTP_PASS=your-password
AUTH_SMTP_SENDER=noreply@example.com

# Brand Configuration
BRAND_NAME="My App"
BASE_DOMAIN="myapp.com"
APP_URL="https://myapp.com"
LOGO_URL="https://myapp.com/logo.png"
COMPANY_ADDRESS="123 Main St, City, Country"
SUPPORT_EMAIL="support@myapp.com"
```

## Testing

Run the comprehensive test suite:

```bash
bash src/tests/unit/test-email-templates.sh
```

Tests cover:
- HTML escaping and XSS prevention
- Variable name sanitization
- Template validation
- Variable substitution
- Template rendering
- Multi-language support
- Tenant isolation
- Directory traversal protection
- Template backup on edit
- Custom template upload
- Export/import functionality
- Subject line rendering

## Best Practices

1. **Always provide both HTML and plain text versions** - Some email clients prefer plain text
2. **Test templates before production use** - Use `preview_email_template` and `test_email_template`
3. **Use semantic variable names** - Clear names like `USER_NAME` instead of `VAR1`
4. **Validate all templates** - Run `validate_all_templates` before deployment
5. **Backup regularly** - Backups are automatic, but export important customizations
6. **Translate completely** - Don't leave English text in non-English templates
7. **Use tenant isolation** - Keep tenant templates separate for white-label deployments
8. **Review security** - Never include user input directly without HTML escaping
9. **Monitor sending** - Log email sends and check for failures
10. **Keep it simple** - Complex templates are harder to maintain

## Troubleshooting

### Templates Not Found

```bash
# Initialize templates first
initialize_email_templates

# Check if templates exist
ls -la "$TEMPLATES_DIR/languages/en/"
```

### SMTP Not Configured

```bash
# Configure SMTP first
nself email setup

# Test SMTP connection
nself email check
```

### Variables Not Substituting

```bash
# Check variable format (must be UPPERCASE with underscores)
# Correct: {{USER_NAME}}
# Wrong: {{userName}}, {{user-name}}

# Verify variables are passed correctly
render_template "welcome" "en" "html" "USER_NAME=Test"
```

### Tenant Templates Not Working

```bash
# Initialize tenant templates first
initialize_tenant_templates "tenant123" "en"

# Verify tenant directory exists
ls -la "$(get_tenant_templates_dir 'tenant123')"
```

### Permission Errors

```bash
# Fix permissions on template directory
chmod -R 755 "$PROJECT_ROOT/branding/email-templates"

# Fix ownership
chown -R $(whoami) "$PROJECT_ROOT/branding/email-templates"
```

## API Reference

See `/Users/admin/Sites/nself/src/lib/whitelabel/email-templates.sh` for full API documentation.

### Core Functions
- `initialize_email_templates()` - Initialize template system
- `render_template(type, lang, format, vars...)` - Render a template
- `send_email_from_template(type, email, lang, vars...)` - Send email
- `list_email_templates(lang)` - List available templates

### Security Functions
- `html_escape(input)` - Escape HTML special characters
- `sanitize_variable_name(var_name)` - Sanitize variable names
- `validate_template_content(file)` - Validate template file
- `substitute_template_variables(content, vars...)` - Safe variable substitution

### Management Functions
- `edit_email_template(name, lang, format)` - Edit template
- `preview_email_template(name, lang, format)` - Preview template
- `upload_custom_template(type, html, txt, lang)` - Upload custom template
- `delete_custom_template(type, lang)` - Delete custom template

### Multi-Language Functions
- `set_email_language(lang)` - Initialize new language
- `list_available_languages()` - List configured languages
- `copy_templates_to_language(src, dst)` - Copy templates between languages

### Multi-Tenant Functions
- `initialize_tenant_templates(tenant_id, lang)` - Initialize tenant templates
- `render_tenant_template(tenant_id, type, lang, format, vars...)` - Render tenant template
- `send_tenant_email(tenant_id, type, email, lang, vars...)` - Send tenant email
- `list_tenant_templates(tenant_id, lang)` - List tenant templates

### Batch Operations
- `validate_all_templates(lang)` - Validate all templates
- `export_all_templates(lang, output_dir)` - Export templates
- `import_all_templates(source_dir, lang)` - Import templates
- `show_template_stats()` - Show system statistics

## License

Part of nself v0.9.0 - White-Label & Customization (Sprint 14, 60pts)
