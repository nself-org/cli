# GitHub Issues to Create - nself Comprehensive Audit

**Generated**: January 30, 2026
**Audit Scope**: Full codebase analysis including v0.4.0 QA audit, v0.9.0 QA report, security audit findings
**Total Issues**: 45 issues across 5 categories
**Estimated Total Effort**: ~280 hours (85 story points)

---

## Issue Organization Strategy

Issues are organized by:
1. **Priority**: Critical → High → Medium → Low
2. **Category**: Security, Code Quality, Features, Testing, Documentation
3. **Estimated Effort**: Hours/days for project planning

---

## CATEGORY 1: SECURITY ISSUES (8 Issues)

### 1. [CRITICAL] SQL Injection Vulnerabilities in Billing System - MOSTLY FIXED

**Type**: Security
**Priority**: Critical
**Effort**: 2 hours (VERIFICATION ONLY)
**Status**: Partially Fixed (29 vulnerabilities fixed in v0.9.0)

**Description**:
The billing system (v0.9.0) had 29 SQL injection vulnerabilities that were fixed via commit c94be85. However, need to verify all were fixed and check for similar patterns in other modules.

**Current State**:
- ✅ Fixed in `/src/lib/billing/core.sh`
- ✅ All queries use parameterized queries with `-v` flag binding
- ⚠️ Need verification that similar patterns don't exist in other functions

**Expected State**:
- All database queries use parameterized queries
- No string interpolation in WHERE/VALUES clauses
- Comprehensive documentation of parameterized query patterns

**Files Affected**:
- `/src/lib/billing/core.sh` - FIXED
- `/src/lib/billing/stripe.sh` - NEEDS REVIEW
- `/src/lib/whitelabel/branding.sh` - NEEDS REVIEW
- `/src/lib/auth/auth-manager.sh` - NEEDS REVIEW

**Acceptance Criteria**:
- [ ] Code review confirms 29 fixes are correct
- [ ] No similar patterns found in other modules
- [ ] Tests verify parameterized queries work
- [ ] Documentation updated with best practices

**Related Issues**:
- Commit: c94be85225463926f8972842102bfb199ae689f4

**Notes**:
This is mostly complete. Create issue only for verification and expansion to other modules.

---

### 2. [HIGH] Secrets Management Enhancement

**Type**: Security
**Priority**: High
**Effort**: 8 hours
**Estimated Story Points**: 5

**Description**:
Implement enhanced secrets management with automatic rotation, audit logging, and secure vaulting. Current `.env` file approach lacks audit trail and rotation capabilities.

**Current State**:
- Secrets stored in `.env` files and environment variables
- No rotation mechanism
- Limited audit logging
- Secrets in git history (risk if exposed)

**Expected State**:
- Integration with HashiCorp Vault or AWS Secrets Manager
- Automatic secret rotation (every 90 days)
- Complete audit logging of secret access
- Zero-knowledge deployment (secrets never stored in git)
- Emergency revocation capability

**Files Affected**:
- `/src/lib/secrets/` (new directory)
- `/src/cli/config.sh` (secrets subcommand)
- `.github/workflows/` (secret scanning in CI)

**Acceptance Criteria**:
- [ ] Vault integration tested
- [ ] Secret rotation working automatically
- [ ] Audit logs showing all access
- [ ] No secrets in git history
- [ ] Documentation complete

**Testing Requirements**:
- [ ] Unit tests for secret rotation
- [ ] Integration tests with Vault/AWS
- [ ] Audit log verification tests
- [ ] Emergency revocation tests

---

### 3. [HIGH] Input Validation Framework Expansion

**Type**: Security
**Priority**: High
**Effort**: 12 hours
**Estimated Story Points**: 8

**Description**:
Current input validation is scattered across modules. Create comprehensive framework for all user input validation with consistent error handling.

**Current State**:
- Basic validation in some functions
- Inconsistent error messages
- No centralized validation rules
- Edge cases not tested

**Expected State**:
- Centralized validation framework (`/src/lib/validation/`)
- Validator for each input type (email, URL, file, JSON, etc.)
- Consistent error responses
- Comprehensive test coverage

**Files Affected**:
- `/src/lib/utils/validation.sh` (expand)
- `/src/lib/validation/` (new directory with modules)
- All CLI commands using user input

**Acceptance Criteria**:
- [ ] Validation framework documented
- [ ] All validators implemented
- [ ] Unit tests for all validators
- [ ] Integration tests with CLI commands
- [ ] Error handling standardized

---

### 4. [HIGH] rm -rf Safety Mechanisms

**Type**: Security
**Priority**: High
**Effort**: 6 hours
**Estimated Story Points**: 3

**Description**:
Several scripts use `rm -rf` without adequate safety checks. Could cause data loss if paths are incorrect.

**Current State**:
- `rm -rf` used in cleanup scripts without validation
- No sanity checks on paths
- No dry-run mode
- No recovery mechanism

**Expected State**:
- Safe delete wrapper function
- Path validation before deletion
- Dry-run mode for dangerous operations
- Automatic backups before deletion
- Recovery capability

**Files Affected**:
- `/src/lib/utils/` (new: safe-delete.sh)
- `/src/cli/clean.sh`
- `/src/cli/reset.sh`
- `/src/lib/init/cleanup.sh`

**Acceptance Criteria**:
- [ ] Safe delete wrapper created
- [ ] All `rm -rf` uses updated
- [ ] Dry-run mode implemented
- [ ] Backup before delete working
- [ ] Tests with real scenarios

