# SOC 2 Compliance Guide for nself

**Last Updated:** 2026-01-31
**Version:** 0.9.6
**Compliance Framework:** SOC 2 Type II

---

## Overview

SOC 2 (Service Organization Control 2) is an auditing standard developed by AICPA for service providers storing customer data in the cloud. This guide maps nself features to SOC 2 Trust Services Criteria (TSC).

⚠️ **Note:** SOC 2 compliance requires an independent audit. This document helps you prepare.

---

## Trust Services Criteria

### CC1: Control Environment

**Description:** The organization demonstrates a commitment to integrity and ethical values.

#### CC1.1: Integrity and Ethical Values

**nself Implementation:**
- Code of conduct for administrators
- Security policy documentation
- Regular security training

**Evidence:**
```bash
# Document security policies
nself compliance generate-security-policy

# Track policy acknowledgments
nself compliance policy-acknowledgment record \
  --user <user_id> \
  --policy security_policy_v1 \
  --acknowledged-at "2026-01-31"

# Security training tracking
nself compliance training-report --type security
```

#### CC1.2: Board Independence and Oversight

**Requirements:**
- Independent oversight of security practices
- Regular board reporting
- Audit committee involvement

**Documentation Template:**
```yaml
governance:
  board_oversight: true
  security_committee: true
  audit_frequency: quarterly

  reporting:
    - "Quarterly security metrics"
    - "Annual compliance status"
    - "Incident response summaries"
    - "Risk assessment updates"
```

#### CC1.3: Organizational Structure

**Requirements:**
- Clear security roles and responsibilities
- Segregation of duties
- Escalation procedures

**nself RBAC:**
```bash
# Define organizational roles
nself auth roles create "security_admin" \
  --permissions "manage:security,view:audit_logs,manage:users"

nself auth roles create "developer" \
  --permissions "deploy:code,view:logs"

nself auth roles create "auditor" \
  --permissions "view:audit_logs,view:compliance_reports"

# Enforce segregation of duties
# Developers cannot approve their own deployments
# Security admins cannot modify audit logs
```

#### CC1.4: Competence

**Requirements:**
- Staff have adequate skills
- Training and certification programs
- Performance evaluations

**Tracking:**
```sql
CREATE TABLE employee_certifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES auth.users(id),
    certification_name TEXT NOT NULL,
    certification_number TEXT,
    issuing_organization TEXT,
    issued_date DATE,
    expiry_date DATE,
    status TEXT CHECK (status IN ('active', 'expired', 'revoked'))
);
```

#### CC1.5: Accountability

**Requirements:**
- Performance reviews
- Accountability for security objectives
- Consequence management

**Implementation:**
```bash
# Track security incidents per user
nself audit query --group-by user_id --metric security_incidents

# Generate user accountability report
nself compliance user-accountability-report <user_id>
```

---

### CC2: Communication and Information

**Description:** Communication of information necessary to support the system.

#### CC2.1: Internal Communication

**Requirements:**
- Security policies communicated to all staff
- Regular security updates
- Incident notification procedures

**nself Implementation:**
```bash
# Broadcast security announcement
nself admin broadcast --audience all_users \
  --message "New security policy effective 2026-02-01" \
  --priority high

# Track message delivery
nself admin broadcast-status <message_id>
```

#### CC2.2: External Communication

**Requirements:**
- Customer notifications
- Vendor communications
- Regulatory disclosures

**Examples:**
- Security incident notifications
- Planned maintenance windows
- Privacy policy updates
- Terms of service changes

#### CC2.3: Communication Objectives and Responsibilities

**Documentation:**
```yaml
communication_matrix:
  security_incidents:
    internal: "Security team, Management, Legal"
    external: "Affected customers within 24h"
    regulatory: "If applicable, within required timeframe"

  planned_maintenance:
    internal: "All staff 7 days prior"
    external: "Customers 7 days prior"

  privacy_policy_changes:
    external: "Customers 30 days prior to effective date"
```

---

### CC3: Risk Assessment

**Description:** Organization identifies, analyzes, and responds to risks.

#### CC3.1: Risk Identification

