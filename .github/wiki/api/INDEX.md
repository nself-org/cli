# API Documentation

Complete API reference for all nself APIs and integrations.

---

## Overview

nself provides multiple APIs for accessing your backend services, including GraphQL, REST, and plugin-specific APIs.

---

## API Documentation

### Complete Reference

- **[Complete API Reference](API-REFERENCE-COMPLETE.md)** - Comprehensive API documentation

### GraphQL API

- **[GraphQL Overview](../architecture/API.md)** - Hasura GraphQL Engine documentation
- Auto-generated from your database schema
- Real-time subscriptions
- Role-based access control

### REST APIs

| API | Documentation |
|-----|---------------|
| **Billing API** | [Billing API Reference](../reference/api/BILLING-API.md) |
| **White-Label API** | [White-Label API Reference](../reference/api/WHITE-LABEL-API.md) |

### Plugin APIs

Plugin-specific APIs are documented in each plugin's documentation:

- **[Stripe Plugin](../plugins/stripe.md)** - Payment processing API
- **[GitHub Plugin](../plugins/github.md)** - Repository and workflow API
- **[Shopify Plugin](../plugins/shopify.md)** - E-commerce API

---

## Quick Start

### GraphQL API

```bash
# Access GraphQL console
open https://api.local.nself.org

# Generate TypeScript types
nself db types
```

### Authentication

```bash
# Get auth endpoints
nself urls auth

# Test authentication
curl https://auth.local.nself.org/healthz
```

---

## Related Documentation

- **[Architecture](../architecture/ARCHITECTURE.md)** - System architecture
- **[Services](../services/SERVICES.md)** - Available services
- **[Security](../security/README.md)** - API security

---

**[← Back to Documentation](../README.md)**
