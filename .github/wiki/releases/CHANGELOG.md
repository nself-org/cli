# Changelog

All notable changes to nself will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Database Seeding System
- **`nself db seed apply`** - Apply seed files with automatic tracking
- **`nself db seed apply <file>`** - Apply specific seed file
- **`nself db seed list`** - List all seeds with their status (✓ Applied / ○ Pending)
- **`nself db seed rollback`** - Rollback last applied seed (removes tracking)
- Seed tracking table (`nself_seeds`) prevents duplicate execution
- Environment-specific seed support (common/local/staging/production)
- Idempotent seed execution with ON CONFLICT handling

#### Authentication Setup Commands
- **`nself auth setup`** - Interactive auth setup wizard
- **`nself auth setup --default-users`** - One-command auth setup (creates 3 staff users)
- **`nself auth create-user`** - Create individual auth users
  - Interactive mode with prompts
  - Non-interactive with flags (--email, --password, --role, --name)
  - Auto-generates secure passwords if not provided
- **`nself auth list-users`** - List all auth users with details
- Proper nHost schema support:
  - Creates users in `auth.users` table
  - Links to email provider in `auth.user_providers` table
  - Uses bcrypt password hashing (cost factor 10)
  - Generates dummy access tokens for seeded users
  - Stores custom roles in JSONB metadata field

#### Hasura Metadata Management
- **`nself hasura`** - New command group for Hasura management
- **`nself hasura metadata apply`** - Apply metadata from files or track defaults
- **`nself hasura metadata export`** - Export current metadata to JSON
- **`nself hasura metadata reload`** - Reload Hasura metadata
- **`nself hasura metadata clear`** - Clear all metadata (with confirmation)
- **`nself hasura track table <schema.table>`** - Track specific database table
- **`nself hasura track schema <schema>`** - Track all tables in schema
- **`nself hasura untrack table <schema.table>`** - Untrack table
- **`nself hasura console`** - Open Hasura console in browser
- Auto-tracks auth schema tables (users, user_providers, providers, etc.)
- Integrated with `nself auth setup` workflow

#### Security Hardening System
- **`nself audit security`** - Comprehensive security audit command
  - Detects weak secrets (minioadmin, changeme, admin, etc.)
  - Checks secret length (warns if < 24 characters)
  - Validates CORS configuration (detects wildcards in production)
  - Audits exposed ports (warns if database exposed in production)
  - Verifies container users (checks for root containers)
  - Subcommands: `all`, `secrets`, `cors`, `ports`, `containers`

- **`nself harden`** - Automated security hardening command
  - Interactive wizard mode (audit → review → apply)
  - `nself harden all` - Apply all fixes automatically
  - `nself harden secrets` - Rotate weak secrets only
  - `nself harden cors` - Fix CORS configuration
  - Generates cryptographically strong secrets
  - Environment-aware hardening (dev/staging/prod)

#### Environment-Aware CORS Configuration
- CORS now respects environment settings (no more wildcard "*" in production)
- **Development**: Permissive CORS for localhost development
  - `http://localhost:*`, `http://*.local.nself.org`, `https://*.local.nself.org`
- **Staging**: Production domains + localhost for testing
  - `https://*.${BASE_DOMAIN}`, `http://localhost:3000`
- **Production**: Strict CORS - only your domain
  - `https://*.${BASE_DOMAIN}` (NO wildcards, NO localhost)
- Can be overridden via `HASURA_GRAPHQL_CORS_DOMAIN` environment variable
- Custom service templates (Express, FastAPI, Flask) use same environment-aware pattern

#### Enhanced Secret Generation
- **Environment-specific secret strength**:
  - Development: 32-64 character secrets (convenient but secure)
  - Staging: 40-80 character secrets (production-like)
  - Production: 48-96 character secrets (maximum security)
- Auto-generates secrets for:
  - PostgreSQL password
  - Hasura admin secret & JWT key
  - MinIO credentials
  - Grafana credentials
  - MeiliSearch/Typesense API keys
  - nself-admin secret key
- Uses cryptographically secure random generation (openssl, /dev/urandom)
- Only generates if empty (preserves existing secrets)
- Integrated into `nself init` wizard

#### Non-Root Container Users
- All containers now run as non-root users for security:
  - PostgreSQL: `user: "70:70"`
  - Hasura: `user: "1001:1001"`
  - Auth: `user: "1001:1001"`
  - Redis: `user: "999:999"`
  - MinIO: `user: "1000:1000"`
  - Mailpit: `user: "1000:1000"`
  - MeiliSearch: `user: "1000:1000"`
  - Grafana: `user: "472:472"`
  - Prometheus: `user: "65534:65534"`
  - All monitoring exporters run as non-root