**nself Risk Assessment:**
```bash
# Automated risk scanning
nself security scan --comprehensive --output risk_report.pdf

# Identified risks:
# - Weak passwords
# - Outdated dependencies
# - Misconfigured security headers
# - Exposed sensitive endpoints
# - Insufficient backup retention
```

#### CC3.2: Risk Analysis

**Risk Scoring:**
```sql
CREATE TABLE risk_register (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    risk_name TEXT NOT NULL,
    risk_description TEXT NOT NULL,
    risk_category TEXT,
    likelihood TEXT CHECK (likelihood IN ('low', 'medium', 'high', 'critical')),
    impact TEXT CHECK (impact IN ('low', 'medium', 'high', 'critical')),
    risk_score INTEGER GENERATED ALWAYS AS (
        CASE likelihood
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'critical' THEN 4
        END *
        CASE impact
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'critical' THEN 4
        END
    ) STORED,
    mitigation_plan TEXT,
    risk_owner UUID REFERENCES auth.users(id),
    identified_at TIMESTAMPTZ DEFAULT NOW(),
    mitigated_at TIMESTAMPTZ,
    status TEXT DEFAULT 'open'
);
```

#### CC3.3: Risk Response

**Mitigation Strategies:**
1. **Accept:** Low risk, monitoring only
2. **Mitigate:** Implement controls
3. **Transfer:** Insurance, contracts
4. **Avoid:** Change process to eliminate risk

**nself Risk Mitigation:**
```bash
# Generate mitigation plan
nself compliance risk-mitigation-plan --risk-id <id>

# Track remediation
nself compliance risk-remediate <risk_id> \
  --action "Implemented MFA" \
  --completed-by <user_id>
```

#### CC3.4: Fraud Risk

**Requirements:**
- Fraud risk assessment
- Anti-fraud controls
- Monitoring and detection

**Controls:**
```bash
# Anomaly detection
nself security anomaly-detection enable

# Unusual access patterns
nself audit query --anomalies --last 7d

# Geographic access monitoring
nself security geo-fence --allowed-countries "US,CA,UK"
```

---

### CC4: Monitoring Activities

**Description:** Monitoring activities to assess quality of internal control performance.

#### CC4.1: Ongoing and Separate Evaluations

**Continuous Monitoring:**
```bash
# Real-time security monitoring
nself monitor start --metrics security,performance,availability

# Daily security checks
0 8 * * * nself security daily-check

# Quarterly comprehensive audit
nself compliance quarterly-audit --standard soc2
```

#### CC4.2: Evaluation and Communication of Deficiencies

**Process:**
1. Detect deficiency (automated or manual)
2. Document in tracking system
3. Assign remediation owner
4. Set remediation deadline
5. Verify remediation
6. Report to management

**Tracking:**
```bash
# Log control deficiency
nself compliance deficiency record \
  --control-id CC5.2 \
  --description "Password expiry not enforced" \
  --severity high \
  --assigned-to <user_id>

# Remediation tracking
nself compliance deficiency remediate <deficiency_id> \
  --action "Enabled PASSWORD_MAX_AGE_DAYS=90"

# Report to management
nself compliance deficiency-report --status open
```

---

### CC5: Control Activities

**Description:** Control activities that contribute to risk mitigation.

#### CC5.1: Logical and Physical Access Controls

**Logical Access:**
```bash
# User provisioning
nself auth users create <email> \
  --role <role> \
  --mfa-required \
  --approval-required

# Access review (quarterly)
nself auth access-review --output access_review_2026_Q1.xlsx

# Orphaned accounts
nself auth users list --inactive-days 90

# Privilege escalation monitoring
nself audit query --action "role_change" --last 30d
```

**Physical Access:** (If self-hosting)
- Badge access logs
- Visitor logs
- Video surveillance
- After-hours access alerts

#### CC5.2: System Development Lifecycle

**Secure Development:**
```bash
# Code review requirements
nself dev enforce-code-review --min-approvers 2

# Security testing
nself dev security-scan --type sast,dast

# Vulnerability scanning
nself dev vulnerability-scan --output vuln_report.json

# Penetration testing (annual)
nself compliance pentest schedule --vendor <vendor> --date 2026-06-01
```

