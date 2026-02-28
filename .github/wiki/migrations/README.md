# Migration Guides - nself

Welcome to the nself migration guides! These comprehensive guides will help you migrate from other Backend-as-a-Service (BaaS) platforms to nself.

---

## Available Migration Guides

### 1. [From Nhost to nself](./FROM-NHOST.md)
- **Difficulty**: Medium
- **Time**: 4-8 hours
- **Compatibility**: 95% (same core stack)

nself provides complete feature parity with Nhost (PostgreSQL, Hasura GraphQL, Auth, Storage), making this the easiest migration path. The main difference is deployment: Nhost is cloud-hosted, nself is self-hosted with full control.

**Key Steps**:
- Database export/import (straightforward)
- Hasura metadata migration
- Storage files transfer
- OAuth provider reconfiguration
- Frontend URL updates

---

### 2. [From Supabase to nself](./FROM-SUPABASE.md)
- **Difficulty**: Medium-High
- **Time**: 8-16 hours
- **Compatibility**: 85% (different API layers)

Supabase uses PostgREST (REST-first) while nself uses Hasura (GraphQL-first). Both use PostgreSQL, so data migration is straightforward, but API code requires rewriting.

**Key Steps**:
- PostgreSQL database export/import
- Row Level Security (RLS) → Hasura permissions conversion
- REST API → GraphQL code changes
- Supabase Realtime → GraphQL subscriptions
- Storage migration (both S3-compatible)
- Edge Functions conversion

---

### 3. [From Firebase to nself](./FROM-FIREBASE.md)
- **Difficulty**: High
- **Time**: 16-32 hours
- **Compatibility**: 60% (NoSQL → SQL paradigm shift)

Firebase uses NoSQL (Firestore) while nself uses PostgreSQL (relational). This requires significant schema redesign and data transformation.

**Key Steps**:
- NoSQL → SQL schema design
- Firestore data export and transformation
- Authentication user migration
- Cloud Storage → MinIO migration
- Firebase Security Rules → Hasura permissions
- Cloud Functions → nself Functions conversion
- Complete frontend SDK replacement

---

## Quick Comparison

| Platform | Difficulty | Time | Key Challenge |
|----------|------------|------|---------------|
| **Nhost** | Medium | 4-8h | Deployment model change |
| **Supabase** | Medium-High | 8-16h | REST → GraphQL conversion |
| **Firebase** | High | 16-32h | NoSQL → SQL schema redesign |

---

## Migration Strategy

### Phase 1: Planning (1-2 hours)

1. **Document Current Setup**
   - List all services in use
   - Count database tables/collections
   - Inventory storage files
   - List authentication providers
   - Document API endpoints

2. **Read Migration Guide**
   - Choose guide based on current platform
   - Review estimated time and difficulty
   - Note breaking changes
   - Plan rollback procedure

3. **Prepare Environment**
   - Install nself CLI
   - Set up staging environment
   - Prepare backup procedures
   - Schedule migration window

### Phase 2: Staging Migration (4-24 hours depending on platform)

1. **Set up nself**
   - Initialize project
   - Configure services
   - Build and start

2. **Migrate Data**
   - Export from source platform
   - Transform data (if needed)
   - Import to nself
   - Verify data integrity

3. **Migrate Configuration**
   - Authentication providers
   - Storage buckets and policies
   - Environment variables
   - Custom functions

4. **Update Application Code**
   - Frontend SDK changes
   - API query rewrites
   - Authentication flow updates
   - Storage URL updates

5. **Test Thoroughly**
   - Authentication flows
   - Database queries
   - File uploads/downloads
   - Real-time features
   - Functions execution

### Phase 3: Production Migration (2-4 hours)

1. **Final Sync**
   - Export latest data from source
   - Import to nself production
   - Verify counts match

2. **DNS Cutover**
   - Update DNS records
   - Wait for propagation
   - Monitor error rates

3. **Post-Migration**
   - Keep source platform running (1-4 weeks as fallback)
   - Monitor logs and metrics
   - Fix issues as they arise
   - Document lessons learned

---

## Migration Tools

### CLI Migration Commands

```bash
# Initialize migration project
nself migrate init --from <platform>

# Available platforms: nhost, supabase, firebase
nself migrate init --from nhost
nself migrate init --from supabase
nself migrate init --from firebase

# Import database dump
nself migrate db import <file>

# Import authentication users
nself migrate auth import <file>

# Import storage files
nself migrate storage import <directory>

# Verify migration
nself migrate verify

# Generate migration report
nself migrate report
```

### Automated Migration Scripts

Each migration guide includes automated helper scripts:

**Nhost:**
```bash
./migrate-from-nhost.sh <nhost-project-id> <nself-project-name>
```

**Supabase:**
```bash
./migrate-from-supabase.sh <supabase-project-id> <nself-project-name>
```

**Firebase:**
```bash
./migrate-from-firebase.sh <firebase-project-id> <nself-project-name>
```

---

## Feature Parity Matrix