- **Exceptions** (require root privileges):
  - Nginx: Needs root for ports 80/443
  - cAdvisor: Requires privileged mode
  - Promtail: Needs root to read system logs

#### Conditional Port Exposure
- Database ports now conditionally exposed based on environment
- **`POSTGRES_EXPOSE_PORT`** variable with three modes:
  - `auto` (default): Expose in dev, hide in prod/staging
  - `true`: Always expose to 127.0.0.1 (localhost only)
  - `false`: Never expose (internal Docker network only)
- **Development**: Port exposed to `127.0.0.1:5432` (database tools work)
- **Production**: Port NOT exposed (internal Docker network only, maximum security)
- Reduces attack surface in production deployments

#### Environment-Conditional Mailpit
- Mailpit security settings now respect environment:
  - **Development**: `MP_SMTP_AUTH_ACCEPT_ANY=1`, `MP_SMTP_AUTH_ALLOW_INSECURE=1`
  - **Production/Staging**: Secure defaults, respects `MAILPIT_ACCEPT_ANY_AUTH` env var
- Production warning added: "Mailpit is for development only and insecure"
- Guides users to configure production email: `nself service email configure`

#### Security Documentation
- **`.wiki/security/HARDENING-GUIDE.md`** - Comprehensive security hardening guide
  - Security philosophy and best practices
  - Environment-specific security configurations
  - Using audit and hardening commands
  - Production deployment checklist
  - Compliance considerations (SOC 2, GDPR, HIPAA)
  - Troubleshooting common security issues
  - Advanced topics (secret rotation, CI/CD, monitoring)

- **`.wiki/security/MIGRATION-V0.10.0.md`** - Migration guide
  - Breaking changes and impact assessment
  - Safe 8-step upgrade process
  - Environment-specific migration procedures
  - Troubleshooting migration issues
  - Post-migration checklist
  - Rollback instructions

#### Seed Templates
- **`src/templates/seeds/001_auth_users.sql.template`** - nHost auth seed template
  - Creates users with proper schema structure
  - Includes owner, admin, and support users
  - Placeholder system for customization
  - Idempotent with ON CONFLICT clauses
- **`src/templates/seeds/README.md`** - Template documentation and examples

#### Documentation
- **`.wiki/guides/DEV_WORKFLOW.md`** - Complete development workflow guide
  - Zero to working auth in <5 minutes
  - Step-by-step instructions
  - Troubleshooting section
  - Best practices
- **`.wiki/guides/AUTH_SETUP.md`** - Comprehensive authentication guide
  - How nself auth works (architecture diagram)
  - Auth schema structure explained
  - Quick setup vs manual setup
  - Creating and testing users
  - Security best practices
  - Password hashing details
- **`.wiki/guides/SEEDING.md`** - Database seeding guide
  - Seed directory structure
  - Creating and applying seeds
  - Environment-specific seeds
  - nHost auth seed examples
  - Best practices and patterns
  - Advanced topics

### Changed

#### Database Commands
- **`nself db seed run`** - Now uses `seed_apply` internally (backward compatible)
- Improved seed execution with better error messages
- Enhanced seed status reporting

#### Auth Commands
- Updated `nself auth` help text to include new USER MANAGEMENT section
- Better integration between auth and Hasura metadata

#### Exec Command
- **Fixed stdin piping support** - Now properly supports:
  - `cat file.sql | nself exec postgres psql`
  - Heredoc syntax: `nself exec postgres psql <<EOF`
  - Multi-line SQL piping
- Always adds `-i` flag for stdin compatibility
- No behavior change for interactive usage

### Fixed

- **Bug #2 from Feedback:** `nself exec` now properly supports stdin piping
- Auth service no longer returns "field 'users' not found" error
- Seed files can now be piped to containers without hanging
- Hasura metadata properly tracks auth tables on setup
- **Security validation timing bug:** `nself build` now loads full environment cascade before security validation
  - Previously: Only loaded `.env` before validation, missing values from `.env.dev`, `.env.staging/prod`, `.env.secrets`
  - Now: Loads complete cascade (.env.dev → .env.staging/prod → .env.secrets → .env) before validation
  - Impact: Security validation now correctly detects secure passwords from environment files
  - Fix: Users no longer need `--allow-insecure` flag when passwords are properly configured

### Performance

- **Time to Working Auth:** Reduced from ~4 hours to **<5 minutes**
  - Before: Manual Hasura config, SQL debugging, trial and error
  - After: One command (`nself auth setup --default-users`)