---

### 5. [MEDIUM] eval() Usage Documentation and Replacement

**Type**: Security
**Priority**: Medium
**Effort**: 10 hours
**Estimated Story Points**: 5

**Description**:
Document all eval() usage and replace unsafe instances with safer alternatives.

**Current State**:
- Some eval() usage in script execution
- Limited documentation
- Potential for code injection
- Not all uses verified as safe

**Expected State**:
- All eval() documented with security justification
- Unsafe instances replaced
- Safe alternatives documented
- Code review checklist for eval()

**Files Affected**:
- `/src/lib/service-init/generator.sh`
- `/src/cli/deploy.sh`
- All files using eval (to be identified)

**Acceptance Criteria**:
- [ ] All eval() instances cataloged
- [ ] Security risk assessment for each
- [ ] Unsafe instances refactored
- [ ] Documentation complete
- [ ] Code review guidelines updated

---

### 6. [MEDIUM] File Upload Security Hardening

**Type**: Security
**Priority**: Medium
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Enhance file upload security with additional validation, scanning, and rate limiting.

**Current State**:
- Basic file type validation
- No virus scanning integration
- Limited rate limiting
- No duplicate detection
- Minimal content inspection

**Expected State**:
- ClamAV integration for virus scanning
- Advanced content inspection
- Rate limiting per user/IP
- Hash-based duplicate detection
- Comprehensive audit logging

**Files Affected**:
- `/src/lib/storage/upload.sh` (enhance)
- `/src/lib/storage/validation.sh` (expand)
- `/src/lib/storage/processing.sh` (expand)

**Acceptance Criteria**:
- [ ] Virus scanning working
- [ ] Content inspection implemented
- [ ] Rate limiting enforced
- [ ] Duplicate detection functional
- [ ] Audit logging complete
- [ ] Tests covering all scenarios

---

### 7. [MEDIUM] OAuth Provider Security Audit

**Type**: Security
**Priority**: Medium
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Complete security audit of OAuth provider implementations, token handling, and state validation.

**Current State**:
- OAuth framework implemented (v0.9.0)
- PKCE support for mobile apps
- Token management basic
- Limited provider-specific handling

**Expected State**:
- Security audit of all providers
- Enhanced token refresh flow
- Scope validation
- Provider-specific claim mapping verified
- Automatic token rotation

**Files Affected**:
- `/src/lib/oauth/providers.sh`
- `/src/lib/auth/oauth-manager.sh` (new)
- `/src/tests/integration/test-oauth.sh`

**Acceptance Criteria**:
- [ ] Security audit documented
- [ ] All providers reviewed
- [ ] Token flow secure
- [ ] Scope validation working
- [ ] Tests comprehensive

---

### 8. [LOW] Encryption Key Management

**Type**: Security
**Priority**: Low
**Effort**: 8 hours
**Estimated Story Points**: 4

**Description**:
Implement proper encryption key management with KMS integration and key rotation.

**Current State**:
- Basic encryption support
- Keys in environment variables
- No rotation mechanism
- Limited KMS integration

**Expected State**:
- AWS KMS or Azure Key Vault integration
- Automatic key rotation (yearly)
- Key versioning
- Transparent re-encryption on rotation

**Files Affected**:
- `/src/lib/crypto/` (new directory)
- `/src/lib/config/` (key management)
- `/src/cli/config.sh` (key commands)

**Acceptance Criteria**:
- [ ] KMS integration working
- [ ] Key rotation automated
- [ ] Re-encryption process verified
- [ ] Tests passing

---

## CATEGORY 2: CODE QUALITY ISSUES (14 Issues)

### 9. [HIGH] Function Complexity Reduction - build/core.sh

**Type**: Code Quality
**Priority**: High
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
`/src/lib/build/core.sh` is 1,037 lines and has high complexity. Split into focused modules.

**Current State**:
- Monolithic 1,037-line file
- Mixed concerns (generation, validation, optimization)
- High cyclomatic complexity
- Difficult to test and maintain

**Expected State**:
- Split into modules:
  - `compose-builder.sh` (500 lines)
  - `nginx-builder.sh` (300 lines)
  - `validation.sh` (150 lines)
  - `optimization.sh` (100 lines)
- Each module single responsibility
- Easier to test
- Better maintainability

**Files Affected**:
- `/src/lib/build/core.sh` (split into 4 files)
- `/src/lib/build/` (new modular structure)

**Acceptance Criteria**:
- [ ] Core.sh split into modules
- [ ] All functions moved with no loss
- [ ] Tests updated and passing
- [ ] Documentation updated
- [ ] No new functionality added

---

### 10. [HIGH] Function Complexity Reduction - ssl/ssl.sh

**Type**: Code Quality
**Priority**: High
**Effort**: 14 hours
**Estimated Story Points**: 7

**Description**:
`/src/lib/ssl/ssl.sh` is 938 lines. Split by functionality.

**Current State**:
- Monolithic 938-line file
- Mixed certificate generation, validation, installation
- Complex certificate lifecycle management
- Difficult to understand and modify

**Expected State**:
- Split into modules:
  - `certificate-generator.sh` (400 lines)
  - `certificate-validator.sh` (250 lines)
  - `certificate-installer.sh` (200 lines)
  - `renewals.sh` (150 lines)

**Files Affected**:
- `/src/lib/ssl/ssl.sh` (split into 4 files)
- `/src/lib/ssl/` (new modular structure)

