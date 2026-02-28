# Feature Documentation

Detailed documentation for specific nself features and capabilities.

## Overview

This directory contains in-depth documentation for nself's key features, including real-time subscriptions, white-label customization, and file upload pipelines.

## Available Features

### Real-Time Features
- **[Real-Time Overview](REALTIME.md)** - Real-time subscriptions and live data
- **[Real-Time Examples](realtime-examples.md)** - Code examples and patterns
- **[Real-Time Chat Service](../examples/REALTIME-CHAT-SERVICE.md)** - Building a chat service

#### What Real-Time Provides

- GraphQL subscriptions via WebSocket
- Live database updates
- Presence tracking
- Typing indicators
- Live notifications
- Real-time analytics

#### Quick Example

```graphql
subscription OnNewMessage {
  messages(order_by: {created_at: desc}, limit: 1) {
    id
    content
    user {
      name
    }
  }
}
```

### White-Label System
- **[White-Label System](WHITELABEL-SYSTEM.md)** - Complete customization system
- **[White-Label Architecture](../architecture/WHITE-LABEL-ARCHITECTURE.md)** - System architecture
- **[Customization Guide](../guides/WHITE-LABEL-CUSTOMIZATION.md)** - Implementation guide
- **[Branding Quick Start](../guides/BRANDING-QUICK-START.md)** - Quick start guide
- **[Themes](../guides/THEMES.md)** - Theme customization

#### What White-Label Provides

- Custom domains per tenant
- Brand colors and logos
- Custom email templates
- Themed UI components
- Tenant-specific assets
- Custom CSS/styling

#### Quick Example

```bash
# Set tenant branding
nself tenant branding set-colors --primary #0066cc --secondary #ff6600

# Upload logo
nself tenant branding logo upload logo.png

# Configure custom domain
nself tenant domains add app.example.com
```

### File Upload Pipeline
- **[File Upload Pipeline](file-upload-pipeline.md)** - Complete upload system
- **[File Upload Examples](../guides/file-upload-examples.md)** - Code examples
- **[File Upload Quick Start](../tutorials/file-uploads-quickstart.md)** - Tutorial
- **[File Upload Security](../security/file-upload-security.md)** - Security best practices

#### What File Uploads Provide

- Direct uploads to storage
- Image thumbnails
- Virus scanning
- File compression
- Metadata extraction
- Access control
- CDN integration

#### Quick Example

```bash
# Upload with all features
nself service storage upload photo.jpg --all-features

# Generates:
# - Original file
# - Thumbnail (200x200)
# - Compressed version
# - Virus scan
# - Database record
```

## Feature Integration

### Combining Features

Features work together seamlessly:

```typescript
// Real-time file upload notifications
subscription OnFileUploaded {
  files(
    where: { tenant_id: { _eq: $tenantId } }
    order_by: { created_at: desc }
  ) {
    id
    filename
    thumbnail_url
    uploaded_by {
      name
      avatar_url
    }
  }
}
```

### Multi-Tenant Features

All features support multi-tenancy:

- **Real-Time**: Tenant-isolated subscriptions
- **White-Label**: Per-tenant branding
- **File Uploads**: Tenant-specific storage buckets

## Related Documentation

### Guides
- **[Real-Time Features Guide](../guides/REALTIME-FEATURES.md)** - Comprehensive real-time guide
- **[White-Label Customization](../guides/WHITE-LABEL-CUSTOMIZATION.md)** - Customization workflow
- **[File Upload Pipeline Guide](../guides/file-upload-pipeline.md)** - Upload implementation

### Commands
- **[Realtime Commands](../commands/REALTIME.md)** - Real-time CLI commands
- **[Storage Commands](../commands/storage.md)** - Storage CLI commands
- **[Tenant Commands](../commands/TENANT.md)** - Multi-tenancy commands

### Architecture
- **[Multi-Tenancy Architecture](../architecture/MULTI-TENANCY.md)** - Tenant isolation
- **[White-Label Architecture](../architecture/WHITE-LABEL-ARCHITECTURE.md)** - Branding system

### Tutorials
- **[Quick Start Tutorials](../tutorials/README.md)** - Step-by-step guides
- **[File Uploads Quick Start](../tutorials/file-uploads-quickstart.md)** - Upload tutorial

## Examples

Find complete examples in:

- **[Examples Directory](../examples/README.md)** - All examples
- **[Real-Time Chat Service](../examples/REALTIME-CHAT-SERVICE.md)** - Chat implementation
- **[Features Overview](../examples/FEATURES-OVERVIEW.md)** - Feature examples

---

**Last Updated**: January 31, 2026
**Version**: v0.9.6