- **Steps Required:** Reduced from ~10+ manual steps to **4 commands**
  ```bash
  nself init --demo
  nself build
  nself start
  nself auth setup --default-users
  ```

### Developer Experience

- **Zero-configuration auth** - Works out of the box
- **Clear error messages** - Actionable suggestions provided
- **Comprehensive guides** - Three detailed documentation guides
- **Better command organization** - Logical grouping of related commands
- **Idempotent operations** - Safe to run commands multiple times

---

## [0.9.8] - 2026-02-10

### Production Readiness & Help Contract Release

**Type:** Quality & Portability Release
**Status:** Production Ready ✅

#### Added
- **Bash 3.2 Compatibility** - Complete removal of Bash 4+ dependencies
  - Removed ALL `declare -A` (associative arrays) from 4 critical files
  - Removed ALL `${var,,}` / `${var^^}` (parameter expansion)
  - Converted to Bash 3.2 compatible patterns (case statements, parallel arrays, `tr` command)
  - Works on macOS Bash 3.2, all Linux distros, WSL environments

- **Help Contract Implementation** - Universal help bypass pattern across 31 commands
  - `--help` or `-h` exits with code 0 (success)
  - Help executes BEFORE environment/Docker checks
  - No side effects (no Docker operations, no .env requirements)
  - Consistent output schema across all commands
  - Documented in `src/lib/help/HELP-CONTRACT.md`

- **Canonical Test Entrypoint** - `src/tests/run-tests.sh`
  - Official entrypoint for running nself tests
  - Delegates to `run-all-tests.sh` with all arguments
  - Provides help output with `-h` or `--help`
  - Works across all platforms

- **Monorepo Support** - `monorepo_check()` function
  - Detects monorepo structure
  - Provides appropriate warnings
  - Handles build path resolution

- **Frontend Directory Support** - FRONTEND_DIR environment variable
  - Build system aware of custom frontend paths
  - Proper routing configuration
  - Works with monorepo structures

#### Fixed
- **P0-001**: Fixed `reset --help` runtime regression (help bypass pattern)
- **P0-002**: Fixed `checklist --help` SCRIPT_DIR global clobber
- **P0-005**: Fixed `help --help` exit code (now exits 0)
- **P0-006**: Fixed `whitelabel --help` readonly constant collisions
- **P0-008**: Fixed `run-all-tests.sh` false-green behavior (5 bugs)
- **P0-009**: Rewrote v1 command structure test for 31 commands
- **P0-011**: Added installer integrity verification (SHA-256 checksum)
- **P0-012**: Fixed feedback ingest marker corruption
- Security check diagnostic output now goes to stderr (CI fix)

#### Improved
- **CI/CD Fail-Closed** - Critical checks now fail CI on errors
  - `nself build` must succeed (was `|| true`)
  - `nself doctor` must exit gracefully (was `|| true`)
  - Trivy scans fail on HIGH/CRITICAL vulnerabilities
  - Coverage collection tracked with success counter
  - Legitimate fail-open preserved for external services

- **Cross-Platform CLI Output** - All commands use standardized output
  - Consistent color coding (green/red/yellow)
  - Proper icons (✓, ✗, ⚠, ℹ)
  - Platform-compatible (no emoji by default)
  - All output uses `printf`, not `echo -e`

- **Developer Experience Enhancements**
  - All help output exits 0
  - No environment requirements for --help
  - Faster feedback loops
  - Better error messages

#### Security
- **P0-003**: Credential remediation (8 credentials sanitized)
- **P0-004**: Wiki private-path cleaned
- Zero credentials in Git

#### Verified
- ✅ **All 15/15 verification checks passing** (100%)
- ✅ **209 service templates** across 17 languages
- ✅ **Plugin system** (Stripe, Shopify, GitHub all working)
- ✅ **Multi-app support** (FRONTEND_APP_1-10 functional)
- ✅ **Complete infrastructure** (PostgreSQL, Hasura, Auth, Nginx, etc.)

#### Distribution
- Published to **5 platforms**: GitHub Release, Homebrew, npm, Docker Hub, AUR
- Multi-arch Docker image (linux/amd64, linux/arm64)
- Comprehensive 13,000+ word release notes
- Migration guide included

For full details, see: [v0.9.8 Release Notes](v0.9.8.md)

---

## [0.9.7] - 2026-02-10

### Security & CI/CD Complete

- Complete security audit and remediation
- CI/CD workflow hardening
- Documentation consolidation
- Production deployment readiness

For details, see: [v0.9.7 Release Notes](v0.9.7.md)

---

## [0.9.6] - 2026-01-31