**Acceptance Criteria**:
- [ ] SSL.sh split into modules
- [ ] All functions working
- [ ] Tests updated
- [ ] Documentation complete

---

### 11. [HIGH] Duplicate Code Consolidation - auto-fix vs autofix

**Type**: Code Quality
**Priority**: High
**Effort**: 10 hours
**Estimated Story Points**: 5

**Description**:
Two directories with overlapping functionality: `src/lib/auto-fix/` (21 files) and `src/lib/autofix/` (9 files). Consolidate into single directory.

**Current State**:
- Two separate directories with similar names
- Overlapping functionality
- Confusion about which to use
- Code duplication

**Expected State**:
- Single canonical `src/lib/autofix/` directory
- All functionality consolidated
- No duplicates
- Clear organization

**Files Affected**:
- `src/lib/auto-fix/` (30 files to consolidate)
- `src/lib/autofix/` (keep this, merge into here)
- All references to old directory

**Acceptance Criteria**:
- [ ] Duplicate code identified
- [ ] Best version of each file selected
- [ ] All files moved/merged
- [ ] Tests updated
- [ ] All references updated
- [ ] Old directory removed

---

### 12. [MEDIUM] Error Handling Standardization

**Type**: Code Quality
**Priority**: Medium
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Error handling inconsistent across modules. Create standard error handling framework.

**Current State**:
- Different error patterns in different modules
- Inconsistent exit codes
- No standard error messages
- Limited error context

**Expected State**:
- Standard error function: `error()`
- Standard warning function: `warn()`
- Standard exit codes (1=error, 2=usage, etc.)
- Consistent error messages with context
- Structured logging

**Files Affected**:
- `/src/lib/utils/error-handler.sh` (new)
- All module files (to be updated)

**Acceptance Criteria**:
- [ ] Error framework created
- [ ] All modules updated
- [ ] Consistent exit codes used
- [ ] Tests updated
- [ ] Documentation complete

---

### 13. [MEDIUM] Code Duplication Analysis and Consolidation

**Type**: Code Quality
**Priority**: Medium
**Effort**: 20 hours
**Estimated Story Points**: 10

**Description**:
Comprehensive code duplication audit and consolidation. Found duplicate logic across 40+ files.

**Current State**:
- Duplicate validation logic
- Duplicate formatting functions
- Duplicate docker operations
- Estimated 15-20% code duplication

**Expected State**:
- All duplication identified
- Shared utilities library created
- Single source of truth for common operations
- 5-10% code duplication target

**Files Affected**:
- Multiple files with duplicated code (40+)
- `/src/lib/utils/shared.sh` (new library)

**Acceptance Criteria**:
- [ ] Duplication audit completed
- [ ] Shared library created
- [ ] All duplicates refactored
- [ ] Tests passing
- [ ] Code review confirms consolidation

---

### 14. [MEDIUM] Logging Standardization

**Type**: Code Quality
**Priority**: Medium
**Effort**: 8 hours
**Estimated Story Points**: 4

**Description**:
Logging inconsistent across modules. Create standard logging framework.

**Current State**:
- Different logging approaches
- No consistent format
- Mixed echo, printf, debug, info levels
- No centralized log management

**Expected State**:
- Standard logging functions (debug, info, warn, error)
- Consistent log format with timestamps
- Log level filtering
- Structured logging support

**Files Affected**:
- `/src/lib/observability/logging.sh` (enhance)
- All modules (to be updated)

**Acceptance Criteria**:
- [ ] Logging framework designed
- [ ] All modules updated
- [ ] Log levels working
- [ ] Tests passing
- [ ] Documentation complete

---

### 15. [MEDIUM] Performance Optimization - Database Query Analysis

**Type**: Code Quality
**Priority**: Medium
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Analyze and optimize database queries, especially in billing and tenant modules.

**Current State**:
- No query optimization
- N+1 query problems possible
- No query indexing analysis
- Limited query monitoring

**Expected State**:
- Query performance analysis
- Optimized queries (use JOINs, avoid N+1)
- Query monitoring in place
- Performance benchmarks

**Files Affected**:
- `/src/lib/billing/quotas.sh` (optimize)
- `/src/lib/billing/core.sh` (optimize)
- `/src/lib/multi-tenancy/` (optimize)

**Acceptance Criteria**:
- [ ] Queries analyzed
- [ ] Slow queries identified
- [ ] Optimizations implemented
- [ ] Benchmarks show improvement
- [ ] Tests passing

---

### 16. [MEDIUM] Comment Density and Documentation Improvement

**Type**: Code Quality
**Priority**: Medium
**Effort**: 10 hours
**Estimated Story Points**: 5

**Description**:
Some complex functions lack sufficient comments explaining logic.

**Current State**:
- Variable comment density (10-40% of code)
- Some functions underdocumented
- Logic not always clear

**Expected State**:
- Minimum 25% comment density
- All complex functions documented
- Clear explanation of algorithms
- Consistent documentation style

**Files Affected**:
- Complex modules (15+ files identified)
- `/src/lib/build/core.sh` (complex logic)
- `/src/lib/ssl/ssl.sh` (complex logic)
- `/src/lib/billing/quotas.sh` (complex logic)

**Acceptance Criteria**:
- [ ] All complex functions documented
- [ ] Comments explain WHY not just WHAT
- [ ] Documentation style consistent
- [ ] Code review approves comments

---

### 17. [LOW] Shell Script Best Practices Enforcement

