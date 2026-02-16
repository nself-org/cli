# nself Feature Comparison

Comprehensive comparison of nself vs other Backend-as-a-Service (BaaS) platforms.

**Last Updated**: January 31, 2026
**nself Version**: 0.9.8

---

## Quick Comparison

| Feature | nself | Supabase | Nhost | Firebase | DIY |
|---------|-------|----------|-------|----------|-----|
| **Deployment** |
| Self-Hosted | ✅ | ✅ | ✅ | ❌ | ✅ |
| Cloud-Hosted | ❌ | ✅ | ✅ | ✅ | ❌ |
| **Database** |
| PostgreSQL | ✅ | ✅ | ✅ | ❌ (Firestore) | ✅ |
| Real-time Subscriptions | ✅ | ✅ | ✅ | ✅ | Manual |
| Row-Level Security | ✅ | ✅ | ✅ | ⚠️ Limited | Manual |
| **API** |
| GraphQL | ✅ (Hasura) | ⚠️ Limited | ✅ (Hasura) | ❌ | Manual |
| REST API | ✅ (PostgREST) | ✅ | ✅ | ✅ | Manual |
| **Authentication** |
| Built-in Auth | ✅ | ✅ | ✅ | ✅ | Manual |
| OAuth Providers | ✅ (13) | ✅ (11) | ✅ (10) | ✅ (8) | Manual |
| **Multi-Tenancy** |
| Built-in Multi-Tenancy | ✅ | ❌ | ❌ | ❌ | Manual |
| Tenant Isolation (RLS) | ✅ | Manual | Manual | Manual | Manual |
| **Enterprise** |
| Billing Integration | ✅ | ❌ | ❌ | ❌ | Manual |
| White-Label | ✅ | ❌ | ❌ | ❌ | Manual |
| Custom Branding | ✅ | ⚠️ Limited | ⚠️ Limited | ❌ | ✅ |
| **Storage** |
| File Storage | ✅ (MinIO) | ✅ | ✅ | ✅ | Manual |
| Image Optimization | ✅ | ✅ | ✅ | ✅ | Manual |
| **Pricing** |
| Open Source | ✅ MIT | ✅ Apache | ✅ MIT | ❌ | ✅ |
| Free Tier | ✅ Unlimited | ✅ Limited | ✅ Limited | ✅ Limited | ✅ |
| Self-Hosted Cost | Infrastructure only | Infrastructure only | Infrastructure only | N/A | Infrastructure only |

---

## Detailed Comparison

### Database & API

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **Database Engine** | PostgreSQL 16 | PostgreSQL 15 | PostgreSQL 15 | Firestore (NoSQL) |
| **Extensions** | 60+ | 45+ | 30+ | N/A |
| **GraphQL API** | ✅ Full (Hasura) | ⚠️ Limited | ✅ Full (Hasura) | ❌ |
| **REST API** | ✅ (PostgREST) | ✅ (PostgREST) | ✅ (Hasura) | ✅ |
| **Real-Time** | ✅ WebSocket | ✅ WebSocket | ✅ WebSocket | ✅ WebSocket |
| **Migrations** | ✅ Full | ✅ Full | ✅ Full | ❌ |
| **Seeding** | ✅ Environment-aware | Manual | Manual | Manual |
| **Type Generation** | ✅ TS/Go/Python | ✅ TypeScript | ✅ TypeScript | ❌ |
| **Schema Designer** | ✅ DBML | ❌ | ❌ | ❌ |

### Authentication & Security

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **Email/Password** | ✅ | ✅ | ✅ | ✅ |
| **Magic Link** | ✅ | ✅ | ✅ | ✅ |
| **OAuth Providers** | 13 | 11 | 10 | 8 |
| **MFA/2FA** | ✅ | ✅ | ✅ | ✅ |
| **Phone Auth** | ⚠️ Planned | ✅ | ✅ | ✅ |
| **JWT Tokens** | ✅ | ✅ | ✅ | ✅ |
| **Row-Level Security** | ✅ | ✅ | ✅ | Rules |
| **Rate Limiting** | ✅ | ⚠️ Cloud only | ⚠️ Cloud only | ✅ |
| **DDoS Protection** | ✅ | ⚠️ Cloud only | ⚠️ Cloud only | ✅ |