### Command Consolidation & Refactoring

- Consolidated 79 commands → 31 top-level commands
- Command tree v1.0 structure
- Improved CLI organization and discoverability

For details, see: [v0.9.6 Release Notes](v0.9.6.md)

---

## [0.9.5] - 2026-01-30

### White-Label System Complete

- Full white-label customization system
- Theme support
- Email template customization
- Branding API

For details, see: [v0.9.5 Release Notes](v0.9.5.md)

---

## [0.9.0] - 2026-01-29

### Major Release - Plugin System & Custom Services

- Plugin system architecture
- 209 service templates across 17 languages
- Official plugins (Stripe, Shopify, GitHub)
- Custom service scaffolding (CS_1 through CS_10)

For details, see: [v0.9.0 Release Notes](v0.9.0.md)

---

## [0.8.0] - 2026-01-29

### Added - Phase 3: Multi-Tenancy & Enterprise (100%)

Complete multi-tenancy implementation with tenant isolation, resource quotas, billing integration, and enterprise collaboration features. All 8 sprints completed with 280 points.

#### Sprint 21: Multi-Tenancy Foundation (40 points) ✓
**Tenant Management:**
- Tenant CRUD operations with schema isolation
- Automatic schema creation per tenant (tenant_*)
- Tenant metadata and settings
- Tenant status management (active/suspended/trial)
- Tenant provisioning and cleanup
- Complete tenant CLI interface

**Database Migrations:**
- `001_tenant_foundation.sql` - Tenant management tables and schemas
- Schema-based tenant isolation
- Per-tenant user management
- Tenant-specific configuration

**CLI Commands:**
- `nself tenant create` - Create new tenant
- `nself tenant list` - List all tenants
- `nself tenant get` - Get tenant details
- `nself tenant update` - Update tenant settings
- `nself tenant delete` - Delete tenant (soft/hard)
- `nself tenant status` - Change tenant status

**Documentation:**
- Multi-tenancy architecture guide
- Tenant isolation patterns
- Schema design and best practices

#### Sprint 22: Tenant Isolation (40 points) ✓
**Data Isolation:**
- Row-level security (RLS) policies
- Tenant context propagation via JWT claims
- Cross-tenant access prevention
- Secure tenant data boundaries

**Resource Isolation:**
- Per-tenant resource limits
- Storage quota enforcement
- API rate limiting per tenant
- Database connection pooling per tenant

**Database Migrations:**
- `002_tenant_isolation.sql` - RLS policies and security
- Tenant context functions
- Cross-tenant protection

**Security Features:**
- Tenant ID validation middleware
- Context switching protection
- Audit logging per tenant
- Tenant data encryption

**Documentation:**
- Tenant isolation security model
- RLS implementation guide
- Multi-tenant security best practices

#### Sprint 23: Tenant Users & Teams (35 points) ✓
**Tenant User Management:**
- User-tenant associations
- Per-tenant user roles
- User invitation system
- Tenant user permissions
- User transfer between tenants

**Team Management:**
- Team CRUD operations
- Team-based access control
- Team roles and permissions
- Team member management
- Team ownership and delegation

**Database Migrations:**
- `003_tenant_teams.sql` - Team and membership tables
- User-tenant associations
- Team-based RBAC

**CLI Commands:**
- `nself tenant users` - Manage tenant users
- `nself tenant teams` - Team management
- `nself tenant invite` - User invitations
- `nself tenant roles` - Tenant-specific roles

**Documentation:**
- Multi-tenant user management
- Team collaboration patterns
- Invitation workflows

#### Sprint 24: Resource Quotas & Billing (45 points) ✓
**Resource Quotas:**
- Configurable tenant quotas
- Real-time usage tracking
- Quota enforcement
- Quota alerts and notifications
- Usage analytics per tenant

**Billing Integration:**
- Stripe integration for subscriptions
- Plan management (free/basic/pro/enterprise)
- Usage-based billing
- Invoice generation
- Payment method management
- Billing history and reports

**Database Migrations:**
- `004_tenant_quotas_billing.sql` - Quotas and billing tables
- Usage tracking tables
- Subscription management
- Invoice storage

**Quota Types:**
- Storage limits
- User count limits
- API request limits
- Custom service limits
- Bandwidth limits

**CLI Commands:**
- `nself tenant quota` - Manage tenant quotas
- `nself tenant usage` - View usage statistics
- `nself tenant billing` - Billing management
- `nself tenant plans` - Subscription plans
- `nself tenant invoice` - Invoice operations

**Documentation:**
- Resource quota configuration
- Billing integration guide
- Stripe setup and testing
- Usage tracking implementation

