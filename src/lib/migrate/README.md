# Migration Tools - Escape Vendor Lock-in

Mission: Free, open-source CLI to help users escape vendor lock-in.

## Overview

nself provides migration tools to help you move from proprietary platforms to your own infrastructure. No vendor lock-in, you own your data.

## Supported Migrations

### 1. Firebase → nself

Migrate your Firebase project to nself with full data preservation:

- **Firestore** → PostgreSQL
- **Firebase Auth** → nHost Auth
- **Firebase Storage** → MinIO

#### Prerequisites

- Node.js and npm installed
- Firebase service account JSON (download from Firebase Console)
- Firebase Admin SDK access
- Optional: MinIO Client (`mc`) for storage migration

#### Quick Start

```bash
# Interactive migration wizard
nself migrate from firebase

# You'll be prompted for:
# - Firebase service account JSON path
# - Collections to migrate (or 'all')
# - Storage bucket name (optional)
# - Output directory
```

#### What Gets Migrated

**Firestore Data:**
- All collections and documents
- Nested data structures (converted to JSONB)
- Document IDs preserved
- Auto-creates PostgreSQL tables matching your collections

**Firebase Auth Users:**
- User IDs (UIDs)
- Email addresses
- Email verification status
- Display names
- Photo URLs
- Account status (enabled/disabled)
- Creation and last sign-in timestamps
- Custom claims (stored as JSONB metadata)

**⚠️ Important:** Password hashes cannot be migrated from Firebase. Users will need to reset passwords after migration.

**Firebase Storage:**
- All files and folder structure
- File metadata (size, content type, timestamps)
- Uploads to MinIO S3-compatible storage

#### Step-by-Step Migration

**Step 1: Export from Firebase**

```bash
nself migrate from firebase
```

This will:
1. Connect to Firebase using your service account
2. Export all data to local files
3. Create organized directory structure:
   ```
   firebase-migration-YYYYMMDD-HHMMSS/
   ├── firestore/
   │   ├── users.json
   │   ├── posts.json
   │   └── _metadata.json
   ├── auth/
   │   ├── users.json
   │   └── _metadata.json
   └── storage/
       ├── images/
       ├── documents/
       └── _manifest.json
   ```

**Step 2: Import to nself**

The export wizard will show you the exact commands to run. Typically:

```bash
# Import Firestore data to PostgreSQL
nself migrate from firebase import-data "./firebase-migration-*/firestore"

# Import Auth users to nHost
nself migrate from firebase import-auth "./firebase-migration-*/auth"

# Import Storage to MinIO (requires mc client)
nself migrate from firebase import-storage "./firebase-migration-*/storage"
```

#### Manual Import (Advanced)

If you prefer manual control:

```bash
# Export only
node export-firestore.js service-account.json ./output all
node export-auth.js service-account.json ./output
node export-storage.js service-account.json ./output bucket-name

# Import to PostgreSQL
node import-to-postgres.js ./output localhost 5432 nhost postgres password

# Import auth users
node import-auth.js ./output localhost 5432 nhost postgres password
```

---

### 2. Supabase → nself

Migrate your Supabase project to nself with full fidelity:

- **PostgreSQL Database** → nself PostgreSQL
- **Supabase Auth** → nHost Auth
- **Supabase Storage** → MinIO

#### Prerequisites

- Supabase project URL and service role key
- Direct database access (connection string)
- PostgreSQL client tools (`pg_dump`, `psql`)
- Optional: MinIO Client (`mc`) for storage migration

#### Quick Start

```bash
# Interactive migration wizard
nself migrate from supabase

# You'll be prompted for:
# - Supabase project URL
# - Service role key
# - Database connection details
# - Output directory
```

#### What Gets Migrated

**Database Schema:**
- All tables, indexes, constraints
- Views and materialized views
- Functions and triggers
- Row-level security policies
- Separated into: public schema, auth schema, storage schema

**Database Data:**
- All table data
- Auth users with full metadata
- Preserves relationships and foreign keys

**Supabase Auth:**
- User accounts
- Email verification status
- User metadata
- Authentication providers
- Sessions and refresh tokens

**Supabase Storage:**
- All buckets
- Files and folder structure
- File metadata
- Bucket policies

#### Step-by-Step Migration

**Step 1: Get Connection Details**

From your Supabase dashboard:
1. Project Settings → Database
2. Copy connection string
3. Get service_role key from API settings

**Step 2: Export from Supabase**

```bash
nself migrate from supabase
```

This will:
1. Export database schema to SQL files
2. Export all data to SQL files
3. Download storage files via API
4. Create organized directory:
   ```
   supabase-migration-YYYYMMDD-HHMMSS/
   ├── schema/
   │   ├── schema.sql
   │   ├── auth-schema.sql
   │   └── storage-schema.sql
   ├── data/
   │   ├── data.sql
   │   ├── auth-data.sql
   │   └── storage-data.sql
   └── storage/
       ├── bucket1/
       ├── bucket2/
       └── buckets.json
   ```