**Change Management:**
```bash
# Track configuration changes
nself config diff --since "2026-01-01"

# Require approval for production changes
PRODUCTION_CHANGE_APPROVAL_REQUIRED=true

# Emergency change process
nself deploy emergency --approval-override --reason "Critical security patch"
# Note: Emergency overrides logged and reviewed
```

#### CC5.3: Configuration Management

**Baseline Configuration:**
```bash
# Export current configuration
nself config export --baseline production_baseline_2026-01-31.yaml

# Detect configuration drift
nself config drift-detection --baseline production_baseline_2026-01-31.yaml

# Enforce configuration
nself config enforce --policy hardened_baseline.yaml
```

#### CC5.4: Vulnerability Management

**Process:**
1. Automated vulnerability scanning
2. Risk assessment and prioritization
3. Patch management
4. Verification testing

**Implementation:**
```bash
# Automated scanning
nself security vulnerability-scan --schedule daily

# Patch management
nself system update check
nself system update apply --security-only

# Vulnerability dashboard
nself security vulnerability-dashboard
```

---

### CC6: Logical and Physical Access Controls

**Description:** Controls to restrict access to authorized users.

#### CC6.1: Logical Access - Identification and Authentication

**Multi-Factor Authentication:**
```bash
# Enforce MFA globally
MFA_REQUIRED=true
MFA_ENFORCEMENT_LEVEL=strict

# Supported MFA methods
MFA_METHODS=totp,webauthn,sms,email

# MFA enrollment tracking
nself auth mfa enrollment-status --all-users
```

**Password Policy:**
```bash
# Strong password requirements
PASSWORD_MIN_LENGTH=14
PASSWORD_REQUIRE_UPPERCASE=true
PASSWORD_REQUIRE_LOWERCASE=true
PASSWORD_REQUIRE_NUMBERS=true
PASSWORD_REQUIRE_SPECIAL=true
PASSWORD_HISTORY_COUNT=24
PASSWORD_MAX_AGE_DAYS=90
PASSWORD_MIN_AGE_DAYS=1

# Prevent common passwords
PASSWORD_DICTIONARY_CHECK=true

# Account lockout
ACCOUNT_LOCKOUT_THRESHOLD=5
ACCOUNT_LOCKOUT_DURATION_MINUTES=30
```

#### CC6.2: Logical Access - Authorization

**Role-Based Access Control:**
```bash
# Principle of least privilege
nself auth roles audit --show-excessive-permissions

# Access certification (quarterly)
nself auth access-certification --quarter Q1 --year 2026

# Privileged access management
nself auth privileged-access-review
```

#### CC6.3: Logical Access - User Access Termination

**Offboarding Process:**
```bash
# Automated user deprovisioning
nself auth users offboard <user_id> \
  --disable-immediately \
  --revoke-all-sessions \
  --transfer-ownership <new_owner_id> \
  --export-activity-log

# Checklist verification
nself auth offboard verify <user_id>
```

#### CC6.6: Physical Access

**If Self-Hosting:**
```bash
# Physical access log
nself compliance physical-access log \
  --facility datacenter-1 \
  --person <name> \
  --time-in "2026-01-31T09:00:00Z" \
  --time-out "2026-01-31T11:30:00Z" \
  --purpose "Server maintenance"

# Access review
nself compliance physical-access report --month 2026-01
```

#### CC6.7: Credentials

**Secret Management:**
```bash
# Credential inventory
nself secrets inventory

# Rotation schedule
nself secrets rotation-schedule

# Emergency credential rotation
nself secrets rotate-all --emergency

# Service account management
nself auth service-accounts list
nself auth service-accounts rotate <account_id>
```

---

### CC7: System Operations

**Description:** System processing integrity, availability, and incident management.

#### CC7.1: Detection of Actual and Potential System Failures

**Monitoring:**
```bash
# Health checks
nself health check --continuous

# Uptime monitoring
nself monitor uptime --alert-threshold 99.5%

# Error rate monitoring
nself monitor errors --threshold 0.1%

# Performance degradation
nself monitor performance --baseline baseline.json
```

#### CC7.2: Incident Management

**Incident Response:**
```bash
# Declare incident
nself incident declare \
  --severity critical \
  --description "Database connection failures" \
  --commander <user_id>

# Incident timeline
nself incident timeline <incident_id>

# Post-incident review
nself incident postmortem <incident_id> --template soc2
```

