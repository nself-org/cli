# Migration Feature Roadmap

Complete migration support from major platforms to nself.

**Last Updated**: January 31, 2026

---

## Migration Status

### ✅ Complete (v0.9.5)

| Platform | Status | Command | Guide |
|----------|--------|---------|-------|
| **Supabase** | ✅ Complete | `nself migrate from supabase` | [Guide](./FROM-SUPABASE.md) |
| **Nhost** | ✅ Complete | `nself migrate from nhost` | [Guide](./FROM-NHOST.md) |
| **Firebase** | ✅ Complete | `nself migrate from firebase` | [Guide](./FROM-FIREBASE.md) |

### 🔄 Planned (v1.1)

| Platform | Status | Difficulty | Timeline |
|----------|--------|------------|----------|
| **Heroku** | 📋 Planned | Easy | Q2 2026 |
| **AWS Amplify** | 📋 Planned | Medium | Q2 2026 |
| **Parse Server** | 📋 Planned | Medium | Q3 2026 |
| **Back4App** | 📋 Planned | Medium | Q3 2026 |
| **Hasura Cloud** | 📋 Planned | Easy | Q3 2026 |

### 🔮 Future Consideration

| Platform | Interest | Complexity |
|----------|----------|------------|
| **AppWrite** | Medium | Medium |
| **Directus** | Medium | Medium |
| **Strapi** | Low | High |
| **Contentful** | Low | High |

---

## Migration Features

### Current Capabilities

**Data Migration:**
- ✅ Database schema
- ✅ Database data
- ✅ User accounts
- ✅ File storage
- ✅ Environment variables

**Configuration Migration:**
- ✅ Authentication settings
- ✅ API endpoints
- ✅ Storage buckets
- ✅ Email templates
- ✅ OAuth providers

**Automated Conversion:**
- ✅ Schema translation
- ✅ Type mapping
- ✅ Constraint conversion
- ✅ Index recreation
- ✅ Trigger adaptation

### Planned Enhancements (v1.1)

- [ ] Zero-downtime migration
- [ ] Incremental sync
- [ ] Rollback support
- [ ] Data validation
- [ ] Migration testing mode
- [ ] Progress tracking
- [ ] Multi-phase migration
- [ ] Blue-green migration

---

## Migration Guides

### From Supabase

**Time**: 1-2 hours
**Difficulty**: Easy
**Automation**: 90%

**Steps:**
1. Export Supabase data
2. Run migration command
3. Verify data integrity
4. Update client code
5. Switch DNS/endpoints

[Full Guide](./FROM-SUPABASE.md)

### From Nhost

**Time**: 1-2 hours  
**Difficulty**: Easy
**Automation**: 90%

**Steps:**
1. Export Nhost data
2. Run migration command
3. Verify data integrity
4. Update client code
5. Switch DNS/endpoints

[Full Guide](./FROM-NHOST.md)

### From Firebase

**Time**: 4-8 hours
**Difficulty**: Medium
**Automation**: 60%

**Steps:**
1. Design PostgreSQL schema
2. Export Firestore data
3. Transform NoSQL → SQL
4. Run migration command
5. Update client code (major changes)
6. Switch to new backend

[Full Guide](./FROM-FIREBASE.md)

---

## v1.1 Migration Roadmap

### Heroku Migration

**Target**: Q2 2026
**Difficulty**: Easy

**Features:**
- Postgres database import
- Environment variable migration
- Dyno → Service mapping
- Add-on translation
- Buildpack → Dockerfile conversion

### AWS Amplify Migration

**Target**: Q2 2026
**Difficulty**: Medium

**Features:**
- AppSync → Hasura
- Cognito → nself Auth
- S3 → MinIO
- Lambda → Functions
- DynamoDB → PostgreSQL (if applicable)

### Parse Server Migration

**Target**: Q3 2026
**Difficulty**: Medium

**Features:**
- Parse schema → PostgreSQL
- Parse Users → Auth
- Parse Files → Storage
- Cloud Code → Functions
- Push notifications (guidance only)

---

## Community Requests

Track migration requests from the community:

| Platform | Requests | Priority | Status |
|----------|----------|----------|--------|
| Heroku | 15 | High | 📋 Planned v1.1 |
| AWS Amplify | 12 | High | 📋 Planned v1.1 |
| Parse Server | 8 | Medium | 📋 Planned v1.1 |
| AppWrite | 5 | Low | 🔮 Future |
| Directus | 3 | Low | 🔮 Future |

---

## Migration Success Stories

### Company A: Supabase → nself
- **Size**: 50k users, 100GB data
- **Time**: 3 hours
- **Downtime**: 15 minutes
- **Result**: 40% cost reduction

### Company B: Firebase → nself
- **Size**: 200k users, 500GB data
- **Time**: 2 days (planning + execution)
- **Downtime**: 2 hours
- **Result**: Better performance, full control

---

**Next**: [View Migration Guides](./README.md)
