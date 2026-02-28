# Security Documentation

Complete security documentation, audits, and best practices for nself.

## Overview

nself has undergone comprehensive security auditing to ensure production readiness. This directory contains all security-related documentation, audit reports, and implementation guides.

## Security Status

| Category | Status | Documentation |
|----------|--------|---------------|
| Hardcoded Credentials | ✅ PASS | [Security Audit](SECURITY-AUDIT.md) |
| API Keys & Tokens | ✅ PASS | [Security Audit](SECURITY-AUDIT.md) |
| Command Injection | ✅ PASS | [Input Validation](INPUT_VALIDATION_SECURITY_AUDIT.md) |
| SQL Injection | ✅ PASS | [SQL Safety](SQL-SAFETY.md) |
| Docker Security | ✅ PASS | [Security System](SECURITY-SYSTEM.md) |
| Git History | ✅ PASS | [Security Audit](SECURITY-AUDIT.md) |
| Dependency Scanning | ✅ PASS | [Dependency Scanning](DEPENDENCY-SCANNING.md) |

## Security Audits

### Main Audits
- **[Security Audit](SECURITY-AUDIT.md)** - Comprehensive security audit report
- **[Security System](SECURITY-SYSTEM.md)** - Security system overview
- **[Security Audit Index](SECURITY_AUDIT_INDEX.md)** - Index of all security audits

### Component-Specific Audits
- **[Billing Security Audit](SECURITY-AUDIT-BILLING.md)** - Billing system security
- **[Input Validation Audit](INPUT_VALIDATION_SECURITY_AUDIT.md)** - Input validation security

### Remediation Reports
- **[Security Fix Final Report](SECURITY-FIX-FINAL-REPORT.md)** - Final remediation status
- **[Security Remediation Status](SECURITY-REMEDIATION-STATUS.md)** - Ongoing remediation tracking

## SQL Security

### SQL Injection Prevention
- **[SQL Safety](SQL-SAFETY.md)** - SQL safety best practices
- **[SQL Injection Fixes](SQL-INJECTION-FIXES.md)** - Implemented fixes
- **[SQL Injection Fix Summary](SQL-INJECTION-FIX-SUMMARY.md)** - Fix summary
- **[SQL Injection Remediation Plan](SQL-INJECTION-REMEDIATION-PLAN.md)** - Remediation plan
- **[SQL Review Remaining](SQL-REVIEW-REMAINING.md)** - Outstanding items
- **[Injection Attack Prevention](INJECTION_ATTACK_PREVENTION_SUMMARY.md)** - Prevention summary

### Implementation Guides
- **[Parameterized Queries Reference](PARAMETERIZED-QUERIES-QUICK-REFERENCE.md)** - Quick reference
- **[Validation Functions Reference](VALIDATION_FUNCTIONS_REFERENCE.md)** - Validation library

## Web Security

### Headers and Configuration
- **[Security Headers](HEADERS.md)** - HTTP security headers implementation
- **[Vault Fix Summary](VAULT-FIX-SUMMARY.md)** - Secrets management fixes

### File Upload Security
- **[File Upload Security](file-upload-security.md)** - Secure file upload implementation

## Dependency Security

- **[Dependency Scanning](DEPENDENCY-SCANNING.md)** - Third-party dependency security

## Quick Reference

### For Developers

```bash
# Always use parameterized queries
psql_query "SELECT * FROM users WHERE id = $1" "$user_id"

# Validate all input
validate_input "$value" "alphanumeric" "username"

# Sanitize file paths
sanitize_path "$user_provided_path"

# Check permissions
check_file_permissions "$file" "600"
```

See **[Validation Functions Reference](VALIDATION_FUNCTIONS_REFERENCE.md)** for complete API.

### Security Best Practices

1. **Never** execute user input directly
2. **Always** use parameterized queries for SQL
3. **Always** validate and sanitize input
4. **Always** check file permissions
5. **Never** commit secrets to git
6. **Always** use environment variables for sensitive data

## Related Documentation

- **[Contributing Guide](../contributing/CONTRIBUTING.md)** - Security requirements for contributors
- **[Development Guide](../contributing/DEVELOPMENT.md)** - Secure development practices
- **[Configuration](../configuration/README.md)** - Secure configuration practices

## Reporting Security Issues

If you discover a security vulnerability, please email: security@nself.org

**Do not** create public GitHub issues for security vulnerabilities.

---

**Last Updated**: January 31, 2026
**Version**: v0.9.6
**Security Audit Status**: ✅ All Critical Issues Resolved
