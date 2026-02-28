# Simple Blog - nself Quickstart Example

A minimal blog application demonstrating core nself features. Perfect for learning nself fundamentals.

## Features

- User authentication (sign up, login, logout)
- Create, read, update, delete blog posts
- Comments on posts
- User profiles
- GraphQL API via Hasura
- React frontend with TypeScript
- Responsive design

## Tech Stack

- **Database:** PostgreSQL
- **API:** Hasura GraphQL Engine
- **Auth:** nHost Authentication
- **Frontend:** React + TypeScript + Vite
- **Styling:** TailwindCSS

## Architecture

```
┌─────────────────┐
│  React Frontend │
│  (Port 3000)    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     Nginx       │
│  (Port 80/443)  │
└────────┬────────┘
         │
    ┌────┴─────┬──────────┐
    ▼          ▼          ▼
┌────────┐ ┌───────┐ ┌────────┐
│ Hasura │ │ Auth  │ │ Static │
│  API   │ │Service│ │ Files  │
└───┬────┘ └───┬───┘ └────────┘
    │          │
    └────┬─────┘
         ▼
   ┌──────────┐
   │PostgreSQL│
   └──────────┘
```

## Quick Start

### Prerequisites

- nself installed ([Installation Guide](../../../getting-started/Installation.md))
- Docker and Docker Compose
- Node.js 18+ (for frontend development)

### 1. Setup Project

```bash
# Clone this example
cd examples/01-simple-blog/

# Copy environment template
cp .env.example .env

# Review and customize settings
nano .env
```

### 2. Initialize nself

```bash
# Initialize the project
nself init

# Generate infrastructure
nself build

# Start all services
nself start
```

### 3. Setup Database

```bash
# Apply database schema
nself db migrate apply

# Seed sample data (optional)
nself db seed
```

### 4. Configure Hasura

```bash
# Apply Hasura metadata
nself service hasura metadata apply

# Open Hasura Console
nself admin hasura
```

### 5. Start Frontend

```bash
# Install dependencies
cd frontend/
npm install

# Start development server
npm run dev
```

### 6. Open Application

- **Blog:** http://localhost:3000
- **Hasura Console:** http://api.localhost
- **Auth Dashboard:** http://auth.localhost

## Project Structure

```
01-simple-blog/
├── .env.example              # Environment template
├── README.md                 # This file
├── TUTORIAL.md               # Step-by-step guide
├── DEPLOYMENT.md             # Production deployment
│
├── database/
│   ├── schema.sql            # Database schema
│   ├── migrations/           # Migration files
│   └── seeds/                # Sample data
│
├── hasura/
│   ├── metadata/             # Hasura metadata
│   ├── migrations/           # Hasura migrations
│   └── config.yaml           # Hasura config
│
├── frontend/
│   ├── src/
│   │   ├── components/       # React components
│   │   ├── pages/            # Page components
│   │   ├── hooks/            # Custom hooks
│   │   ├── lib/              # Utilities
│   │   └── graphql/          # GraphQL queries
│   ├── public/               # Static assets
│   ├── package.json
│   ├── vite.config.ts
│   └── tailwind.config.js
│
└── docs/
    ├── API.md                # API documentation
    └── TROUBLESHOOTING.md    # Common issues
```

## Database Schema

### Tables

**users** - User accounts (managed by Auth service)
- `id` (uuid, primary key)
- `email` (text, unique)
- `display_name` (text)
- `avatar_url` (text)
- `created_at` (timestamp)

**posts** - Blog posts
- `id` (uuid, primary key)
- `title` (text)
- `slug` (text, unique)
- `content` (text)
- `excerpt` (text)
- `author_id` (uuid, foreign key → users)
- `published` (boolean)
- `published_at` (timestamp)
- `created_at` (timestamp)
- `updated_at` (timestamp)

**comments** - Post comments
- `id` (uuid, primary key)
- `post_id` (uuid, foreign key → posts)
- `author_id` (uuid, foreign key → users)
- `content` (text)
- `created_at` (timestamp)
- `updated_at` (timestamp)

### Relationships

```
users ──┬─── posts (author_id)
        └─── comments (author_id)

posts ───── comments (post_id)
```

## Key Features Explained

### 1. Authentication

Using nHost Auth service:

```typescript
import { nhost } from './lib/nhost'

// Sign up
await nhost.auth.signUp({
  email: 'user@example.com',
  password: 'SecurePassword123!'
})

// Sign in
await nhost.auth.signIn({
  email: 'user@example.com',
  password: 'SecurePassword123!'
})

// Sign out
await nhost.auth.signOut()
```

