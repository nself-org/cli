# GDPR Compliance Guide for nself

**Last Updated:** 2026-01-31
**Version:** 0.9.6
**Applies To:** EU/EEA deployments

---

## Overview

This document outlines how nself addresses General Data Protection Regulation (GDPR) requirements and provides guidance for operators to maintain compliance when deploying applications using nself.

⚠️ **Important:** nself is infrastructure software. GDPR compliance is the responsibility of the application operator. This guide helps you understand how nself's features support compliance.

---

## GDPR Principles & nself Support

### 1. Lawfulness, Fairness, and Transparency

**GDPR Requirement:** Process personal data lawfully, fairly, and transparently.

**nself Support:**
- ✅ Audit logging system tracks all data access
- ✅ User authentication and session tracking
- ✅ Transparent data access through GraphQL API

**Operator Responsibilities:**
- Obtain valid legal basis for processing (consent, contract, legal obligation, etc.)
- Maintain records of processing activities
- Provide privacy notices to users

**Implementation:**
```bash
# Enable comprehensive audit logging
AUDIT_LOGGING_ENABLED=true
AUDIT_LOG_RETENTION_DAYS=365

# Enable user consent tracking
CONSENT_MANAGEMENT_ENABLED=true
```

---

### 2. Purpose Limitation

**GDPR Requirement:** Collect data for specified, explicit, and legitimate purposes.

**nself Support:**
- ✅ Role-based access control (RBAC) limits data access
- ✅ Custom claims for purpose-specific data access
- ✅ Metadata tracking for data collection purpose

**Operator Responsibilities:**
- Document purpose for each data collection point
- Limit data collection to necessary fields only
- Prevent data repurposing without consent

**Implementation:**
```sql
-- Add purpose field to user consent table
ALTER TABLE auth.user_consent ADD COLUMN purpose TEXT NOT NULL;

-- Track data collection purpose
INSERT INTO auth.user_consent (user_id, purpose, granted_at)
VALUES ('...', 'newsletter', NOW());
```

---

### 3. Data Minimization

**GDPR Requirement:** Collect only data that is adequate, relevant, and necessary.

**nself Support:**
- ✅ Configurable user profile fields
- ✅ Optional fields in authentication system
- ✅ Selective data replication in multi-tenant mode

**Operator Responsibilities:**
- Only enable necessary user fields
- Regular data cleanup of unused fields
- Justify each personal data field collected

**Configuration:**
```bash
# Minimal user profile (email only)
AUTH_USER_FIELDS="email"

# Extended profile (add phone, name)
AUTH_USER_FIELDS="email,phone,display_name"
```

---

### 4. Accuracy

**GDPR Requirement:** Keep personal data accurate and up to date.

**nself Support:**
- ✅ User profile management API
- ✅ Email verification system
- ✅ Phone verification (if enabled)
- ✅ Self-service profile updates

**Operator Responsibilities:**
- Provide mechanism for users to update their data
- Verify email and phone number accuracy
- Regular data quality audits

**API Usage:**
```bash
# User can update their profile
nself auth users update-profile <user_id> --email new@email.com

# Trigger email verification
nself auth users send-verification <user_id>
```

---

### 5. Storage Limitation

**GDPR Requirement:** Keep data only as long as necessary.

**nself Support:**
- ✅ Configurable data retention policies
- ✅ Automated data deletion
- ✅ Soft delete with grace period
- ✅ Backup retention configuration

**Operator Responsibilities:**
- Define retention periods for each data type
- Implement automated deletion workflows
- Document retention policy

**Configuration:**
```bash
# Data retention configuration
USER_DATA_RETENTION_DAYS=365
INACTIVE_USER_DELETION_DAYS=730
BACKUP_RETENTION_DAYS=90

# Enable automated cleanup
nself config set DATA_RETENTION_ENABLED true
nself config set AUTO_DELETE_INACTIVE_USERS true
```

**Implementation:**
```sql
-- Create retention policy
CREATE TABLE data_retention_policies (
    table_name TEXT PRIMARY KEY,
    retention_days INTEGER NOT NULL,
    deletion_field TEXT NOT NULL
);

-- Schedule cleanup job
-- Use nself functions or cron job
```

---

### 6. Integrity and Confidentiality (Security)

**GDPR Requirement:** Process data securely with appropriate technical measures.

**nself Support:**
- ✅ Encryption in transit (HTTPS/TLS)
- ✅ Authentication and authorization
- ✅ Role-based access control
- ✅ Audit logging
- ✅ Secret management
- ✅ Security headers