#### Sprint 25: Tenant Analytics & Monitoring (35 points) ✓
**Analytics System:**
- Per-tenant analytics tracking
- User activity metrics
- Resource usage analytics
- Custom event tracking
- Analytics data retention policies

**Monitoring Dashboards:**
- Tenant health monitoring
- Real-time metrics per tenant
- Performance monitoring
- Error tracking
- Alert system

**Database Migrations:**
- `005_tenant_analytics.sql` - Analytics tables
- Event tracking schema
- Metrics storage

**Metrics Tracked:**
- User login/logout events
- API usage patterns
- Resource consumption
- Feature adoption
- Error rates

**CLI Commands:**
- `nself tenant analytics` - View analytics
- `nself tenant metrics` - Access metrics
- `nself tenant events` - Event tracking
- `nself tenant health` - Health checks

**Prometheus Integration:**
- Per-tenant metrics export
- Custom metric definitions
- Grafana dashboard templates

**Documentation:**
- Analytics implementation guide
- Monitoring setup
- Custom metrics creation

#### Sprint 26: Tenant Backup & Restore (30 points) ✓
**Backup System:**
- Automated tenant backups
- Point-in-time recovery
- Backup scheduling (hourly/daily/weekly)
- Backup retention policies
- Cross-region backup storage

**Restore Operations:**
- Full tenant restore
- Selective data restore
- Restore to new tenant
- Restore validation
- Rollback capabilities

**Database Migrations:**
- `006_tenant_backups.sql` - Backup metadata tables
- Restore tracking
- Backup verification

**Backup Types:**
- Full database backup
- Schema-only backup
- Data-only backup
- Incremental backups

**CLI Commands:**
- `nself tenant backup create` - Create backup
- `nself tenant backup list` - List backups
- `nself tenant backup restore` - Restore from backup
- `nself tenant backup schedule` - Configure auto-backup
- `nself tenant backup verify` - Verify backup integrity

**Storage Options:**
- Local filesystem
- S3-compatible storage (MinIO)
- Azure Blob Storage
- Google Cloud Storage

**Documentation:**
- Backup and restore guide
- Disaster recovery procedures
- Backup best practices

#### Sprint 27: Tenant Migration Tools (35 points) ✓
**Migration System:**
- Tenant data export
- Tenant data import
- Cross-environment migration (dev→staging→prod)
- Tenant cloning
- Data transformation pipelines

**Export Formats:**
- JSON export
- CSV export
- SQL dump
- Custom format support

**Import Features:**
- Schema validation
- Data mapping
- Conflict resolution
- Dry-run mode
- Import rollback

**Database Migrations:**
- `007_tenant_migrations.sql` - Migration tracking tables
- Migration history
- Migration validation

**CLI Commands:**
- `nself tenant export` - Export tenant data
- `nself tenant import` - Import tenant data
- `nself tenant clone` - Clone tenant
- `nself tenant migrate` - Cross-env migration
- `nself tenant validate` - Validate migration data

**Migration Features:**
- Zero-downtime migrations
- Data consistency checks
- Foreign key preservation
- Index rebuilding
- Migration progress tracking

**Documentation:**
- Migration guide
- Data transformation examples
- Troubleshooting migrations

#### Sprint 28: Enterprise Collaboration (40 points) ✓
**Collaboration Features:**
- Real-time notifications
- Activity feeds per tenant
- Comment system
- @mentions and tagging
- Notification preferences

**Workspace Management:**
- Multiple workspaces per tenant
- Workspace permissions
- Cross-workspace collaboration
- Workspace templates

**Communication:**
- In-app messaging
- Notification channels (email, SMS, webhook)
- Activity streams
- Read receipts
- Notification batching

**Database Migrations:**
- `008_tenant_collaboration.sql` - Collaboration tables
- Notification system
- Workspace management

**Features:**
- User presence indicators
- Typing indicators
- Activity timestamps
- Notification badges
- Email digests

**CLI Commands:**
- `nself tenant notify` - Send notifications
- `nself tenant activity` - View activity feed
- `nself tenant workspace` - Workspace management
- `nself tenant messages` - Messaging operations

**Integration:**
- Slack notifications
- Discord webhooks
- Email templates
- SMS via Twilio
- Push notifications

**Documentation:**
- Collaboration features guide
- Notification system setup
- Workspace management best practices

### Technical Improvements
- Complete schema isolation per tenant
- Row-level security enforcement
- Per-tenant connection pooling
- Tenant context propagation
- Resource quota enforcement
- Real-time analytics processing
- Automated backup system
- Zero-downtime migrations
- Enterprise-grade collaboration

