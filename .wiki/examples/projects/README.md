# nself Example Projects

This directory contains complete, production-ready example projects demonstrating various use cases for nself v0.9.9.

## Available Examples

### 1. Simple Blog (Quickstart)
**Directory:** `01-simple-blog/`
**Difficulty:** Beginner
**Time to Complete:** 15-30 minutes

A minimal blog application demonstrating core nself features:
- PostgreSQL database with basic schema
- Hasura GraphQL API
- nHost Authentication
- React frontend
- Basic deployment

**Perfect for:** Learning nself fundamentals, getting started quickly

---

### 2. SaaS Starter (Multi-Tenant)
**Directory:** `02-saas-starter/`
**Difficulty:** Intermediate
**Time to Complete:** 2-4 hours

A complete SaaS application template with:
- Multi-tenant architecture with data isolation
- Stripe billing integration
- User invitations and team management
- Role-based access control (RBAC)
- Admin dashboard
- API rate limiting

**Perfect for:** Building SaaS products, understanding multi-tenancy

---

### 3. E-commerce Platform
**Directory:** `03-ecommerce/`
**Difficulty:** Intermediate
**Time to Complete:** 3-5 hours

Full-featured e-commerce solution:
- Product catalog and inventory
- Shopping cart and checkout
- Stripe payment processing
- Order management
- Email notifications (MailPit)
- MinIO for product images

**Perfect for:** Online stores, marketplace platforms

---

### 4. Real-Time Chat
**Directory:** `04-realtime-chat/`
**Difficulty:** Intermediate
**Time to Complete:** 2-3 hours

Real-time messaging application:
- WebSocket connections via Hasura
- Presence tracking
- Message history and search (MeiliSearch)
- File attachments (MinIO)
- Push notifications
- Read receipts

**Perfect for:** Chat apps, collaboration tools, real-time features

---

### 5. API-First Backend
**Directory:** `05-api-backend/`
**Difficulty:** Advanced
**Time to Complete:** 3-4 hours

Production-grade REST + GraphQL APIs:
- REST endpoints (Express.js custom service)
- GraphQL API (Hasura)
- OpenAPI documentation
- Multiple auth strategies (JWT, API keys)
- Rate limiting (Redis)
- Comprehensive monitoring

**Perfect for:** Mobile app backends, API services, microservices

---

### 6. Machine Learning Platform
**Directory:** `06-ml-platform/`
**Difficulty:** Advanced
**Time to Complete:** 4-6 hours

Complete ML workflow platform:
- MLflow for experiment tracking
- Model training with custom services
- Model serving API
- Dataset management (MinIO)
- Jupyter notebooks integration
- GPU support (optional)

**Perfect for:** ML/AI applications, data science platforms

---

## Quick Start

Each example includes:
- `README.md` - Overview and features
- `TUTORIAL.md` - Step-by-step walkthrough
- `.env.example` - Configuration template
- `schema.sql` - Database schema
- `hasura/` - Hasura metadata
- Complete source code

### Running an Example

```bash
# 1. Choose an example
cd examples/01-simple-blog/

# 2. Copy environment template
cp .env.example .env

# 3. Review and customize .env
nano .env

# 4. Initialize nself
nself init

# 5. Build infrastructure
nself build

# 6. Start services
nself start

# 7. Follow the tutorial
cat TUTORIAL.md
```

## Learning Path

**New to nself?** Start here:
1. **01-simple-blog** - Learn the basics
2. **04-realtime-chat** - Understand real-time features
3. **02-saas-starter** - Master multi-tenancy

**Building a SaaS?** Follow this path:
1. **02-saas-starter** - Foundation
2. **05-api-backend** - API design
3. **03-ecommerce** - Payments (if needed)

**Data/ML Focus?** Try these:
1. **05-api-backend** - API foundation
2. **06-ml-platform** - ML workflow

## Example Comparison

| Feature | Simple Blog | SaaS Starter | E-commerce | Realtime Chat | API Backend | ML Platform |
|---------|-------------|--------------|------------|---------------|-------------|-------------|
| **Difficulty** | Beginner | Intermediate | Intermediate | Intermediate | Advanced | Advanced |
| **Services** | 4 | 8 | 9 | 7 | 10 | 11 |
| **Multi-Tenant** | No | Yes | Optional | No | Yes | No |
| **Payments** | No | Yes | Yes | No | No | No |
| **Real-time** | No | No | No | Yes | No | No |
| **ML/AI** | No | No | No | No | No | Yes |
| **Custom Services** | 0 | 2 | 3 | 1 | 4 | 3 |
| **Frontend** | React | Next.js | Next.js | Vue.js | None | React |

## Production Deployment

All examples include production deployment guides covering:
- Server requirements and setup
- SSL certificates (Let's Encrypt)
- Security hardening
- Backup strategies
- Monitoring and alerts
- Scaling considerations

See each example's `DEPLOYMENT.md` for details.

## Contributing Examples

Have a great nself example? We'd love to include it!

See `CONTRIBUTING.md` in the root directory for guidelines.

## Support

- **Documentation:** [docs.nself.org](https://github.com/nself-org/cli/wiki)
- **Tutorials:** [Tutorials](../../tutorials/README.md)
- **Issues:** [GitHub Issues](https://github.com/nself-org/cli/issues)
- **Discussions:** [GitHub Discussions](https://github.com/nself-org/cli/discussions)

## License

All examples are licensed under MIT. See individual example directories for details.

---

**Version:** 0.9.8
**Last Updated:** January 2026
