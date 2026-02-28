# Organization Management Guide

**nself v0.8.0** - Complete guide to organizations, teams, and RBAC

> **Note:** As of v0.9.6, organization commands have been consolidated under `nself tenant org`. Throughout this guide, `nself org` refers to `nself tenant org` in the new command structure.

## Table of Contents

- [Overview](#overview)
- [Core Concepts](#core-concepts)
- [Organization Structure](#organization-structure)
- [Team Management](#team-management)
- [CLI Usage](#cli-usage)
- [Permission System](#permission-system)
- [Access Control Patterns](#access-control-patterns)
- [Integration with Other Systems](#integration-with-other-systems)
- [Best Practices](#best-practices)
- [Advanced Topics](#advanced-topics)

---

## Overview

Organizations in nself provide a complete workspace management and access control system. They enable you to:

- Structure teams hierarchically within your company or project
- Manage member access and roles at multiple levels
- Control resource permissions with fine-grained RBAC
- Span multiple tenants under a single organizational umbrella
- Implement enterprise-grade access control patterns

### What Are Organizations?

**Organizations** are the top-level entity in nself's workspace hierarchy. Think of them as:
- A company or business unit
- A project workspace
- A client account
- Any grouping of users and resources that need centralized management

### Relationship to Tenants

Organizations and tenants have a **many-to-many relationship**:

```
Organization "Acme Corp"
├── Tenant: Production Database
├── Tenant: Staging Environment
└── Tenant: Development Sandbox

Organization "Project Phoenix"
├── Tenant: Customer Portal
└── Tenant: Admin Dashboard
```

**Key difference**:
- **Tenants** = Data isolation (separate database schemas)
- **Organizations** = Team structure and access control

### Use Cases

**Corporate Structure**
```
Organization: Acme Corporation
├── Team: Engineering
├── Team: Sales
└── Team: Support
    └── Members with different roles and permissions
```

**Project Management**
```
Organization: Project Alpha
├── Team: Backend Developers
├── Team: Frontend Developers
└── Team: QA Team
    └── Each team has access to specific resources
```

**Access Control**
```
Organization: SaaS Platform
├── Owner: Full control
├── Admins: Manage members and settings
├── Members: Standard access
└── Guests: Read-only access
```

---

## Core Concepts

### 1. Organizations

**Definition**: Top-level workspace containing teams, members, and resources.

**Properties**:
- `id` - Unique UUID identifier
- `slug` - Human-readable URL identifier (e.g., "acme-corp")
- `name` - Display name (e.g., "Acme Corporation")
- `status` - `active`, `suspended`, or `deleted`
- `billing_plan` - `free`, `pro`, `enterprise`, etc.
- `owner_user_id` - User who owns the organization
- `settings` - JSONB metadata for custom configuration
- `metadata` - Additional JSONB data

**Lifecycle**:
1. **Created** - Organization is initialized with an owner
2. **Active** - Normal operation with members and teams
3. **Suspended** - Temporarily disabled (e.g., billing issues)
4. **Deleted** - Soft-deleted, can be recovered

### 2. Teams

**Definition**: Sub-groups within an organization for logical grouping of members.

**Properties**:
- `id` - Unique UUID identifier
- `org_id` - Parent organization
- `name` - Team display name (e.g., "Engineering")
- `slug` - URL-safe identifier (e.g., "engineering")
- `description` - Optional team description
- `settings` - JSONB configuration

**Team Roles**:
- **Lead** - Team manager with full team control
- **Member** - Standard team member

### 3. Members

**Definition**: Users who belong to organizations and teams.

**Organization Membership**:
- `user_id` - Reference to auth.users
- `org_id` - Organization they belong to
- `role` - Organization-level role
- `joined_at` - Timestamp of joining
- `invited_by` - User who invited them

**Team Membership**:
- `user_id` - Reference to auth.users
- `team_id` - Team they belong to
- `role` - Team-level role (lead or member)
- `added_by` - User who added them to team

### 4. Roles

**Definition**: Named collections of permissions that can be assigned to users.

**Built-in Organization Roles**:
- **Owner** - Created the org, full control, cannot be removed
- **Admin** - Nearly full control, can manage members and settings
- **Member** - Standard access, can use resources
- **Guest** - Limited read-only access

**Custom Roles**:
- Created per-organization
- Composed of granular permissions
- Can be assigned to users with scopes

### 5. Permissions

**Definition**: Granular access controls in `resource:action` format.

**Permission Format**:
```
{resource_type}.{action}
```

**Examples**:
- `tenant.create` - Can create new tenants
- `user.delete` - Can delete users
- `team.manage` - Full team management
- `org.billing` - Manage billing settings

### 6. Permission Scopes

Permissions can be granted at different levels:

- **Global** - Applies to all resources in the organization
- **Tenant** - Scoped to a specific tenant
- **Team** - Scoped to a specific team

**Example**:
```sql
-- Global permission: can manage ALL teams
scope = 'global', scope_id = NULL

-- Team-specific permission: can only manage Engineering team
scope = 'team', scope_id = 'engineering-team-uuid'
```

---

## Organization Structure

### Creating an Organization

```bash
# Create with auto-generated slug
nself tenant org create "Acme Corporation"

# Create with custom slug
nself tenant org create "Acme Corporation" --slug acme

# Create with specific owner
nself tenant org create "Acme Corp" --slug acme --owner <user_uuid>
```

**What happens**:
1. Organization record created
2. Owner automatically added as member with `owner` role
3. Slug generated (or uses your custom slug)
4. Default organization created on first run

### Organization Settings

Organizations include flexible settings stored as JSONB:

```sql
-- Example settings
{
  "branding": {
    "logo_url": "https://...",
    "primary_color": "#007bff"
  },
  "features": {
    "sso_enabled": true,
    "audit_logs_retention": 90
  },
  "notifications": {
    "email": "admin@acme.com",
    "slack_webhook": "https://..."
  }
}
```

### Listing Organizations

```bash
# Table format
nself tenant org list

# JSON output
nself tenant org list --json
```

**Output**:
```
 id                                   | slug        | name              | status | billing_plan | created_at
--------------------------------------+-------------+-------------------+--------+--------------+---------------------------
 a1b2c3d4-...                         | acme        | Acme Corporation  | active | free         | 2026-01-15 10:30:00+00
 e5f6g7h8-...                         | project-x   | Project X         | active | pro          | 2026-01-20 14:00:00+00
```

### Viewing Organization Details

```bash
# By ID or slug
nself tenant org show acme
nself tenant org show a1b2c3d4-e5f6-...
```

**Output includes**:
- Full organization details
- Member count
- Team count
- Billing information
- Creation and update timestamps

### Deleting an Organization

```bash
nself tenant org delete acme
```

**Confirmation required**:
```
Are you sure you want to delete organization 'acme'? (yes/no): yes
```

**Cascade deletion**:
- All teams deleted
- All memberships removed
- All roles and permissions removed
- Tenant relationships preserved (tenants remain)

---

## Team Management

### Creating Teams

```bash
# Create team in organization
nself tenant org team create acme "Engineering"
nself tenant org team create acme "Sales Team"
nself tenant org team create project-x "Backend Developers"
```

**Auto-generated slug**:
- "Engineering" → `engineering`
- "Sales Team" → `sales-team`
- "Backend Developers" → `backend-developers`

### Listing Teams

```bash
# List all teams in organization
nself tenant org team list acme
```

**Output**:
```
 id                                   | slug                | name                | member_count | created_at
--------------------------------------+---------------------+---------------------+--------------+---------------------------
 1a2b3c4d-...                         | engineering         | Engineering         | 12           | 2026-01-15 11:00:00+00
 5e6f7g8h-...                         | sales-team          | Sales Team          | 8            | 2026-01-16 09:30:00+00
```

### Team Details

```bash
# Show team information
nself tenant org team show engineering
nself tenant org team show 1a2b3c4d-...
```

### Managing Team Members

```bash
# Add user to team as member (default)
nself tenant org team add engineering user-uuid-123

# Add user as team lead
nself tenant org team add engineering user-uuid-456 lead

# Remove user from team
nself tenant org team remove engineering user-uuid-123
```

### Deleting Teams

```bash
nself tenant org team delete engineering
```

**Note**: Team members are removed, but users remain in the organization.

---

## CLI Usage

### Organization Commands

#### Initialize Organization System

```bash
nself tenant org init
```

**What it does**:
1. Runs migration `010_create_organization_system.sql`
2. Creates database schemas: `organizations`, `permissions`
3. Sets up tables, functions, triggers, and RLS policies
4. Inserts default permissions
5. Creates default organization

#### Create Organization

```bash
nself tenant org create <name> [options]

Options:
  --slug <slug>       Custom slug (auto-generated if omitted)
  --owner <user_id>   Owner user ID (defaults to first user)

Examples:
  nself tenant org create "Acme Corp"
  nself tenant org create "Acme Corp" --slug acme
  nself tenant org create "Acme Corp" --slug acme --owner a1b2c3d4-...
```

#### List Organizations

```bash
nself tenant org list [--json]

Examples:
  nself tenant org list
  nself tenant org list --json | jq '.[] | {name, slug, status}'
```

#### Show Organization

```bash
nself tenant org show <id_or_slug>

Examples:
  nself tenant org show acme
  nself tenant org show a1b2c3d4-e5f6-...
```

#### Delete Organization

```bash
nself tenant org delete <id_or_slug>

Examples:
  nself tenant org delete acme
```

### Member Management Commands

#### Add Member

```bash
nself tenant org member add <org> <user_id> [role]

Roles:
  owner      - Full control (only one per org)
  admin      - Manage members and settings
  member     - Standard access (default)
  guest      - Read-only access

Examples:
  nself tenant org member add acme user-123 admin
  nself tenant org member add acme user-456          # Defaults to 'member'
```

#### Remove Member

```bash
nself tenant org member remove <org> <user_id>

Examples:
  nself tenant org member remove acme user-123
```

#### List Members

```bash
nself tenant org member list <org>

Examples:
  nself tenant org member list acme
```

**Output**:
```
 user_id                              | role   | joined_at
--------------------------------------+--------+---------------------------
 a1b2c3d4-...                         | owner  | 2026-01-15 10:30:00+00
 e5f6g7h8-...                         | admin  | 2026-01-15 11:00:00+00
 i9j0k1l2-...                         | member | 2026-01-16 09:00:00+00
```

#### Change Member Role

```bash
nself tenant org member role <org> <user_id> <new_role>

Examples:
  nself tenant org member role acme user-123 admin
  nself tenant org member role acme user-456 guest
```

### Team Commands

#### Create Team

```bash
nself tenant org team create <org> <name>

Examples:
  nself tenant org team create acme "Engineering"
  nself tenant org team create acme "Customer Success"
```

#### List Teams

```bash
nself tenant org team list <org>

Examples:
  nself tenant org team list acme
```

#### Show Team

```bash
nself tenant org team show <team_id_or_slug>

Examples:
  nself tenant org team show engineering
  nself tenant org team show 1a2b3c4d-...
```

#### Delete Team

```bash
nself tenant org team delete <team_id_or_slug>

Examples:
  nself tenant org team delete engineering
```

#### Add Team Member

```bash
nself tenant org team add <team> <user_id> [role]

Roles:
  lead       - Team manager
  member     - Standard team member (default)

Examples:
  nself tenant org team add engineering user-123 lead
  nself tenant org team add engineering user-456       # Defaults to 'member'
```

#### Remove Team Member

```bash
nself tenant org team remove <team> <user_id>

Examples:
  nself tenant org team remove engineering user-123
```

### Role & Permission Commands

#### Create Custom Role

```bash
nself tenant org role create <org> <role_name>

Examples:
  nself tenant org role create acme "Developer"
  nself tenant org role create acme "Support Agent"
```

#### List Roles

```bash
nself tenant org role list <org>

Examples:
  nself tenant org role list acme
```

**Output**:
```
 id                                   | name           | description | is_builtin | permission_count | created_at
--------------------------------------+----------------+-------------+------------+------------------+---------------------------
 a1b2c3d4-...                         | Developer      | NULL        | f          | 5                | 2026-01-15 12:00:00+00
 e5f6g7h8-...                         | Support Agent  | NULL        | f          | 3                | 2026-01-16 10:00:00+00
```

#### Assign Role to User

```bash
nself tenant org role assign <org> <user_id> <role_name>

Examples:
  nself tenant org role assign acme user-123 Developer
  nself tenant org role assign acme user-456 "Support Agent"
```

#### Revoke Role from User

```bash
nself tenant org role revoke <org> <user_id> <role_name>

Examples:
  nself tenant org role revoke acme user-123 Developer
```

#### Grant Permission to Role

```bash
nself tenant org permission grant <role_name> <permission>

Examples:
  nself tenant org permission grant Developer tenant.create
  nself tenant org permission grant "Support Agent" user.read
```

#### Revoke Permission from Role

```bash
nself tenant org permission revoke <role_name> <permission>

Examples:
  nself tenant org permission revoke Developer tenant.delete
```

#### List All Permissions

```bash
nself tenant org permission list
```

**Output**:
```
 name             | resource_type | action | description
------------------+---------------+--------+--------------------------------
 tenant.create    | tenant        | create | Create new tenants
 tenant.read      | tenant        | read   | View tenant details
 tenant.update    | tenant        | update | Update tenant settings
 tenant.delete    | tenant        | delete | Delete tenants
 tenant.manage    | tenant        | manage | Full tenant management
 user.create      | user          | create | Create new users
 user.read        | user          | read   | View user details
 ...
```

---

## Permission System

### Permission Format

Permissions follow the pattern: `{resource_type}.{action}`

### Default Permissions

nself ships with these built-in permissions:

#### Tenant Permissions
- `tenant.create` - Create new tenants
- `tenant.read` - View tenant details
- `tenant.update` - Update tenant settings
- `tenant.delete` - Delete tenants
- `tenant.manage` - Full tenant management

#### User Permissions
- `user.create` - Create new users
- `user.read` - View user details
- `user.update` - Update user information
- `user.delete` - Delete users
- `user.manage` - Full user management

#### Team Permissions
- `team.create` - Create new teams
- `team.read` - View team details
- `team.update` - Update team settings
- `team.delete` - Delete teams
- `team.manage` - Full team management

#### Organization Permissions
- `org.billing` - Manage organization billing
- `org.settings` - Manage organization settings
- `org.members` - Manage organization members

### Built-in Roles

#### Owner Role
- **Auto-assigned**: To organization creator
- **Permissions**: All permissions (implicit)
- **Cannot be**: Removed from organization
- **Count**: One per organization

#### Admin Role
- **Typical permissions**:
  - `org.settings`
  - `org.members`
  - `team.manage`
  - `user.manage`
  - `tenant.manage`
- **Cannot**: Change billing or delete organization

#### Member Role
- **Typical permissions**:
  - `tenant.read`
  - `user.read`
  - `team.read`
- **Can**: Use resources, view information

#### Guest Role
- **Typical permissions**:
  - Very limited read access
- **Common for**: External contractors, auditors

### Custom Roles

You can create roles tailored to your needs:

```bash
# Create role
nself org role create acme "QA Engineer"

# Grant permissions
nself org permission grant "QA Engineer" tenant.read
nself org permission grant "QA Engineer" tenant.update
nself org permission grant "QA Engineer" user.read

# Assign to users
nself org role assign acme user-123 "QA Engineer"
```

### Permission Inheritance

Permissions aggregate across all user roles:

```
User has roles:
├── Developer (permissions: tenant.read, tenant.update)
└── QA Engineer (permissions: user.read, tenant.delete)

Effective permissions:
├── tenant.read
├── tenant.update
├── tenant.delete
└── user.read
```

### Permission Scopes

Control where permissions apply:

#### Global Scope
```sql
-- User can manage ALL teams in organization
scope = 'global'
scope_id = NULL
```

#### Tenant Scope
```sql
-- User can manage specific tenant only
scope = 'tenant'
scope_id = 'tenant-uuid-123'
```

#### Team Scope
```sql
-- User can manage specific team only
scope = 'team'
scope_id = 'team-uuid-456'
```

### Checking Permissions

Use the built-in PostgreSQL function:

```sql
-- Check if user has permission
SELECT permissions.has_permission(
  'user-uuid-123',           -- user_id
  'org-uuid-456',            -- org_id
  'tenant.create',           -- permission name
  'global',                  -- scope (optional, default 'global')
  NULL                       -- scope_id (optional)
);
```

### Getting All User Permissions

```sql
-- Get all permissions for user
SELECT *
FROM permissions.get_user_permissions(
  'user-uuid-123',           -- user_id
  'org-uuid-456'             -- org_id
);
```

**Returns**:
```
 permission_name | resource_type | action | scope  | scope_id
-----------------+---------------+--------+--------+--------------
 tenant.read     | tenant        | read   | global | NULL
 tenant.update   | tenant        | update | team   | team-uuid-789
 user.read       | user          | read   | global | NULL
```

---

## Access Control Patterns

### Pattern 1: Department-Based Access

**Scenario**: Different departments need different access levels.

```bash
# Create organization
nself tenant org create "Acme Corp" --slug acme

# Create teams for departments
nself tenant org team create acme "Engineering"
nself tenant org team create acme "Sales"
nself tenant org team create acme "Support"

# Create custom roles
nself tenant org role create acme "Engineer"
nself tenant org role create acme "Sales Rep"
nself tenant org role create acme "Support Agent"

# Grant permissions to Engineer role
nself tenant org permission grant Engineer tenant.create
nself tenant org permission grant Engineer tenant.update
nself tenant org permission grant Engineer tenant.delete
nself tenant org permission grant Engineer user.read

# Grant permissions to Sales Rep role
nself tenant org permission grant "Sales Rep" tenant.read
nself tenant org permission grant "Sales Rep" user.create

# Grant permissions to Support Agent role
nself tenant org permission grant "Support Agent" tenant.read
nself tenant org permission grant "Support Agent" user.read

# Add members and assign roles
nself tenant org member add acme user-eng-1 member
nself tenant org role assign acme user-eng-1 Engineer
nself tenant org team add engineering user-eng-1

nself tenant org member add acme user-sales-1 member
nself tenant org role assign acme user-sales-1 "Sales Rep"
nself tenant org team add sales user-sales-1
```

### Pattern 2: Project-Based Isolation

**Scenario**: Multiple projects under one organization with isolated access.

```bash
# Create organization
nself tenant org create "Software Agency" --slug agency

# Create teams for projects
nself tenant org team create agency "Project Alpha"
nself tenant org team create agency "Project Beta"

# Create project-specific roles
nself tenant org role create agency "Alpha Developer"
nself tenant org role create agency "Beta Developer"

# Grant scoped permissions (using SQL for scope)
-- This would be done via database or API, as CLI doesn't support scopes yet
```

### Pattern 3: Read-Only Auditors

**Scenario**: External auditors need read-only access.

```bash
# Create organization
nself tenant org create "Financial Corp" --slug fincorp

# Create auditor role
nself tenant org role create fincorp "Auditor"

# Grant read-only permissions
nself tenant org permission grant Auditor tenant.read
nself tenant org permission grant Auditor user.read
nself tenant org permission grant Auditor team.read

# Add auditor as guest
nself tenant org member add fincorp auditor-user-123 guest
nself tenant org role assign fincorp auditor-user-123 Auditor
```

### Pattern 4: Tenant-Specific Permissions

**Scenario**: Users should only access specific tenants.

```sql
-- Grant user permission to manage only production tenant
INSERT INTO permissions.user_roles (user_id, role_id, org_id, scope, scope_id)
SELECT
  'user-uuid-123',
  r.id,
  'org-uuid-456',
  'tenant',
  'prod-tenant-uuid-789'
FROM permissions.roles r
WHERE r.name = 'Developer';
```

### Pattern 5: Team Lead Permissions

**Scenario**: Team leads can manage their team but not other teams.

```bash
# Add user to team as lead
nself tenant org team add engineering user-123 lead

# Create team-specific role
nself tenant org role create acme "Team Manager"
nself tenant org permission grant "Team Manager" team.update
nself tenant org permission grant "Team Manager" user.read
nself tenant org role assign acme user-123 "Team Manager"

# In database, scope to their team
-- UPDATE permissions.user_roles
-- SET scope = 'team', scope_id = 'engineering-team-uuid'
-- WHERE user_id = 'user-123' AND role_id = (SELECT id FROM permissions.roles WHERE name = 'Team Manager');
```

---

## Integration with Other Systems

### Organizations + Tenants

Organizations can span multiple tenants for logical separation:

```
Organization: Acme Corp
├── Tenant: acme_prod (production data)
├── Tenant: acme_staging (staging data)
└── Tenant: acme_dev (development data)

All tenants accessible to Acme Corp members based on their roles/permissions
```

**Linking organizations to tenants**:
```sql
-- Link tenant to organization
INSERT INTO organizations.org_tenants (org_id, tenant_id)
VALUES (
  (SELECT id FROM organizations.organizations WHERE slug = 'acme'),
  (SELECT id FROM tenants.tenants WHERE tenant_key = 'acme_prod')
);
```

**Use case**:
- Development team has access to all three tenants
- QA team only has staging and dev tenants
- Executives only see production tenant data

### Organizations + Projects

Organizations can manage multiple projects:

```
Organization: Software Agency
├── Project: Client A Website
│   ├── Team: Frontend Developers
│   └── Team: Backend Developers
├── Project: Client B App
│   ├── Team: Mobile Developers
│   └── Team: API Developers
```

### Organizations + Hasura

Organizations integrate with Hasura through session variables:

```javascript
// JWT token includes
{
  "https://hasura.io/jwt/claims": {
    "x-hasura-user-id": "user-uuid-123",
    "x-hasura-org-id": "org-uuid-456",
    "x-hasura-role": "admin",
    "x-hasura-allowed-roles": ["admin", "member"],
    "x-hasura-team-ids": "{team-uuid-1,team-uuid-2}"
  }
}
```

**Row-level security uses this**:
```sql
-- Only show organizations user is member of
CREATE POLICY org_member_select ON organizations.organizations
  FOR SELECT
  USING (
    organizations.is_org_member(id, tenants.current_user_id())
  );
```

### Organizations + Authentication

nHost Auth service can be extended to include organization context:

```javascript
// On login, include organization memberships
const user = await auth.signIn(email, password);
const orgs = await db.query(`
  SELECT o.id, o.slug, om.role
  FROM organizations.org_members om
  INNER JOIN organizations.organizations o ON om.org_id = o.id
  WHERE om.user_id = $1
`, [user.id]);

// Return in session
return {
  user,
  organizations: orgs
};
```

---

## Best Practices

### When to Create Organizations vs Teams

**Create an Organization when**:
- You need separate billing
- You need isolated workspace
- You have distinct client/customer accounts
- You need separate compliance or audit trails

**Create a Team when**:
- You need to group users within the same workspace
- You want departmental separation
- You need project-based groupings
- You want to scope permissions to specific groups

### Role Assignment Strategies

#### Strategy 1: Minimal Permissions (Recommended)
Start with minimal access and grant more as needed:

```bash
# Add as member first
nself tenant org member add acme user-123 member

# Grant specific role
nself tenant org role create acme "Junior Developer"
nself tenant org permission grant "Junior Developer" tenant.read
nself tenant org permission grant "Junior Developer" user.read
nself tenant org role assign acme user-123 "Junior Developer"

# Promote later when needed
nself tenant org permission grant "Junior Developer" tenant.update
```

#### Strategy 2: Role Templates
Create role templates for common job functions:

```bash
# Backend Developer template
nself tenant org role create acme "Backend Developer"
nself tenant org permission grant "Backend Developer" tenant.create
nself tenant org permission grant "Backend Developer" tenant.update
nself tenant org permission grant "Backend Developer" user.read

# Frontend Developer template
nself tenant org role create acme "Frontend Developer"
nself tenant org permission grant "Frontend Developer" tenant.read
nself tenant org permission grant "Frontend Developer" user.read

# Full Stack template = Backend + Frontend
# Assign both roles to full stack developers
```

#### Strategy 3: Hierarchical Roles
Build roles that inherit from each other (conceptually):

```
Guest → Member → Developer → Senior Developer → Admin → Owner
  (Each level includes all previous permissions)
```

### Permission Design Patterns

#### Pattern 1: Resource-Action Matrix

Create a matrix of what each role can do:

| Role              | tenant.create | tenant.read | tenant.update | tenant.delete | user.manage |
|-------------------|---------------|-------------|---------------|---------------|-------------|
| Owner             | ✓             | ✓           | ✓             | ✓             | ✓           |
| Admin             | ✓             | ✓           | ✓             | ✓             | ✓           |
| Senior Developer  | ✓             | ✓           | ✓             | ✗             | ✗           |
| Developer         | ✗             | ✓           | ✓             | ✗             | ✗           |
| Support Agent     | ✗             | ✓           | ✗             | ✗             | ✗           |
| Guest             | ✗             | ✓           | ✗             | ✗             | ✗           |

#### Pattern 2: Scope-Based Permissions

Use scopes to limit blast radius:

```sql
-- Global permission (can manage ALL tenants)
scope = 'global'

-- Tenant-scoped (can only manage specific tenant)
scope = 'tenant', scope_id = 'tenant-uuid'

-- Team-scoped (can only manage team resources)
scope = 'team', scope_id = 'team-uuid'
```

#### Pattern 3: Temporary Elevated Access

Grant time-limited permissions for specific tasks:

```sql
-- Grant elevated access
INSERT INTO permissions.user_roles (...)
VALUES (..., NOW() + INTERVAL '1 hour'); -- Auto-expire after 1 hour

-- Create trigger to auto-revoke
CREATE TRIGGER revoke_expired_roles ...
```

### Audit Logging Setup

Enable comprehensive audit trails:

```sql
-- Automatically logs to permissions.permission_audit
INSERT INTO permissions.permission_audit (
  user_id,
  org_id,
  action,
  resource_type,
  resource_id,
  permission_name,
  performed_by,
  metadata
) VALUES (
  'user-uuid-123',
  'org-uuid-456',
  'grant',
  'role',
  'role-uuid-789',
  'tenant.delete',
  'admin-uuid-000',
  '{"reason": "Emergency maintenance", "ticket": "INC-12345"}'
);
```

**Query audit logs**:
```sql
-- View recent permission changes
SELECT
  pa.action,
  pa.permission_name,
  pa.timestamp,
  u.email as performed_by_email
FROM permissions.permission_audit pa
LEFT JOIN auth.users u ON pa.performed_by = u.id
WHERE pa.org_id = 'org-uuid-456'
ORDER BY pa.timestamp DESC
LIMIT 100;
```

### Organization Naming Conventions

**Slugs should be**:
- Lowercase
- Hyphen-separated
- Unique and descriptive
- URL-safe

**Examples**:
- `acme-corp`
- `project-phoenix`
- `client-abc-prod`

**Team slugs should**:
- Match organization naming style
- Be descriptive of function
- Avoid abbreviations unless universal

**Examples**:
- `engineering`
- `customer-success`
- `backend-team`

### Multi-Organization Users

Users can belong to multiple organizations:

```sql
-- User in multiple organizations
SELECT
  o.slug,
  o.name,
  om.role
FROM organizations.org_members om
INNER JOIN organizations.organizations o ON om.org_id = o.id
WHERE om.user_id = 'user-uuid-123';
```

**Best practice**: Applications should allow users to switch active organization context:

```javascript
// Frontend: Organization switcher
const switchOrganization = (orgId) => {
  localStorage.setItem('active_org_id', orgId);
  // Refresh session with new org context
  window.location.reload();
};
```

---

## Advanced Topics

### SSO Integration with Organizations

Configure Single Sign-On per organization:

```sql
-- Add SSO settings to organization
UPDATE organizations.organizations
SET settings = jsonb_set(
  settings,
  '{sso}',
  '{
    "enabled": true,
    "provider": "okta",
    "domain": "acme.okta.com",
    "client_id": "...",
    "client_secret": "..."
  }'::jsonb
)
WHERE slug = 'acme';
```

**Auto-join on SSO login**:
```javascript
// On SSO callback
const ssoOrg = await db.query(`
  SELECT id FROM organizations.organizations
  WHERE settings->>'sso'->>'domain' = $1
`, [userSSODomain]);

if (ssoOrg) {
  // Auto-add user to organization
  await db.query(`
    INSERT INTO organizations.org_members (org_id, user_id, role)
    VALUES ($1, $2, 'member')
    ON CONFLICT DO NOTHING
  `, [ssoOrg.id, user.id]);
}
```

### Organization Transfer and Ownership

Transfer organization to a new owner:

```sql
-- Transfer ownership
BEGIN;

-- Update owner
UPDATE organizations.organizations
SET owner_user_id = 'new-owner-uuid'
WHERE id = 'org-uuid';

-- Update org_members role
UPDATE organizations.org_members
SET role = 'admin'
WHERE org_id = 'org-uuid' AND user_id = 'old-owner-uuid';

UPDATE organizations.org_members
SET role = 'owner'
WHERE org_id = 'org-uuid' AND user_id = 'new-owner-uuid';

COMMIT;
```

### Bulk Member Operations

Add multiple members at once:

```sql
-- Bulk add members
INSERT INTO organizations.org_members (org_id, user_id, role)
SELECT
  'org-uuid-456',
  user_id,
  'member'
FROM (
  VALUES
    ('user-uuid-1'),
    ('user-uuid-2'),
    ('user-uuid-3')
) AS users(user_id)
ON CONFLICT (org_id, user_id) DO NOTHING;
```

**CSV import example**:
```bash
# CSV format: email,role
# john@acme.com,admin
# jane@acme.com,member

# Import script
cat members.csv | while IFS=, read email role; do
  user_id=$(psql -t -c "SELECT id FROM auth.users WHERE email = '$email'")
  nself org member add acme "$user_id" "$role"
done
```

### Permission Automation

Automate role assignment based on conditions:

```sql
-- Trigger: Auto-assign role to new members
CREATE OR REPLACE FUNCTION organizations.auto_assign_default_role()
RETURNS TRIGGER AS $$
BEGIN
  -- Assign "Member" role to new organization members
  INSERT INTO permissions.user_roles (user_id, role_id, org_id)
  SELECT
    NEW.user_id,
    r.id,
    NEW.org_id
  FROM permissions.roles r
  WHERE r.org_id = NEW.org_id
  AND r.name = 'Member'
  AND r.is_builtin = true;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER assign_default_role_on_join
  AFTER INSERT ON organizations.org_members
  FOR EACH ROW
  EXECUTE FUNCTION organizations.auto_assign_default_role();
```

### Custom Permission Validators

Create application-specific permission logic:

```sql
-- Function: Check if user can perform action on resource
CREATE OR REPLACE FUNCTION permissions.can_user_perform_action(
  p_user_id UUID,
  p_org_id UUID,
  p_resource_type TEXT,
  p_action TEXT,
  p_resource_id UUID DEFAULT NULL
)
RETURNS BOOLEAN AS $$
DECLARE
  v_permission_name TEXT;
  v_has_permission BOOLEAN;
BEGIN
  -- Build permission name
  v_permission_name := p_resource_type || '.' || p_action;

  -- Check global permission
  SELECT permissions.has_permission(
    p_user_id,
    p_org_id,
    v_permission_name,
    'global'
  ) INTO v_has_permission;

  IF v_has_permission THEN
    RETURN true;
  END IF;

  -- Check resource-specific permission
  IF p_resource_id IS NOT NULL THEN
    -- Additional resource-level checks here
    -- E.g., check if user is owner of resource
  END IF;

  RETURN false;
END;
$$ LANGUAGE plpgsql STABLE;
```

### Organization Hierarchies (Future Enhancement)

While not yet implemented, you could extend organizations to support hierarchies:

```sql
-- Parent-child organization relationships
ALTER TABLE organizations.organizations
ADD COLUMN parent_org_id UUID REFERENCES organizations.organizations(id);

-- Function: Get all child organizations
CREATE OR REPLACE FUNCTION organizations.get_child_orgs(p_org_id UUID)
RETURNS TABLE (id UUID, name TEXT, depth INT) AS $$
  WITH RECURSIVE org_tree AS (
    -- Base case
    SELECT id, name, 0 as depth
    FROM organizations.organizations
    WHERE id = p_org_id

    UNION ALL

    -- Recursive case
    SELECT o.id, o.name, ot.depth + 1
    FROM organizations.organizations o
    INNER JOIN org_tree ot ON o.parent_org_id = ot.id
  )
  SELECT * FROM org_tree;
$$ LANGUAGE sql STABLE;
```

### Integration with External Identity Providers

Sync organization structure from external systems:

```javascript
// Sync from external HR system
const syncFromHR = async () => {
  const departments = await hrSystem.getDepartments();

  for (const dept of departments) {
    // Create or update organization
    const org = await db.query(`
      INSERT INTO organizations.organizations (slug, name)
      VALUES ($1, $2)
      ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
      RETURNING id
    `, [dept.slug, dept.name]);

    // Create teams
    for (const team of dept.teams) {
      await db.query(`
        INSERT INTO organizations.teams (org_id, name, slug)
        VALUES ($1, $2, $3)
        ON CONFLICT (org_id, slug) DO UPDATE SET name = EXCLUDED.name
      `, [org.id, team.name, team.slug]);
    }

    // Sync members
    for (const employee of dept.employees) {
      // Get or create user
      const user = await getOrCreateUser(employee.email);

      // Add to organization
      await db.query(`
        INSERT INTO organizations.org_members (org_id, user_id, role)
        VALUES ($1, $2, $3)
        ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role
      `, [org.id, user.id, employee.role]);
    }
  }
};
```

---

## Related Documentation

- [Multi-Tenant Guide](../architecture/MULTI-TENANCY.md) - Tenant isolation and management
- [Database Workflow Guide](DATABASE-WORKFLOW.md) - Database operations
- [Security Guide](SECURITY.md) - Security best practices
- [Deployment Architecture](DEPLOYMENT-ARCHITECTURE.md) - Production setup

## Troubleshooting

### Issue: User cannot see organization

**Possible causes**:
1. User is not a member of the organization
2. Organization is suspended or deleted
3. Row-level security policy blocking access

**Solution**:
```bash
# Check membership
nself tenant org member list acme

# Add user if missing
nself tenant org member add acme user-123

# Check organization status
nself tenant org show acme
```

### Issue: Permission denied despite having role

**Possible causes**:
1. Role doesn't have the required permission
2. Permission is scoped to wrong resource
3. RLS policy blocking access

**Solution**:
```sql
-- Check user's permissions
SELECT * FROM permissions.get_user_permissions('user-uuid', 'org-uuid');

-- Check role's permissions
SELECT p.name
FROM permissions.role_permissions rp
INNER JOIN permissions.permissions p ON rp.permission_id = p.id
WHERE rp.role_id = (SELECT id FROM permissions.roles WHERE name = 'Developer');

-- Grant missing permission
-- Use CLI: nself tenant org permission grant Developer tenant.create
```

### Issue: Cannot delete organization

**Possible causes**:
1. User is not owner or admin
2. Foreign key constraints (teams, members exist)

**Solution**:
```bash
# Check your role
nself tenant org show acme

# Organization deletion cascades, so this should work
# If not, check database logs
nself tenant org delete acme
```

---

## Summary

Organizations in nself provide a powerful, flexible system for managing workspaces, teams, and access control:

- **Organizations** structure your workspace at the highest level
- **Teams** provide logical groupings within organizations
- **Members** can have multiple roles with aggregated permissions
- **Roles** define collections of permissions
- **Permissions** control fine-grained access to resources
- **Scopes** limit permissions to specific contexts

By combining these elements, you can implement enterprise-grade access control tailored to your specific needs.

For questions or support, refer to the [nself documentation](https://nself.org) or open an issue on [GitHub](https://github.com/nself-org/cli).
