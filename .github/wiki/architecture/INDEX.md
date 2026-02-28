# Architecture Documentation

Complete system architecture and design documentation for nself.

## Overview

This directory contains detailed architectural documentation covering system design, command structure, multi-tenancy, billing, and internal organization.

## Core Architecture

### System Design
- **[Main Architecture](ARCHITECTURE.md)** - Complete system architecture overview
- **[Project Structure](PROJECT_STRUCTURE.md)** - File and directory organization
- **[Directory Structure](DIRECTORY_STRUCTURE.md)** - Detailed directory layout
- **[Build Architecture](BUILD_ARCHITECTURE.md)** - How the build system works
- **[Library Overview](LIBRARY-OVERVIEW.md)** - Core library structure

### API Architecture
- **[API Documentation](API.md)** - GraphQL API architecture with Hasura
- **[API Documentation Rewrite](API-DOCUMENTATION-REWRITE.md)** - API documentation improvements

## Feature Architecture

### Multi-Tenancy
- **[Multi-Tenancy Architecture](MULTI-TENANCY.md)** - Tenant isolation and organization
- **[Billing Architecture](BILLING-ARCHITECTURE.md)** - Usage tracking and billing system
- **[White-Label Architecture](WHITE-LABEL-ARCHITECTURE.md)** - Branding and customization system

## Command Structure (v1.0)

### Command Consolidation (v0.9.6)
The CLI underwent a major consolidation from 79 → 31 top-level commands:

- **[Command Consolidation Map](COMMAND-CONSOLIDATION-MAP.md)** - Complete mapping of old → new commands
- **[Command Reorganization Index](COMMAND-REORGANIZATION-INDEX.md)** - Index of reorganization documentation
- **[Command Reorganization Proposal](COMMAND-REORGANIZATION-PROPOSAL.md)** - Original proposal and rationale
- **[Command Reorganization Visual](COMMAND-REORGANIZATION-VISUAL.md)** - Visual command tree
- **[Command Reorganization Checklist](COMMAND-REORGANIZATION-CHECKLIST.md)** - Implementation checklist

## Quick Navigation

| I want to... | Document |
|-------------|----------|
| Understand the overall system | [ARCHITECTURE.md](ARCHITECTURE.md) |
| See how files are organized | [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md) |
| Learn about the build system | [BUILD_ARCHITECTURE.md](BUILD_ARCHITECTURE.md) |
| Understand the API | [API.md](API.md) |
| Learn about multi-tenancy | [MULTI-TENANCY.md](MULTI-TENANCY.md) |
| See command changes in v0.9.6 | [COMMAND-CONSOLIDATION-MAP.md](COMMAND-CONSOLIDATION-MAP.md) |

## Related Documentation

- **[Services Documentation](../services/SERVICES.md)** - Available services
- **[Configuration Reference](../configuration/README.md)** - Configuration options
- **[Command Reference](../commands/COMMANDS.md)** - All CLI commands
- **[Deployment Guide](../deployment/README.md)** - Production deployment

---

**Last Updated**: January 31, 2026
**Version**: v0.9.6