#### CC7.3: Management of System Capacity

**Capacity Planning:**
```bash
# Resource utilization
nself monitor resources --trend 30d

# Capacity forecast
nself capacity forecast --next 90d

# Scaling triggers
nself scale configure --cpu-threshold 70% --memory-threshold 80%
```

#### CC7.4: Change Management

**Change Control:**
```sql
CREATE TABLE change_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    change_title TEXT NOT NULL,
    change_description TEXT NOT NULL,
    change_type TEXT CHECK (change_type IN ('standard', 'normal', 'emergency')),
    requested_by UUID REFERENCES auth.users(id),
    requested_at TIMESTAMPTZ DEFAULT NOW(),
    approved_by UUID[] DEFAULT ARRAY[]::UUID[],
    approval_required INTEGER DEFAULT 2,
    implemented_at TIMESTAMPTZ,
    rollback_plan TEXT,
    testing_completed BOOLEAN DEFAULT FALSE,
    status TEXT DEFAULT 'pending'
);
```

#### CC7.5: Data Backup

**Backup Strategy:**
```bash
# Automated backups
BACKUP_ENABLED=true
BACKUP_SCHEDULE="0 2 * * *"  # Daily at 2 AM
BACKUP_RETENTION_DAYS=90

# Backup encryption
BACKUP_ENCRYPTION=true
BACKUP_ENCRYPTION_ALGORITHM=AES-256

# Backup testing (monthly)
nself backup test --random-selection

# Restore test
nself backup restore --test --backup-id <id>
```

---

### CC8: Change Management

**Description:** Management of changes to the system.

#### CC8.1: Authorization of Changes

**Approval Process:**
```bash
# Submit change request
nself change request \
  --title "Upgrade PostgreSQL to 15.1" \
  --description "Security patches and performance improvements" \
  --impact low \
  --downtime "5 minutes" \
  --approvers <user1_id>,<user2_id>

# Approve change
nself change approve <change_id> --approver <user_id>

# Implement after approval
nself change implement <change_id>
```

#### CC8.2: Development and Acquisition

**Software Development:**
- Code review (2+ reviewers)
- Security testing (SAST, DAST)
- Dependency scanning
- License compliance

**Third-Party Software:**
```bash
# Vendor risk assessment
nself vendor assess --vendor <name> --questionnaire soc2

# Software inventory
nself compliance software-inventory

# License compliance
nself compliance license-check
```

---

### CC9: Risk Mitigation

**Description:** Additional control activities for risk mitigation.

#### CC9.1: Vendor Management

**Vendor Lifecycle:**
```sql
CREATE TABLE vendors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_name TEXT NOT NULL,
    vendor_type TEXT,
    services_provided TEXT[],
    soc2_certified BOOLEAN DEFAULT FALSE,
    soc2_report_date DATE,
    contract_start_date DATE,
    contract_end_date DATE,
    risk_level TEXT CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    last_assessment_date DATE,
    next_assessment_date DATE,
    contract_document_url TEXT,
    primary_contact TEXT,
    primary_contact_email TEXT
);
```

**Vendor Assessment:**
```bash
# Initial vendor assessment
nself vendor assess <vendor_id> --questionnaire security_assessment

# Annual re-assessment
nself vendor reassess <vendor_id>

# Vendor performance review
nself vendor performance-review <vendor_id> --period 2026
```

---

## SOC 2 Type II - Evidence Collection

### Automated Evidence

```bash
# Generate evidence package
nself compliance evidence-package \
  --standard soc2_type2 \
  --period "2025-02-01 to 2026-01-31" \
  --output soc2_evidence_2026.zip

# Evidence included:
# - Audit logs (full year)
# - Access review reports (quarterly)
# - Vulnerability scan results
# - Penetration test reports
# - Incident response records
# - Change management logs
# - Backup test results
# - Security training records
# - Policy acknowledgments
# - Configuration baselines
```

### Continuous Control Monitoring

```bash
# Automated control testing
nself compliance control-test --control CC6.1 --automated

# Control effectiveness
nself compliance control-effectiveness-report --quarter Q1

# Deficiency tracking
nself compliance deficiencies --status open
```