**Type**: Code Quality
**Priority**: Low
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Enforce shell script best practices throughout codebase (quoting, set -e, etc.).

**Current State**:
- Inconsistent best practices
- Some unsafe patterns
- Limited shellcheck integration

**Expected State**:
- Consistent best practices
- All scripts pass shellcheck
- Proper quoting throughout
- Set -e/set -u standards

**Files Affected**:
- All shell scripts in `/src/`

**Acceptance Criteria**:
- [ ] Shellcheck passes all scripts
- [ ] Best practices documented
- [ ] Code review guidelines updated

---

### 18. [LOW] Test Coverage Expansion

**Type**: Code Quality
**Priority**: Low
**Effort**: 24 hours
**Estimated Story Points**: 12

**Description**:
Expand test coverage from 34% to 70%+.

**Current State**:
- 47 test files
- 34% coverage (estimated)
- Many edge cases untested
- Integration tests limited

**Expected State**:
- 70%+ coverage
- Edge cases covered
- Integration tests comprehensive
- Performance tests added

**Files Affected**:
- All source files
- `/src/tests/` (expand)

**Acceptance Criteria**:
- [ ] Coverage at 70%+
- [ ] Edge cases covered
- [ ] Tests passing

---

### 19. [LOW] Type Checking Framework (Pre-commit)

**Type**: Code Quality
**Priority**: Low
**Effort**: 8 hours
**Estimated Story Points**: 4

**Description**:
Add optional type checking for shell scripts using ShellCheck and custom validators.

**Current State**:
- No type checking for shell
- No variable type validation
- Error handling could be safer

**Expected State**:
- ShellCheck integration
- Custom type validators
- Pre-commit hooks

**Files Affected**:
- `.github/` (pre-commit config)
- `/src/lib/utils/` (type validators)

**Acceptance Criteria**:
- [ ] ShellCheck integrated
- [ ] Custom validators created
- [ ] Tests passing

---

### 20. [LOW] Code Review Checklist Automation

**Type**: Code Quality
**Priority**: Low
**Effort**: 6 hours
**Estimated Story Points**: 3

**Description**:
Automate code review checklist checks in CI/CD.

**Current State**:
- Manual code review checklist
- No automated verification
- Easy to miss items

**Expected State**:
- Automated checks in CI
- PR comments for failures
- Checklist items as tests

**Files Affected**:
- `.github/workflows/` (new checks)

**Acceptance Criteria**:
- [ ] Automated checks working
- [ ] All items verified
- [ ] Tests passing

---

### 22. [LOW] Refactoring Opportunity - DRY Principle Violations