### Multi-Tenancy

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **Built-in Multi-Tenancy** | ✅ | ❌ | ❌ | ❌ |
| **Tenant Isolation** | ✅ RLS | Manual | Manual | Manual |
| **Tenant Management** | ✅ CLI | Manual | Manual | Manual |
| **Per-Tenant Billing** | ✅ | Manual | Manual | Manual |
| **Member Management** | ✅ | Manual | Manual | Manual |
| **Organization Support** | ✅ | Manual | Manual | Manual |

### Enterprise Features

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **Billing Integration** | ✅ Stripe | ❌ | ❌ | ❌ |
| **Usage Tracking** | ✅ | ⚠️ Cloud only | ⚠️ Cloud only | ✅ |
| **White-Label** | ✅ Full | ❌ | ❌ | ❌ |
| **Custom Domains** | ✅ | ✅ | ✅ | ✅ |
| **Custom Branding** | ✅ | ⚠️ Limited | ⚠️ Limited | ❌ |
| **Email Templates** | ✅ Customizable | ⚠️ Limited | ⚠️ Limited | ⚠️ Limited |
| **Compliance (GDPR)** | ✅ 85% | ✅ | ✅ | ✅ |
| **Compliance (HIPAA)** | ✅ 75% | ✅ Cloud | ✅ Cloud | ✅ Cloud |
| **SOC 2** | ✅ 70% | ✅ Cloud | ❌ | ✅ Cloud |

### Deployment & Operations

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **Self-Hosted** | ✅ | ✅ | ✅ | ❌ |
| **Cloud Providers** | 26+ | AWS only | AWS/GCP | Google only |
| **Kubernetes** | ✅ | ✅ | ✅ | N/A |
| **Docker Compose** | ✅ | ✅ | ✅ | N/A |
| **One-Click Deploy** | ✅ | ✅ | ✅ | N/A |
| **Zero-Downtime** | ✅ | ✅ | ✅ | ✅ |
| **Auto-Scaling** | ✅ | ✅ Cloud | ✅ Cloud | ✅ |
| **Backup/Restore** | ✅ | ✅ | ✅ | ✅ |
| **Monitoring** | ✅ Full Stack | ⚠️ Limited | ⚠️ Limited | ✅ Cloud |

### Developer Experience

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **CLI Tool** | ✅ Full | ✅ Full | ✅ Full | ✅ Full |
| **Local Development** | ✅ | ✅ | ✅ | ✅ Emulators |
| **Service Templates** | 40+ | ❌ | ~10 | ❌ |
| **Database GUI** | ✅ (Admin) | ✅ Studio | ✅ Console | ✅ Console |
| **API Docs** | ✅ Auto-generated | ✅ Auto-generated | ✅ Auto-generated | ✅ |
| **Type Safety** | ✅ TS/Go/Python | ✅ TypeScript | ✅ TypeScript | ⚠️ Limited |
| **Testing Tools** | ✅ | ⚠️ Limited | ⚠️ Limited | ✅ |

### Storage & Files

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **File Upload** | ✅ | ✅ | ✅ | ✅ |
| **S3-Compatible** | ✅ MinIO | ✅ | ✅ | ❌ |
| **Image Resize** | ✅ | ✅ | ✅ | ✅ |
| **Image Optimization** | ✅ | ✅ | ✅ | ✅ |
| **Virus Scanning** | ✅ | ⚠️ Enterprise | ⚠️ Enterprise | ❌ |
| **CDN Integration** | ✅ | ✅ | ✅ | ✅ |
| **Resumable Uploads** | ✅ | ✅ | ✅ | ✅ |

### Serverless Functions

| Feature | nself | Supabase | Nhost | Firebase |
|---------|-------|----------|-------|----------|
| **Functions Support** | ✅ TypeScript | ✅ Edge Functions | ✅ TypeScript | ✅ Cloud Functions |
| **Triggers** | ✅ DB/Auth | ✅ DB/Auth | ✅ DB/Auth | ✅ |
| **Cron Jobs** | ✅ | ✅ | ✅ | ✅ |
| **WebSockets** | ✅ | ⚠️ Limited | ⚠️ Limited | ❌ |
| **Custom Runtime** | ✅ | ❌ | ❌ | ❌ |

---

## Unique Features

### Only in nself

