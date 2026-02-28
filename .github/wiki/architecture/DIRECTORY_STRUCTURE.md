# nself Directory Structure Guide

## Overview

This document outlines the directory structure for nself v0.3.9, designed for maintainability, modularity, and clear separation of concerns.

## Root Directory Structure

```
nself/
├── README.md                    # Project overview and quick start
├── CHANGELOG.md                 # Version history and release notes
├── LICENSE                      # MIT License
├── install.sh                   # Installation script for system-wide setup
├── .gitignore                   # Git ignore patterns
├── bin/                         # Executable entry points
├── src/                         # Source code and implementation
└── docs/                        # Documentation and guides
```

## Source Code Organization (`src/`)

### Core Structure

```
src/
├── VERSION                      # Current version (v0.3.9)
├── cli/                         # Command-line interface implementations
├── lib/                         # Shared libraries and utilities
├── services/                    # Service management modules
├── templates/                   # Configuration and Docker templates
└── tools/                       # Development and maintenance tools
```

## CLI Commands (`src/cli/`)

All command implementations following consistent patterns:

```
cli/
├── nself.sh                     # Main CLI dispatcher and router
├── help.sh                      # Help system and documentation
├── init.sh                      # Project initialization
├── build.sh                     # Infrastructure generation
├── start.sh                     # Service startup management
├── stop.sh                      # Service shutdown management
├── restart.sh                   # Service restart management
├── status.sh                    # Service status monitoring
├── logs.sh                      # Log viewing and management
├── doctor.sh                    # System diagnostics
├── backup.sh                    # Backup and restore operations
├── db.sh                        # Database operations
├── email.sh                     # Email service configuration
├── ssl.sh                       # SSL certificate management
├── urls.sh                      # Service URL display
├── prod.sh                      # Production configuration
├── trust.sh                     # SSL certificate installation
├── validate.sh                  # Configuration validation
├── exec.sh                      # Container command execution
├── scale.sh                     # Resource scaling management
├── metrics.sh                   # Metrics and monitoring
├── clean.sh                     # Docker cleanup operations
├── diff.sh                      # Configuration difference analysis
├── reset.sh                     # Project reset operations
├── rollback.sh                  # Version rollback management
├── monitor.sh                   # Real-time monitoring
├── scaffold.sh                  # Service scaffolding
├── update.sh                    # nself version updates
├── version.sh                   # Version information
├── admin.sh                     # Admin UI management (v0.3.9)
├── search.sh                    # Search service management (v0.3.9)
├── deploy.sh                    # VPS deployment (v0.3.9)
└── wizard/                      # Interactive setup wizard (v0.3.9)
    ├── init-wizard.sh           # Main wizard controller
    ├── detection.sh             # Project framework detection
    ├── prompts.sh               # Interactive UI components
    └── templates.sh             # Project template definitions
```

## Shared Libraries (`src/lib/`)

Reusable functionality organized by purpose:

```
lib/
├── utils/                       # Common utilities
│   ├── display.sh               # Output formatting and colors
│   ├── env.sh                   # Environment file handling
│   ├── docker.sh                # Docker operations
│   ├── network.sh               # Network utilities
│   └── validation.sh            # Input validation helpers
├── config/                      # Configuration management
│   ├── constants.sh             # System constants
│   ├── smart-defaults.sh        # Intelligent default values
│   └── validation.sh            # Configuration validation
├── hooks/                       # Command lifecycle hooks
│   ├── pre-command.sh           # Pre-execution hooks
│   └── post-command.sh          # Post-execution hooks
├── ssl/                         # SSL certificate management
│   ├── auto-ssl.sh              # Automatic SSL generation
│   └── auto-renew.sh            # Certificate renewal
├── monitoring/                  # System monitoring
│   ├── health.sh                # Health check utilities
│   ├── metrics.sh               # Metrics collection
│   └── alerts.sh                # Alert management
├── backup/                      # Backup operations
│   ├── core.sh                  # Core backup functionality
│   └── s3.sh                    # S3 integration
├── deployment/                  # Deployment utilities
│   ├── ssh.sh                   # SSH deployment helpers
│   └── validation.sh            # Deployment validation
└── wizard/                      # Wizard system (v0.3.9)
    └── environment-manager.sh   # Multi-environment management
```

## Service Management (`src/services/`)

Service-specific logic and configurations:

```
services/
├── postgres/                    # PostgreSQL management
├── redis/                       # Redis configuration
├── minio/                       # MinIO object storage
├── hasura/                      # Hasura GraphQL engine
├── auth/                        # Authentication service
├── nginx/                       # Nginx reverse proxy
├── docker/                      # Docker Compose generation
└── monitoring/                  # Monitoring stack
```

## Templates (`src/templates/`)

Configuration templates and examples:

```
templates/
├── .env.example                 # Complete environment reference
├── docker-compose/              # Docker Compose templates
├── nginx/                       # Nginx configuration templates
├── ssl/                         # SSL certificate templates
└── certs/                       # Pre-generated certificates
    ├── localhost/               # Local development certificates
    └── nself-org/               # nself.org domain certificates
```

## Documentation (`docs/`)

Comprehensive documentation for users and developers:

```
docs/
├── COMMANDS.md                  # Complete command reference
├── v0.3.9.md                    # v0.3.9 release notes
├── ARCHITECTURE.MD              # System architecture overview
├── ENVIRONMENT_CONFIGURATION.MD # Environment setup guide
├── TROUBLESHOOTING.MD           # Common issues and solutions
├── API.MD                       # API documentation
├── CHANGELOG.MD                 # Version history
├── ROADMAP.md                   # Future development plans
├── CONTRIBUTING.MD              # Contribution guidelines
├── RELEASES.MD                  # Release information
├── EXAMPLES.MD                  # Usage examples
└── README.MD                    # Getting started guide
```

## Executable Entry Points (`bin/`)

Simple wrappers for system-wide installation:

```
bin/
├── nself                        # Main nself command wrapper
├── urls                         # Direct URL display
└── [other-commands]             # Additional command aliases
```

## Design Principles

### Modularity
- Each command is self-contained in its own file
- Shared functionality is centralized in `lib/`
- Clear separation between CLI, logic, and templates

### Consistency
- All commands follow the same structure and patterns
- Consistent naming conventions throughout
- Standardized help and error handling

### Extensibility
- New commands can be added easily in `src/cli/`
- Service modules are pluggable and independent
- Template system supports customization

### Maintainability
- Clear file organization by functionality
- Documented interfaces between modules
- Comprehensive testing structure

### Backward Compatibility
- Existing command structure preserved
- New v0.3.9 features are additive
- Legacy functionality remains unchanged

## File Naming Conventions

- **Commands**: `command-name.sh` (kebab-case with .sh extension)
- **Libraries**: `module-name.sh` (descriptive, organized by directory)
- **Templates**: `template-name.ext` (matches target file extension)
- **Documentation**: `TOPIC.MD` (uppercase .MD for main docs, lowercase .md for version-specific)

## Execution Flow

1. **Entry Point**: `bin/nself` → `src/cli/nself.sh`
2. **Command Routing**: `nself.sh` identifies and sources appropriate command file
3. **Library Loading**: Commands source required libraries from `src/lib/`
4. **Template Processing**: Commands use templates from `src/templates/`
5. **Service Integration**: Commands interact with services via `src/services/`

This structure supports nself's evolution from a simple CLI tool to a comprehensive backend platform while maintaining clarity and ease of development.