**Operator Responsibilities:**
- Enable HTTPS for all traffic
- Rotate secrets regularly
- Monitor access logs
- Implement encryption at rest
- Regular security audits

**Security Hardening:**
```bash
# Force HTTPS
SSL_ENABLED=true
FORCE_HTTPS=true

# Strong security headers
SECURITY_HEADERS_MODE=strict
HSTS_MAX_AGE=31536000
HSTS_INCLUDE_SUBDOMAINS=true

# Enable audit logging
AUDIT_LOGGING_ENABLED=true

# Rotate secrets
nself secrets rotate POSTGRES_PASSWORD
nself secrets rotate JWT_SECRET
```

---

### 7. Accountability

**GDPR Requirement:** Be able to demonstrate compliance.

**nself Support:**
- ✅ Comprehensive audit logging
- ✅ Change tracking in database
- ✅ Access log retention
- ✅ Configuration version control

**Operator Responsibilities:**
- Maintain audit log records
- Document compliance measures
- Regular compliance audits
- Data Protection Impact Assessments (DPIAs)

**Audit Configuration:**
```bash
# Enable comprehensive logging
AUDIT_LOGGING_ENABLED=true
AUDIT_LOG_LEVEL=detailed
AUDIT_LOG_RETENTION_DAYS=2555  # 7 years

# Export audit logs
nself logs export --type audit --start 2026-01-01 --end 2026-01-31
```

---

## GDPR Rights & Implementation

### Right to Access (Article 15)

**User Right:** Obtain confirmation of data processing and access to their data.

**Implementation:**
```bash
# Export all user data
nself auth users export <user_id> --format json > user_data.json

# Generate data access report
nself compliance data-access-report <user_id>
```

**API Endpoint:**
```graphql
query GetUserData($userId: uuid!) {
  auth_users(where: {id: {_eq: $userId}}) {
    id
    email
    phone
    display_name
    created_at
    last_seen
    metadata
  }

  # Include all related data
  user_profiles(where: {user_id: {_eq: $userId}}) {
    # ... profile fields
  }
}
```

---

### Right to Rectification (Article 16)

**User Right:** Correct inaccurate personal data.

**Implementation:**
```bash
# User updates their profile
nself auth users update <user_id> \
  --email corrected@email.com \
  --display-name "Corrected Name"

# Verify email after change
nself auth users send-verification <user_id>
```

**Self-Service API:**
```graphql
mutation UpdateUserProfile($userId: uuid!, $updates: auth_users_set_input!) {
  update_auth_users(
    where: {id: {_eq: $userId}}
    _set: $updates
  ) {
    affected_rows
    returning {
      id
      email
      updated_at
    }
  }
}
```

---

### Right to Erasure / "Right to be Forgotten" (Article 17)

**User Right:** Request deletion of personal data.

**Implementation:**

**Option 1: Hard Delete (Immediate)**
```bash
# Permanently delete user and all related data
nself auth users delete <user_id> --hard --confirm

# This will:
# 1. Delete user from auth.users
# 2. Delete all user sessions
# 3. Delete user profile data
# 4. Delete user metadata
# 5. Anonymize audit logs
```

**Option 2: Soft Delete (Grace Period)**
```bash
# Mark for deletion (30-day grace period)
nself auth users delete <user_id> --soft

# User data is anonymized but retained for 30 days
# After 30 days, automated cleanup runs

# Restore if requested within grace period
nself auth users restore <user_id>
```

**Automated Deletion:**
```sql
-- Schedule deletion job
CREATE OR REPLACE FUNCTION process_user_deletions()
RETURNS void AS $$
BEGIN
  -- Delete users marked for deletion > 30 days ago
  DELETE FROM auth.users
  WHERE deleted_at IS NOT NULL
    AND deleted_at < NOW() - INTERVAL '30 days';
END;
$$ LANGUAGE plpgsql;

-- Run daily via cron or pg_cron
SELECT cron.schedule('process-deletions', '0 2 * * *', 'SELECT process_user_deletions()');
```

---

### Right to Restriction of Processing (Article 18)

**User Right:** Request limitation of data processing.

**Implementation:**
```bash
# Suspend user account (no processing)
nself auth users suspend <user_id> --reason "User requested restriction"

# Resume when allowed
nself auth users unsuspend <user_id>
```