---

## SOC 2 Report Preparation

### Audit Readiness

```bash
# Pre-audit checklist
nself compliance audit-readiness-check --standard soc2_type2

# Missing evidence identification
nself compliance evidence-gaps --standard soc2_type2

# Remediation plan
nself compliance remediation-plan --output remediation.pdf
```

### Auditor Access

```bash
# Create auditor user
nself auth users create auditor@firm.com \
  --role external_auditor \
  --read-only \
  --access-duration 90d

# Grant specific permissions
nself auth users grant-permissions auditor@firm.com \
  --permissions view:audit_logs,view:compliance_reports,view:configurations
```

---

## SOC 2 Trust Service Categories

In addition to Common Criteria (CC1-CC9), implement specific Trust Service Categories:

### Security (Always Required)

Covered by CC1-CC9 above.

### Availability (A1)

**A1.1: Availability Commitments**

```bash
# Define SLA
SLA_UPTIME_PERCENTAGE=99.9
SLA_RESPONSE_TIME_MS=200
SLA_ERROR_RATE_MAX=0.1

# Monitor SLA compliance
nself sla monitor --alert-on-breach

# SLA reporting
nself sla report --month 2026-01
```

**A1.2: Monitoring and Incident Response**

```bash
# 24/7 monitoring
nself monitor enable --continuous --alert-channels slack,pagerduty

# Incident response time
INCIDENT_RESPONSE_TIME_MINUTES=15
```

### Processing Integrity (P1)

**P1.1: Accurate Processing**

```bash
# Data validation
nself data validation-rules enable

# Processing reconciliation
nself data reconcile --daily

# Error detection and correction
nself data error-detection --automated-correction
```

### Confidentiality (C1)

**C1.1: Confidentiality Commitments**

```bash
# Data classification
nself data classify --auto-tagging

# Confidential data handling
nself security confidential-data-policy enforce

# DLP (Data Loss Prevention)
nself security dlp enable
```

### Privacy (PR)

See GDPR-COMPLIANCE.md for detailed privacy requirements.

---

## SOC 2 Compliance Checklist

### Phase 1: Preparation (Months 1-3)

- [ ] Define scope and system boundaries
- [ ] Document policies and procedures
- [ ] Implement required controls
- [ ] Configure monitoring and logging
- [ ] Select SOC 2 auditor
- [ ] Conduct gap analysis

### Phase 2: Implementation (Months 4-9)

- [ ] Remediate identified gaps
- [ ] Train staff on policies
- [ ] Establish change management process
- [ ] Implement vendor management
- [ ] Configure automated evidence collection
- [ ] Conduct internal testing

### Phase 3: Audit (Months 10-12)

- [ ] Pre-audit readiness check
- [ ] Provide evidence to auditors
- [ ] Respond to auditor inquiries
- [ ] Address audit findings
- [ ] Receive SOC 2 report

### Ongoing Maintenance

- [ ] Quarterly control testing
- [ ] Annual policy review
- [ ] Continuous monitoring
- [ ] Regular training
- [ ] Vendor assessments
- [ ] Incident management
- [ ] Annual re-audit

---

## nself SOC 2 Features

```bash
# Compliance dashboard
nself compliance dashboard --standard soc2

# Control testing
nself compliance test-controls --automated

# Evidence generation
nself compliance generate-evidence --period quarterly

# Audit log export
nself audit export --soc2-format

# Access review
nself auth access-review --quarterly

# Risk register
nself compliance risk-register --export
```

---

## Resources

- [AICPA SOC 2 Overview](https://www.aicpa.org/interestareas/frc/assuranceadvisoryservices/serviceorganization-smanagement.html)
- [Trust Services Criteria](https://www.aicpa.org/content/dam/aicpa/interestareas/frc/assuranceadvisoryservices/downloadabledocuments/trust-services-criteria.pdf)
- [SOC 2 Compliance Guide](https://www.vanta.com/products/soc-2)

---

**Disclaimer:** This guide provides technical implementation guidance for SOC 2 preparation. Actual SOC 2 certification requires an independent audit by a licensed CPA firm. Consult with SOC 2 auditors and compliance experts.

**Last Updated:** 2026-01-31
**Next Review:** 2026-04-30
