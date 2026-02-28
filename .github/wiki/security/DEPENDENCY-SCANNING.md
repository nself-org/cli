# Dependency Scanning Guide

This document describes the comprehensive dependency and security scanning implemented in nself's CI/CD pipeline.

## Overview

nself implements multiple layers of security scanning to ensure the safety and integrity of the codebase and generated services.

## Security Scanning Tools

### 1. ShellCheck (Shell Script Security)

**Purpose**: Detect security issues and bugs in shell scripts

**What it checks**:
- Command injection vulnerabilities
- Unquoted variables that could lead to injection
- Use of eval and other dangerous constructs
- Path traversal vulnerabilities
- Improper error handling

**CI Integration**: Runs on every push and PR
**Local Usage**:
```bash
# Install
brew install shellcheck  # macOS
sudo apt install shellcheck  # Ubuntu

# Run
shellcheck -S error src/**/*.sh
```

### 2. Gitleaks (Secret Scanning)

**Purpose**: Detect secrets, passwords, and API keys in code and git history

**What it checks**:
- AWS keys and secrets
- API tokens
- Private keys
- Database passwords
- OAuth tokens
- Generic secrets (high entropy strings)

**CI Integration**: Runs on every push with full git history
**Local Usage**:
```bash
# Install
brew install gitleaks  # macOS

# Run on entire repository
gitleaks detect --source . --verbose

# Scan specific commits
gitleaks detect --source . --log-opts="--since=1.week"
```

### 3. TruffleHog (Advanced Secret Scanning)

**Purpose**: Find secrets with verification (checks if secrets are valid)

**What it checks**:
- Verified secrets (actually tests if they work)
- 700+ credential types
- Cloud provider keys
- Database connection strings
- Generic high-entropy secrets

**CI Integration**: Runs on every push and PR
**Local Usage**:
```bash
# Install
brew install trufflesecurity/trufflehog/trufflehog  # macOS

# Scan repository
trufflehog git file://. --only-verified

# Scan since last commit
trufflehog git file://. --since-commit HEAD~1
```

### 4. Trivy (Container & Dependency Scanning)

**Purpose**: Comprehensive vulnerability scanner for containers and dependencies

**What it scans**:
- Docker images for CVEs
- Operating system packages
- Application dependencies
- Misconfigurations
- License compliance

**CI Integration**: Runs on push, PRs, and daily schedule
**Local Usage**:
```bash
# Install
brew install aquasecurity/trivy/trivy  # macOS

# Scan filesystem
trivy fs .

# Scan Docker image
trivy image nginx:latest

# Scan specific Dockerfile
trivy config Dockerfile
```

### 5. Semgrep (SAST - Static Application Security Testing)

**Purpose**: Find security vulnerabilities and code quality issues

**What it checks**:
- OWASP Top 10 vulnerabilities
- SQL injection patterns
- XSS vulnerabilities
- Command injection
- Path traversal
- Insecure cryptography
- Docker security issues

**CI Integration**: Runs on every push and PR
**Local Usage**:
```bash
# Install
brew install semgrep  # macOS
pip install semgrep  # Python

# Run security audit
semgrep --config=p/security-audit .

# Run OWASP Top 10 checks
semgrep --config=p/owasp-top-ten .

# Run Docker checks
semgrep --config=p/docker .
```

### 6. Hadolint (Dockerfile Linting)

**Purpose**: Best practice and security linting for Dockerfiles

**What it checks**:
- Best practice violations
- Security issues
- Image optimization
- Layer caching
- Non-root users
- HEALTHCHECK instructions

**Pre-commit Hook**: Runs automatically before commit
**Local Usage**:
```bash
# Install
brew install hadolint  # macOS

# Scan Dockerfile
hadolint Dockerfile

# Scan all Dockerfiles
find . -name Dockerfile -exec hadolint {} \;
```

## CI/CD Pipeline

### Workflow: `.github/workflows/security-scan.yml`

Comprehensive security scanning workflow that runs:
- **On every push** to main/develop
- **On every pull request**
- **Daily at 2 AM UTC** (scheduled scan)
- **Manually** via workflow_dispatch

### Jobs

#### 1. ShellCheck Security
- Scans all `.sh` files
- Fails on security errors
- Reports security-related warnings

#### 2. Secret Scanning
- Runs Gitleaks on full git history
- Runs TruffleHog with verification
- Uploads results to GitHub Security tab

#### 3. Dependency Scanning
- Scans entire filesystem for vulnerabilities
- Scans Docker base images used in templates
- Uploads SARIF results to GitHub Security

#### 4. Container Image Scanning
- Builds representative test images
- Scans with Trivy for CVEs
- Reports HIGH and CRITICAL vulnerabilities

