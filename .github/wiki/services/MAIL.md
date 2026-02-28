# Mail - Email Service

## Overview

The Mail service in nself provides email sending and testing capabilities across all environments. In development, nself uses MailPit as a local email testing server that captures all outgoing emails for inspection without delivering them to real recipients. In staging and production, nself integrates with external SMTP providers such as SendGrid, Amazon SES, Postmark, and Mailgun for actual email delivery.

Mail is one of the 7 optional services in nself. For development email testing, enable it with `MAILPIT_ENABLED=true` in your `.env` file. For production, configure your SMTP provider credentials instead. The nself Auth service automatically uses the configured mail service for password resets, email verification, and other authentication-related emails.

## Features

### MailPit (Development)

- **Email Capture** - Intercepts all outgoing SMTP messages from your application
- **Web Interface** - Browser-based UI for viewing, searching, and inspecting emails
- **HTML/Text Preview** - Render HTML emails or view plain text and raw source
- **Attachment Support** - View and download email attachments
- **Search and Filter** - Search emails by sender, recipient, subject, or content
- **API Access** - RESTful API for programmatic email inspection in tests
- **SMTP Relay** - Acts as a standard SMTP server on port 1025
- **No External Delivery** - Emails never leave your local environment
- **Real-time Updates** - New emails appear instantly in the web UI

### Production SMTP

- **Provider Agnostic** - Works with any standard SMTP provider
- **Template Support** - HTML email templates for transactional messages
- **Delivery Tracking** - Monitor send status through provider dashboards
- **Retry Logic** - Automatic retry on temporary delivery failures
- **Rate Limiting** - Configurable send rate to respect provider limits

### Integration Points

| Service | Integration | Purpose |
|---------|------------|---------|
| Auth | SMTP relay | Password resets, email verification, MFA codes |
| Custom Services (CS_N) | SMTP connection | Application transactional emails |
| Functions | Event handler | Triggered email sending via serverless functions |
| Hasura | Event triggers | Database-driven email notifications |

## Configuration

### Development Setup (MailPit)

Enable MailPit in your `.env` file:

```bash
# MailPit Configuration (Development)
MAILPIT_ENABLED=true
```

All other settings have sensible defaults.

### Complete MailPit Configuration

```bash
# Required
MAILPIT_ENABLED=true

# Version
MAILPIT_VERSION=latest               # Docker image tag (default: latest)

# Port Configuration
MAILPIT_SMTP_PORT=1025               # SMTP port for sending (default: 1025)
MAILPIT_UI_PORT=8025                 # Web UI port (default: 8025)

# Route Configuration
MAILPIT_ROUTE=mail                   # Creates mail.yourdomain.com

# SMTP Settings
MAILPIT_MAX_MESSAGES=500             # Maximum stored messages (default: 500)
MAILPIT_SMTP_AUTH_ACCEPT_ANY=true    # Accept any SMTP credentials (default: true)

# UI Settings
MAILPIT_UI_AUTH_FILE=                # Optional: htpasswd file for UI auth
MAILPIT_WEBROOT=/                    # Web UI root path (default: /)
```

### Production Setup (SMTP Provider)

For staging and production, configure an external SMTP provider:

```bash
# SMTP Provider Configuration
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=your-sendgrid-api-key
SMTP_FROM=noreply@yourdomain.com
SMTP_FROM_NAME=Your App Name
SMTP_SECURE=true                     # Use TLS (default: true)
SMTP_AUTH_METHOD=LOGIN               # Options: LOGIN, PLAIN, CRAM-MD5
```

### Provider-Specific Configurations

#### SendGrid

```bash
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=SG.your-api-key-here
SMTP_SECURE=true
SMTP_FROM=noreply@yourdomain.com
```

#### Amazon SES

```bash
SMTP_HOST=email-smtp.us-east-1.amazonaws.com
SMTP_PORT=587
SMTP_USER=your-ses-smtp-user
SMTP_PASSWORD=your-ses-smtp-password
SMTP_SECURE=true
SMTP_FROM=noreply@yourdomain.com
```

#### Postmark

```bash
SMTP_HOST=smtp.postmarkapp.com
SMTP_PORT=587
SMTP_USER=your-server-token
SMTP_PASSWORD=your-server-token
SMTP_SECURE=true
SMTP_FROM=noreply@yourdomain.com
```

#### Mailgun

```bash
SMTP_HOST=smtp.mailgun.org
SMTP_PORT=587
SMTP_USER=postmaster@mg.yourdomain.com
SMTP_PASSWORD=your-mailgun-password
SMTP_SECURE=true
SMTP_FROM=noreply@yourdomain.com
```

#### Custom SMTP Server

```bash
SMTP_HOST=mail.yourdomain.com
SMTP_PORT=587
SMTP_USER=your-username
SMTP_PASSWORD=your-password
SMTP_SECURE=true
SMTP_FROM=noreply@yourdomain.com
SMTP_AUTH_METHOD=LOGIN
```

### Auth Service Integration

The nself Auth service automatically uses the configured mail settings for authentication emails:

