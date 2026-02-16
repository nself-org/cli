# Documentation Structure

This document describes the organization of nself's documentation.

## Overview

The documentation is organized into logical categories to help users find information quickly. Each directory has a specific purpose and contains related documentation.

## Directory Structure

```
docs/
├── README.md                    # Main documentation index
├── Home.md                      # GitHub wiki homepage
├── _Sidebar.md                  # GitHub wiki navigation sidebar
├── STRUCTURE.md                 # This file - documentation organization
│
├── getting-started/             # NEW USER START HERE
│   ├── Installation.md          # Installation guide
│   ├── Quick-Start.md           # 5-minute quick start
│   └── FAQ.md                   # Frequently asked questions
│
├── guides/                      # HOW-TO GUIDES & WORKFLOWS
│   ├── README.md                # Guides index
│   ├── DATABASE-WORKFLOW.md     # Database design workflow
│   ├── DEPLOYMENT-ARCHITECTURE.md
│   ├── Deployment.md
│   ├── ENVIRONMENTS.md          # Environment management
│   ├── EXAMPLES.md
│   ├── MULTI_APP_SETUP.md
│   ├── BACKUP_GUIDE.md
│   ├── TROUBLESHOOTING.md
│   ├── SECURITY.md
│   ├── ORGANIZATION-MANAGEMENT.md
│   ├── BILLING-AND-USAGE.md
│   ├── SERVICE-CODE-GENERATION.md
│   ├── SERVICE-TO-SERVICE-COMMUNICATION.md
│   ├── PLUGIN-DEVELOPMENT.md
│   ├── REALTIME-FEATURES.md
│   ├── OAUTH-SETUP.md
│   ├── OAUTH-QUICK-START.md
│   ├── OAUTH-COMPLETE-FLOWS.md
│   ├── WHITE-LABEL-CUSTOMIZATION.md
│   ├── WHITELABEL-Quick-Start.md
│   ├── domain-selection-guide.md
│   ├── file-upload-examples.md
│   ├── file-upload-pipeline.md
│   ├── PROVIDERS-COMPLETE.md    # Cloud providers (moved from providers/)
│   ├── RLS_IMPLEMENTATION_SUMMARY.md  # Database security (moved from database/)
│   ├── ROW_LEVEL_SECURITY.md
│   ├── CORE-IMPLEMENTATION.md   # Billing (moved from billing/)
│   ├── QUOTAS.md
│   ├── STRIPE_IMPLEMENTATION.md
│   ├── USAGE-IMPLEMENTATION-SUMMARY.md
│   ├── USAGE-QUICK-REFERENCE.md
│   ├── USAGE-TRACKING.md
│   ├── BRANDING-FUNCTION-REFERENCE.md  # White-label (moved from whitelabel/)
│   ├── BRANDING-QUICK-START.md
│   ├── BRANDING-SYSTEM.md
│   ├── EMAIL-TEMPLATES.md
│   ├── THEMES-QUICK-START.md
│   └── THEMES.md
│
├── tutorials/                   # STEP-BY-STEP TUTORIALS
│   ├── README.md
│   ├── QUICK-REFERENCE.md
│   ├── QUICK-START-AGENCY.md    # Use-case specific tutorials
│   ├── QUICK-START-B2B.md
│   ├── QUICK-START-MARKETPLACE.md
│   ├── QUICK-START-SAAS.md
│   ├── CUSTOM-DOMAINS.md
│   ├── STRIPE-INTEGRATION.md
│   └── file-uploads-quickstart.md
│
├── commands/                    # CLI COMMAND REFERENCE
│   ├── README.md                # Commands overview
│   ├── COMMANDS.md              # All commands index
│   ├── COMMAND-TREE-V1.md       # v1.0 command tree
│   ├── REFACTORING-SUMMARY.md
│   ├── [COMMAND].md             # Individual command docs (90+ files)
│   ├── auth-consolidation.md    # CLI docs (moved from cli/)
│   ├── oauth.md
│   └── storage.md
│
├── configuration/               # CONFIGURATION REFERENCE
│   ├── README.md
│   ├── ENVIRONMENT-VARIABLES.md
│   ├── ENV-COMPLETE-REFERENCE.md
│   ├── CUSTOM-SERVICES-ENV-VARS.md
│   ├── START-COMMAND-OPTIONS.md
│   ├── Admin-UI.md
│   └── SSL.md
│
├── architecture/                # SYSTEM ARCHITECTURE
│   ├── README.md
│   ├── ARCHITECTURE.md          # Main architecture doc
│   ├── PROJECT_STRUCTURE.md
│   ├── DIRECTORY_STRUCTURE.md
│   ├── BUILD_ARCHITECTURE.md
│   ├── API.md
│   ├── MULTI-TENANCY.md
│   ├── BILLING-ARCHITECTURE.md
│   ├── WHITE-LABEL-ARCHITECTURE.md
│   ├── COMMAND-CONSOLIDATION-MAP.md
│   ├── COMMAND-REORGANIZATION-INDEX.md
│   ├── COMMAND-REORGANIZATION-PROPOSAL.md
│   ├── COMMAND-REORGANIZATION-VISUAL.md
│   └── COMMAND-REORGANIZATION-CHECKLIST.md
│
├── services/                    # SERVICES DOCUMENTATION
│   ├── SERVICES.md              # Services overview
│   ├── SERVICES_REQUIRED.md     # Required services (4)
│   ├── SERVICES_OPTIONAL.md     # Optional services (7)
│   ├── SERVICES_CUSTOM.md       # Custom services
│   ├── SERVICE_REFERENCE.md
│   ├── DEMO_SETUP.md
│   ├── MONITORING-BUNDLE.md     # Monitoring stack (10)
│   ├── NSELF_ADMIN.md
│   ├── SEARCH.md
│   └── TYPESENSE.md
│
├── deployment/                  # DEPLOYMENT GUIDES
│   ├── README.md
│   ├── PRODUCTION-DEPLOYMENT.md
│   ├── CLOUD-PROVIDERS.md
│   ├── CUSTOM-SERVICES-PRODUCTION.md
│   └── examples/
│       ├── README.md
│       └── blue-green-deployment.md
│
├── plugins/                     # PLUGIN SYSTEM
│   ├── index.md                 # Plugin overview
│   ├── README.md
│   ├── development.md           # Plugin development
│   ├── github.md                # GitHub plugin
│   ├── shopify.md               # Shopify plugin
│   └── stripe.md                # Stripe plugin
│
├── reference/                   # REFERENCE DOCUMENTATION
│   ├── SERVICE_TEMPLATES.md     # 40+ service templates
│   ├── COMMAND-REFERENCE.md     # Quick command reference
│   ├── QUICK-NAVIGATION.md
│   ├── SERVICE-SCAFFOLDING-CHEATSHEET.md
│   └── api/                     # API reference (moved from api/)
│       ├── README.md
│       ├── BILLING-API.md
│       ├── OAUTH-API.md
│       └── WHITE-LABEL-API.md
│
├── examples/                    # EXAMPLE CODE & PATTERNS
│   ├── README.md
│   ├── INDEX.md
│   ├── FEATURES-OVERVIEW.md
│   └── REALTIME-CHAT-SERVICE.md
│
├── features/                    # FEATURE DOCUMENTATION
│   ├── REALTIME.md
│   ├── WHITELABEL-SYSTEM.md
│   ├── file-upload-pipeline.md
│   └── realtime-examples.md
│
├── releases/                    # VERSION HISTORY & ROADMAP
│   ├── INDEX.md                 # Release index
│   ├── ROADMAP.md               # Future roadmap
│   ├── CHANGELOG.md             # Complete changelog
│   ├── v0.1.0.md                # Version-specific notes
│   ├── v0.2.0.md
│   ├── ... (45+ version files)
│   ├── v0.9.6.md                # Latest release
│   ├── PHASE1-COMPLETE.md       # Development phases
│   ├── PHASE1-PROGRESS.md
│   ├── PHASE2-COMPLETE.md
│   ├── PHASE2-ROADMAP.md
│   ├── PHASE3-ROADMAP.md
│   ├── STATUS-REPORT-v0.5.0.md
│   └── COMMAND-TREE-v0.9.0.md
│
├── migrations/                  # PLATFORM MIGRATION GUIDES
│   ├── README.md
│   ├── FROM-FIREBASE.md         # Migrate from Firebase
│   ├── FROM-NHOST.md            # Migrate from Nhost
│   ├── FROM-SUPABASE.md         # Migrate from Supabase
│   ├── INFRA-CONSOLIDATION.md   # Internal migrations
│   └── V1-MIGRATION-STATUS.md   # v1.0 migration status
│
├── security/                    # SECURITY DOCUMENTATION
│   ├── README.md
│   ├── SECURITY-AUDIT.md        # Main security audit
│   ├── SECURITY-SYSTEM.md
│   ├── HEADERS.md
│   ├── SQL-SAFETY.md
│   ├── SECURITY-AUDIT-BILLING.md
│   ├── SECURITY-FIX-FINAL-REPORT.md
│   ├── SECURITY_AUDIT_INDEX.md
│   ├── DEPENDENCY-SCANNING.md
│   ├── SQL-INJECTION-FIXES.md
│   ├── SQL-INJECTION-FIX-SUMMARY.md
│   ├── SQL-REVIEW-REMAINING.md
│   ├── INJECTION_ATTACK_PREVENTION_SUMMARY.md
│   ├── INPUT_VALIDATION_SECURITY_AUDIT.md
│   ├── PARAMETERIZED-QUERIES-QUICK-REFERENCE.md
│   ├── VALIDATION_FUNCTIONS_REFERENCE.md
│   └── file-upload-security.md
│
├── troubleshooting/             # TROUBLESHOOTING GUIDES
│   ├── BILLING-TROUBLESHOOTING.md
│   └── WHITE-LABEL-TROUBLESHOOTING.md
│
├── contributing/                # CONTRIBUTOR DOCUMENTATION
│   ├── README.md
│   ├── CONTRIBUTING.md          # How to contribute (moved from root)
│   ├── CODE_OF_CONDUCT.md
│   ├── DEVELOPMENT.md           # Dev environment setup
│   ├── CROSS-PLATFORM-COMPATIBILITY.md
│   ├── CLI-OUTPUT-LIBRARY.md    # Dev tools (moved from development/)
│   └── CLI-OUTPUT-QUICK-REFERENCE.md
│
└── qa/                          # QUALITY ASSURANCE REPORTS
    ├── README.md
    ├── QA-SUMMARY.md
    ├── ISSUES-TO-FIX.md
    ├── V1-COMMAND-STRUCTURE-QA-REPORT.md
    ├── V1-QA-REPORT.md
    ├── V1-QA-SUMMARY.md
    └── v1.0-final-validation-report.md
```