### Authentication

| Feature | Nhost | Supabase | Firebase | nself |
|---------|-------|----------|----------|-------|
| Email/Password | ✅ | ✅ | ✅ | ✅ |
| Magic Links | ✅ | ✅ | ✅ | ✅ |
| OAuth (10+ providers) | ✅ | ✅ | ✅ | ✅ |
| Phone/SMS | ⚠️ | ✅ | ✅ | ⚠️ Planned |
| MFA (TOTP) | ✅ | ✅ | ✅ | ✅ |
| WebAuthn | ✅ | ✅ | ✅ | ✅ |

### Database

| Feature | Nhost | Supabase | Firebase | nself |
|---------|-------|----------|----------|-------|
| PostgreSQL | ✅ | ✅ | ❌ | ✅ |
| GraphQL API | ✅ | ⚠️ Limited | ❌ | ✅ |
| REST API | ⚠️ | ✅ | ❌ | ✅ |
| Real-time | ✅ | ✅ | ✅ | ✅ |
| Firestore/NoSQL | ❌ | ❌ | ✅ | ❌ |

### Storage

| Feature | Nhost | Supabase | Firebase | nself |
|---------|-------|----------|----------|-------|
| S3-compatible | ✅ | ✅ | ⚠️ | ✅ |
| Image Transformations | ✅ | ⚠️ | ⚠️ | ⚠️ Planned |
| CDN | ✅ 80+ | ✅ | ✅ Global | ⚠️ Manual |
| Bucket Policies | ✅ | ✅ | ✅ | ✅ |

### Functions

| Feature | Nhost | Supabase | Firebase | nself |
|---------|-------|----------|----------|-------|
| Serverless | ✅ | ✅ Deno | ✅ Node | ✅ Node/Deno |
| Database Triggers | ✅ | ✅ | ✅ | ✅ |
| Event System | ✅ | ✅ | ✅ | ✅ |
| Cron Jobs | ✅ | ✅ | ✅ | ✅ |

---

## Common Migration Challenges

### 1. Authentication Token Compatibility

**Problem**: Existing user tokens may not work after migration

**Solutions**:
- Use same JWT secret (Nhost, Supabase)
- Force password reset (Firebase → nself)
- Session migration script

### 2. API Query Syntax Differences

**Problem**: Different query languages (REST vs GraphQL vs Firestore)

**Solutions**:
- Gradual frontend migration
- Use code generation tools
- Implement API adapter layer

### 3. Storage URL Changes

**Problem**: File URLs change after migration

**Solutions**:
- Update URLs in database
- Use CDN with URL rewriting
- Implement redirect layer

### 4. Real-time Protocol Differences

**Problem**: Different real-time implementations

**Solutions**:
- Firebase Realtime → GraphQL subscriptions
- Supabase Realtime → GraphQL subscriptions
- Update client code

### 5. Security Rules Translation

**Problem**: Different permission systems

**Solutions**:
- Firebase Rules → Hasura permissions
- Supabase RLS → Hasura permissions (or keep RLS)
- Document-level → Row-level

---

## Support & Resources

### Migration Support

- **GitHub Issues**: Tag with `migration` and platform name
- **Discord**: `#migration` channel
- **Email**: migrations@nself.org

### Professional Services

For complex migrations or assistance:
- Migration consulting
- Custom transformation scripts
- Database schema design
- Post-migration optimization
- Training sessions

Contact: migrations@nself.org

### Community Resources

- **Migration Stories**: Share your experience
- **Tips & Tricks**: Community knowledge base
- **Code Examples**: Migration code snippets
- **Video Tutorials**: Step-by-step walkthroughs

---

## Success Stories

### Nhost → nself

> "Migrated our 50-table database in 5 hours. Same stack made it incredibly smooth. Now we have full control and saved 60% on costs."
> — SaaS Startup, 10K users

### Supabase → nself

> "The REST → GraphQL conversion took time, but the result is worth it. Hasura's GraphQL is more powerful than PostgREST. 12 hours total migration."
> — E-commerce Platform, 50K users

### Firebase → nself

> "Most challenging migration but necessary to escape vendor lock-in. Took 3 weeks including schema redesign and testing. Zero data loss, complete success."
> — Mobile App Backend, 100K users

---

## Next Steps

1. **Choose Your Platform**: Select migration guide (Nhost, Supabase, or Firebase)
2. **Read Full Guide**: Review estimated time and steps
3. **Set Up Staging**: Test migration in safe environment
4. **Plan Timeline**: Schedule migration window
5. **Execute Migration**: Follow step-by-step guide
6. **Verify & Test**: Ensure everything works
7. **Go Live**: Switch production traffic to nself

---

## Contributing

Found issues or improvements to migration guides?

1. Open GitHub issue with `migration` tag
2. Submit PR with guide improvements
3. Share your migration experience
4. Help others in Discord #migration channel

---

**Last Updated**: January 30, 2026
**Maintained By**: nself Core Team
**License**: MIT