```bash
# Auth email settings (uses SMTP_* variables above)
AUTH_EMAIL_ENABLED=true
AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED=true

# Customize Auth email templates
AUTH_EMAIL_TEMPLATE_FETCH_URL=http://functions:3400/api/email-templates

# Password reset settings
AUTH_EMAIL_PASSWORD_RESET_ENABLED=true
AUTH_EMAIL_PASSWORD_RESET_REDIRECT_URL=https://yourapp.com/reset-password

# Email verification settings
AUTH_EMAIL_VERIFY_ENABLED=true
AUTH_EMAIL_VERIFY_REDIRECT_URL=https://yourapp.com/verify-email
```

## Access

### MailPit Web Interface

**Local Development:**
- URL: `https://mail.local.nself.org`
- No authentication required by default

**Direct Access:**
- URL: `http://localhost:8025`

### SMTP Endpoint

**Within Docker Network (MailPit):**
- Host: `mailpit`
- Port: `1025`

**From Host Machine (MailPit):**
- Host: `localhost`
- Port: `1025`

## Usage

### CLI Commands

Mail is managed through the `nself service email` command group:

```bash
# Check mail service status
nself service email status

# View mail configuration
nself service email config

# Send a test email
nself service email test --to user@example.com

# View recent emails (MailPit only)
nself service email list

# Clear all captured emails (MailPit only)
nself service email clear

# Open MailPit web interface
nself service email open

# Verify SMTP connectivity (production)
nself service email verify
```

### General Service Commands

```bash
# View mail service logs
nself logs mailpit

# Restart mail service
nself restart mailpit

# Check all service URLs
nself urls
```

### Sending Emails from Custom Services

#### Node.js (Nodemailer)

```javascript
import nodemailer from 'nodemailer';

const transporter = nodemailer.createTransport({
  host: process.env.SMTP_HOST || 'mailpit',
  port: parseInt(process.env.SMTP_PORT || '1025'),
  secure: process.env.SMTP_SECURE === 'true',
  auth: process.env.SMTP_USER ? {
    user: process.env.SMTP_USER,
    pass: process.env.SMTP_PASSWORD,
  } : undefined,
});

await transporter.sendMail({
  from: process.env.SMTP_FROM || 'noreply@local.nself.org',
  to: 'user@example.com',
  subject: 'Welcome to the application',
  html: '<h1>Welcome</h1><p>Your account has been created.</p>',
});
```

#### Python

```python
import smtplib
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
import os

smtp_host = os.environ.get('SMTP_HOST', 'mailpit')
smtp_port = int(os.environ.get('SMTP_PORT', 1025))

msg = MIMEMultipart('alternative')
msg['Subject'] = 'Welcome to the application'
msg['From'] = os.environ.get('SMTP_FROM', 'noreply@local.nself.org')
msg['To'] = 'user@example.com'

html = '<h1>Welcome</h1><p>Your account has been created.</p>'
msg.attach(MIMEText(html, 'html'))

with smtplib.SMTP(smtp_host, smtp_port) as server:
    if os.environ.get('SMTP_SECURE') == 'true':
        server.starttls()
    if os.environ.get('SMTP_USER'):
        server.login(os.environ['SMTP_USER'], os.environ['SMTP_PASSWORD'])
    server.send_message(msg)
```

#### Go

```go
import (
    "net/smtp"
    "os"
)

host := getEnvOrDefault("SMTP_HOST", "mailpit")
port := getEnvOrDefault("SMTP_PORT", "1025")

msg := []byte("From: noreply@local.nself.org\r\n" +
    "To: user@example.com\r\n" +
    "Subject: Welcome\r\n" +
    "Content-Type: text/html\r\n\r\n" +
    "<h1>Welcome</h1><p>Your account has been created.</p>")

err := smtp.SendMail(
    host+":"+port,
    nil, // No auth for MailPit
    "noreply@local.nself.org",
    []string{"user@example.com"},
    msg,
)
```

### Using the MailPit API for Testing

MailPit exposes a REST API for automated testing:

```bash
# List all messages
curl http://localhost:8025/api/v1/messages

# Search messages
curl http://localhost:8025/api/v1/search?query=welcome

# Get a specific message
curl http://localhost:8025/api/v1/message/{id}

# Delete all messages
curl -X DELETE http://localhost:8025/api/v1/messages
```

#### Integration Test Example (Node.js)

```javascript
import { describe, it, expect } from 'vitest';

describe('Email sending', () => {
  it('should send welcome email on registration', async () => {
    // Trigger registration
    await registerUser({ email: 'test@example.com' });

    // Wait briefly for email delivery
    await new Promise(resolve => setTimeout(resolve, 1000));

    // Check MailPit for the email
    const response = await fetch('http://localhost:8025/api/v1/messages');
    const data = await response.json();

    const welcomeEmail = data.messages.find(
      msg => msg.To[0].Address === 'test@example.com'
    );
    expect(welcomeEmail).toBeDefined();
    expect(welcomeEmail.Subject).toContain('Welcome');
  });
});
```

## Network and Routing