### Security
- Tenant isolation via RLS policies
- Cross-tenant access prevention
- Encrypted tenant data at rest
- Per-tenant audit logging
- Secure tenant provisioning
- Tenant data deletion compliance (GDPR)
- API key scoping per tenant

### Database Schema
**tenants schema:**
- tenants (core tenant management)
- tenant_users (user-tenant associations)
- tenant_teams (team management)
- tenant_invitations (user invitations)
- tenant_quotas (resource limits)
- tenant_usage (usage tracking)
- tenant_subscriptions (billing)
- tenant_invoices (invoice history)
- tenant_analytics_events (event tracking)
- tenant_metrics (metrics storage)
- tenant_backups (backup metadata)
- tenant_migrations (migration tracking)
- tenant_notifications (notification system)
- tenant_workspaces (workspace management)
- tenant_activity (activity feeds)

### CLI Commands Added
- `nself tenant` - Complete tenant management suite
- `nself tenant users` - Tenant user management
- `nself tenant teams` - Team collaboration
- `nself tenant quota` - Resource quotas
- `nself tenant billing` - Billing operations
- `nself tenant analytics` - Analytics and metrics
- `nself tenant backup` - Backup and restore
- `nself tenant migrate` - Migration tools
- `nself tenant notify` - Notification system
- `nself tenant workspace` - Workspace management

### Performance
- Optimized per-tenant queries
- Connection pooling per tenant
- Analytics data aggregation
- Efficient backup strategies
- Indexed tenant lookups
- Cached quota checks
- Async notification delivery

### Documentation
- Multi-tenancy architecture guide: `/docs/architecture/multi-tenancy.md`
- Tenant isolation security model: `/docs/security/tenant-isolation.md`
- Resource quota configuration: `/docs/configuration/quotas.md`
- Billing integration guide: `/docs/integrations/billing.md`
- Backup and restore procedures: `/docs/operations/backup-restore.md`
- Migration guide: `/docs/operations/migrations.md`
- Collaboration features: `/docs/features/collaboration.md`

### Statistics - Phase 3
- **Total Points Completed:** 280/280 (100%)
- **Database Migrations:** 8 migrations
- **New CLI Commands:** 10+ command groups
- **Database Tables Added:** 15+ tables
- **Lines of Code:** ~8,000 lines
- **Git Commits:** 40+ commits

## [0.7.0] - 2026-01-29

### Added - Phase 2: Advanced Backend Features (100%)

Complete advanced backend implementation with real-time collaboration, performance optimization, developer tools, migration utilities, and enhanced security. All sprints completed with 270 points.

#### Sprint 16: Real-Time Collaboration (70 points) ✓
- WebSocket server implementation
- Real-time presence tracking
- Live cursor sharing
- Collaborative editing
- Pub/sub messaging
- Room management
- Complete websocket CLI

#### Sprint 17: Security Hardening (25 points) ✓
- Security audit checklist
- Firewall configuration
- SSL/TLS automation with Let's Encrypt
- Secrets scanning
- Vulnerability management
- Security headers configuration
- Compliance reporting

#### Sprint 18: Performance & Optimization (45 points) ✓
- Query optimization tools
- Database indexing strategies
- Caching layer (Redis)
- CDN integration
- Load testing framework
- Performance monitoring
- Bottleneck analysis

#### Sprint 19: Developer Tools (30 points) ✓
- API documentation generator
- Schema introspection
- GraphQL playground integration
- Development environment setup
- Debugging utilities
- Testing frameworks

#### Sprint 20: Migration Tools (40 points) ✓
- Database migration system
- Schema versioning
- Data transformation utilities
- Rollback capabilities
- Migration validation
- Import/export tools

### Database Migrations
- Real-time collaboration schema
- Security audit tables
- Performance metrics tables
- Migration tracking system

### CLI Commands Added
- `nself websocket` - WebSocket management
- `nself security` - Security operations
- `nself performance` - Performance tools
- `nself dev` - Developer utilities
- `nself migrate` - Migration management

### Documentation
- Real-time collaboration guide
- Security hardening checklist
- Performance optimization guide
- Developer tools documentation
- Migration system guide

## [0.6.0] - 2026-01-29

### Added - Phase 1: Enterprise Authentication & Security (91.5%)

#### Sprint 1: Core Authentication (100%) ✓
- Password authentication with bcrypt hashing
- Email/password signup and login flows
- Password reset with token expiration
- Email verification system
- Account linking (multiple auth methods per user)
- Secure session management
- CLI commands: signup, login, verify, reset

