# Authentication Setup Guide

Comprehensive guide to understanding and setting up authentication in nself using nHost auth system.

---

## Table of Contents

1. [How nself Auth Works](#how-nself-auth-works)
2. [Auth Schema Structure](#auth-schema-structure)
3. [Quick Setup](#quick-setup)
4. [Manual Setup](#manual-setup)
5. [Creating Users](#creating-users)
6. [Testing Authentication](#testing-authentication)
7. [Troubleshooting](#troubleshooting)

---

## How nself Auth Works

nself uses **nHost** authentication service, which provides a complete auth system with:

- **Email/password authentication**
- **OAuth providers** (Google, GitHub, etc.)
- **Magic links**
- **Multi-factor authentication**
- **JWT tokens** for API access
- **Refresh tokens** for session management

### Architecture

```
┌──────────────┐      ┌───────────────┐      ┌──────────────┐      ┌──────────────┐
│   Frontend   │─────▶│  Auth Service │─────▶│    Hasura    │─────▶│  PostgreSQL  │
│  (Browser)   │      │    (nHost)    │      │   GraphQL    │      │   Database   │
└──────────────┘      └───────────────┘      └──────────────┘      └──────────────┘
                             │                                            │
                             │                                            │
                             └────────────────────────────────────────────┘
                                       Direct database access
                                       for user creation/validation
```

**Flow:**
1. **Frontend** sends login request to **Auth Service**
2. **Auth Service** validates credentials against **PostgreSQL**
3. **Auth Service** queries **Hasura** for user metadata
4. **Auth Service** returns JWT access token
5. **Frontend** uses token to query **Hasura GraphQL API**
6. **Hasura** validates JWT and enforces permissions

---

## Auth Schema Structure

nself auth uses **three tables** for authentication:

### 1. `auth.providers`

Stores available authentication providers.

```sql
CREATE TABLE auth.providers (
  id TEXT PRIMARY KEY  -- 'email', 'google', 'github', etc.
);
```

**Default providers:**
- `email` - Email/password authentication

### 2. `auth.users`

Stores user accounts and metadata.

```sql
CREATE TABLE auth.users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  display_name TEXT,
  avatar_url TEXT,
  locale TEXT DEFAULT 'en',

  -- Authentication
  password_hash TEXT,  -- bcrypt hash
  email_verified BOOLEAN DEFAULT false,
  phone_verified BOOLEAN DEFAULT false,
  disabled BOOLEAN DEFAULT false,

  -- Roles
  default_role TEXT DEFAULT 'user',

  -- Metadata (JSONB for flexible data)
  metadata JSONB DEFAULT '{}',

  -- Timestamps
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  last_seen TIMESTAMPTZ
);
```

**Important fields:**
- `password_hash` - **bcrypt** hashed password (NOT plain text!)
- `metadata` - Stores custom data like `{"role": "owner"}`
- `default_role` - Hasura role for permissions

### 3. `auth.user_providers`

Links users to their provider identities (emails, OAuth profiles, etc.).

```sql
CREATE TABLE auth.user_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL REFERENCES auth.providers(id),

  -- Provider-specific identity
  provider_user_id TEXT NOT NULL,  -- Email for 'email' provider

  -- Tokens (OAuth or dummy for seeded users)
  access_token TEXT,
  refresh_token TEXT,
  expires_at TIMESTAMPTZ,

  -- Timestamps
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),

  UNIQUE(provider_id, provider_user_id)
);
```

**Key points:**
- `provider_user_id` is the **email address** for email provider
- `access_token` can be **dummy value** for seeded users (e.g., `seed_token_<uuid>`)
- Unique constraint prevents duplicate email registrations

### Relationships

```
auth.providers (1) ──────────< (N) auth.user_providers
                                           │
                                           │ (N)
                                           ▼
                                      (1) auth.users
```

One user can have multiple providers (email + Google + GitHub).

---

## Quick Setup

**Fastest way to get auth working:**

```bash
# 1. Start services
nself start

# 2. Setup auth (one command!)
nself auth setup --default-users
```

Done! You now have:
- ✅ Hasura tracking auth tables
- ✅ 3 staff users created
- ✅ Auth service configured

Test it:
```bash
curl -k -X POST https://auth.local.nself.org/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{"email":"owner@nself.org","password":"npass123"}'
```

---

## Manual Setup

If you want more control or to understand what's happening:

### Step 1: Check Hasura Metadata

```bash
# Check if auth tables are tracked
nself hasura metadata export

# Look for auth tables in hasura/metadata/export.json
grep -A 5 '"schema": "auth"' hasura/metadata/export.json
```

If auth tables are NOT tracked:

```bash
# Track all auth tables
nself hasura track schema auth

# Or track individually
nself hasura track table auth.users
nself hasura track table auth.user_providers
nself hasura track table auth.providers
```

### Step 2: Create Auth Provider

```bash
# Insert email provider
nself exec postgres psql -U postgres -d your_db <<EOF
INSERT INTO auth.providers (id) VALUES ('email')
ON CONFLICT DO NOTHING;
EOF
```

### Step 3: Create Users Manually

```bash
# Generate bcrypt hash using PostgreSQL
nself exec postgres psql -U postgres -d your_db -c \
  "SELECT crypt('your_password', gen_salt('bf', 10));"

# Copy the hash, then insert user
nself exec postgres psql -U postgres -d your_db <<EOF
INSERT INTO auth.users (
  id, display_name, password_hash, email_verified, metadata
) VALUES (
  gen_random_uuid(),
  'Your Name',
  '$2a$10$HASH_FROM_PREVIOUS_COMMAND',
  true,
  '{"role": "owner"}'::jsonb
);
EOF
```

### Step 4: Link User to Email

```bash
# Get user ID from previous insert
nself db query "SELECT id FROM auth.users WHERE display_name = 'Your Name'"

# Insert user_provider link
nself exec postgres psql -U postgres -d your_db <<EOF
INSERT INTO auth.user_providers (
  id, user_id, provider_id, provider_user_id, access_token
) VALUES (
  gen_random_uuid(),
  'USER_ID_FROM_ABOVE',
  'email',
  'your@email.com',
  'seed_token_' || gen_random_uuid()::text
);
EOF
```

### Step 5: Verify

```bash
# Check user was created
nself auth list-users

# Test login
curl -k -X POST https://auth.local.nself.org/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{"email":"your@email.com","password":"your_password"}'
```

---

## Creating Users

### Method 1: Using nself Command (Recommended)

```bash
# Interactive mode
nself auth create-user

# Prompts:
# Email: newuser@example.com
# Password: (leave empty for auto-generated)

# Non-interactive mode
nself auth create-user \
  --email=newuser@example.com \
  --password=SecurePass123! \
  --role=admin \
  --name="New Admin User"
```

**What it does:**
1. Generates UUID for user
2. Hashes password with bcrypt
3. Inserts into `auth.users`
4. Links to email provider in `auth.user_providers`
5. Uses dummy access token for seeded users

### Method 2: Using Seed Files

Create `nself/seeds/common/001_auth_users.sql`:

```sql
-- Ensure provider exists
INSERT INTO auth.providers (id) VALUES ('email') ON CONFLICT DO NOTHING;

-- Create user
INSERT INTO auth.users (
  id,
  display_name,
  password_hash,
  email_verified,
  locale,
  default_role,
  metadata,
  created_at,
  updated_at
) VALUES (
  '11111111-1111-1111-1111-111111111111',
  'Admin User',
  crypt('password123', gen_salt('bf', 10)),
  true,
  'en',
  'user',
  '{"role": "admin"}'::jsonb,
  NOW(),
  NOW()
) ON CONFLICT (id) DO UPDATE SET
  password_hash = EXCLUDED.password_hash,
  updated_at = NOW();

-- Link to provider
INSERT INTO auth.user_providers (
  id,
  user_id,
  provider_id,
  provider_user_id,
  access_token,
  created_at,
  updated_at
) VALUES (
  gen_random_uuid(),
  '11111111-1111-1111-1111-111111111111',
  'email',
  'admin@example.com',
  'seed_token_' || gen_random_uuid()::text,
  NOW(),
  NOW()
) ON CONFLICT (provider_id, provider_user_id) DO NOTHING;
```

Apply seed:
```bash
nself db seed apply
```

### Method 3: Via Auth Service API (Production)

```bash
# Sign up endpoint
curl -k -X POST https://auth.local.nself.org/signup/email-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "newuser@example.com",
    "password": "SecurePass123!",
    "options": {
      "displayName": "New User",
      "metadata": {"role": "user"}
    }
  }'
```

This is the **production method** - generates real tokens, sends verification emails, etc.

---

## Testing Authentication

### Test 1: User Exists in Database

```bash
# List all users
nself auth list-users

# Or query directly
nself db query "SELECT u.id, up.provider_user_id as email, u.display_name, u.metadata
FROM auth.users u
LEFT JOIN auth.user_providers up ON u.id = up.user_id
WHERE up.provider_id = 'email'"
```

### Test 2: Login via Auth Service

```bash
curl -k -X POST https://auth.local.nself.org/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "owner@nself.org",
    "password": "npass123"
  }'
```

**Success response:**
```json
{
  "session": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "...",
    "expires_in": 900,
    "user": {
      "id": "11111111-1111-1111-1111-111111111111",
      "email": "owner@nself.org",
      "displayName": "Platform Owner"
    }
  }
}
```

### Test 3: Query via Hasura with Token

```bash
# Extract token from login response
ACCESS_TOKEN="<token_from_above>"

# Query Hasura
curl -X POST https://api.local.nself.org/v1/graphql \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"{ users { id display_name } }"}'
```

### Test 4: Admin Query (No Auth Required)

```bash
# Using admin secret instead of user token
curl -X POST http://localhost:8080/v1/graphql \
  -H "x-hasura-admin-secret: $HASURA_GRAPHQL_ADMIN_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"query":"{ users { id display_name metadata } }"}'
```

---

## Troubleshooting

### Issue: "field 'users' not found in type: 'query_root'"

**Cause:** Hasura hasn't tracked auth.users table.

**Solution:**
```bash
nself hasura track schema auth
nself hasura metadata reload
```

### Issue: Login returns 401 Unauthorized

**Possible causes:**
1. Wrong password
2. User doesn't exist
3. Email not verified (if required)

**Debug:**
```bash
# Check user exists
nself db query "SELECT * FROM auth.users WHERE id = (
  SELECT user_id FROM auth.user_providers WHERE provider_user_id = 'your@email.com'
)"

# Check password hash
nself db query "SELECT password_hash FROM auth.users WHERE id = '<user_id>'"

# Test password hash
nself exec postgres psql -U postgres -d your_db -c \
  "SELECT password_hash = crypt('your_password', password_hash) AS password_valid
   FROM auth.users WHERE id = '<user_id>'"
```

### Issue: Auth service won't start

**Check logs:**
```bash
nself logs auth --tail 50
```

**Common issues:**
- Missing `HASURA_GRAPHQL_ADMIN_SECRET`
- Can't connect to Hasura
- Can't connect to PostgreSQL
- Auth tables don't exist

**Solution:**
```bash
# Verify environment
grep HASURA .env

# Recreate auth schema
nself db migrate up

# Restart auth service
nself restart auth
```

### Issue: Seeded users can't login

**Check:**
1. Password hash is bcrypt (starts with `$2a$` or `$2b$`)
2. User exists in both `auth.users` and `auth.user_providers`
3. Email matches in `auth.user_providers.provider_user_id`

**Fix:**
```bash
# Recreate user with command
nself auth create-user --email=user@example.com --password=newpass
```

---

## Security Best Practices

### Development
- ✅ Use default users with weak passwords (`npass123`)
- ✅ Enable `email_verified = true` for seeded users
- ✅ Use dummy access tokens (`seed_token_<uuid>`)

### Production
- ❌ **NEVER** use default passwords
- ✅ Force password strength requirements
- ✅ Require email verification
- ✅ Use rate limiting on auth endpoints
- ✅ Enable MFA for admin users
- ✅ Rotate JWT secrets regularly
- ✅ Use HTTPS only
- ✅ Monitor failed login attempts

---

## Password Hashing

nself uses **bcrypt** with cost factor **10** for password hashing.

### Why bcrypt?

- ✅ Slow by design (prevents brute force)
- ✅ Adaptive (can increase cost over time)
- ✅ Salt included automatically
- ✅ Industry standard

### Generating bcrypt hashes

**Via PostgreSQL (recommended):**
```sql
SELECT crypt('password123', gen_salt('bf', 10));
-- Returns: $2a$10$eAGrChCvMYFQxMKD6TzpGuKGzHXPQZBQlRrQKNFkCvf3lBXqL4aZW
```

**Via command line:**
```bash
# Using Python
python3 -c "import bcrypt; print(bcrypt.hashpw(b'password123', bcrypt.gensalt(10)).decode())"

# Using Node.js
node -e "const bcrypt = require('bcrypt'); console.log(bcrypt.hashSync('password123', 10))"
```

### Verifying passwords

**PostgreSQL:**
```sql
SELECT password_hash = crypt('user_input_password', password_hash) AS is_valid
FROM auth.users
WHERE id = '<user_id>';
```

---

## Access Tokens for Seeded Users

When creating users via seeds or commands (not via signup API), we use **dummy access tokens**:

```
seed_token_<uuid>
```

**Why dummy tokens?**

- Real tokens are generated by auth service on login
- Seeded users never "logged in" via API
- Dummy token satisfies NOT NULL constraint
- Auth service ignores these tokens (generates new ones on login)

**Important:** Dummy tokens are **NOT VALID** for API access. Users must login via auth service to get real JWT tokens.

---

## Next Steps

- Read [DEV_WORKFLOW.md](./DEV_WORKFLOW.md) for complete workflow
- Read [SEEDING.md](./SEEDING.md) for advanced seeding patterns
- Explore Hasura permissions and roles
- Set up OAuth providers
- Enable MFA for production

---

**Questions? Issues?**
- GitHub: https://github.com/nself-org/cli/issues
- Docs: https://docs.nself.org
