# nself Video Tutorial Scripts

Complete scripts for video tutorials, including voiceover text, screen actions, and key points.

---

## Video 1: Getting Started with nself (5 minutes)

### Target Audience
Complete beginners who want to understand what nself is and how to get started.

### Learning Objectives
- Understand what nself is
- Install nself
- Create first project
- See services running

---

### Script

**[00:00 - 00:30] INTRO**

*Screen: nself logo animation*

**Voiceover:**
"Hi, I'm [Name], and in this video, I'll show you how to get started with nself - the fastest way to build full-stack applications. In just 5 minutes, you'll have a complete backend with PostgreSQL, GraphQL, authentication, and more - all configured and running locally."

*Screen: Show diagram of nself architecture*

**Voiceover:**
"nself gives you everything you need: a database, API layer, authentication, storage, and even monitoring - all pre-configured and ready to use. Let's dive in."

---

**[00:30 - 01:30] INSTALLATION**

*Screen: Terminal window, clear screen*

**Voiceover:**
"First, let's install nself. On macOS, it's as simple as using Homebrew."

*Screen: Type command*
```bash
brew install nself-org/nself/nself
```

**Voiceover:**
"For Linux or Windows with WSL, you can use the install script."

*Screen: Show command*
```bash
curl -sSL https://install.nself.org | bash
```

**Voiceover:**
"Let's verify the installation."

*Screen: Type and execute*
```bash
nself version
```

*Screen: Output shows: nself v0.9.9*

**Voiceover:**
"Perfect! nself is installed. Now let's create our first project."

---

**[01:30 - 02:30] PROJECT INITIALIZATION**

*Screen: Create directory and navigate*

**Voiceover:**
"I'll create a new directory for my project called 'my-app'."

*Screen: Type*
```bash
mkdir my-app
cd my-app
```

**Voiceover:**
"Now, let's initialize nself with the demo configuration. This gives us a full-featured setup to start with."

*Screen: Type*
```bash
nself init --demo
```

*Screen: Show initialization output with progress indicators*

**Voiceover:**
"nself is creating the project structure and generating a .env file with sensible defaults. This includes PostgreSQL, Hasura for GraphQL, authentication, Redis, MinIO for storage, and even a complete monitoring stack."

*Screen: Show created files*
```bash
ls -la
```

**Voiceover:**
"You can see the .env file and project structure. Let's take a quick look at what we've configured."

*Screen: Show .env file with syntax highlighting*

**Voiceover:**
"Here you can see all the services that will be available. For production, you'd want to change these secrets, but for development, these defaults work great."

---

**[02:30 - 03:30] BUILD AND START**

*Screen: Terminal*

**Voiceover:**
"Now let's build our infrastructure. This command generates all the necessary Docker configurations, nginx routing, and service definitions."

*Screen: Type*
```bash
nself build
```

*Screen: Show build output*

**Voiceover:**
"nself is generating docker-compose.yml with 25 services, nginx configuration for routing, SSL certificates for local development, and initialization scripts for the database."

**Voiceover:**
"Now the exciting part - let's start everything!"

*Screen: Type*
```bash
nself start
```

*Screen: Show containers starting with real-time status*

**Voiceover:**
"Watch as nself starts all 25 Docker containers. This includes your database, GraphQL API, authentication service, Redis cache, MinIO storage, monitoring tools, and more."

*Screen: Show health checks passing*

**Voiceover:**
"All services are healthy and running. Let's check the status."

*Screen: Type*
```bash
nself status
```

*Screen: Show status output with green checkmarks*

---

**[03:30 - 04:30] EXPLORING SERVICES**

*Screen: Browser window*

**Voiceover:**
"Let's see what we have running. First, let's check all our service URLs."

*Screen: Terminal*
```bash
nself urls
```

*Screen: Show list of URLs*

**Voiceover:**
"We have URLs for the GraphQL API, authentication, admin interface, MinIO storage console, Grafana monitoring, and more. Let's open the Hasura Console."

*Screen: Type*
```bash
nself admin hasura
```

*Screen: Browser opens to Hasura Console*

**Voiceover:**
"Here's the Hasura Console where we can design our database schema, create GraphQL queries, and set permissions. Let's create a simple table."

*Screen: Navigate to Data tab, create 'users' table*