#### Sprint 2: OAuth & MFA (100%) ✓
**OAuth Providers (14 total):**
- Google OAuth 2.0
- GitHub OAuth 2.0
- Facebook OAuth 2.0
- Discord OAuth 2.0
- Microsoft Azure AD OAuth 2.0
- LinkedIn OAuth 2.0
- Slack OAuth v2
- Twitch OAuth 2.0
- Custom OIDC provider with auto-discovery
- Apple Sign In with JWT client secret
- Twitter/X OAuth 2.0 with PKCE
- GitLab OAuth 2.0 (supports self-hosted instances)
- Bitbucket OAuth 2.0

**MFA Methods:**
- TOTP (Time-based One-Time Password) with QR code generation
- SMS MFA (Twilio, AWS SNS, dev mode)
- Email MFA with customizable templates
- Backup codes (10 one-time recovery codes)
- MFA policies (global, role-based, user exemptions)
- WebAuthn/FIDO2 support (YubiKey, TouchID, Windows Hello)
- Complete MFA CLI interface

**User Management:**
- User CRUD operations with soft delete
- User profiles (avatar, bio, custom fields)
- User import/export (JSON, CSV formats)
- User metadata with versioning and history
- User search and filtering

#### Sprint 3: RBAC & Auth Hooks (81.5%)
**Role-Based Access Control:**
- Role CRUD operations (create, update, delete)
- System vs custom roles
- Default role management
- User-role assignments and revocation
- Comprehensive role CLI with permissions management

**Permission Management:**
- Fine-grained permissions (resource:action format)
- Role-permission associations
- User permission aggregation from multiple roles
- Permission checking and validation
- Permission inheritance

**Auth Hooks System:**
- Pre/post signup hooks
- Pre/post login hooks
- Custom claims generation hooks
- Pre/post MFA hooks
- Priority-based hook execution
- Hook logging and audit trail
- Pluggable architecture for custom logic

**JWT Management:**
- JWT configuration (algorithm, TTL, issuer)
- RS256 key pair generation with OpenSSL
- Automatic key rotation (configurable interval)
- Multiple keys support for gradual rotation
- Key storage in PostgreSQL

**Session Management:**
- Session lifecycle management
- Refresh token rotation for enhanced security
- Session revocation (single, all, all-except-current)
- Last activity tracking
- Automatic cleanup of expired sessions
- Session listing per user

**Custom Claims:**
- Generate custom JWT claims from user roles/permissions
- Hasura-compatible JWT claims format
- Claims caching (5-minute TTL for performance)
- Claims validation and refresh

#### Sprint 4: API Keys & Secrets Vault (100%) ✓
**API Key Management:**
- Secure API key generation with prefix support
- SHA-256 hashing for key storage
- Scope-based permissions (resource:action format)
- Key expiration and automatic rotation
- Usage tracking (request count + last used timestamp)
- Keys shown only once on creation for security
- Key revocation and management

**Encrypted Secrets Vault:**
- AES-256-CBC encryption with OpenSSL
- Encryption key generation and rotation (90-day default)
- Encrypted secret storage in PostgreSQL
- Secret versioning with full history
- Rollback to previous versions
- Comprehensive audit trail for compliance
- Environment separation (default/dev/staging/prod)
- Secret sync and promotion workflows (dev→staging→prod)
- Suspicious activity detection
- Secrets comparison across environments
- Complete vault CLI interface

#### Sprint 5: Rate Limiting & Throttling (72.6%)
**Rate Limiting Algorithms:**
- Token bucket (allows bursts, flexible)
- Leaky bucket (smooth rate, no bursts)
- Fixed window (simple, fast)
- Sliding window (accurate, fair)
- Sliding log (most accurate, storage intensive)
- Adaptive rate limiting (adjusts based on success rate)
- Burst protection (detects and blocks traffic spikes)

**Rate Limiting Types:**
- IP-based rate limiting with whitelist/blocklist
- User-based rate limiting with tier support
- Endpoint-based rate limiting with regex rules engine
- Combined limiting (IP+endpoint, user+endpoint)
- Method-based limits (GET/POST/PUT/DELETE)

**Rate Limit Management:**
- IP whitelist and blocklist
- Rule-based endpoint rate limits with priority
- User quota management
- Tier-based limits (free/basic/pro/enterprise/unlimited)
- Rate limit statistics and monitoring
- Comprehensive audit logging
- Rate limit headers (X-RateLimit-*)
- Complete rate-limit CLI interface

### Added - Phase 2: Advanced Features

#### Webhook System
- Webhook endpoint management (create, list, delete)
- Event subscriptions (11 core events)
- HMAC signature verification (sha256)
- Async webhook delivery with retries (3 attempts, 60s delay)
- Delivery status tracking
- Custom headers support
- Complete webhooks CLI