## Directory Purposes

### Root Level Files
- **README.md** - Main documentation index and quick navigation
- **Home.md** - GitHub wiki homepage
- **_Sidebar.md** - GitHub wiki sidebar navigation
- **STRUCTURE.md** - This file

### getting-started/
First stop for new users. Contains installation, quick start, and FAQ.

**Target Audience**: New users, first-time installation

### guides/
How-to guides and workflows for specific features and use cases. Comprehensive documentation for accomplishing specific tasks.

**Target Audience**: All users looking to implement specific features

**Content**: Database workflows, deployment, environments, billing, OAuth, white-label, security, etc.

### tutorials/
Step-by-step tutorials with code examples. More prescriptive than guides.

**Target Audience**: Users learning specific patterns or use cases

**Content**: Quick starts for different scenarios (SaaS, B2B, Agency, Marketplace), integrations

### commands/
Complete CLI command reference. Documentation for every nself command.

**Target Audience**: Users looking up specific commands

**Content**: 90+ command documentation files, command tree, consolidation maps

### configuration/
Configuration reference for environment variables, start options, and settings.

**Target Audience**: Users configuring their nself setup

**Content**: Environment variables, SSL, Admin UI, custom services configuration

### architecture/
System architecture, design decisions, and internal structure.

**Target Audience**: Advanced users, contributors, architects