**Database Implementation:**
```sql
ALTER TABLE auth.users ADD COLUMN processing_restricted BOOLEAN DEFAULT FALSE;
ALTER TABLE auth.users ADD COLUMN restriction_reason TEXT;

-- Restrict processing
UPDATE auth.users
SET processing_restricted = true,
    restriction_reason = 'User request per GDPR Article 18'
WHERE id = '...';

-- Enforce in RLS policies
CREATE POLICY restrict_processing ON auth.users
  FOR SELECT
  USING (
    NOT processing_restricted OR
    auth.uid() = id OR
    auth.has_role('admin')
  );
```

---

### Right to Data Portability (Article 20)

**User Right:** Receive personal data in structured, machine-readable format.

**Implementation:**
```bash
# Export user data in JSON format
nself auth users export <user_id> --format json > user_data.json

# Export in CSV format
nself auth users export <user_id> --format csv > user_data.csv

# Export in XML format
nself auth users export <user_id> --format xml > user_data.xml
```

**Export Function:**
```javascript
// Custom export function
async function exportUserData(userId) {
  const { data, error } = await client
    .from('auth_users')
    .select(`
      *,
      profiles (*),
      orders (*),
      preferences (*)
    `)
    .eq('id', userId)
    .single();

  return {
    exportDate: new Date().toISOString(),
    userData: data,
    format: 'JSON',
    gdprCompliant: true
  };
}
```

---

### Right to Object (Article 21)

**User Right:** Object to processing of their data.

**Implementation:**
```bash
# Mark user as objecting to processing
nself auth users set-metadata <user_id> \
  --key gdpr_objection \
  --value '{"objection": true, "date": "2026-01-31", "reason": "Marketing"}'

# Stop specific processing
nself auth users set-metadata <user_id> \
  --key marketing_consent \
  --value false
```

**Database Tracking:**
```sql
CREATE TABLE user_objections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
    objection_type TEXT NOT NULL,
    reason TEXT,
    objected_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

-- Record objection
INSERT INTO user_objections (user_id, objection_type, reason)
VALUES ('...', 'marketing', 'User opted out via email');
```

---

### Rights Related to Automated Decision-Making (Article 22)

**User Right:** Not be subject to automated decisions with legal/significant effect.

**nself Guidance:**
- If your application uses automated decision-making, you must:
  1. Inform users clearly
  2. Provide human review option
  3. Allow users to contest decisions

**Implementation:**
```sql
CREATE TABLE automated_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES auth.users(id),
    decision_type TEXT NOT NULL,
    decision_result JSONB NOT NULL,
    explanation TEXT,
    human_review_available BOOLEAN DEFAULT TRUE,
    decided_at TIMESTAMPTZ DEFAULT NOW(),
    reviewed_by UUID REFERENCES auth.users(id),
    reviewed_at TIMESTAMPTZ
);
```

---

## Data Processing Records

### Article 30: Records of Processing Activities

**Requirement:** Maintain written records of processing activities.

**Template:**
```yaml
processing_activity:
  name: "User Authentication"
  controller: "Your Company Name"
  dpo_contact: "dpo@yourcompany.com"

  purposes:
    - "User authentication and authorization"
    - "Session management"

  data_categories:
    - "Email address"
    - "Password hash"
    - "Session tokens"

  data_subjects:
    - "Registered users"

  recipients:
    - "Internal authentication service"
    - "Database administrators (for maintenance)"

  transfers:
    third_countries: false
    safeguards: "N/A"

  retention:
    period: "Account lifetime + 30 days"
    criteria: "Account deletion + grace period"

  security_measures:
    - "Encryption in transit (TLS 1.2+)"
    - "Password hashing (bcrypt)"
    - "Access control (RBAC)"
    - "Audit logging"
```

**Generate with nself:**
```bash
nself compliance generate-processing-record > processing_activities.yaml
```

---

## Data Protection Impact Assessment (DPIA)

### When Required
- Large-scale processing of sensitive data
- Systematic monitoring of public areas
- Automated decision-making with legal effects

### DPIA Template

```bash
# Generate DPIA template
nself compliance generate-dpia-template > dpia.md
```

**Key Sections:**
1. Description of processing operations
2. Necessity and proportionality assessment
3. Risk assessment
4. Measures to address risks
5. Stakeholder consultation

---

## Data Breach Management

### Article 33: Breach Notification to Authority

**Requirement:** Notify supervisory authority within 72 hours.

**nself Incident Response:**

1. **Detection:**
```bash
# Monitor for unauthorized access
nself logs query --filter "failed_login_attempts > 100"

# Check for data exports
nself audit query --action "data_export" --last 24h
```