**Step 3: Import to nself**

```bash
# Import public schema
nself migrate from supabase import-schema "./supabase-migration-*/schema/schema.sql"

# Import public data
nself migrate from supabase import-data "./supabase-migration-*/data/data.sql"

# Import auth schema and data
nself migrate from supabase import-schema "./supabase-migration-*/schema/auth-schema.sql"
nself migrate from supabase import-data "./supabase-migration-*/data/auth-data.sql"

# Import storage to MinIO
nself migrate from supabase import-storage "./supabase-migration-*/storage"
```

#### Direct Database Migration (Fastest)

If both databases are accessible:

```bash
# Direct schema migration
pg_dump -h supabase-host -U postgres -d postgres --schema-only | \
  psql -h localhost -U postgres -d nhost

# Direct data migration
pg_dump -h supabase-host -U postgres -d postgres --data-only | \
  psql -h localhost -U postgres -d nhost
```

---

## Common Migration Scenarios

### Full Migration with Zero Downtime

1. **Export data** from old platform
2. **Set up nself** in parallel
3. **Import data** to nself
4. **Run in parallel** (dual-write if needed)
5. **Verify data** integrity
6. **Switch DNS/traffic** to nself
7. **Decommission** old platform

### Gradual Migration

1. **Start with read-only** data (analytics, historical)
2. **Migrate authentication** users
3. **Migrate storage** files
4. **Migrate live data** during low-traffic period
5. **Switch services** one by one

### Testing Migration

```bash
# Test with sample data first
nself migrate from firebase --collections="test_collection"

# Verify imported data
nself db query "SELECT COUNT(*) FROM test_collection;"

# Compare before/after
nself migrate diff firebase nself
```

---

## Migration Helpers

### Data Validation

```bash
# Check record counts
nself db query "SELECT
  table_name,
  (SELECT COUNT(*) FROM \${table_name}) as row_count
FROM information_schema.tables
WHERE table_schema = 'public';"

# Verify auth users
nself auth list | wc -l
```

### Post-Migration Checklist

- [ ] All tables created with correct schema
- [ ] Record counts match source
- [ ] Auth users imported successfully
- [ ] Storage files accessible
- [ ] Relationships and constraints intact
- [ ] Indexes created
- [ ] Application connects successfully
- [ ] All features working
- [ ] Performance acceptable
- [ ] Backup created

### Rollback Plan

```bash
# Before migration, create backup
nself db backup --label "pre-migration"

# If migration fails
nself db restore --backup "pre-migration-*"

# Verify restoration
nself db query "SELECT COUNT(*) FROM users;"
```

---

## Troubleshooting

### Firebase Migration Issues

**Problem:** Service account authentication fails
```bash
# Solution: Verify service account has correct permissions
# Firebase Console → Project Settings → Service Accounts
# Download new service account JSON
```

**Problem:** Firestore export timeout
```bash
# Solution: Export specific collections instead of 'all'
nself migrate from firebase --collections="users,posts,comments"
```

**Problem:** Storage download fails
```bash
# Solution: Check bucket permissions and Firebase Storage Rules
# Ensure service account has storage.objects.get permission
```

### Supabase Migration Issues

**Problem:** Database connection refused
```bash
# Solution: Check connection pooling mode
# Supabase uses pooled connections - use port 6543 for direct connection
# Or use connection string with ?sslmode=require
```

**Problem:** pg_dump not found
```bash
# Solution: Install PostgreSQL client tools
# macOS: brew install postgresql
# Ubuntu: apt install postgresql-client
# CentOS: yum install postgresql
```

**Problem:** Storage API rate limit
```bash
# Solution: Add delays between file downloads
# Or download buckets one at a time
# Or use direct database access to storage.objects table
```

---

## Security Notes

### Credentials Management

- **Never commit** service account keys to git
- **Store credentials** in `.secrets` file (gitignored)
- **Use environment variables** for sensitive data
- **Rotate keys** after migration completes

### Data Protection

- **Encrypt exports** if they contain sensitive data:
  ```bash
  tar -czf migration.tar.gz firebase-migration-*/
  gpg -c migration.tar.gz  # Creates migration.tar.gz.gpg
  ```

- **Secure transfer** for remote migrations:
  ```bash
  rsync -avz -e ssh migration/ user@server:/path/
  ```

- **Clean up** temporary files:
  ```bash
  # After successful migration
  rm -rf firebase-migration-*/
  rm -rf /tmp/nself-firebase-migration-*
  ```

---

## Need Help?

- **Documentation:** https://github.com/nself-org/cli/wiki/Migrations
- **Issues:** https://github.com/nself-org/cli/issues
- **Discussions:** https://github.com/nself-org/cli/discussions
- **Discord:** [Coming soon]

---

## Philosophy

> **No vendor lock-in. You own your data. You control your infrastructure.**

nself exists to give developers freedom. These migration tools are designed to make it easy to escape proprietary platforms and take control of your stack.

**Free forever. Open source. Community-driven.**
