# nself Examples Roadmap

This document outlines the complete example project suite for nself v0.9.9.

## Status Overview

| Example | Status | Difficulty | Time | Completion |
|---------|--------|------------|------|------------|
| 01 - Simple Blog | ✅ Complete | Beginner | 30 min | 100% |
| 02 - SaaS Starter | ✅ Complete | Intermediate | 2-4 hours | 100% |
| 03 - E-commerce | 📋 Planned | Intermediate | 3-5 hours | 0% |
| 04 - Realtime Chat | 📋 Planned | Intermediate | 2-3 hours | 0% |
| 05 - API Backend | 📋 Planned | Advanced | 3-4 hours | 0% |
| 06 - ML Platform | 📋 Planned | Advanced | 4-6 hours | 0% |

---

## Completed Examples

### ✅ Example 01: Simple Blog

**Location:** `/examples/01-simple-blog/`

**Status:** Complete with full documentation

**Includes:**
- ✅ README.md - Complete overview
- ✅ TUTORIAL.md - Step-by-step guide
- ✅ .env.example - Environment template
- ✅ database/schema.sql - Complete database schema
- ✅ Database functions and triggers
- ✅ Sample data structure
- 🔄 Frontend code (to be added)
- 🔄 Hasura metadata (to be added)

**Features Demonstrated:**
- Basic CRUD operations
- PostgreSQL with auth integration
- Hasura GraphQL API
- User authentication
- Comments system
- Auto-slug generation
- RLS basics

**Next Steps:**
- Add frontend React code
- Add Hasura metadata export
- Add deployment guide
- Create video tutorial

---

### ✅ Example 02: SaaS Starter

**Location:** `/examples/02-saas-starter/`

**Status:** Complete architecture documentation

**Includes:**
- ✅ README.md - Comprehensive overview
- ✅ Architecture diagrams
- ✅ Multi-tenancy explanation
- ✅ Billing integration guide
- ✅ Team management flow
- 🔄 Database schema (to be added)
- 🔄 API service code (to be added)
- 🔄 Frontend code (to be added)

**Features Demonstrated:**
- Multi-tenant architecture
- Row-Level Security (RLS)
- Stripe billing integration
- Team and invitation management
- Role-based permissions
- Usage tracking
- Admin dashboard
- Subscription management

**Next Steps:**
- Add complete database schema
- Add NestJS API service
- Add Next.js frontend
- Add Hasura metadata
- Create deployment guide
- Build tutorial walkthrough

---

## Planned Examples

### 📋 Example 03: E-commerce Platform

**Location:** `/examples/03-ecommerce/`

**Difficulty:** Intermediate
**Time:** 3-5 hours
**Priority:** High

**Planned Features:**
- Product catalog with categories
- Shopping cart (Redis-based)
- Stripe payment processing
- Order management
- Inventory tracking
- Product images (MinIO)
- Email notifications (order confirmations)
- Invoice generation
- Customer accounts
- Admin dashboard

**Technology Stack:**
- PostgreSQL - Product database
- Redis - Cart and sessions
- MinIO - Product images
- Hasura - GraphQL API
- NestJS - Payment processing API
- Next.js - Storefront + Admin
- MailPit/SMTP - Email notifications

**Database Tables:**
- products
- categories
- product_images
- cart_items
- orders
- order_items
- customers
- inventory
- payment_transactions

**Key Learning Points:**
- E-commerce data modeling
- Payment processing (Stripe)
- Inventory management
- Order workflows
- Email automation
- Image handling

---

### 📋 Example 04: Real-Time Chat Application

**Location:** `/examples/04-realtime-chat/`

**Difficulty:** Intermediate
**Time:** 2-3 hours
**Priority:** High

**Planned Features:**
- Real-time messaging (WebSocket)
- Direct messages
- Group channels
- Message history
- File attachments
- User presence (online/offline)
- Read receipts
- Typing indicators
- Message search (MeiliSearch)
- Push notifications

**Technology Stack:**
- PostgreSQL - Message storage
- Hasura - Real-time subscriptions
- Redis - Presence tracking
- MinIO - File attachments
- MeiliSearch - Message search
- Vue.js - Chat UI

**Database Tables:**
- channels
- channel_members
- messages
- message_attachments
- user_presence
- read_receipts

**Key Learning Points:**
- WebSocket subscriptions
- Real-time data sync
- Presence tracking
- File upload handling
- Full-text search
- Performance optimization

---

### 📋 Example 05: API-First Backend

**Location:** `/examples/05-api-backend/`

**Difficulty:** Advanced
**Time:** 3-4 hours
**Priority:** Medium

**Planned Features:**
- RESTful API (Express/NestJS)
- GraphQL API (Hasura)
- API versioning
- OpenAPI/Swagger documentation
- Multiple auth strategies (JWT, API keys, OAuth)
- Rate limiting (Redis)
- Request validation
- Response caching
- API analytics
- Webhook system

**Technology Stack:**
- PostgreSQL - Data storage
- Redis - Rate limiting & caching
- Hasura - GraphQL layer
- NestJS - REST API
- OpenAPI - Documentation
- Prometheus - Metrics

**Custom Services:**
- CS_1: REST API (NestJS) - Port 8001
- CS_2: Webhook processor - Port 8002
- CS_3: Analytics service - Port 8003
- CS_4: Rate limiter - Port 8004

**Key Learning Points:**
- API design best practices
- Authentication strategies
- Rate limiting implementation
- API documentation
- Caching strategies
- Monitoring and analytics

---

### 📋 Example 06: ML Platform

**Location:** `/examples/06-ml-platform/`

**Difficulty:** Advanced
**Time:** 4-6 hours
**Priority:** Medium