1. **Built-in Multi-Tenancy** - Complete tenant management out of the box
2. **Billing Integration** - Stripe integration with usage tracking
3. **White-Label Platform** - Full customization for resellers
4. **40+ Service Templates** - Microservices in any language
5. **DBML Schema Workflow** - Design → Import → Migrate → Seed
6. **Environment-Aware Seeding** - Different data for local/staging/prod
7. **26+ Cloud Providers** - Deploy anywhere with one command
8. **Complete Admin UI** - Full-featured management dashboard

### Only in Supabase

1. **Edge Functions** - Deno-based serverless at the edge
2. **Managed Cloud** - Fully managed SaaS option
3. **Larger Community** - More users and contributors

### Only in Nhost

1. **GraphQL-First** - GraphQL is the primary API
2. **Managed Cloud** - Fully managed option available

### Only in Firebase

1. **Google Infrastructure** - Leverages Google Cloud
2. **Mobile SDKs** - First-class mobile support
3. **Crashlytics** - Built-in crash reporting
4. **Analytics** - Built-in analytics

---

## Cost Comparison

### Self-Hosted (Monthly)

| Component | nself | Supabase | Nhost |
|-----------|-------|----------|-------|
| **Small (2GB RAM, 2 vCPU)** | $10-20 | $10-20 | $10-20 |
| **Medium (4GB RAM, 2 vCPU)** | $20-40 | $20-40 | $20-40 |
| **Large (8GB RAM, 4 vCPU)** | $40-80 | $40-80 | $40-80 |

*Actual costs depend on provider. These are DigitalOcean estimates.*

### Cloud-Hosted (Monthly)

| Usage | nself | Supabase | Nhost | Firebase |
|-------|-------|----------|-------|----------|
| **Free Tier** | Unlimited | 500MB DB, 1GB storage | 1GB DB, 1GB storage | 1GB storage, 50K reads |
| **Small Project** | Infrastructure only | $25 | $25 | $25-100 |
| **Medium Project** | Infrastructure only | $99 | $99 | $100-500 |
| **Large Project** | Infrastructure only | $599+ | $599+ | $500-2000+ |

**Note**: nself is self-hosted only, so you only pay for infrastructure (VPS, cloud, etc.)

---

## When to Choose

### Choose nself if you need:
- ✅ Complete multi-tenancy out of the box
- ✅ Built-in billing and subscription management
- ✅ White-label platform for reselling
- ✅ Deploy anywhere (26+ cloud providers)
- ✅ Full control over your infrastructure
- ✅ No vendor lock-in
- ✅ Custom service templates
- ✅ Enterprise features without enterprise pricing

### Choose Supabase if you need:
- ✅ Managed cloud service
- ✅ Edge functions at CDN locations
- ✅ Don't want to manage infrastructure
- ✅ Larger community and ecosystem

### Choose Nhost if you need:
- ✅ GraphQL-first architecture
- ✅ Managed cloud option
- ✅ Hasura expertise available

### Choose Firebase if you need:
- ✅ Mobile-first development
- ✅ Google Cloud infrastructure
- ✅ Built-in analytics
- ✅ Established enterprise option

### Choose DIY if you need:
- ✅ Complete customization
- ✅ Learning experience
- ✅ No framework constraints

---

## Migration Support

### From Supabase to nself

**Difficulty**: Easy
**Time**: 1-2 hours
**Guide**: docs/migrations/FROM-SUPABASE.md

**Migration includes:**
- Database schema
- Authentication users
- Storage files
- Environment variables
- Configuration

### From Nhost to nself

**Difficulty**: Easy
**Time**: 1-2 hours
**Guide**: docs/migrations/FROM-NHOST.md

### From Firebase to nself

**Difficulty**: Medium
**Time**: 4-8 hours
**Guide**: docs/migrations/FROM-FIREBASE.md

**Note**: Firebase uses NoSQL (Firestore), requires schema redesign for PostgreSQL

---

## Conclusion

nself is the **only self-hosted BaaS with built-in multi-tenancy, billing, and white-label support**.

If you need:
- Control over your infrastructure
- Multi-tenancy out of the box
- Billing integration
- White-label customization
- Deploy anywhere flexibility

**nself is the best choice.**

For managed cloud services, Supabase and Nhost are excellent alternatives.

---

**Comparison Updated**: January 31, 2026
**Based on**: nself v0.9.9, Supabase v2024.01, Nhost v2024.01, Firebase v2024.01
