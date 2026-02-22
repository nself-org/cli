# Email Command

> **⚠️ DEPRECATED**: `nself email` is deprecated and will be removed in v1.0.0.
> Please use `nself service email` instead.
> Run `nself service email --help` for full usage information.

Configure and manage email services for your nself project.

## Quick Start

```bash
# Development mode (zero config - just works!)
nself start  # MailPit is automatically enabled

# View captured emails
open https://mail.local.nself.org

# Production setup
nself email setup  # Interactive wizard
```

## Commands

| Command | Description |
|---------|-------------|
| `nself email` | Show help and current configuration |
| `nself email setup` | Interactive setup wizard (recommended) |
| `nself email list` | List all supported providers |
| `nself email configure <provider>` | Configure a specific provider |
| `nself email validate` | Validate your configuration |
| `nself email check` | SMTP connection pre-flight check |
| `nself email test [email]` | Send a test email |
| `nself email docs [provider]` | Show setup documentation |
| `nself email detect` | Detect current provider |

## Development Mode

By default, nself uses **MailPit** for development. This requires zero configuration:

- All emails are captured locally
- View emails at: `https://mail.<your-domain>`
- SMTP available at: `mailpit:1025` (inside Docker)
- No external dependencies

## Production Setup

### Interactive Wizard

```bash
nself email setup
```

The wizard will:
1. Ask which provider you want to use
2. Guide you through required settings
3. Validate your configuration
4. Test the connection

### Supported Providers (16+)

| Category | Providers |
|----------|-----------|
| **Transactional** | SendGrid, Mailgun, Postmark, Amazon SES |
| **Marketing** | Mailchimp Transactional (Mandrill), SparkPost |
| **Enterprise** | Microsoft 365, Google Workspace |
| **Infrastructure** | Mailjet, Sendinblue (Brevo), SMTP2GO |
| **Self-Hosted** | Custom SMTP, Mailu, Mail-in-a-Box |
| **Development** | MailPit, Mailhog, Mailtrap |

### Direct Configuration

```bash
# Configure a specific provider
nself email configure sendgrid

# Or set environment variables directly
AUTH_SMTP_HOST=smtp.sendgrid.net
AUTH_SMTP_PORT=587
AUTH_SMTP_USER=apikey
AUTH_SMTP_PASS=your-api-key
AUTH_SMTP_SECURE=true
AUTH_SMTP_SENDER=noreply@yourdomain.com
```

## Pre-flight Check

Before sending emails, verify your SMTP connection:

```bash
nself email check
```

This checks:
- DNS resolution
- TCP connectivity
- SMTP banner response
- TLS certificate validity

## Testing

```bash
# Send a test email
nself email test admin@example.com

# For MailPit, view at:
open https://mail.local.nself.org
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTH_SMTP_HOST` | SMTP server hostname | `mailpit` |
| `AUTH_SMTP_PORT` | SMTP port | `1025` (dev), `587` (prod) |
| `AUTH_SMTP_USER` | SMTP username | - |
| `AUTH_SMTP_PASS` | SMTP password/API key | - |
| `AUTH_SMTP_SECURE` | Use TLS | `true` |
| `AUTH_SMTP_SENDER` | Default from address | - |

## Troubleshooting

### Connection Refused
```bash
# Check if mail service is running
nself status mailpit

# Verify port is correct
nself email check
```

### Authentication Failed
- Verify credentials are correct
- Some providers (SendGrid, Postmark) use API keys as password
- Check if 2FA is enabled on provider account

### Emails Not Delivered
1. Check spam/junk folder
2. Verify sender domain is configured in provider
3. Check provider dashboard for delivery status
4. Review SPF/DKIM/DMARC records

## Provider-Specific Notes

### SendGrid
- Use `apikey` as username
- API key as password
- Verify sender identity first

### AWS SES
- SMTP credentials are different from AWS access keys
- Must verify sender email/domain
- Consider using region-specific endpoint

### Microsoft 365
- Enable SMTP AUTH in admin center
- May need app password if 2FA enabled
- Use `smtp.office365.com`