**Planned Features:**
- Experiment tracking (MLflow)
- Model training pipeline
- Model versioning
- Model serving API
- Dataset management (MinIO)
- Jupyter notebook integration
- Training job queue (BullMQ)
- Model monitoring
- A/B testing
- Feature store

**Technology Stack:**
- PostgreSQL - Metadata storage
- MLflow - Experiment tracking
- MinIO - Dataset & model storage
- Redis - Job queue
- Python API - Model serving
- BullMQ - Job processing
- Grafana - Model monitoring

**Custom Services:**
- CS_1: Training worker (Python) - Port 8001
- CS_2: Inference API (Python) - Port 8002
- CS_3: Job scheduler (BullMQ) - Port 8003

**Database Tables:**
- experiments
- model_versions
- datasets
- training_jobs
- predictions
- feature_store

**Key Learning Points:**
- ML workflow orchestration
- Model versioning
- Dataset management
- Distributed training
- Model serving
- Monitoring ML models

---

## Example Development Timeline

### Phase 1: Core Examples (Complete)
- ✅ 01 - Simple Blog
- ✅ 02 - SaaS Starter (documentation)

### Phase 2: E-commerce & Chat (Weeks 1-2)
- 03 - E-commerce Platform
- 04 - Real-Time Chat

### Phase 3: Advanced Examples (Weeks 3-4)
- 05 - API Backend
- 06 - ML Platform

### Phase 4: Polish & Documentation (Week 5)
- Complete all frontend code
- Add deployment guides
- Create video tutorials
- Write blog posts

---

## Common Patterns Across Examples

All examples follow consistent structure:

```
example-name/
├── README.md              # Overview & features
├── TUTORIAL.md            # Step-by-step guide
├── DEPLOYMENT.md          # Production deployment
├── .env.example           # Configuration template
├── database/
│   ├── schema.sql        # Database schema
│   ├── functions/        # SQL functions
│   ├── migrations/       # Migration files
│   └── seeds/            # Sample data
├── hasura/
│   ├── metadata/         # Hasura configuration
│   └── migrations/       # Hasura migrations
├── api/                  # Custom API service (if needed)
│   ├── src/
│   ├── Dockerfile
│   └── package.json
├── frontend/             # Frontend application
│   ├── src/
│   └── package.json
└── docs/                 # Additional documentation
    ├── API.md
    ├── ARCHITECTURE.md
    └── TROUBLESHOOTING.md
```

---

## Documentation Standards

Each example must include:

### README.md
- Clear overview of what's built
- Feature list
- Architecture diagram
- Quick start instructions
- Technology stack
- Project structure
- Configuration guide
- Testing instructions
- Deployment overview

### TUTORIAL.md
- Step-by-step walkthrough
- Estimated time to complete
- Prerequisites
- Learning objectives
- Code examples with explanations
- Screenshots/diagrams
- Common pitfalls
- Next steps

### DEPLOYMENT.md
- Production checklist
- Server requirements
- Security hardening
- SSL setup
- Environment variables
- Backup configuration
- Monitoring setup
- Scaling guide

---

## Testing Requirements

Each example must be:

1. **Functional** - All features work as documented
2. **Tested** - Includes unit and integration tests
3. **Documented** - Clear instructions and comments
4. **Deployable** - Can be deployed to production
5. **Maintainable** - Follows best practices

### Test Checklist

- [ ] All services start successfully
- [ ] Database schema applies without errors
- [ ] Hasura metadata imports correctly
- [ ] Frontend builds and runs
- [ ] All API endpoints work
- [ ] Authentication flows function
- [ ] Permissions are correct
- [ ] Tests pass
- [ ] Deployment succeeds

---

## Community Contributions

We welcome community-contributed examples!

**Potential community examples:**
- Mobile app backend (React Native)
- IoT data platform
- Social media app
- CRM system
- Project management tool
- Video streaming platform
- Educational platform
- Healthcare application
- Real estate platform
- Booking system

**Contribution guidelines:**
- Follow example structure
- Include complete documentation
- Provide working code
- Add tests
- Create tutorial

See [CONTRIBUTING.md](../../contributing/CONTRIBUTING.md) for details.

---

## Resources for Example Development

### Templates
- `/src/templates/services/` - Custom service templates
- `/docs/reference/SERVICE-SCAFFOLDING-CHEATSHEET.md` - Service guide

### Documentation
- `/docs/tutorials/` - Tutorial references
- `/docs/examples/` - Example configuration files
- `/docs/reference/` - Quick reference guides

### Tools
- nself CLI - Project scaffolding
- Hasura Console - GraphQL configuration
- pgAdmin - Database management
- Postman - API testing

---

## Success Metrics

**Example Quality Indicators:**

- Time to complete < estimated time
- Clear, understandable code
- Comprehensive documentation
- Working deployment
- Positive community feedback
- Low issue count
- High reusability

**Target Metrics:**

- 90%+ users complete successfully
- <5 issues per example
- 100% test coverage
- <10 minutes to deploy
- 4.5+ star rating

---

## Maintenance

### Regular Updates

- Update for new nself versions
- Fix reported issues
- Improve documentation
- Add community suggestions
- Security patches

### Version Support

- Examples target current nself version
- Archive examples for old versions
- Migration guides between versions

---

## Future Examples (v0.10.0+)

Potential future examples:
- Serverless functions platform
- GraphQL federation example
- Kubernetes deployment
- Multi-region setup
- Plugin system example
- Custom authentication provider
- Advanced monitoring setup
- CI/CD pipeline example

---

**Last Updated:** January 31, 2026
**Version:** 0.9.8
**Status:** Phase 1 Complete, Phase 2 In Planning