**Type**: Code Quality
**Priority**: Low
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Multiple violations of DRY (Don't Repeat Yourself) principle with repeated patterns.

**Current State**:
- Similar patterns in 15+ files
- Copy-paste implementations
- Maintenance burden

**Expected State**:
- Shared pattern libraries
- Single source of truth
- Reduced maintenance burden

**Files Affected**:
- Service management (5 files)
- Docker operations (6 files)
- Configuration (4 files)

**Acceptance Criteria**:
- [ ] Pattern library created
- [ ] All references refactored
- [ ] Tests passing
- [ ] Maintenance easier

---

## CATEGORY 3: MISSING FEATURES (12 Issues)

### 23. [HIGH] S3/GCS Backup Export Feature

**Type**: Feature
**Priority**: High
**Effort**: 20 hours
**Estimated Story Points**: 10

**Description**:
Add capability to export backups to AWS S3 or Google Cloud Storage for off-site backup and compliance.

**Current State**:
- Local backup only (`nself backup create`)
- No cloud export
- No off-site redundancy

**Expected State**:
- S3 export option
- GCS export option
- Automatic S3/GCS backups
- Backup encryption for cloud
- Backup retention policies

**Files Affected**:
- `/src/cli/backup.sh` (new options)
- `/src/lib/backup/` (new cloud modules)
- `/src/lib/backup/s3-export.sh` (new)
- `/src/lib/backup/gcs-export.sh` (new)

**Acceptance Criteria**:
- [ ] S3 export working
- [ ] GCS export working
- [ ] Encryption verified
- [ ] Retention policies enforced
- [ ] Tests passing

**Related Issues**:
- Feature: Cloud backup integration

---

### 24. [HIGH] Client SDK Generation

**Type**: Feature
**Priority**: High
**Effort**: 32 hours
**Estimated Story Points**: 16

**Description**:
Complete implementation of client SDK generation for multiple languages (TypeScript, Python, Go, etc.).

**Current State**:
- Framework exists (v0.9.0)
- Templates partially implemented
- Missing language support (Swift, Java, etc.)

**Expected State**:
- TypeScript SDK generation
- Python SDK generation
- Go client generation
- Swift SDK generation
- Java/Kotlin client generation
- React hooks for data fetching
- Tests for generated code

**Files Affected**:
- `/src/lib/service-init/generator.sh` (expand)
- `/src/templates/sdks/` (new templates)
- `/src/tests/integration/test-sdk-generation.sh` (new)

**Acceptance Criteria**:
- [ ] All language SDKs generated
- [ ] Generated code working
- [ ] Documentation complete
- [ ] Tests comprehensive

**Related Issues**:
- Feature: Service Code Generation (v0.9.0)
- Dependency: GraphQL schema introspection

---

### 25. [HIGH] PDF Compliance Reports

**Type**: Feature
**Priority**: High
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Generate PDF reports for compliance audits, security assessments, and billing verification.

**Current State**:
- No PDF report generation
- Manual compliance documentation
- No audit trail reports

**Expected State**:
- Billing summary PDF reports
- Security audit PDF reports
- Compliance certification reports
- Usage analytics reports

**Files Affected**:
- `/src/cli/reports.sh` (new command)
- `/src/lib/reporting/` (new directory)
- `/src/lib/reporting/pdf-generator.sh` (new)

**Acceptance Criteria**:
- [ ] PDF generation working
- [ ] All report types available
- [ ] Branding customizable
- [ ] Tests passing

---

### 26. [MEDIUM] Advanced Password Management (AUTH-004)

**Type**: Feature
**Priority**: Medium
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Implement password manager integration and credential vaulting for secure credential handling.

**Current State**:
- Basic password hashing
- No password manager integration
- Limited credential handling

**Expected State**:
- 1Password integration
- Bitwarden integration
- Credential rotation
- Secure password recovery

**Files Affected**:
- `/src/lib/auth/password-manager.sh` (new)
- `/src/cli/auth.sh` (new subcommands)

**Acceptance Criteria**:
- [ ] Password manager integration working
- [ ] Credential rotation functional
- [ ] Tests passing
- [ ] Documentation complete

---

### 27. [MEDIUM] Tenant-Specific SMTP Configuration

**Type**: Feature
**Priority**: Medium
**Effort**: 10 hours
**Estimated Story Points**: 5

**Description**:
Allow per-tenant SMTP configuration for email sending with custom branding.

**Current State**:
- Global SMTP configuration only
- No per-tenant customization
- Limited email branding

**Expected State**:
- Per-tenant SMTP settings
- Per-tenant email templates
- Per-tenant sender identity
- Fallback to global SMTP

**Files Affected**:
- `/src/lib/email/tenant-smtp.sh` (new)
- `/src/lib/whitelabel/email.sh` (enhance)
- `/src/cli/email.sh` (new options)

**Acceptance Criteria**:
- [ ] Tenant SMTP working
- [ ] Fallback mechanism functional
- [ ] Tests passing
- [ ] Documentation complete

---

### 28. [MEDIUM] Billing Notifications and Reminders

**Type**: Feature
**Priority**: Medium
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Implement billing notifications, payment reminders, and dunning management.

**Current State**:
- Basic billing system (v0.9.0)
- No notifications
- No payment reminders
- No dunning process

**Expected State**:
- Email notifications for invoices
- Payment reminder emails
- Dunning process for overdue payments
- Customizable notification templates
- Notification preference management

**Files Affected**:
- `/src/lib/billing/notifications.sh` (new)
- `/src/lib/billing/dunning.sh` (new)
- `/src/cli/billing.sh` (new subcommands)

**Acceptance Criteria**:
- [ ] Notifications sending
- [ ] Dunning process working
- [ ] Templates customizable
- [ ] Tests passing

---

### 29. [MEDIUM] Advanced Analytics and Business Intelligence

**Type**: Feature
**Priority**: Medium
**Effort**: 24 hours
**Estimated Story Points**: 12

**Description**:
Implement advanced analytics, reporting, and business intelligence features.

**Current State**:
- Basic usage tracking
- Limited analytics
- No forecasting

**Expected State**:
- Revenue forecasting
- Churn prediction
- Customer segmentation
- Cohort analysis
- Custom reporting dashboard
- Data export for BI tools

**Files Affected**:
- `/src/lib/analytics/` (new directory)
- `/src/cli/analytics.sh` (new command)
- `/src/lib/analytics/forecasting.sh` (new)

**Acceptance Criteria**:
- [ ] Analytics working
- [ ] All features implemented
- [ ] Forecasting accurate
- [ ] Tests passing

---

### 30. [MEDIUM] WebSocket Real-Time Updates for Monitoring

**Type**: Feature
**Priority**: Medium
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Add WebSocket support for real-time monitoring dashboards and status updates.

**Current State**:
- HTTP polling only for status
- No real-time updates
- Monitoring laggy

**Expected State**:
- WebSocket server for real-time data
- Live service status updates
- Real-time log streaming
- Real-time metrics
- Browser-based dashboard

**Files Affected**:
- `/src/lib/realtime/websocket.sh` (new)
- `/src/services/websocket-server/` (new)
- `/src/cli/monitor.sh` (enhance)

**Acceptance Criteria**:
- [ ] WebSocket server running
- [ ] Real-time updates working
- [ ] Dashboard functional
- [ ] Tests passing

---

### 31. [LOW] Kubernetes Helm Chart Generator

**Type**: Feature
**Priority**: Low
**Effort**: 20 hours
**Estimated Story Points**: 10

**Description**:
Auto-generate Kubernetes Helm charts from nself configuration.

**Current State**:
- Docker Compose configuration only
- No Kubernetes support

**Expected State**:
- Helm chart generation
- Kubernetes deployment manifests
- Helm values customization
- Kubectl integration

**Files Affected**:
- `/src/lib/deployment/helm-generator.sh` (new)
- `/src/cli/deploy.sh` (new options)

**Acceptance Criteria**:
- [ ] Helm charts generated
- [ ] K8s deployment working
- [ ] Values customizable
- [ ] Tests passing

---

### 32. [LOW] Terraform Module Generation

**Type**: Feature
**Priority**: Low
**Effort**: 24 hours
**Estimated Story Points**: 12

**Description**:
Generate Terraform modules for infrastructure as code.

**Current State**:
- Manual infrastructure setup
- No IaC support

**Expected State**:
- Terraform module generation
- AWS/GCP/Azure support
- State management
- Variable customization

**Files Affected**:
- `/src/lib/deployment/terraform-generator.sh` (new)
- `/src/cli/deploy.sh` (new options)

**Acceptance Criteria**:
- [ ] Terraform modules generated
- [ ] IaC working
- [ ] Variables customizable
- [ ] Tests passing

---

### 34. [LOW] Mobile App Templates

**Type**: Feature
**Priority**: Low
**Effort**: 32 hours
**Estimated Story Points**: 16

**Description**:
Generate mobile app templates (React Native, Flutter) from schema.

**Current State**:
- No mobile templates

**Expected State**:
- React Native starter
- Flutter starter
- Native iOS (Swift) template
- Native Android (Kotlin) template
- Automatic API client generation

**Files Affected**:
- `/src/templates/mobile/` (new directory)
- `/src/lib/service-init/mobile-generator.sh` (new)

**Acceptance Criteria**:
- [ ] Templates working
- [ ] Starters building
- [ ] API client generated
- [ ] Tests passing

---

## CATEGORY 4: TESTING IMPROVEMENTS (7 Issues)

### 35. [HIGH] Kubernetes/Helm Test Implementation

**Type**: Testing
**Priority**: High
**Effort**: 20 hours
**Estimated Story Points**: 10

**Description**:
Implement comprehensive tests for Kubernetes and Helm deployments.

**Current State**:
- Docker Compose tests only
- No K8s testing
- No Helm testing

**Expected State**:
- K8s manifest validation tests
- Helm template rendering tests
- Deployment verification tests
- Pod health checks
- Service connectivity tests

**Files Affected**:
- `/src/tests/integration/test-kubernetes.sh` (new)
- `/src/tests/integration/test-helm.sh` (new)

**Acceptance Criteria**:
- [ ] K8s tests implemented
- [ ] Helm tests working
- [ ] Tests passing
- [ ] Coverage adequate

---

### 36. [HIGH] Realtime Function Integration Tests

**Type**: Testing
**Priority**: High
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Implement tests for real-time collaboration features.

**Current State**:
- Basic realtime tests
- Limited integration testing
- Edge cases untested

**Expected State**:
- Comprehensive realtime tests
- Concurrent user tests
- Conflict resolution tests
- Performance benchmarks
- Edge case coverage

**Files Affected**:
- `/src/tests/integration/test-realtime.sh` (enhance)
- `/src/tests/benchmarks/realtime-bench.sh` (new)

**Acceptance Criteria**:
- [ ] All scenarios tested
- [ ] Concurrent users working
- [ ] Conflicts resolved correctly
- [ ] Performance acceptable

---

### 37. [HIGH] Deploy Server Management Tests

**Type**: Testing
**Priority**: High
**Effort**: 14 hours
**Estimated Story Points**: 7

**Description**:
Implement tests for server management, deployment, and remote operations.

**Current State**:
- Basic deployment tests
- Limited server management testing
- No remote operation tests

**Expected State**:
- Server provisioning tests
- Remote deployment tests
- Health check tests
- Failover tests
- Rollback tests

**Files Affected**:
- `/src/tests/integration/test-deploy-server.sh` (new)
- `/src/tests/integration/test-remote-operations.sh` (new)

**Acceptance Criteria**:
- [ ] Server tests working
- [ ] Deployment tested
- [ ] Failover working
- [ ] Tests passing

---

### 38. [MEDIUM] Performance and Load Testing

**Type**: Testing
**Priority**: Medium
**Effort**: 20 hours
**Estimated Story Points**: 10

**Description**:
Comprehensive performance and load testing framework.

**Current State**:
- Limited performance testing
- No load testing
- No baseline metrics

**Expected State**:
- Load testing framework
- Performance baselines
- Stress testing
- Scalability verification
- Performance regression tests

**Files Affected**:
- `/src/tests/benchmarks/` (expand)
- `/src/tests/performance/` (new directory)
- `/src/lib/testing/load-generator.sh` (new)

**Acceptance Criteria**:
- [ ] Load tests working
- [ ] Performance measured
- [ ] Baselines established
- [ ] Regression tests in CI

---

### 39. [MEDIUM] Security Regression Testing

**Type**: Testing
**Priority**: Medium
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Implement automated security regression tests.

**Current State**:
- Manual security testing
- No regression test suite
- Vulnerability patterns not tested

**Expected State**:
- Security regression tests
- SQL injection tests
- XSS prevention tests
- CSRF protection tests
- Authentication bypass tests
- Authorization bypass tests

**Files Affected**:
- `/src/tests/security/` (new directory)
- `/src/tests/security/test-sql-injection.sh` (new)
- `/src/tests/security/test-xss.sh` (new)

**Acceptance Criteria**:
- [ ] Security tests comprehensive
- [ ] All OWASP Top 10 tested
- [ ] Tests passing
- [ ] CI integration done

---

### 40. [MEDIUM] Cross-Platform Testing Matrix

**Type**: Testing
**Priority**: Medium
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Expand cross-platform testing across all major platforms.

**Current State**:
- Ubuntu and macOS tested
- Limited Alpine testing
- No RHEL/CentOS testing
- No FreeBSD testing

**Expected State**:
- Ubuntu 20.04, 22.04, 24.04 tested
- macOS 12, 13, 14, 15 tested
- Alpine Linux tested
- RHEL 8, 9 tested
- CentOS tested
- WSL testing

**Files Affected**:
- `.github/workflows/` (expand test matrix)
- `/src/tests/` (platform-specific tests)

**Acceptance Criteria**:
- [ ] All platforms in matrix
- [ ] Tests passing on all
- [ ] CI configured

---

### 41. [LOW] Mutation Testing Implementation

**Type**: Testing
**Priority**: Low
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Implement mutation testing to verify test quality.

**Current State**:
- Code coverage metrics only
- No mutation testing
- Test quality unknown

**Expected State**:
- Mutation testing integrated
- Mutation score > 80%
- Weak tests identified
- Test improvements made

**Files Affected**:
- `.github/workflows/` (mutation test workflow)
- `/src/tests/` (test improvements)

**Acceptance Criteria**:
- [ ] Mutation testing running
- [ ] Score > 80%
- [ ] Weak tests fixed
- [ ] CI integrated

---

## CATEGORY 5: DOCUMENTATION (4 Issues)

### 42. [HIGH] GraphQL API Reference Documentation

**Type**: Documentation
**Priority**: High
**Effort**: 16 hours
**Estimated Story Points**: 8

**Description**:
Complete GraphQL API reference with schema documentation, examples, and playground integration.

**Current State**:
- Limited GraphQL documentation
- No schema documentation
- No query examples
- No authentication guide

**Expected State**:
- Complete schema documentation
- Query examples for common tasks
- Mutation examples
- Subscription examples
- Authentication guide
- Playground integration

**Files Affected**:
- `/docs/api/graphql/` (new directory)
- `/docs/api/graphql/schema.md` (new)
- `/docs/api/graphql/queries.md` (new)
- `/docs/api/graphql/mutations.md` (new)

**Acceptance Criteria**:
- [ ] Schema documented
- [ ] All queries documented
- [ ] All mutations documented
- [ ] Examples working
- [ ] Authentication documented

---

### 43. [HIGH] REST API Endpoints Documentation

**Type**: Documentation
**Priority**: High
**Effort**: 12 hours
**Estimated Story Points**: 6

**Description**:
Document REST API endpoints with examples and error codes.

**Current State**:
- Partial REST API documentation
- No error code reference
- No rate limiting guide

**Expected State**:
- All endpoints documented
- Error codes explained
- Rate limiting guide
- Authentication guide
- Pagination guide
- Filtering guide

**Files Affected**:
- `/docs/api/rest/` (new directory)
- `/docs/api/rest/endpoints.md` (new)
- `/docs/api/rest/errors.md` (new)
- `/docs/api/rest/rate-limiting.md` (new)

**Acceptance Criteria**:
- [ ] All endpoints documented
- [ ] Examples working
- [ ] Error codes listed
- [ ] Rate limiting explained

---

### 44. [MEDIUM] Authentication Flow Diagrams and Guide

**Type**: Documentation
**Priority**: Medium
**Effort**: 10 hours
**Estimated Story Points**: 5

**Description**:
Create comprehensive authentication flow documentation with diagrams.

**Current State**:
- Basic authentication docs
- No flow diagrams
- No sequence diagrams
- No OAuth flow explanation

**Expected State**:
- Flow diagrams (signup, login, logout, refresh)
- Sequence diagrams
- OAuth flow explained
- MFA flow explained
- Session management guide
- Security best practices

**Files Affected**:
- `/docs/guides/authentication-flows.md` (new)
- `/docs/architecture/authentication.md` (enhance)
- Diagrams (ASCII art or images)

**Acceptance Criteria**:
- [ ] All flows documented
- [ ] Diagrams clear
- [ ] Security explained
- [ ] Examples provided

---

### 45. [MEDIUM] Plugin Development Guide

**Type**: Documentation
**Priority**: Medium
**Effort**: 14 hours
**Estimated Story Points**: 7

**Description**:
Complete plugin development guide with examples and best practices.

**Current State**:
- Plugin system minimal
- No development guide
- No examples
- No best practices

**Expected State**:
- Plugin architecture documented
- Plugin development guide
- Working examples (5+)
- Best practices guide
- Testing guide
- Publishing guide

**Files Affected**:
- `/docs/guides/plugin-development.md` (new)
- `/docs/src/examples/plugins/` (new examples)
- `/src/lib/plugin/` (documentation)

**Acceptance Criteria**:
- [ ] Architecture documented
- [ ] Examples working
- [ ] Best practices clear
- [ ] Testing guide complete

---

## SUMMARY TABLE

| # | Issue | Type | Priority | Effort (hrs) | Story Points | Status |
|---|-------|------|----------|-------------|--------------|--------|
| 1 | SQL Injection Verification | Security | CRITICAL | 2 | - | Verify |
| 2 | Secrets Management | Security | HIGH | 8 | 5 | TODO |
| 3 | Input Validation Framework | Security | HIGH | 12 | 8 | TODO |
| 4 | rm -rf Safety | Security | HIGH | 6 | 3 | TODO |
| 5 | eval() Documentation | Security | MEDIUM | 10 | 5 | TODO |
| 6 | File Upload Security | Security | MEDIUM | 16 | 8 | TODO |
| 7 | OAuth Security Audit | Security | MEDIUM | 12 | 6 | TODO |
| 8 | Encryption Key Management | Security | LOW | 8 | 4 | TODO |
| 9 | Simplify build/core.sh | Code Quality | HIGH | 16 | 8 | TODO |
| 10 | Simplify ssl/ssl.sh | Code Quality | HIGH | 14 | 7 | TODO |
| 11 | Consolidate auto-fix dirs | Code Quality | HIGH | 10 | 5 | TODO |
| 12 | Error Handling Standard | Code Quality | MEDIUM | 12 | 6 | TODO |
| 13 | Code Duplication Audit | Code Quality | MEDIUM | 20 | 10 | TODO |
| 14 | Logging Standardization | Code Quality | MEDIUM | 8 | 4 | TODO |
| 15 | DB Query Optimization | Code Quality | MEDIUM | 16 | 8 | TODO |
| 16 | Comment Density | Code Quality | MEDIUM | 10 | 5 | TODO |
| 17 | Shell Best Practices | Code Quality | LOW | 12 | 6 | TODO |
| 18 | Test Coverage to 70% | Code Quality | LOW | 24 | 12 | TODO |
| 19 | Type Checking Framework | Code Quality | LOW | 8 | 4 | TODO |
| 20 | Code Review Automation | Code Quality | LOW | 6 | 3 | TODO |
| 22 | DRY Violations | Code Quality | LOW | 16 | 8 | TODO |
| 23 | S3/GCS Backup Export | Feature | HIGH | 20 | 10 | TODO |
| 24 | Client SDK Generation | Feature | HIGH | 32 | 16 | TODO |
| 25 | PDF Reports | Feature | HIGH | 16 | 8 | TODO |
| 26 | Password Management | Feature | MEDIUM | 12 | 6 | TODO |
| 27 | Tenant SMTP Config | Feature | MEDIUM | 10 | 5 | TODO |
| 28 | Billing Notifications | Feature | MEDIUM | 12 | 6 | TODO |
| 29 | Advanced Analytics | Feature | MEDIUM | 24 | 12 | TODO |
| 30 | WebSocket Real-Time | Feature | MEDIUM | 16 | 8 | TODO |
| 31 | Helm Chart Generator | Feature | LOW | 20 | 10 | TODO |
| 32 | Terraform Generator | Feature | LOW | 24 | 12 | TODO |
| 34 | Mobile Templates | Feature | LOW | 32 | 16 | TODO |
| 35 | K8s/Helm Tests | Testing | HIGH | 20 | 10 | TODO |
| 36 | Realtime Tests | Testing | HIGH | 16 | 8 | TODO |
| 37 | Deploy Server Tests | Testing | HIGH | 14 | 7 | TODO |
| 38 | Performance Testing | Testing | MEDIUM | 20 | 10 | TODO |
| 39 | Security Regression | Testing | MEDIUM | 12 | 6 | TODO |
| 40 | Cross-Platform Matrix | Testing | MEDIUM | 12 | 6 | TODO |
| 41 | Mutation Testing | Testing | LOW | 16 | 8 | TODO |
| 42 | GraphQL API Docs | Documentation | HIGH | 16 | 8 | TODO |
| 43 | REST API Docs | Documentation | HIGH | 12 | 6 | TODO |
| 44 | Auth Flow Docs | Documentation | MEDIUM | 10 | 5 | TODO |
| 45 | Plugin Dev Guide | Documentation | MEDIUM | 14 | 7 | TODO |

---

## Effort Summary

- **Total Issues**: 45
- **Total Effort**: ~500 hours
- **Total Story Points**: 285
- **By Priority**:
  - Critical: 1 issue (2 hours)
  - High: 12 issues (140 hours)
  - Medium: 21 issues (238 hours)
  - Low: 11 issues (120 hours)

- **By Category**:
  - Security: 8 issues (72 hours)
  - Code Quality: 14 issues (166 hours)
  - Features: 12 issues (198 hours)
  - Testing: 7 issues (110 hours)
  - Documentation: 4 issues (52 hours)

---

## Prioritization Recommendation

### Phase 1 (Next Sprint - 40-50 hours)
Focus on security and critical code quality:
1. Secrets Management Enhancement (8 hrs)
2. Input Validation Framework (12 hrs)
3. rm -rf Safety Mechanisms (6 hrs)
4. Consolidate auto-fix dirs (10 hrs)
5. Error Handling Standardization (12 hrs)

### Phase 2 (Following 2 Sprints - 80-100 hours)
Code quality and testing:
1. Simplify build/core.sh (16 hrs)
2. Simplify ssl/ssl.sh (14 hrs)
3. Code Duplication Audit (20 hrs)
4. K8s/Helm Tests (20 hrs)
5. Realtime Tests (16 hrs)

### Phase 3 (Q2 2026 - 120+ hours)
Features and advanced capabilities:
1. Client SDK Generation (32 hrs)
2. S3/GCS Backup Export (20 hrs)
3. Advanced Analytics (24 hrs)
4. Mobile Templates (32 hrs)
5. Full documentation (60+ hrs)

---

## Notes

- Issues marked as "VERIFICATION ONLY" (like SQL Injection #1) only require confirming existing fixes
- Many features depend on other features (SDKs depend on code generation, Helm charts depend on K8s support)
- Documentation should be ongoing alongside feature development
- Security issues should be prioritized across all phases
- Testing improvements should be integrated into sprint velocity

---

**End of GitHub Issues Document**

This document is ready for import into GitHub Issues. Each issue can be created individually or in batch using GitHub CLI with:
```bash
gh issue create --title "..." --body "..." --labels "security,high-priority"
```
