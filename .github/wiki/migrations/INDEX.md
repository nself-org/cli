# Migration Documentation

Guides for migrating to ɳSelf from other platforms and upgrading between versions.

## Overview

- **[Migration Overview](README.md)** - General migration information

## Platform Migration Guides

Migrate from other Backend-as-a-Service platforms:

### From Commercial Platforms

- **[From Supabase](FROM-SUPABASE.md)** - Migrate from Supabase
  - Database migration
  - Auth migration
  - Storage migration
  - API endpoint mapping

- **[From nHost](FROM-NHOST.md)** - Migrate from nHost
  - Service mapping
  - Configuration migration
  - Auth setup
  - GraphQL compatibility

- **[From Firebase](FROM-FIREBASE.md)** - Migrate from Firebase
  - Firestore to PostgreSQL
  - Firebase Auth to nself Auth
  - Cloud Functions migration
  - Storage migration

## Version Migration

### v1.0 Command Structure

- **[v1.0 Migration Status](V1-MIGRATION-STATUS.md)** - v1.0 migration progress
- **[Infrastructure Consolidation](INFRA-CONSOLIDATION.md)** - Infra command changes

### Command Consolidation (v0.9.6)

The v0.9.6 release consolidated commands from 79 → 31 top-level commands:

**Key Changes:**
- `nself billing` → `nself tenant billing`
- `nself org` → `nself tenant org`
- `nself staging` → `nself deploy staging`
- `nself prod` → `nself deploy production`
- `nself storage` → `nself service storage`
- `nself oauth` → `nself auth oauth`

**See:** [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md)

### Backward Compatibility

All old commands still work with deprecation warnings:

```bash
# Old command (still works)
nself billing usage

# Output:
# ⚠️  DEPRECATED: Use 'nself tenant billing usage' instead
# [command executes normally]

# New command
nself tenant billing usage
```

## Migration Process

### From Supabase

1. **Export Supabase Data**
   ```bash
   # Export database
   supabase db dump > supabase_dump.sql

   # Export auth users
   supabase db dump --table auth.users > users.sql
   ```

2. **Set Up ɳSelf**
   ```bash
   # Initialize project
   nself init
   nself build && nself start
   ```

3. **Import Data**
   ```bash
   # Import database
   nself db restore supabase_dump.sql

   # Migrate auth users
   nself db migrate-auth-from-supabase users.sql
   ```

4. **Update Configuration**
   - Map Supabase environment variables to ɳSelf
   - Update API endpoints in frontend
   - Configure storage buckets

See **[From Supabase Guide](FROM-SUPABASE.md)** for complete details.

### From Firebase

1. **Export Firestore Data**
   ```bash
   # Using Firebase CLI
   firebase firestore:export firestore_backup
   ```

2. **Convert to PostgreSQL**
   ```bash
   # ɳSelf provides conversion tool
   nself db import-from-firebase firestore_backup/
   ```

3. **Migrate Authentication**
   ```bash
   # Export Firebase users
   firebase auth:export users.json

   # Import to ɳSelf
   nself auth import-from-firebase users.json
   ```

4. **Update Application Code**
   - Replace Firebase SDK with ɳSelf SDK
   - Update queries (Firestore → GraphQL)
   - Migrate Cloud Functions to ɳSelf Functions

See **[From Firebase Guide](FROM-FIREBASE.md)** for complete details.

### From nHost

ɳSelf uses the same core services as nHost (Hasura, nHost Auth, PostgreSQL), making migration straightforward:

1. **Database Migration**
   ```bash
   # Export from nHost
   nhost db dump > nhost_dump.sql

   # Import to ɳSelf
   nself db restore nhost_dump.sql
   ```

2. **Configuration Mapping**
   Most nHost environment variables map directly to ɳSelf variables.

3. **Service Compatibility**
   - GraphQL API: Fully compatible (same Hasura)
   - Auth: Fully compatible (same nHost Auth)
   - Storage: Compatible with S3 API (MinIO)

See **[From nHost Guide](FROM-NHOST.md)** for complete details.

## Breaking Changes

### v0.9.6 (Command Consolidation)

**No breaking changes** - All old commands still work with deprecation warnings.

**Recommended Migration:**
1. Update scripts to use new command structure
2. Update CI/CD pipelines
3. Update documentation

**Migration Tool:**
```bash
# Scan and update scripts
nself dev migration scan-scripts ./
nself dev migration update-scripts ./
```

### v1.0 (Planned)

**Potential Changes:**
- Remove deprecated command aliases
- Standardize configuration format
- Update plugin API

See **[v1.0 Migration Status](V1-MIGRATION-STATUS.md)** for details.

## Platform Comparison

### Feature Parity

| Feature | Supabase | nHost | Firebase | ɳSelf |
|---------|----------|-------|----------|-------|
| PostgreSQL | ✅ | ✅ | ❌ | ✅ |
| GraphQL | ✅ | ✅ | ❌ | ✅ |
| Auth | ✅ | ✅ | ✅ | ✅ |
| Storage | ✅ | ✅ | ✅ | ✅ |
| Functions | ✅ | ✅ | ✅ | ✅ |
| Real-time | ✅ | ✅ | ✅ | ✅ |
| Self-Hosted | ✅ | ✅ | ❌ | ✅ |
| Multi-Tenant | ❌ | ❌ | ❌ | ✅ |
| Billing | ❌ | ❌ | ❌ | ✅ |
| White-Label | ❌ | ❌ | ❌ | ✅ |

### Cost Comparison

**Supabase Cloud:**
- Free tier: Limited
- Pro: $25/month + usage
- Team: $599/month

**nHost Cloud:**
- Free tier: Limited
- Pro: $25/month + usage
- Team: Custom pricing

**Firebase:**
- Pay-as-you-go pricing
- Can get expensive at scale

**ɳSelf:**
- Free and open-source
- Only pay for infrastructure
- Full control over costs

## Migration Support

### Tools Provided

ɳSelf provides migration tools for common platforms:

```bash
# Supabase
nself db import-from-supabase dump.sql
nself auth import-from-supabase users.json

# Firebase
nself db import-from-firebase firestore_backup/
nself auth import-from-firebase users.json

# nHost
nself db import-from-nhost dump.sql
nself auth import-from-nhost users.json
```

### Community Support

- [GitHub Discussions](https://github.com/nself-org/cli/discussions)
- [Migration Examples](https://github.com/nself-org/cli-examples/migrations)
- [Discord Community](https://discord.gg/nself)

### Professional Services

For enterprise migrations, contact: migrations@nself.org

## Related Documentation

- [Database Commands](../commands/DB.md)
- [Auth Commands](../commands/AUTH.md)
- [Architecture Overview](../architecture/ARCHITECTURE.md)
- [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md)

---

**[← Back to Documentation Home](../README.md)**