| Access Point | Address | Purpose |
|-------------|---------|---------|
| Web UI (Browser) | `https://mail.local.nself.org` | MailPit inbox viewer |
| SMTP (Docker) | `mailpit:1025` | Service-to-service email sending |
| SMTP (Host) | `localhost:1025` | Local development access |
| Web UI (Host) | `http://localhost:8025` | Direct MailPit UI access |
| API (Host) | `http://localhost:8025/api/v1/` | MailPit REST API |

## Resource Requirements

| Resource | Minimum | Recommended | Notes |
|----------|---------|-------------|-------|
| CPU | 0.05 cores | 0.1 cores | Very lightweight |
| Memory | 50MB | 128MB | Increases with stored messages |
| Storage | 50MB | 200MB | Depends on message volume |
| Network | Minimal | Low | SMTP traffic only |

MailPit is one of the lightest services in the nself stack. Storage usage is bounded by the `MAILPIT_MAX_MESSAGES` setting.

## Monitoring

### Health Checks

```bash
# Check mail service health
nself health mailpit

# Docker health check (built into compose config)
# Uses: wget --spider http://localhost:8025/
# Interval: 15s
# Timeout: 5s
# Retries: 3

# Verify SMTP is accepting connections
nself exec mailpit nc -z localhost 1025
```

### Production Email Monitoring

For production SMTP providers, monitor delivery through the provider's dashboard:

| Provider | Dashboard URL |
|----------|--------------|
| SendGrid | app.sendgrid.com |
| Amazon SES | AWS Console > SES |
| Postmark | account.postmarkapp.com |
| Mailgun | app.mailgun.com |

## Security

### Development (MailPit)

- MailPit is only accessible within the Docker network and on localhost
- No authentication required by default (development tool)
- Optionally protect the UI with htpasswd: `MAILPIT_UI_AUTH_FILE=/path/to/htpasswd`
- No emails are delivered externally

### Production (SMTP)

1. Store SMTP credentials in `.secrets` (never in `.env.dev`)
2. Use TLS for all SMTP connections (`SMTP_SECURE=true`)
3. Verify sender domain with SPF, DKIM, and DMARC records
4. Use dedicated API keys with minimal permissions
5. Monitor for unusual sending patterns
6. Set up bounce and complaint handling with your provider

### Email Template Security

- Sanitize all user-provided data before inserting into templates
- Never include sensitive data (passwords, tokens) in email bodies
- Use short-lived, one-time-use tokens for password reset links
- Set appropriate redirect URLs to prevent open redirect attacks

## Troubleshooting

### MailPit not starting

```bash
# Check MailPit logs
nself logs mailpit

# Verify MailPit is enabled
grep MAILPIT_ENABLED .env

# Check for port conflicts
lsof -i :1025
lsof -i :8025

# Run diagnostics
nself doctor
```

### Emails not appearing in MailPit

```bash
# Verify your service is sending to the correct SMTP host
# Inside Docker: host=mailpit, port=1025
# From host: host=localhost, port=1025

# Check MailPit is receiving connections
nself logs mailpit --follow

# Send a test email
nself service email test --to test@example.com

# Verify SMTP connectivity from your service container
nself exec your-service nc -z mailpit 1025
```

### Production emails not being delivered

```bash
# Verify SMTP configuration
nself service email verify

# Test SMTP connectivity
nself service email test --to your-real-email@example.com

# Check Auth service logs for email errors
nself logs auth | grep -i email

# Common issues:
# - Incorrect SMTP credentials
# - Sender domain not verified with provider
# - Provider rate limits exceeded
# - TLS/SSL configuration mismatch
```

### Auth emails not sending

```bash
# Verify Auth email configuration
grep AUTH_EMAIL .env

# Check Auth service logs
nself logs auth

# Ensure SMTP settings are correct
grep SMTP .env

# Verify MailPit is running (development)
nself status mailpit
```

### Web UI not loading

```bash
# Check nginx routing
nself urls

# Verify MailPit is running
nself status

# Test direct access
curl -s http://localhost:8025/

# Rebuild nginx configuration
nself build --force && nself restart nginx
```

## Switching Between Development and Production

When deploying to staging or production, replace MailPit with your SMTP provider:

```bash
# Development .env.dev
MAILPIT_ENABLED=true

# Production .env.prod (overrides .env.dev)
MAILPIT_ENABLED=false
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=your-api-key
SMTP_FROM=noreply@yourdomain.com
SMTP_SECURE=true
```

The nself Auth service and your custom services will automatically use the correct SMTP configuration based on the active environment file.

## Related Documentation

- [Optional Services Overview](SERVICES_OPTIONAL.md) - All optional services
- [Services Overview](SERVICES.md) - Complete service listing
- [Auth Documentation](../commands/AUTH.md) - Authentication email configuration
- [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md) - Full configuration reference
- [Custom Services](SERVICES_CUSTOM.md) - Sending emails from custom services
- [Secrets Management](../configuration/SECRETS-MANAGEMENT.md) - Storing SMTP credentials securely
- [Troubleshooting](../troubleshooting/README.md) - Common issues and solutions