**Webhook Events:**
- user.created, user.updated, user.deleted
- user.login, user.logout
- session.created, session.revoked
- mfa.enabled, mfa.disabled
- role.assigned, role.revoked

#### Device Management
- Device registration and tracking
- Device fingerprinting
- OS, browser, IP detection
- Trusted device management (skip MFA on trusted devices)
- Last seen tracking
- Device revocation
- Device CLI
- Multi-device session support

#### Audit Logging
- Comprehensive event tracking for all auth actions
- Actor, resource, and action tracking
- Result tracking (success/failure)
- Metadata support (JSON)
- IP address and user agent tracking
- Queryable audit trail with filters
- Audit CLI for investigation
- Compliance ready (SOC 2, ISO 27001, GDPR)

### Technical Improvements
- Cross-platform compatibility (Bash 3.2+, macOS/Linux)
- PostgreSQL-backed with proper schema design
- Modular architecture with exported functions
- Comprehensive CLI tooling (10+ commands)
- Security-first approach (bcrypt, SHA-256, AES-256-CBC)
- Docker-based deployment
- Clean error handling and validation
- Extensive documentation

### Security
- OWASP Top 10 mitigations
- CSRF protection
- SQL injection prevention (parameterized queries)
- XSS mitigation
- Secure password storage (bcrypt with salt)
- Encrypted secrets at rest (AES-256-CBC)
- Rate limiting against brute force and DoS
- Audit logging for compliance and forensics
- Refresh token rotation
- JWT key rotation
- Session security best practices

### Database Schema
**auth schema:**
- users (with soft delete)
- sessions
- mfa_secrets, mfa_codes
- roles, permissions, user_roles, role_permissions
- oauth_accounts, email_verifications
- jwt_keys, jwt_config
- password_reset_tokens
- user_metadata, user_metadata_history
- webauthn_credentials, webauthn_challenges
- devices

**secrets schema:**
- encryption_keys
- vault, vault_versions
- audit_log

**rate_limit schema:**
- buckets
- rules
- log
- whitelist, blocklist
- user_quotas

**webhooks schema:**
- endpoints
- deliveries

**audit schema:**
- events

### CLI Commands
- `nself auth` - Authentication management
- `nself mfa` - Multi-factor authentication
- `nself roles` - Role and permission management
- `nself vault` - Encrypted secrets management
- `nself rate-limit` - Rate limiting configuration
- `nself webhooks` - Webhook management
- `nself devices` - Device management
- `nself audit` - Audit log queries
- `nself secrets` - Production secrets generation (separate from vault)

### Performance
- Token bucket algorithm O(1) time complexity
- PostgreSQL indexing on all critical queries
- Claims caching for JWT generation
- Async webhook delivery (non-blocking)
- Connection pooling ready

### Documentation
- Comprehensive README
- API documentation
- CLI usage guides
- Security best practices
- Deployment guides
- Integration examples
- Phase 1 progress summary

## Statistics

- **Total Files Created:** 60+ files
- **Lines of Code:** ~14,000 lines
- **CLI Commands:** 10 commands
- **OAuth Providers:** 14 providers
- **MFA Methods:** 6 methods
- **Rate Limit Strategies:** 7 algorithms
- **Webhook Events:** 11 events
- **Database Tables:** 35+ tables
- **Git Commits:** 120+ commits

## Completion Status

### Phase 1: 91.5% (269/294 points)
- Sprint 1: 100% ✓
- Sprint 2: 100% ✓
- Sprint 3: 81.5%
- Sprint 4: 100% ✓
- Sprint 5: 72.6%

### Phase 2: Started
- Webhook System ✓
- Device Management ✓
- Audit Logging ✓

## Next Release (v0.7.0)

### Planned
- Complete Sprint 3 tests
- Distributed rate limiting with Redis
- Admin dashboard API
- Email templates system
- Analytics and metrics
- More documentation
- Integration guides

### Under Consideration
- SAML 2.0 support
- LDAP/Active Directory integration
- Advanced monitoring integrations
- Developer SDKs (JS, Python, Go)
- Mobile app support
- Multi-tenancy

## Contributing

See [CONTRIBUTING.md](../contributing/CONTRIBUTING.md) for development guidelines.

## Security

See [SECURITY.md](../guides/SECURITY.md) for security policy and reporting vulnerabilities.

## License

See [LICENSE](../LICENSE.md) for license information.

---

**Note:** This project is under active development. Features and APIs may change before v1.0.0 stable release.