### 2. GraphQL Queries

Fetching posts with Hasura:

```graphql
query GetPosts {
  posts(
    where: { published: { _eq: true } }
    order_by: { published_at: desc }
  ) {
    id
    title
    slug
    excerpt
    published_at
    author {
      display_name
      avatar_url
    }
  }
}
```

### 3. Permissions

Row-level security configured in Hasura:

- **Public:** Can read published posts
- **Authenticated Users:** Can create posts and comments
- **Post Authors:** Can update/delete own posts
- **Comment Authors:** Can update/delete own comments

### 4. Real-time Updates

Subscribe to new posts:

```typescript
const { data, loading } = useSubscription(
  gql`
    subscription NewPosts {
      posts(
        where: { published: { _eq: true } }
        order_by: { published_at: desc }
        limit: 10
      ) {
        id
        title
        slug
        published_at
      }
    }
  `
)
```

## Environment Configuration

Key variables in `.env`:

```bash
# Project
PROJECT_NAME=simple-blog
BASE_DOMAIN=localhost

# Database
POSTGRES_DB=blog_db
POSTGRES_PASSWORD=your-secure-password

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=your-admin-secret

# Auth
AUTH_JWT_SECRET=your-jwt-secret

# Frontend
FRONTEND_APP_1_NAME=blog-frontend
FRONTEND_APP_1_PORT=3000
FRONTEND_APP_1_ROUTE=/
```

## Development Workflow

### Making Changes

```bash
# 1. Update database schema
nano database/schema.sql

# 2. Create migration
nself db migrate create add_new_feature

# 3. Apply migration
nself db migrate apply

# 4. Update Hasura metadata
nself service hasura metadata reload

# 5. Test changes
cd frontend/
npm run dev
```

### Adding Features

See `TUTORIAL.md` for detailed guides on:
- Adding new tables
- Creating custom queries
- Implementing authorization
- Adding file uploads
- Email notifications

## Testing

```bash
# Backend tests (database + GraphQL)
nself db test

# Frontend tests
cd frontend/
npm test

# E2E tests
npm run test:e2e
```

## Deployment

See `DEPLOYMENT.md` for complete production deployment guide.

Quick deploy to a VPS:

```bash
# 1. Setup server
nself deploy provision --server your-server.com

# 2. Configure SSL
nself auth ssl cert --domain blog.yourdomain.com

# 3. Deploy
nself deploy push production

# 4. Run migrations
nself deploy exec production "nself db migrate apply"
```

## Common Tasks

### Reset Database

```bash
nself db reset --confirm
nself db migrate apply
nself db seed
```

### View Logs

```bash
# All services
nself logs

# Specific service
nself logs postgres
nself logs hasura
```

### Backup Database

```bash
# Create backup
nself backup create blog-backup-$(date +%Y%m%d)

# Restore backup
nself backup restore blog-backup-20260131
```

## Troubleshooting

### Services Won't Start

```bash
# Check status
nself status

# View detailed logs
nself logs --verbose

# Restart services
nself restart
```

### GraphQL Errors

```bash
# Check Hasura logs
nself logs hasura

# Reload metadata
nself service hasura metadata reload

# Verify permissions
nself admin hasura
```

### Frontend Issues

```bash
# Clear cache
cd frontend/
rm -rf node_modules/ .vite/
npm install

# Check API connection
curl http://api.localhost/v1/graphql
```

See `docs/TROUBLESHOOTING.md` for more solutions.

## Next Steps

Once you've completed this example:

1. **Add Features:**
   - Categories and tags
   - Post drafts
   - Image uploads (MinIO)
   - Search (MeiliSearch)

2. **Try Advanced Examples:**
   - [SaaS Starter](../02-saas-starter/) - Multi-tenancy
   - [Real-time Chat](../04-realtime-chat/) - WebSockets

3. **Deploy to Production:**
   - Follow `DEPLOYMENT.md`
   - Setup monitoring
   - Configure backups

## Resources

- **Tutorial:** [TUTORIAL.md](./TUTORIAL.md)
- **API Docs:** docs/API.md
- **nself Docs:** [nself Documentation](../../../README.MD)
- **Hasura Docs:** https://hasura.io/docs/

## Support

- **Issues:** [GitHub Issues](https://github.com/nself-org/cli/issues)
- **Discussions:** [GitHub Discussions](https://github.com/nself-org/cli/discussions)

## License

MIT License - See LICENSE

---

**Version:** 0.9.8
**Difficulty:** Beginner
**Time to Complete:** 15-30 minutes