2. **Assessment:**
```bash
# Generate breach assessment report
nself compliance breach-assessment \
  --incident-id <id> \
  --affected-users <file> \
  --data-types "email,name,phone"
```

3. **Notification:**
```bash
# Notify supervisory authority (prepare report)
nself compliance generate-breach-report \
  --incident-id <id> \
  --output breach_report.pdf

# Notify affected users (if high risk)
nself compliance notify-affected-users \
  --incident-id <id> \
  --template breach_notification.html
```

---

## Cross-Border Data Transfers

### Transfers Outside EU/EEA

**Options:**
1. **Adequacy Decision:** Transfer to countries with adequate protection
2. **Standard Contractual Clauses (SCCs):** Use EU-approved contracts
3. **Binding Corporate Rules (BCRs):** For multinational organizations

**nself Configuration:**
```bash
# Restrict data location (EU-only)
DATA_LOCATION=eu
ALLOWED_REGIONS=eu-west-1,eu-central-1

# If using multi-region:
REQUIRE_DATA_TRANSFER_AGREEMENT=true
```

---

## Consent Management

### Valid Consent Requirements
- Freely given
- Specific
- Informed
- Unambiguous
- Withdrawable

**Implementation:**
```sql
CREATE TABLE user_consents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    consent_text TEXT NOT NULL,
    given_at TIMESTAMPTZ DEFAULT NOW(),
    withdrawn_at TIMESTAMPTZ,
    ip_address INET,
    user_agent TEXT
);

-- Record consent
INSERT INTO user_consents (user_id, purpose, consent_text, ip_address)
VALUES (
    '...',
    'marketing_emails',
    'I consent to receive marketing emails about products and services.',
    '192.168.1.1'
);

-- Withdraw consent
UPDATE user_consents
SET withdrawn_at = NOW()
WHERE user_id = '...' AND purpose = 'marketing_emails';
```

**API:**
```bash
# Grant consent
nself auth consent grant <user_id> \
  --purpose marketing \
  --text "I consent to marketing emails"

# Withdraw consent
nself auth consent revoke <user_id> --purpose marketing

# Check consent status
nself auth consent status <user_id>
```

---

## Compliance Checklist

### Initial Setup
- [ ] Appoint Data Protection Officer (if required)
- [ ] Document processing activities (Article 30)
- [ ] Conduct DPIA (if required)
- [ ] Update privacy policy
- [ ] Implement consent mechanism
- [ ] Configure data retention policies

### Technical Implementation
- [ ] Enable HTTPS/TLS
- [ ] Configure security headers
- [ ] Enable audit logging
- [ ] Implement access controls
- [ ] Set up data export functionality
- [ ] Implement deletion workflows
- [ ] Configure backup retention

### Ongoing Operations
- [ ] Regular security audits
- [ ] Monitor data breaches
- [ ] Process user rights requests
- [ ] Review and update policies
- [ ] Train staff on GDPR
- [ ] Maintain compliance documentation

---

## nself Compliance Features

### Built-in Features
```bash
# Data export
nself auth users export <user_id>

# Data deletion
nself auth users delete <user_id>

# Consent tracking
nself auth consent grant/revoke

# Audit logging
nself audit query

# Access reports
nself compliance access-report <user_id>
```

### Additional Commands
```bash
# Generate compliance reports
nself compliance generate-report --type gdpr

# Validate compliance
nself compliance validate

# Export audit logs
nself audit export --start 2026-01-01 --end 2026-01-31
```

---

## Resources

### Official GDPR Resources
- [GDPR Full Text](https://gdpr-info.eu/)
- [ICO Guidelines (UK)](https://ico.org.uk/for-organisations/guide-to-data-protection/guide-to-the-general-data-protection-regulation-gdpr/)
- [EDPB Guidelines](https://edpb.europa.eu/our-work-tools/general-guidance/gdpr-guidelines-recommendations-best-practices_en)

### Supervisory Authorities
- [List of EU DPAs](https://edpb.europa.eu/about-edpb/board/members_en)

### Documentation Templates
- `docs/compliance/processing-activities-template.yaml`
- `docs/compliance/dpia-template.md`
- `docs/compliance/privacy-policy-template.md`

---

**Disclaimer:** This document provides guidance on using nself features to support GDPR compliance. It is not legal advice. Consult with a data protection lawyer for your specific situation.

**Last Updated:** 2026-01-31