**Content**: Architecture diagrams, build system, project structure, multi-tenancy design

### services/
Documentation for all available services (required, optional, monitoring, custom).

**Target Audience**: Users understanding service options

**Content**: Service overviews, monitoring bundle, demo setup, search services

### deployment/
Production deployment guides and examples.

**Target Audience**: Users deploying to staging/production

**Content**: Production deployment, cloud providers, deployment examples

### plugins/
Plugin system documentation and available plugins.

**Target Audience**: Users extending nself with third-party integrations

**Content**: Plugin development, GitHub/Shopify/Stripe plugins

### reference/
Quick reference materials, cheat sheets, and API documentation.

**Target Audience**: Users needing quick lookups

**Content**: Command reference, API docs, service templates, scaffolding cheatsheet

### examples/
Real-world code examples and patterns.

**Target Audience**: Users looking for copy-paste examples

**Content**: Feature examples, chat service, database patterns

### features/
Feature-specific documentation.

**Target Audience**: Users learning about specific features

**Content**: Realtime, white-label, file uploads

### releases/
Version history, changelogs, and roadmap.

**Target Audience**: Users tracking versions and planning upgrades

**Content**: 45+ version release notes, roadmap, phase completion reports

### migrations/
Migration guides from other platforms (Firebase, Nhost, Supabase) and internal migration documentation.