#### 5. SAST (Static Analysis)
- Runs Semgrep with multiple rulesets
- Checks for OWASP Top 10 issues
- Analyzes Docker configurations

#### 6. License Compliance
- Verifies LICENSE file exists
- Checks for GPL/AGPL dependencies
- Reports restrictive licenses

#### 7. Docker Security Benchmark
- Validates Dockerfile security
- Checks for non-root users
- Verifies HEALTHCHECK instructions
- Detects hardcoded secrets

#### 8. Security Report
- Aggregates all scan results
- Generates summary report
- Uploads as artifact (90-day retention)

## Pre-commit Hooks

Install pre-commit hooks for local security scanning:

```bash
# Install pre-commit
pip install pre-commit

# Install hooks
pre-commit install

# Run manually on all files
pre-commit run --all-files
```

### Configured Hooks

1. **detect-secrets**: Find secrets before commit
2. **shellcheck**: Lint shell scripts
3. **hadolint**: Lint Dockerfiles
4. **check-yaml/json**: Validate syntax
5. **check-added-large-files**: Prevent large files
6. **Custom checks**:
   - Check .env files for secrets
   - Validate Dockerfile security
   - Detect hardcoded IPs

## Security Scanning Best Practices

### For Developers

1. **Run pre-commit hooks** before committing
2. **Fix CRITICAL issues immediately**
3. **Review HIGH severity issues** before merging
4. **Keep dependencies updated**
5. **Never commit secrets** (use .env files properly)

### For CI/CD

1. **Security scans run automatically** on every push
2. **Results uploaded** to GitHub Security tab
3. **SARIF format** for integration with GitHub Advanced Security
4. **Artifacts retained** for 90 days

### For Production

1. **Scan container images** before deployment
2. **Use specific version tags**, not `:latest`
3. **Run containers as non-root**
4. **Enable HEALTHCHECK** instructions
5. **Minimize base image size**

## Vulnerability Severity Levels

### CRITICAL
- **Action**: Fix immediately
- **Timeline**: Within 24 hours
- **Examples**: Remote code execution, SQL injection, authentication bypass

### HIGH
- **Action**: Fix in next release
- **Timeline**: Within 1 week
- **Examples**: XSS, CSRF, information disclosure

### MEDIUM
- **Action**: Schedule fix
- **Timeline**: Within 1 month
- **Examples**: Missing security headers, weak cryptography

### LOW
- **Action**: Track and fix when convenient
- **Timeline**: Backlog
- **Examples**: Deprecated functions, code quality issues

## Scan Results Location

### GitHub Security Tab
- Navigate to: `Repository → Security → Code scanning`
- View all security alerts
- Filter by severity
- Track remediation status

### GitHub Actions Artifacts
- Navigate to: `Actions → Security Scan workflow`
- Download security-report.md
- Review detailed findings

### Local Scan Results
```bash
# ShellCheck
shellcheck src/**/*.sh 2>&1 | tee shellcheck-results.txt

# Trivy
trivy fs . --format json --output trivy-results.json

# Semgrep
semgrep --config=p/security-audit . --json --output semgrep-results.json
```

## False Positives

### Suppressing False Positives

**ShellCheck**:
```bash
# Disable specific check for one line
# shellcheck disable=SC2086
command $variable

# Disable for entire file
# shellcheck disable=SC2086
```

**Semgrep**:
```yaml
# .semgrepignore
# Ignore specific paths
tests/
*.test.sh
```

**Trivy**:
```yaml
# .trivyignore
# Ignore specific CVE
CVE-2023-12345
```

## Security Contact

For security vulnerabilities, please email: security@nself.org

**Do NOT open public issues for security vulnerabilities.**

## Additional Resources

- [OWASP Top 10](https://owasp.org/Top10/)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [GitHub Security Features](https://docs.github.com/en/code-security)
- [Trivy Documentation](https://aquasecurity.github.io/trivy/)
- [Semgrep Rules](https://semgrep.dev/explore)

## Compliance

These security scanning tools help nself comply with:
- **OWASP Top 10** security risks
- **CIS Docker Benchmark** for container security
- **NIST Cybersecurity Framework**
- **SOC 2** security controls (CC6, CC7)
- **PCI-DSS Requirement 6** (secure development)

## Roadmap

Future security enhancements:
- [ ] Dependency auto-updates (Dependabot)
- [ ] DAST (Dynamic Application Security Testing)
- [ ] Penetration testing automation
- [ ] Security training integration
- [ ] Automated remediation suggestions

---

**Last Updated**: 2026-01-30
**Version**: 1.0
**Owner**: Security Team
