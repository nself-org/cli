# Migrating from Firebase to nself

**Last Updated**: January 30, 2026
**Migration Difficulty**: High
**Estimated Time**: 16-32 hours
**Compatibility**: 60% (NoSQL → SQL, major architecture shift)

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Architecture Differences](#architecture-differences)
4. [Schema Design Conversion](#schema-design-conversion)
5. [Step-by-Step Migration](#step-by-step-migration)
6. [Firestore to PostgreSQL Migration](#firestore-to-postgresql-migration)
7. [Authentication Migration](#authentication-migration)
8. [Cloud Storage to MinIO Migration](#cloud-storage-to-minio-migration)
9. [Cloud Functions to nself Functions Migration](#cloud-functions-to-nself-functions-migration)
10. [Realtime Database Migration](#realtime-database-migration)
11. [Security Rules to Hasura Permissions](#security-rules-to-hasura-permissions)
12. [Frontend Code Changes](#frontend-code-changes)
13. [Common Pitfalls](#common-pitfalls)
14. [Rollback Procedure](#rollback-procedure)
15. [Automated Migration Tools](#automated-migration-tools)

---

## Overview

Migrating from Firebase to nself is the most complex migration due to fundamental architectural differences:

**Firebase**: NoSQL (Firestore/Realtime DB) + Proprietary services + Cloud-only
**nself**: Relational (PostgreSQL) + Open-source stack + Self-hosted

### Why Migrate to nself?

- **Escape vendor lock-in** - Firebase is proprietary, nself is open-source
- **Relational database power** - Joins, complex queries, transactions
- **Full control** - Self-hosted infrastructure, own your data
- **Cost predictability** - No surprise bills from Firebase usage spikes
- **Advanced features** - Multi-tenancy, billing, plugins
- **SQL expertise** - Use existing SQL knowledge

### Key Challenges

| Challenge | Difficulty | Time Estimate |
|-----------|------------|---------------|
| NoSQL → SQL schema design | High | 4-8 hours |
| Data transformation & import | High | 4-8 hours |
| Authentication migration | Medium | 2-3 hours |
| Security rules → Hasura permissions | High | 2-4 hours |
| Frontend SDK replacement | High | 4-8 hours |
| Cloud Functions conversion | Medium | 2-4 hours |

**Total**: 16-32 hours

---

## Prerequisites

### Before You Start

- [ ] Full Firebase project backup
- [ ] Firestore data export (JSON)
- [ ] Cloud Storage inventory
- [ ] List of all Cloud Functions
- [ ] Security Rules documentation
- [ ] Authentication providers list
- [ ] nself installed
- [ ] Understanding of relational database design

### Required Tools

```bash
# Install nself
brew install nself
# OR
curl -sSL https://install.nself.org | bash

# Install Firebase CLI
npm install -g firebase-tools

# Install PostgreSQL client
brew install postgresql  # macOS
sudo apt-get install postgresql-client  # Ubuntu

# Install jq for JSON processing
brew install jq

# Optional: firebase2graphql tool
npm install -g firebase2graphql
```

### Migration Checklist

```bash
# 1. Authenticate with Firebase
firebase login

# 2. Export Firestore data
firebase firestore:export gs://[YOUR-BUCKET]/firestore-export

# 3. Export Authentication users
# From Firebase Console → Authentication → Users → Export Users

# 4. List Cloud Functions
firebase functions:list

# 5. Export Cloud Storage files
# Use Firebase Console or gsutil

# 6. Document all Security Rules
firebase deploy --only firestore:rules --dry-run
```

---

## Architecture Differences

### NoSQL vs. SQL

**Firebase Firestore (NoSQL):**
```javascript
// Document structure (denormalized)
{
  "posts": {
    "post1": {
      "title": "Hello",
      "author": {
        "id": "user1",
        "name": "John",
        "email": "john@example.com"  // Duplicated data
      },
      "comments": [
        { "text": "Nice!", "user": "Alice" },
        { "text": "Great!", "user": "Bob" }
      ]
    }
  }
}
```

**nself PostgreSQL (Relational):**
```sql
-- Normalized schema
CREATE TABLE users (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  email TEXT UNIQUE NOT NULL
);

CREATE TABLE posts (
  id UUID PRIMARY KEY,
  title TEXT NOT NULL,
  author_id UUID REFERENCES users(id),
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE comments (
  id UUID PRIMARY KEY,
  post_id UUID REFERENCES posts(id),
  user_id UUID REFERENCES users(id),
  text TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### API Differences

**Firebase SDK:**
```javascript
import { getFirestore, collection, query, where, getDocs } from 'firebase/firestore'

const db = getFirestore()
const q = query(collection(db, 'posts'), where('published', '==', true))
const snapshot = await getDocs(q)
```

**nself GraphQL:**
```graphql
query GetPosts {
  posts(where: { published: { _eq: true } }) {
    id
    title
    author {
      name
    }
  }
}
```

---

## Schema Design Conversion

### Step 1: Document Firebase Data Structure

Analyze your Firestore collections and documents:

```bash
# Export schema analysis
firebase firestore:export gs://[YOUR-BUCKET]/firestore-export
gsutil -m cp -r gs://[YOUR-BUCKET]/firestore-export .

# Analyze structure
cat firestore-export/all_namespaces/all_kinds/all_namespaces_all_kinds.export_metadata | jq
```

**Example Firebase structure:**
```
Collections:
  - users
    - uid: string
    - email: string
    - displayName: string
    - photoURL: string
    - posts: array (subcollection reference)

  - posts
    - postId: string
    - title: string
    - content: string
    - authorId: string
    - authorName: string (denormalized)
    - tags: array
    - createdAt: timestamp
    - comments: subcollection

  - posts/{postId}/comments
    - commentId: string
    - text: string
    - userId: string
    - userName: string (denormalized)
```

### Step 2: Design Relational Schema

Convert to normalized PostgreSQL schema:

**schema.sql:**
```sql
-- Users table
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  firebase_uid TEXT UNIQUE,  -- For migration mapping
  email TEXT UNIQUE NOT NULL,
  display_name TEXT,
  photo_url TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Posts table
CREATE TABLE posts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  firebase_id TEXT UNIQUE,  -- For migration mapping
  title TEXT NOT NULL,
  content TEXT,
  author_id UUID REFERENCES users(id) ON DELETE CASCADE,
  published BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Tags table (normalize array → table)
CREATE TABLE tags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT UNIQUE NOT NULL
);

-- Post tags junction table
CREATE TABLE post_tags (
  post_id UUID REFERENCES posts(id) ON DELETE CASCADE,
  tag_id UUID REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (post_id, tag_id)
);

-- Comments table
CREATE TABLE comments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  firebase_id TEXT UNIQUE,
  post_id UUID REFERENCES posts(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  text TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_posts_author ON posts(author_id);
CREATE INDEX idx_posts_created ON posts(created_at DESC);
CREATE INDEX idx_comments_post ON comments(post_id);
CREATE INDEX idx_comments_user ON comments(user_id);
```

### Step 3: Create Migration Mapping

Document how Firebase documents map to PostgreSQL tables:

**migration-mapping.md:**
```markdown
# Firebase → PostgreSQL Mapping

## Collections → Tables

- `users` collection → `users` table
  - `uid` (Firebase) → `firebase_uid` (tracking) + `id` (new UUID)
  - `email` → `email`
  - `displayName` → `display_name`
  - `photoURL` → `photo_url`

- `posts` collection → `posts` table
  - Document ID → `firebase_id` + `id` (new UUID)
  - `authorId` → `author_id` (FK to users)
  - `authorName` → REMOVED (query via join)
  - `tags[]` → `post_tags` junction table

- `posts/{postId}/comments` subcollection → `comments` table
  - `userId` → `user_id` (FK to users)
  - `userName` → REMOVED (query via join)

## Denormalization → Normalization

- Author names stored in posts → JOIN with users table
- Commenter names in comments → JOIN with users table
- Tags array → Separate tags table + junction table
```

---

## Step-by-Step Migration

### Phase 1: Setup nself Project (30 minutes)

```bash
# 1. Initialize project
mkdir firebase-to-nself && cd firebase-to-nself
nself init --wizard

# 2. Configure .env
nano .env
```

**`.env` Configuration:**

```bash
PROJECT_NAME=firebase-migration
ENV=dev
BASE_DOMAIN=localhost

# Database
POSTGRES_DB=firebase_migration
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your-secure-password

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=your-admin-secret
HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"your-jwt-secret-min-32-chars"}

# Auth
AUTH_SERVER_URL=http://auth.localhost
AUTH_CLIENT_URL=http://localhost:3000

# Storage
MINIO_ENABLED=true
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin

# Services
REDIS_ENABLED=true
FUNCTIONS_ENABLED=true
MAILPIT_ENABLED=true
```

```bash
# 3. Build and start
nself build
nself start

# 4. Verify
nself doctor
```

---

### Phase 2: Schema Creation (1-2 hours)

```bash
# 1. Create schema file
nano schema.sql
# (paste schema from Step 2 above)

# 2. Apply schema
nself db shell < schema.sql

# 3. Verify tables created
nself db shell
```

```sql
\dt  -- List tables
\d users  -- Describe users table
\d posts
\d comments
\q
```

---

### Phase 3: Firestore Data Export (1-2 hours)

#### Step 1: Export Firestore Data

```bash
# Export to Google Cloud Storage
firebase firestore:export gs://[YOUR-BUCKET]/firestore-backup

# Download to local
gsutil -m cp -r gs://[YOUR-BUCKET]/firestore-backup ./firestore-export

# Convert to JSON (if needed)
# Use Firebase Admin SDK or firebase2graphql tool
```

#### Step 2: Transform to SQL

**Example transformation script:**

```javascript
// firebase-to-sql.js
const admin = require('firebase-admin')
const fs = require('fs')

admin.initializeApp({
  credential: admin.credential.cert(require('./serviceAccountKey.json'))
})

const db = admin.firestore()

async function exportToSQL() {
  const users = []
  const posts = []
  const comments = []
  const tags = new Set()
  const postTags = []

  // Export users
  const usersSnapshot = await db.collection('users').get()
  usersSnapshot.forEach(doc => {
    const data = doc.data()
    users.push({
      firebase_uid: doc.id,
      email: data.email,
      display_name: data.displayName || null,
      photo_url: data.photoURL || null,
      created_at: data.createdAt?.toDate() || new Date()
    })
  })

  // Export posts
  const postsSnapshot = await db.collection('posts').get()
  for (const doc of postsSnapshot.docs) {
    const data = doc.data()

    posts.push({
      firebase_id: doc.id,
      title: data.title,
      content: data.content,
      author_firebase_uid: data.authorId,  // Will map to UUID later
      published: data.published || false,
      created_at: data.createdAt?.toDate() || new Date()
    })

    // Extract tags
    if (data.tags && Array.isArray(data.tags)) {
      data.tags.forEach(tag => {
        tags.add(tag)
        postTags.push({
          post_firebase_id: doc.id,
          tag_name: tag
        })
      })
    }

    // Export comments subcollection
    const commentsSnapshot = await db
      .collection('posts')
      .doc(doc.id)
      .collection('comments')
      .get()

    commentsSnapshot.forEach(commentDoc => {
      const commentData = commentDoc.data()
      comments.push({
        firebase_id: commentDoc.id,
        post_firebase_id: doc.id,
        user_firebase_uid: commentData.userId,
        text: commentData.text,
        created_at: commentData.createdAt?.toDate() || new Date()
      })
    })
  }

  // Write to JSON files
  fs.writeFileSync('users.json', JSON.stringify(users, null, 2))
  fs.writeFileSync('posts.json', JSON.stringify(posts, null, 2))
  fs.writeFileSync('comments.json', JSON.stringify(comments, null, 2))
  fs.writeFileSync('tags.json', JSON.stringify([...tags], null, 2))
  fs.writeFileSync('post_tags.json', JSON.stringify(postTags, null, 2))

  console.log('✅ Export complete!')
  console.log(`Users: ${users.length}`)
  console.log(`Posts: ${posts.length}`)
  console.log(`Comments: ${comments.length}`)
  console.log(`Tags: ${tags.size}`)
}

exportToSQL()
```

```bash
# Run export script
node firebase-to-sql.js
```

#### Step 3: Generate SQL INSERT Statements

```javascript
// json-to-sql.js
const fs = require('fs')

function generateInserts(tableName, records, columns) {
  const inserts = []

  for (const record of records) {
    const values = columns.map(col => {
      const val = record[col]
      if (val === null || val === undefined) return 'NULL'
      if (typeof val === 'string') return `'${val.replace(/'/g, "''")}'`
      if (val instanceof Date) return `'${val.toISOString()}'`
      return val
    }).join(', ')

    inserts.push(`INSERT INTO ${tableName} (${columns.join(', ')}) VALUES (${values});`)
  }

  return inserts.join('\n')
}

// Load JSON files
const users = JSON.parse(fs.readFileSync('users.json'))
const posts = JSON.parse(fs.readFileSync('posts.json'))
const comments = JSON.parse(fs.readFileSync('comments.json'))
const tags = JSON.parse(fs.readFileSync('tags.json'))
const postTags = JSON.parse(fs.readFileSync('post_tags.json'))

// Generate SQL
let sql = '-- Users\n'
sql += generateInserts('users', users, ['firebase_uid', 'email', 'display_name', 'photo_url', 'created_at'])

sql += '\n\n-- Tags\n'
sql += tags.map(tag => `INSERT INTO tags (name) VALUES ('${tag}');`).join('\n')

sql += '\n\n-- Posts (with author_id from users)\n'
sql += `
UPDATE posts p
SET author_id = u.id
FROM users u
WHERE p.author_firebase_uid = u.firebase_uid;
`

// Write to file
fs.writeFileSync('import.sql', sql)
console.log('✅ SQL generated: import.sql')
```

```bash
node json-to-sql.js
```

#### Step 4: Import to PostgreSQL

```bash
# Import data
nself db shell < import.sql

# Verify counts
nself db shell
```

```sql
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM posts;
SELECT COUNT(*) FROM comments;
SELECT COUNT(*) FROM tags;

-- Check relationships
SELECT p.title, u.display_name AS author
FROM posts p
JOIN users u ON p.author_id = u.id
LIMIT 10;
```

---

### Phase 4: Authentication Migration (2-3 hours)

Firebase Authentication → nHost Auth

#### Step 1: Export Firebase Users

```bash
# From Firebase Console:
# 1. Go to Authentication → Users
# 2. Click "Export Users" → Download CSV

# OR use Firebase Admin SDK:
```

```javascript
// export-auth-users.js
const admin = require('firebase-admin')
const fs = require('fs')

admin.initializeApp({
  credential: admin.credential.cert(require('./serviceAccountKey.json'))
})

async function exportAuthUsers() {
  const listUsers = await admin.auth().listUsers()
  const users = listUsers.users.map(user => ({
    uid: user.uid,
    email: user.email,
    emailVerified: user.emailVerified,
    displayName: user.displayName,
    photoURL: user.photoURL,
    disabled: user.disabled,
    metadata: {
      creationTime: user.metadata.creationTime,
      lastSignInTime: user.metadata.lastSignInTime
    },
    providerData: user.providerData
  }))

  fs.writeFileSync('firebase-auth-users.json', JSON.stringify(users, null, 2))
  console.log(`✅ Exported ${users.length} users`)
}

exportAuthUsers()
```

#### Step 2: Import to nself Auth

```bash
# nHost Auth uses the same auth.users table structure
nself db shell
```

```sql
-- Import users into auth.users table
INSERT INTO auth.users (
  id,
  email,
  email_verified,
  display_name,
  avatar_url,
  disabled,
  created_at,
  last_seen
)
SELECT
  firebase_uid::uuid,  -- Convert Firebase UID to UUID
  email,
  true,  -- Assume verified (or import from Firebase data)
  display_name,
  photo_url,
  false,
  created_at,
  NOW()
FROM users
WHERE firebase_uid IS NOT NULL;
```

#### Step 3: Handle OAuth Providers

```bash
# Configure OAuth providers in .env (same as Firebase)

# GitHub
AUTH_PROVIDER_GITHUB_ENABLED=true
AUTH_PROVIDER_GITHUB_CLIENT_ID=your-client-id
AUTH_PROVIDER_GITHUB_CLIENT_SECRET=your-secret

# Google
AUTH_PROVIDER_GOOGLE_ENABLED=true
AUTH_PROVIDER_GOOGLE_CLIENT_ID=your-client-id
AUTH_PROVIDER_GOOGLE_CLIENT_SECRET=your-secret

# Update redirect URLs in provider dashboards:
# OLD: https://[project-id].firebaseapp.com/__/auth/handler
# NEW: http://auth.localhost/v1/auth/callback/github (dev)
#      https://auth.yourdomain.com/v1/auth/callback/github (prod)
```

#### Step 4: Force Password Reset

Users must reset passwords (Firebase password hashes are not portable):

```bash
# Send password reset emails to all users
cat > send-reset-emails.sh << 'EOF'
#!/bin/bash
AUTH_URL="http://auth.localhost/v1"

nself db shell -c "SELECT email FROM auth.users;" | tail -n +3 | head -n -2 | while read EMAIL; do
  echo "Sending reset to: $EMAIL"
  curl -X POST "$AUTH_URL/user/password-reset" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$EMAIL\"}"
  sleep 1
done
EOF

chmod +x send-reset-emails.sh
./send-reset-emails.sh
```

---

### Phase 5: Storage Migration (2-3 hours)

Firebase Cloud Storage → MinIO

#### Step 1: Download Firebase Storage

```bash
# Using gsutil
gsutil -m cp -r gs://[your-project].appspot.com ./firebase-storage

# OR use Firebase Admin SDK
```

```javascript
// download-storage.js
const admin = require('firebase-admin')
const fs = require('fs')
const path = require('path')

admin.initializeApp({
  credential: admin.credential.cert(require('./serviceAccountKey.json')),
  storageBucket: '[your-project].appspot.com'
})

const bucket = admin.storage().bucket()

async function downloadAllFiles() {
  const [files] = await bucket.getFiles()

  for (const file of files) {
    const destPath = `./firebase-storage/${file.name}`
    const destDir = path.dirname(destPath)

    if (!fs.existsSync(destDir)) {
      fs.mkdirSync(destDir, { recursive: true })
    }

    await file.download({ destination: destPath })
    console.log(`Downloaded: ${file.name}`)
  }

  console.log(`✅ Downloaded ${files.length} files`)
}

downloadAllFiles()
```

#### Step 2: Upload to MinIO

```bash
# Create buckets
docker exec -it $(docker ps -qf "name=minio") mc alias set local http://localhost:9000 minioadmin minioadmin
docker exec -it $(docker ps -qf "name=minio") mc mb local/default
docker exec -it $(docker ps -qf "name=minio") mc mb local/avatars

# Upload files
docker exec -it $(docker ps -qf "name=minio") mc mirror ./firebase-storage local/default

# Set bucket policies (if public)
docker exec -it $(docker ps -qf "name=minio") mc policy set download local/default
```

#### Step 3: Update Storage URLs in Database

```sql
-- Update image URLs from Firebase to MinIO
UPDATE posts
SET image_url = REPLACE(
  image_url,
  'https://firebasestorage.googleapis.com/v0/b/[project].appspot.com/o/',
  'http://minio.localhost/default/'
);

UPDATE users
SET photo_url = REPLACE(
  photo_url,
  'https://firebasestorage.googleapis.com/v0/b/[project].appspot.com/o/',
  'http://minio.localhost/avatars/'
);
```

---

### Phase 6: Security Rules Migration (2-4 hours)

Firebase Security Rules → Hasura Permissions

#### Firebase Security Rules Example

```javascript
// firestore.rules
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    // Users can read/write their own document
    match /users/{userId} {
      allow read, write: if request.auth != null && request.auth.uid == userId;
    }

    // Posts
    match /posts/{postId} {
      allow read: if true;  // Anyone can read
      allow create: if request.auth != null;  // Authenticated users can create
      allow update, delete: if request.auth.uid == resource.data.authorId;  // Only author can edit
    }

    // Comments
    match /posts/{postId}/comments/{commentId} {
      allow read: if true;
      allow create: if request.auth != null;
      allow update, delete: if request.auth.uid == resource.data.userId;
    }
  }
}
```

#### Convert to Hasura Permissions

**Access Hasura Console:**
```bash
# Open http://api.localhost
# Go to Data → [table] → Permissions
```

**Users table:**
```yaml
role: user
permissions:
  select:
    filter:
      id: { _eq: X-Hasura-User-Id }
    columns: [id, email, display_name, photo_url, created_at]

  update:
    filter:
      id: { _eq: X-Hasura-User-Id }
    check:
      id: { _eq: X-Hasura-User-Id }
    columns: [display_name, photo_url]
```

**Posts table:**
```yaml
role: user
permissions:
  select:
    filter: {}  # Anyone can read (like Firebase "allow read: if true")
    columns: [id, title, content, author_id, published, created_at]

  insert:
    check:
      author_id: { _eq: X-Hasura-User-Id }
    columns: [title, content, published]

  update:
    filter:
      author_id: { _eq: X-Hasura-User-Id }  # Only author
    check:
      author_id: { _eq: X-Hasura-User-Id }
    columns: [title, content, published]

  delete:
    filter:
      author_id: { _eq: X-Hasura-User-Id }
```

**Comments table:**
```yaml
role: user
permissions:
  select:
    filter: {}  # Anyone can read
    columns: [id, post_id, user_id, text, created_at]

  insert:
    check:
      user_id: { _eq: X-Hasura-User-Id }
    columns: [post_id, text]

  update:
    filter:
      user_id: { _eq: X-Hasura-User-Id }
    check:
      user_id: { _eq: X-Hasura-User-Id }
    columns: [text]

  delete:
    filter:
      user_id: { _eq: X-Hasura-User-Id }
```

**Anonymous role:**
```yaml
role: anonymous
permissions:
  select:
    filter:
      published: { _eq: true }  # Only published posts
    columns: [id, title, content, created_at]
```

---

### Phase 7: Cloud Functions Migration (2-4 hours)

Firebase Cloud Functions → nself Functions

#### Firebase Function Example

```typescript
// Firebase Cloud Functions (functions/src/index.ts)
import * as functions from 'firebase-functions'
import * as admin from 'firebase-admin'

admin.initializeApp()

// HTTP function
export const helloWorld = functions.https.onRequest((req, res) => {
  res.json({ message: 'Hello from Firebase!' })
})

// Firestore trigger
export const onPostCreate = functions.firestore
  .document('posts/{postId}')
  .onCreate(async (snap, context) => {
    const postData = snap.data()
    const authorId = postData.authorId

    // Send notification
    await admin.firestore().collection('notifications').add({
      userId: authorId,
      message: `Your post "${postData.title}" was created`,
      createdAt: admin.firestore.FieldValue.serverTimestamp()
    })
  })

// Auth trigger
export const onUserCreate = functions.auth.user().onCreate(async (user) => {
  // Create user profile in Firestore
  await admin.firestore().collection('users').doc(user.uid).set({
    email: user.email,
    displayName: user.displayName,
    photoURL: user.photoURL,
    createdAt: admin.firestore.FieldValue.serverTimestamp()
  })
})
```

#### Convert to nself Functions

**HTTP Function:**

```typescript
// functions/src/hello.ts
import { Request, Response } from 'express'

export default (req: Request, res: Response) => {
  res.json({ message: 'Hello from nself!' })
}
```

**Database Trigger (via Hasura Event):**

```typescript
// functions/src/on-post-create.ts
import { Request, Response } from 'express'
import { GraphQLClient, gql } from 'graphql-request'

const client = new GraphQLClient(process.env.GRAPHQL_URL!, {
  headers: {
    'x-hasura-admin-secret': process.env.HASURA_ADMIN_SECRET!
  }
})

export default async (req: Request, res: Response) => {
  const { event } = req.body

  if (event.op === 'INSERT') {
    const post = event.data.new

    // Create notification
    const CREATE_NOTIFICATION = gql`
      mutation CreateNotification($userId: uuid!, $message: String!) {
        insert_notifications_one(object: { user_id: $userId, message: $message }) {
          id
        }
      }
    `

    await client.request(CREATE_NOTIFICATION, {
      userId: post.author_id,
      message: `Your post "${post.title}" was created`
    })
  }

  res.json({ success: true })
}
```

**Configure Hasura Event Trigger:**

In Hasura Console → Events → Create Event Trigger:
- Table: `posts`
- Operations: INSERT
- Webhook URL: `http://functions:3000/on-post-create`

**Auth Trigger (via Hasura Event on auth.users):**

Similar approach - create event trigger on `auth.users` table.

---

## Frontend Code Changes

### Firebase SDK → GraphQL Client

**Before (Firebase SDK):**

```typescript
import { initializeApp } from 'firebase/app'
import {
  getFirestore,
  collection,
  query,
  where,
  getDocs,
  addDoc,
  updateDoc,
  deleteDoc,
  doc
} from 'firebase/firestore'
import { getAuth, signInWithEmailAndPassword } from 'firebase/auth'

const app = initializeApp(firebaseConfig)
const db = getFirestore(app)
const auth = getAuth(app)

// Query posts
const postsRef = collection(db, 'posts')
const q = query(postsRef, where('published', '==', true))
const snapshot = await getDocs(q)
const posts = snapshot.docs.map(doc => ({ id: doc.id, ...doc.data() }))

// Create post
await addDoc(collection(db, 'posts'), {
  title: 'New Post',
  content: 'Content',
  authorId: auth.currentUser.uid,
  createdAt: serverTimestamp()
})

// Update post
await updateDoc(doc(db, 'posts', postId), {
  title: 'Updated Title'
})

// Delete post
await deleteDoc(doc(db, 'posts', postId))

// Sign in
await signInWithEmailAndPassword(auth, email, password)
```

**After (nself GraphQL):**

```typescript
import { ApolloClient, InMemoryCache, gql, useQuery, useMutation } from '@apollo/client'

const client = new ApolloClient({
  uri: 'http://api.localhost/v1/graphql',
  cache: new InMemoryCache(),
  headers: {
    Authorization: `Bearer ${accessToken}`
  }
})

// Query posts
const GET_POSTS = gql`
  query GetPosts {
    posts(where: { published: { _eq: true } }) {
      id
      title
      content
      author {
        display_name
      }
    }
  }
`

const { data } = useQuery(GET_POSTS)
const posts = data?.posts

// Create post
const CREATE_POST = gql`
  mutation CreatePost($title: String!, $content: String!) {
    insert_posts_one(object: { title: $title, content: $content }) {
      id
    }
  }
`

const [createPost] = useMutation(CREATE_POST)
await createPost({ variables: { title: 'New Post', content: 'Content' } })

// Update post
const UPDATE_POST = gql`
  mutation UpdatePost($id: uuid!, $title: String!) {
    update_posts_by_pk(pk_columns: { id: $id }, _set: { title: $title }) {
      id
    }
  }
`

const [updatePost] = useMutation(UPDATE_POST)
await updatePost({ variables: { id: postId, title: 'Updated Title' } })

// Delete post
const DELETE_POST = gql`
  mutation DeletePost($id: uuid!) {
    delete_posts_by_pk(id: $id) {
      id
    }
  }
`

const [deletePost] = useMutation(DELETE_POST)
await deletePost({ variables: { id: postId } })

// Sign in (use fetch or axios)
const response = await fetch('http://auth.localhost/v1/signin/email-password', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ email, password })
})
const { accessToken } = await response.json()
```

### Realtime Updates

**Firebase Realtime:**

```typescript
import { onSnapshot } from 'firebase/firestore'

const unsubscribe = onSnapshot(
  query(collection(db, 'posts'), where('published', '==', true)),
  (snapshot) => {
    const posts = snapshot.docs.map(doc => ({ id: doc.id, ...doc.data() }))
    setPosts(posts)
  }
)
```

**nself GraphQL Subscriptions:**

```typescript
import { useSubscription, gql } from '@apollo/client'

const POSTS_SUBSCRIPTION = gql`
  subscription OnPosts {
    posts(where: { published: { _eq: true } }, order_by: { created_at: desc }) {
      id
      title
      content
      author {
        display_name
      }
    }
  }
`

const { data } = useSubscription(POSTS_SUBSCRIPTION)
const posts = data?.posts
```

---

## Common Pitfalls

### Pitfall 1: Document ID vs UUID

**Problem**: Firebase document IDs are strings, PostgreSQL UUIDs are different

**Solution**:
```sql
-- Keep Firebase IDs for mapping
ALTER TABLE posts ADD COLUMN firebase_id TEXT;

-- Use UUIDs for new primary keys
ALTER TABLE posts ADD COLUMN id UUID PRIMARY KEY DEFAULT gen_random_uuid();
```

### Pitfall 2: Arrays in Firestore

**Problem**: Firestore supports arrays, PostgreSQL doesn't (well)

**Solution**: Normalize to junction table
```sql
-- Instead of tags: ['javascript', 'node']
-- Create tags table + post_tags junction table
```

### Pitfall 3: Timestamps

**Problem**: Firebase timestamps vs PostgreSQL timestamps

**Solution**:
```javascript
// Convert Firebase Timestamp to Date
const createdAt = firestoreDoc.data().createdAt.toDate()
```

### Pitfall 4: Security Rules Syntax

**Problem**: Firebase `request.auth.uid` doesn't exist in Hasura

**Solution**: Use Hasura session variables
```yaml
# Firebase: request.auth.uid == userId
# Hasura: user_id: { _eq: X-Hasura-User-Id }
```

---

## Rollback Procedure

If migration fails:

1. Keep Firebase project active during migration
2. Test nself thoroughly before DNS switch
3. Keep Firebase running for 2-4 weeks as fallback
4. Monitor error rates after switch
5. Have rollback script ready:

```bash
# Rollback to Firebase
# 1. Change DNS back to Firebase
# 2. Revert frontend to Firebase SDK
# 3. Rebuild and deploy
```

---

## Automated Migration Helper

```bash
#!/bin/bash
# firebase-to-nself.sh

set -e

echo "🚀 Firebase to nself Migration Helper"
echo "======================================"

FIREBASE_PROJECT=$1
NSELF_PROJECT=$2

if [ -z "$FIREBASE_PROJECT" ] || [ -z "$NSELF_PROJECT" ]; then
  echo "Usage: ./firebase-to-nself.sh <firebase-project-id> <nself-project-name>"
  exit 1
fi

echo "📥 Step 1: Exporting from Firebase..."
mkdir -p migration-data
firebase firestore:export gs://$FIREBASE_PROJECT.appspot.com/export
gsutil -m cp -r gs://$FIREBASE_PROJECT.appspot.com/export ./migration-data/

echo "🔧 Step 2: Initializing nself..."
mkdir -p $NSELF_PROJECT
cd $NSELF_PROJECT
nself init --name $NSELF_PROJECT

echo "📦 Step 3: Building nself..."
nself build
nself start

sleep 30

echo "✅ nself is ready!"
echo ""
echo "⚠️  MANUAL STEPS REQUIRED:"
echo "1. Design PostgreSQL schema (see docs/migration/FROM-FIREBASE.md)"
echo "2. Transform Firestore data to SQL"
echo "3. Import data to PostgreSQL"
echo "4. Convert Security Rules to Hasura Permissions"
echo "5. Update frontend code (Firebase SDK → GraphQL)"
echo ""
echo "📚 Full guide: docs/migration/FROM-FIREBASE.md"
```

---

## Conclusion

Migrating from Firebase to nself is a significant undertaking due to NoSQL → SQL paradigm shift. However, the benefits are substantial:

- ✅ **Relational power** - Joins, complex queries, transactions
- ✅ **No vendor lock-in** - Open-source stack
- ✅ **Full control** - Own your infrastructure and data
- ✅ **Cost savings** - Predictable costs vs Firebase usage bills

**Timeline**: 16-32 hours for complete migration

**Recommended Approach**:
1. Design PostgreSQL schema carefully (4-8 hours)
2. Migrate to staging environment first
3. Test thoroughly for 2-4 weeks
4. Migrate production during low-traffic period
5. Keep Firebase running for 1 month as fallback

Good luck with your migration! 🚀