**Target Audience**: Users migrating from other platforms

**Content**: Platform migration guides, internal refactoring documentation

### security/
Security audits, best practices, and security documentation.

**Target Audience**: Security-conscious users, compliance requirements

**Content**: Security audits, SQL safety, injection prevention, validation

### troubleshooting/
Troubleshooting guides for common issues.

**Target Audience**: Users experiencing problems

**Content**: Feature-specific troubleshooting (billing, white-label)

### contributing/
Contributor documentation and development guides.

**Target Audience**: Contributors, developers working on nself

**Content**: Contributing guide, development setup, coding standards, CLI output library

### qa/
Quality assurance reports and test results.

**Target Audience**: Maintainers, QA team

**Content**: QA reports, validation results, issue tracking

## Changes Made

### Consolidated Directories
- **api/** → **reference/api/** (API docs now under reference)
- **development/** → **contributing/** (dev docs merged with contributing)
- **migration/** → **migrations/** (consolidated migration docs)
- **database/** → **guides/** (database guides moved to guides)
- **providers/** → **guides/** (provider docs moved to guides)
- **billing/** → **guides/** (billing guides moved to guides)
- **whitelabel/** → **guides/** (white-label guides moved to guides)
- **cli/** → **commands/** (CLI docs moved to commands)
- **quick-reference/** → **reference/** (consolidated reference docs)
- **planning/** → Removed (empty directory)

### Moved Files
- **Installation.md** → **getting-started/**
- **Quick-Start.md** → **getting-started/**
- **FAQ.md** → **getting-started/**
- **CONTRIBUTING.md** → **contributing/**
- **V1-MIGRATION-STATUS.md** → **migrations/**

## Navigation Tips

### I want to...

| Goal | Directory |
|------|-----------|
| Get started quickly | `getting-started/` |
| Learn how to do something | `guides/` |
| Follow a tutorial | `tutorials/` |
| Look up a command | `commands/` |
| Configure my setup | `configuration/` |
| Understand the architecture | `architecture/` |
| Deploy to production | `deployment/` |
| Extend with plugins | `plugins/` |
| See code examples | `examples/` |
| Check version history | `releases/` |
| Migrate from another platform | `migrations/` |
| Review security | `security/` |
| Fix a problem | `troubleshooting/` or `guides/TROUBLESHOOTING.md` |
| Contribute to nself | `contributing/` |

## Maintenance

When adding new documentation:

1. **Determine the category** - Which directory does it belong in?
2. **Check for duplicates** - Is there already similar documentation?
3. **Use consistent naming** - Follow existing naming conventions (UPPERCASE for major docs)
4. **Update indexes** - Add to relevant README.md files
5. **Cross-reference** - Link to related documentation
6. **Update navigation** - Add to _Sidebar.md if it's a major doc

## Links and Cross-References

When linking between docs, use relative paths:

```markdown
# From getting-started/ to guides/
[Database Workflow](guides/DATABASE-WORKFLOW.md)

# From commands/ to configuration/
[Environment Variables](configuration/ENVIRONMENT-VARIABLES.md)

# From guides/ to reference/api/
```

---

**Last Updated**: January 30, 2026
**Documentation Version**: v0.9.6
