# Nginx Architecture

How Nginx routes traffic in nself.

## Overview

Nginx acts as:
- **Reverse proxy** - Routes requests to backend services
- **SSL terminator** - Handles HTTPS
- **Load balancer** - Distributes traffic
- **Security gateway** - Applies security headers

## Routing

```
api.* → Hasura GraphQL
auth.* → Auth service
admin.* → Admin UI
* → Custom services
```

See [Build Configuration](../configuration/BUILD-CONFIG.md) for routing details.