**Voiceover:**
"I'll create a 'users' table with a few columns. Hasura automatically generates GraphQL queries and mutations for us."

*Screen: Switch to API tab, run query*
```graphql
query {
  users {
    id
    name
    email
  }
}
```

**Voiceover:**
"And just like that, we have a working GraphQL API. No code required."

---

**[04:30 - 05:00] WRAP UP**

*Screen: Split screen showing terminal and browser*

**Voiceover:**
"In just 5 minutes, we've installed nself, created a project, and deployed a complete backend with database, GraphQL API, authentication, storage, and monitoring."

*Screen: Show architecture diagram again*

**Voiceover:**
"Everything is running locally, ready for development. When you're ready to deploy to production, it's just as easy - one command to deploy to any server."

*Screen: Show command (don't run)*
```bash
nself deploy push production
```

**Voiceover:**
"That's nself - powerful, fast, and simple. Check the links in the description for more tutorials, documentation, and example projects. Thanks for watching, and happy building!"

*Screen: nself logo with links*
- Documentation: docs.nself.org
- GitHub: github.com/nself-org/cli
- Examples: github.com/nself-org/cli/examples

---

### Key Points to Emphasize

1. **Speed**: "5 minutes to full-stack"
2. **Completeness**: "Everything included"
3. **Simplicity**: "One command to start"
4. **Production-ready**: "Same setup for dev and prod"

### B-Roll Suggestions

- Code editor showing project files
- Architecture diagrams
- Service dashboards
- Browser with multiple tabs of services

### Call to Action

- Subscribe for more tutorials
- Check out example projects
- Join Discord community

---

## Video 2: Building Your First API (10 minutes)

### Target Audience
Developers familiar with APIs who want to build a complete CRUD API with GraphQL.

### Learning Objectives
- Design database schema
- Configure Hasura permissions
- Create relationships
- Test GraphQL queries
- Secure with authentication

---

### Script

**[00:00 - 00:45] INTRO**

*Screen: VS Code with empty project*

**Voiceover:**
"In this tutorial, we'll build a complete REST and GraphQL API for a blog application. We'll create database tables, set up relationships, configure permissions, and add authentication - all in about 10 minutes."

*Screen: Show final result - GraphQL Playground with queries*

**Voiceover:**
"By the end, you'll have a production-ready API that supports creating posts, adding comments, user authentication, and fine-grained permissions. Let's get started."

---

**[00:45 - 02:00] DATABASE SCHEMA**

*Screen: Create schema.sql file*

**Voiceover:**
"First, let's design our database schema. We'll create three tables: users, posts, and comments."

*Screen: Type SQL*
```sql
CREATE TABLE posts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  author_id UUID NOT NULL REFERENCES auth.users(id),
  published BOOLEAN DEFAULT false,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE comments (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  post_id UUID NOT NULL REFERENCES posts(id),
  author_id UUID NOT NULL REFERENCES auth.users(id),
  content TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);
```

**Voiceover:**
"Notice we're referencing auth.users for the author - that's managed by nself's authentication service. Now let's apply this schema."

*Screen: Terminal*
```bash
nself db execute --file schema.sql
```

*Screen: Show success message*

**Voiceover:**
"Our tables are created. Let's open Hasura and track them."

---

**[02:00 - 03:30] HASURA CONFIGURATION**

*Screen: Hasura Console*

**Voiceover:**
"In the Hasura Console, we need to track our new tables so they're available in GraphQL."

*Screen: Navigate to Data tab*

**Voiceover:**
"I'll click 'Track All' to track both posts and comments tables."

*Screen: Tables appear in sidebar*

**Voiceover:**
"Now let's set up relationships. A post has an author, which is a user. And a post has many comments."

*Screen: Click on posts table, Relationships tab*

**Voiceover:**
"I'll add an object relationship called 'author' pointing to auth.users via author_id."

*Screen: Create relationship*

**Voiceover:**
"And an array relationship called 'comments' from comments table via post_id."

*Screen: Create relationship*

**Voiceover:**
"Perfect. Now our GraphQL schema understands these relationships. Let's test a query."

---

**[03:30 - 05:00] GRAPHQL QUERIES**

*Screen: API tab in Hasura*

**Voiceover:**
"Let's write a query to fetch posts with their authors and comments."

*Screen: Type GraphQL query*
```graphql
query GetPosts {
  posts {
    id
    title
    content
    author {
      displayName
      email
    }
    comments {
      id
      content
      author {
        displayName
      }
    }
  }
}
```

**Voiceover:**
"This query gets posts, includes the author's info, and all comments with their authors. Let's run it."

*Screen: Execute query, show empty result*

**Voiceover:**
"No data yet. Let's add some test data with a mutation."

*Screen: Type mutation*
```graphql
mutation CreatePost {
  insert_posts_one(object: {
    title: "My First Post",
    content: "This is the content",
    author_id: "user-uuid-here"
  }) {
    id
    title
  }
}
```

*Screen: Execute, show success*

**Voiceover:**
"Great! Now if we run our query again..."

*Screen: Re-run GetPosts query*

**Voiceover:**
"We see our post with author information. Our API is working!"

---

**[05:00 - 07:00] PERMISSIONS**

*Screen: Permissions tab*

**Voiceover:**
"Now let's add security. We don't want anonymous users reading everything, and users should only be able to edit their own posts."

*Screen: Click on posts table, Permissions tab*

**Voiceover:**
"For anonymous users, let's allow them to read only published posts."

*Screen: Add row select permission for 'anonymous' role*
```json
{
  "published": {
    "_eq": true
  }
}
```

**Voiceover:**
"And they can only see certain columns - not internal fields."

*Screen: Select columns: id, title, content, created_at*

**Voiceover:**
"For authenticated users, they can read all published posts, but only create, update, and delete their own."

*Screen: Add permissions for 'user' role*

**Insert permission:**
```json
{
  "author_id": {
    "_eq": "X-Hasura-User-Id"
  }
}
```

*Screen: Set column preset author_id = X-Hasura-User-Id*

**Update permission:**
```json
{
  "author_id": {
    "_eq": "X-Hasura-User-Id"
  }
}
```

**Voiceover:**
"This ensures users can only modify their own posts. The X-Hasura-User-Id comes from the JWT token."

---

**[07:00 - 08:30] TESTING WITH AUTH**

*Screen: Terminal*

**Voiceover:**
"Let's test this with actual authentication. First, I'll create a user."

*Screen: Type curl command*
```bash
curl -X POST http://auth.localhost/signup/email-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePassword123!"
  }'
```

*Screen: Show response with JWT token*

**Voiceover:**
"Perfect. We got a JWT token back. Now let's use this to create a post via the API."

*Screen: Type GraphQL mutation with Authorization header*
```bash
curl -X POST http://api.localhost/v1/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE" \
  -d '{
    "query": "mutation { insert_posts_one(object: {title: \"Authenticated Post\", content: \"Created via API\"}) { id } }"
  }'
```

*Screen: Show success*

**Voiceover:**
"And it works! The post was created with the correct author_id from our JWT token."

---

**[08:30 - 09:30] ADVANCED FEATURES**

*Screen: Back to schema.sql*

**Voiceover:**
"Let's add some advanced features like full-text search and auto-updating timestamps."

*Screen: Add to schema*
```sql
-- Add search column
ALTER TABLE posts ADD COLUMN search_vector tsvector;

-- Update search vector on changes
CREATE TRIGGER posts_search_update
BEFORE INSERT OR UPDATE ON posts
FOR EACH ROW EXECUTE FUNCTION
tsvector_update_trigger(search_vector, 'pg_catalog.english', title, content);

-- Create index for fast searching
CREATE INDEX posts_search_idx ON posts USING gin(search_vector);
```

**Voiceover:**
"Now we can search posts efficiently."

*Screen: Hasura Console, create custom function*
```sql
CREATE FUNCTION search_posts(search text)
RETURNS SETOF posts AS $$
  SELECT * FROM posts
  WHERE search_vector @@ plainto_tsquery('english', search)
  ORDER BY ts_rank(search_vector, plainto_tsquery('english', search)) DESC;
$$ LANGUAGE sql STABLE;
```

*Screen: Track function in Hasura*

**Voiceover:**
"Track this function in Hasura, and now we can search from GraphQL."

*Screen: Test search query*
```graphql
query SearchPosts {
  search_posts(args: {search: "nself"}) {
    id
    title
    content
  }
}
```

---

**[09:30 - 10:00] WRAP UP**

*Screen: Show complete API in action*

**Voiceover:**
"And there you have it! A complete API with database schema, GraphQL queries, authentication, permissions, and even full-text search - all in 10 minutes."

*Screen: Show documentation*

**Voiceover:**
"Your API is now ready for production. You can deploy it with one command, add monitoring, setup backups, and scale as needed."

*Screen: Show related resources*

**Voiceover:**
"Check out the links in the description for the complete source code, more advanced tutorials, and deployment guides. Thanks for watching!"

---

### Key Points to Emphasize

1. **No boilerplate**: "Database to API in minutes"
2. **Built-in security**: "Permissions out of the box"
3. **Production-ready**: "Everything you need"
4. **GraphQL power**: "Complex queries made simple"

---

## Video 3: Multi-Tenant Apps Made Easy (15 minutes)

### Target Audience
Developers building SaaS applications who need multi-tenancy.

### Learning Objectives
- Understand multi-tenant architecture
- Implement Row-Level Security
- Create tenant isolation
- Handle tenant switching
- Setup billing per tenant

---

### Script

**[00:00 - 01:00] INTRO**

*Screen: Show popular SaaS apps (Slack, Notion, etc.)*

**Voiceover:**
"What do Slack, Notion, and Asana have in common? They're all multi-tenant applications - one codebase serving thousands of customers with complete data isolation. In this tutorial, I'll show you how to build the same architecture using nself, complete with tenant isolation, billing, and team management."

*Screen: Show final demo - switching between tenants*

**Voiceover:**
"By the end, you'll understand how to architect multi-tenant SaaS applications and have a working example you can deploy today. Let's dive in."

---

**[01:00 - 03:00] MULTI-TENANCY EXPLAINED**

*Screen: Architecture diagram*

**Voiceover:**
"First, what is multi-tenancy? Instead of deploying a separate application for each customer, you run one application that serves all customers, with their data completely isolated."

*Screen: Show three isolation approaches*

**Voiceover:**
"There are three common approaches: separate databases, separate schemas, or shared schema with Row-Level Security. We'll use RLS because it's cost-effective and maintainable."

*Screen: Show RLS concept*

**Voiceover:**
"With Row-Level Security, every table has a tenant_id column, and PostgreSQL automatically filters data based on the current user's tenant. Let's implement it."

---

**[03:00 - 05:30] DATABASE SCHEMA**

*Screen: Create multi-tenant schema*

**Voiceover:**
"Our schema needs a tenants table and tenant_users to handle the many-to-many relationship."

*Screen: Show SQL*
```sql
CREATE TABLE tenants (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT UNIQUE NOT NULL
);

CREATE TABLE tenant_users (
  tenant_id UUID REFERENCES tenants(id),
  user_id UUID REFERENCES auth.users(id),
  role TEXT NOT NULL,
  PRIMARY KEY (tenant_id, user_id)
);
```

**Voiceover:**
"Now every business table includes tenant_id and RLS policies."

*Screen: Show projects table with RLS*
```sql
CREATE TABLE projects (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL
);

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON projects
  USING (
    tenant_id IN (
      SELECT tenant_id FROM tenant_users
      WHERE user_id = current_setting('hasura.user.id')::UUID
    )
  );
```

**Voiceover:**
"This policy ensures users can only see projects from their tenants. Let's test it."

---

**[Continues with sections on Testing Isolation, Hasura Configuration, Frontend Integration, Billing, and Deployment...]**

---

## Video 4: Production Deployment Walkthrough (20 minutes)

*(Script continues with detailed deployment walkthrough...)*

---

## Video 5: Complete SaaS in 60 Minutes (60 minutes)

*(Comprehensive tutorial combining all concepts...)*

---

## Production Notes

### Equipment
- Screen recording: 1920x1080 minimum
- Microphone: Studio quality
- Video editing: DaVinci Resolve or Final Cut Pro

### Style Guide
- Clear, friendly tone
- Show keyboard shortcuts
- Highlight important lines of code
- Use zooms for readability
- Add captions for accessibility

### Publishing
- YouTube: Main platform
- Vimeo: Backup
- Embedded in documentation
- Social media clips (TikTok, Instagram Reels)

---

**Version:** 0.9.8
**Last Updated:** January 2026
