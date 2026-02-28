# HIPAA Compliance Guide for nself

**Last Updated:** 2026-01-31
**Version:** 0.9.6
**Applies To:** US Healthcare Applications

---

## Overview

This document provides guidance on using nself to build HIPAA-compliant (Health Insurance Portability and Accountability Act) healthcare applications. It covers the Security Rule, Privacy Rule, and Breach Notification Rule requirements.

⚠️ **Critical Notice:**
- nself is infrastructure software, not a HIPAA-compliant service provider by default
- HIPAA compliance requires additional configuration and operational practices
- You must sign Business Associate Agreements (BAAs) with cloud providers
- Consult with healthcare compliance experts and legal counsel

---

## HIPAA Overview

### What is Protected Health Information (PHI)?

PHI includes any health information that can identify an individual:
- Names, addresses, dates (birth, admission, discharge, death)
- Phone/fax numbers, email addresses
- Social Security numbers
- Medical record numbers
- Health plan beneficiary numbers
- Account numbers
- Certificate/license numbers
- Vehicle identifiers
- Device identifiers and serial numbers
- URLs, IP addresses
- Biometric identifiers (fingerprints, voice prints)
- Photos
- **Any health-related information** (diagnoses, treatments, medications, etc.)

---

## HIPAA Security Rule (45 CFR §164.308-318)

### Administrative Safeguards

#### 1. Security Management Process (§164.308(a)(1))

**Required:** Risk analysis, risk management, sanction policy, information system activity review.

**nself Implementation:**

```bash
# Enable comprehensive audit logging
AUDIT_LOGGING_ENABLED=true
AUDIT_LOG_LEVEL=detailed
AUDIT_LOG_RETENTION_DAYS=2555  # 7 years minimum for HIPAA

# Enable all security features
SECURITY_HEADERS_MODE=strict
FORCE_HTTPS=true
SSL_MINIMUM_VERSION=TLS1.2

# Conduct regular risk assessments
nself compliance risk-assessment --standard hipaa
```

**Risk Analysis Checklist:**
- [ ] Identify all systems storing/processing PHI
- [ ] Document potential vulnerabilities
- [ ] Assess likelihood and impact of threats
- [ ] Document existing security measures
- [ ] Identify gaps and remediation plans

**System Activity Review:**
```bash
# Daily audit log review
nself audit query --since yesterday --severity high

# Export logs for compliance review
nself audit export --start 2026-01-01 --end 2026-01-31 --format csv

# Monitor unauthorized access attempts
nself audit query --filter "unauthorized_access" --last 7d
```

---

#### 2. Assigned Security Responsibility (§164.308(a)(2))

**Required:** Designate a security official responsible for HIPAA compliance.

**Documentation:**
```yaml
security_official:
  name: "John Doe"
  title: "Chief Information Security Officer"
  email: "security@healthcare-org.com"
  phone: "+1-555-0100"
  responsibilities:
    - "Oversee HIPAA security compliance"
    - "Conduct risk assessments"
    - "Manage security incidents"
    - "Review audit logs"
    - "Update security policies"
```

---

#### 3. Workforce Security (§164.308(a)(3))

**Required:** Procedures for workforce access authorization and supervision.

**nself Implementation:**

```bash
# Role-based access control (RBAC)
nself auth roles create "physician" \
  --permissions "read:patient_records,write:patient_records"

nself auth roles create "nurse" \
  --permissions "read:patient_records,write:vitals"

nself auth roles create "billing" \
  --permissions "read:billing_records,write:billing"

nself auth roles create "receptionist" \
  --permissions "read:appointments,write:appointments"

# Assign users to roles
nself auth users assign-role <user_id> --role physician

# Minimum necessary access (least privilege)
nself auth roles list --show-permissions
```

**Access Authorization Process:**
1. New employee → Request access form
2. Manager approval
3. Security officer review
4. Account creation with appropriate role
5. Security training completion verification
6. Document in personnel file

**Workforce Clearance:**
```bash
# Background check tracking
nself auth users set-metadata <user_id> \
  --key background_check \
  --value '{"completed": "2026-01-15", "status": "cleared", "vendor": "BackgroundCheck Inc"}'

# Training completion
nself auth users set-metadata <user_id> \
  --key hipaa_training \
  --value '{"completed": "2026-01-20", "expiry": "2027-01-20", "certificate": "CERT-12345"}'
```

**Termination Procedures:**
```bash
# Immediate access revocation
nself auth users disable <user_id> --reason "Terminated on 2026-01-31"

# Audit terminated user's access
nself audit query --user <user_id> --last 90d > termination_audit.csv

# Remove from all roles
nself auth users remove-all-roles <user_id>
```

---

#### 4. Information Access Management (§164.308(a)(4))

**Required:** Implement policies to authorize access to ePHI.

**Access Control Policies:**

```sql
-- Row-Level Security (RLS) for patient data
ALTER TABLE patient_records ENABLE ROW LEVEL SECURITY;

-- Physicians can only see their patients
CREATE POLICY physician_access ON patient_records
  FOR SELECT
  USING (
    auth.has_role('physician') AND
    EXISTS (
      SELECT 1 FROM care_assignments
      WHERE patient_id = patient_records.id
        AND physician_id = auth.uid()
    )
  );

-- Emergency override (break-glass)
CREATE POLICY emergency_access ON patient_records
  FOR SELECT
  USING (
    auth.has_role('emergency_override') AND
    EXISTS (
      SELECT 1 FROM emergency_access_log
      WHERE user_id = auth.uid()
        AND created_at > NOW() - INTERVAL '1 hour'
    )
  );
```

**Break-Glass Emergency Access:**
```bash
# Grant emergency access (requires justification)
nself auth emergency-access grant <user_id> \
  --duration 1h \
  --reason "Patient code blue - Room 302" \
  --patient-id <patient_id>

# All emergency access is logged
nself audit query --filter "emergency_access" --last 24h
```

---

#### 5. Security Awareness and Training (§164.308(a)(5))

**Required:** Security training for all workforce members.

**Training Tracking:**
```sql
CREATE TABLE hipaa_training (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES auth.users(id),
    training_type TEXT NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    certificate_id TEXT,
    score INTEGER,
    passed BOOLEAN DEFAULT FALSE
);

-- Record training
INSERT INTO hipaa_training (user_id, training_type, completed_at, expires_at, passed)
VALUES (
    '...',
    'HIPAA Security Rule Annual Training',
    NOW(),
    NOW() + INTERVAL '1 year',
    true
);
```

**Training Reminders:**
```bash
# Automated training expiry notifications
nself compliance training-expiry-check --notify

# Generate training compliance report
nself compliance training-report --format pdf
```

---

#### 6. Security Incident Procedures (§164.308(a)(6))

**Required:** Identify and respond to security incidents.

**Incident Response Plan:**

1. **Detection**
```bash
# Monitor for suspicious activity
nself security monitor --alerts-only

# Automated breach detection
nself security scan --type unauthorized-access
```

2. **Containment**
```bash
# Immediately disable compromised account
nself auth users disable <user_id> --reason "Security incident SI-2026-001"

# Revoke all active sessions
nself auth sessions revoke-all --user <user_id>

# Block IP address
nself security firewall block <ip_address>
```

3. **Investigation**
```bash
# Generate incident timeline
nself audit query --user <user_id> --start "2026-01-31 10:00" --end "2026-01-31 14:00"

# Identify affected records
nself audit query --action "read" --user <user_id> --resource "patient_records"
```

4. **Notification**
```bash
# Generate breach notification report
nself compliance breach-report \
  --incident-id SI-2026-001 \
  --affected-records 150 \
  --data-types "name,dob,diagnosis"

# Notify affected individuals (if > 500, also media)
nself compliance notify-breach \
  --incident-id SI-2026-001 \
  --template breach_notification.html
```

**Incident Log:**
```sql
CREATE TABLE security_incidents (
    incident_id TEXT PRIMARY KEY,
    discovered_at TIMESTAMPTZ NOT NULL,
    incident_type TEXT NOT NULL,
    severity TEXT CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    description TEXT NOT NULL,
    affected_users INTEGER,
    affected_records INTEGER,
    phi_compromised BOOLEAN DEFAULT FALSE,
    phi_types TEXT[],
    contained_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    reported_to_hhs BOOLEAN DEFAULT FALSE,
    reported_at TIMESTAMPTZ,
    root_cause TEXT,
    remediation TEXT,
    created_by UUID REFERENCES auth.users(id)
);
```

---

#### 7. Contingency Plan (§164.308(a)(7))

**Required:** Data backup, disaster recovery, and emergency mode operation plans.

**Backup Configuration:**
```bash
# Automated daily backups
BACKUP_ENABLED=true
BACKUP_SCHEDULE="0 2 * * *"  # 2 AM daily
BACKUP_RETENTION_DAYS=90      # HIPAA requires sufficient retention
BACKUP_ENCRYPTION=true

# Run manual backup
nself backup create --encrypt --description "Pre-maintenance backup"

# Test backup restoration (quarterly)
nself backup restore --test --backup-id <id>
```

**Disaster Recovery:**
```bash
# Document Recovery Time Objective (RTO) and Recovery Point Objective (RPO)
RTO_HOURS=24  # System must be back online within 24 hours
RPO_HOURS=1   # Maximum 1 hour of data loss acceptable

# Disaster recovery drill
nself dr drill --scenario "datacenter-failure" --dry-run

# Generate DR report
nself dr report --last-drill
```

**Emergency Mode Operation:**
```yaml
emergency_mode:
  read_only: false
  offline_access: true
  paper_fallback: true
  critical_functions_only: true

  critical_functions:
    - "Patient lookup"
    - "Medication administration"
    - "Vital signs entry"
    - "Emergency department registration"

  procedures:
    - "Notify all staff of downtime"
    - "Activate paper forms"
    - "Manual documentation"
    - "Post-recovery data entry"
```

---

#### 8. Evaluation (§164.308(a)(8))

**Required:** Regular technical and non-technical evaluation.

**Annual Evaluation:**
```bash
# Run compliance evaluation
nself compliance evaluate --standard hipaa --output evaluation_report.pdf

# Security assessment
nself security scan --comprehensive

# Generate findings report
nself compliance findings --standard hipaa
```

---

### Technical Safeguards

#### 1. Access Control (§164.312(a)(1))

**Required:** Unique user identification, emergency access, automatic logoff, encryption.

**Implementation:**

```bash
# Unique user identification (required)
AUTH_REQUIRE_EMAIL_VERIFICATION=true
AUTH_ALLOW_MULTIPLE_ACCOUNTS=false

# Automatic logoff (addressable)
SESSION_TIMEOUT_MINUTES=30
SESSION_IDLE_TIMEOUT_MINUTES=15

# Encryption and decryption (addressable)
ENCRYPTION_AT_REST_ENABLED=true
DATABASE_ENCRYPTION=true
BACKUP_ENCRYPTION=true
```

**Session Management:**
```bash
# Configure session timeouts
nself config set SESSION_TIMEOUT_MINUTES 30
nself config set SESSION_IDLE_TIMEOUT_MINUTES 15

# Enforce automatic logoff
nself auth sessions cleanup-idle --threshold 15m
```

**Access Control Lists:**
```sql
-- User must be explicitly granted access to patient records
CREATE TABLE patient_access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES auth.users(id),
    patient_id UUID REFERENCES patients(id),
    granted_by UUID REFERENCES auth.users(id),
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    reason TEXT NOT NULL
);
```

---

#### 2. Audit Controls (§164.312(b))

**Required:** Hardware, software, and procedural mechanisms to record and examine activity.

**Comprehensive Audit Logging:**

```bash
# Enable all audit logging
AUDIT_LOGGING_ENABLED=true
AUDIT_LOG_LEVEL=detailed
AUDIT_LOG_PHI_ACCESS=true  # Log all PHI access

# Audit log types
AUDIT_LOG_AUTHENTICATION=true
AUDIT_LOG_AUTHORIZATION=true
AUDIT_LOG_DATA_ACCESS=true
AUDIT_LOG_DATA_MODIFICATION=true
AUDIT_LOG_ADMINISTRATIVE_ACTIONS=true
```

**What to Log:**
- User login/logout
- Failed login attempts
- PHI record access (read/write/delete)
- User creation/modification/deletion
- Role/permission changes
- Configuration changes
- Backup creation/restoration
- Emergency access usage
- Export of data

**Audit Log Schema:**
```sql
CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    user_id UUID,
    user_email TEXT,
    user_role TEXT,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    patient_id UUID,  -- If action involved patient data
    ip_address INET,
    user_agent TEXT,
    session_id UUID,
    result TEXT,  -- success, failure, denied
    details JSONB,
    phi_accessed BOOLEAN DEFAULT FALSE
);

-- Index for performance
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_log_user ON audit_log(user_id);
CREATE INDEX idx_audit_log_patient ON audit_log(patient_id);
CREATE INDEX idx_audit_log_phi ON audit_log(phi_accessed) WHERE phi_accessed = true;
```

**Query Audit Logs:**
```bash
# All PHI access in last 24 hours
nself audit query --filter "phi_accessed=true" --last 24h

# Specific user's activity
nself audit query --user <user_id> --last 30d

# Failed access attempts
nself audit query --result "denied" --last 7d

# Patient record access
nself audit query --patient <patient_id> --last 90d
```

---

#### 3. Integrity Controls (§164.312(c)(1))

**Required:** Policies and procedures to ensure ePHI is not improperly altered or destroyed.

**Implementation:**

```sql
-- Prevent accidental deletion (soft delete only)
ALTER TABLE patient_records ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE patient_records ADD COLUMN deleted_by UUID REFERENCES auth.users(id);

-- Audit trail for modifications
CREATE TABLE record_modifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    record_type TEXT NOT NULL,
    record_id UUID NOT NULL,
    modified_by UUID REFERENCES auth.users(id),
    modified_at TIMESTAMPTZ DEFAULT NOW(),
    field_name TEXT NOT NULL,
    old_value TEXT,
    new_value TEXT,
    reason TEXT
);
```

**Digital Signatures (for documents):**
```bash
# Sign clinical document
nself security sign-document <document_id> --user <user_id>

# Verify document integrity
nself security verify-document <document_id>
```

**Message Authentication:**
```bash
# Enable message integrity checks
MESSAGE_INTEGRITY_ENABLED=true
MESSAGE_SIGNING_ALGORITHM=HMAC-SHA256
```

---

#### 4. Person or Entity Authentication (§164.312(d))

**Required:** Verify that a person or entity seeking access is who they claim to be.

**Multi-Factor Authentication:**

```bash
# Require MFA for all users
MFA_REQUIRED=true
MFA_METHODS=totp,sms,email  # Multiple options

# Enforce MFA for PHI access
MFA_REQUIRED_FOR_PHI_ACCESS=true

# Configure MFA
nself auth mfa enable <user_id> --method totp
nself auth mfa backup-codes generate <user_id>
```

**Strong Password Policy:**
```bash
# Password requirements
PASSWORD_MIN_LENGTH=12
PASSWORD_REQUIRE_UPPERCASE=true
PASSWORD_REQUIRE_LOWERCASE=true
PASSWORD_REQUIRE_NUMBERS=true
PASSWORD_REQUIRE_SPECIAL=true
PASSWORD_HISTORY_COUNT=12  # Remember last 12 passwords
PASSWORD_MAX_AGE_DAYS=90   # Force change every 90 days
```

**Biometric Authentication (optional):**
```bash
# Enable WebAuthn for hardware keys
WEBAUTHN_ENABLED=true

# Enable biometric authentication
nself auth webauthn register <user_id>
```

---

#### 5. Transmission Security (§164.312(e)(1))

**Required:** Guard against unauthorized access to ePHI transmitted over electronic communications networks.

**Encryption in Transit:**

```bash
# Force HTTPS
FORCE_HTTPS=true
SSL_ENABLED=true
SSL_MINIMUM_VERSION=TLS1.2

# Strong cipher suites only
SSL_CIPHERS="ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-AES128-GCM-SHA256"

# Disable weak protocols
SSL_PROTOCOLS="TLSv1.2 TLSv1.3"
```

**Nginx SSL Configuration:**
```nginx
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers 'ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-AES128-GCM-SHA256';
ssl_prefer_server_ciphers on;
ssl_session_timeout 10m;
ssl_session_cache shared:SSL:10m;

# HSTS
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

**Email Encryption:**
```bash
# S/MIME or PGP for email containing PHI
EMAIL_ENCRYPTION_REQUIRED=true
EMAIL_ENCRYPTION_METHOD=SMIME
```

---

### Physical Safeguards

#### 1. Facility Access Controls (§164.310(a)(1))

**Note:** If self-hosting, you must implement physical security measures.

**If Using Cloud Provider:**
- Sign Business Associate Agreement (BAA)
- Verify SOC 2 Type II certification
- Review physical security controls
- Ensure data center compliance

**Documentation for Cloud:**
```yaml
cloud_provider:
  name: "AWS"
  baa_signed: true
  baa_date: "2026-01-15"
  certifications:
    - "SOC 2 Type II"
    - "HIPAA"
    - "ISO 27001"
  regions:
    - "us-east-1"  # Ensure US-only for HIPAA
```

---

#### 2. Workstation Use (§164.310(b))

**Required:** Implement policies for workstation use.

**Workstation Security Policy:**
- Automatic screen lock after 5 minutes
- Full disk encryption required
- Antivirus software required
- Automatic security updates
- No personal devices (or MDM enrollment required)

**Enforcement:**
```bash
# Device registration (if using device management)
nself auth devices register <device_id> \
  --user <user_id> \
  --type workstation \
  --encryption-verified true

# Check device compliance
nself auth devices check-compliance <device_id>
```

---

#### 3. Workstation Security (§164.310(c))

**Required:** Implement physical safeguards for workstations.

**Security Measures:**
- Privacy screens on monitors
- Cable locks for laptops
- Restricted physical access
- Clean desk policy

---

#### 4. Device and Media Controls (§164.310(d)(1))

**Required:** Implement policies for disposal, removal, and re-use of electronic media.

**Media Disposal:**
```bash
# Document media disposal
nself compliance media-disposal record \
  --device-id <id> \
  --disposal-method "Professional wiping service" \
  --certificate <certificate_number> \
  --disposed-by <user_id>

# Generate disposal report
nself compliance media-disposal report --year 2026
```

**Disposal Methods:**
- Hard drive: Professional wiping (DoD 5220.22-M) or physical destruction
- Backup media: Degaussing or incineration
- Mobile devices: Factory reset + encryption key destruction

---

## HIPAA Privacy Rule

### Minimum Necessary Standard

**Principle:** Use and disclose only the minimum PHI necessary.

**Implementation:**

```sql
-- Limit data returned by queries (do not SELECT *)
-- BAD:
SELECT * FROM patient_records WHERE id = '...';

-- GOOD:
SELECT id, name, dob, diagnosis FROM patient_records WHERE id = '...';

-- Role-based data filtering
CREATE VIEW nurse_patient_view AS
SELECT
  id,
  name,
  dob,
  current_vitals,
  current_medications,
  allergies
FROM patient_records
WHERE id IN (SELECT patient_id FROM nurse_assignments WHERE nurse_id = auth.uid());
```

**API Design:**
```graphql
# Limited fields for different roles
query GetPatientSummary($patientId: uuid!) {
  patients(where: {id: {_eq: $patientId}}) {
    id
    name
    dob
    # Only minimum necessary fields
  }
}
```

---

### Notice of Privacy Practices (NPP)

**Required:** Provide patients with notice of privacy practices.

**Implementation:**
```bash
# Track NPP acknowledgment
nself compliance npp-acknowledgment record \
  --patient <patient_id> \
  --version v2026.1 \
  --acknowledged-at "2026-01-31T10:00:00Z"

# Generate NPP for patient
nself compliance generate-npp --patient <patient_id> --format pdf
```

---

### Patient Rights

#### Right to Access (45 CFR §164.524)

**Patient right to access their medical records.**

```bash
# Patient requests access
nself patient-rights access-request create \
  --patient <patient_id> \
  --requested-by <patient_email> \
  --requested-at "2026-01-31T10:00:00Z"

# Generate patient record export (30 days to fulfill)
nself patient-rights export <patient_id> \
  --format pdf \
  --include-notes \
  --include-lab-results \
  --include-imaging-reports

# Log fulfillment
nself patient-rights access-request fulfill <request_id> \
  --delivered-at "2026-02-05T14:00:00Z"
```

**Access Request Log:**
```sql
CREATE TABLE access_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id UUID REFERENCES patients(id),
    requested_by TEXT NOT NULL,
    requested_at TIMESTAMPTZ DEFAULT NOW(),
    request_method TEXT,  -- mail, email, portal
    records_requested TEXT[],
    format_requested TEXT,  -- paper, electronic
    fulfilled_at TIMESTAMPTZ,
    delivered_method TEXT,
    denial_reason TEXT,  -- If denied
    fee_charged DECIMAL(10,2)
);
```

---

#### Right to Amend (45 CFR §164.526)

**Patient right to request amendment of their records.**

```bash
# Patient requests amendment
nself patient-rights amendment-request create \
  --patient <patient_id> \
  --record-id <record_id> \
  --requested-amendment "Incorrect birthdate listed" \
  --proposed-correction "DOB should be 1985-03-15"

# Approve amendment
nself patient-rights amendment-request approve <request_id> \
  --amended-by <provider_id>

# Deny amendment (with reason)
nself patient-rights amendment-request deny <request_id> \
  --reason "Record is accurate as documented by treating physician"
```

---

#### Right to Accounting of Disclosures (45 CFR §164.528)

**Patient right to receive an accounting of PHI disclosures.**

```bash
# Generate accounting of disclosures (6 years)
nself patient-rights accounting-of-disclosures <patient_id> \
  --start-date "2020-01-01" \
  --end-date "2026-01-31" \
  --format pdf

# Track disclosure
nself compliance disclosure record \
  --patient <patient_id> \
  --disclosed-to "Insurance Company XYZ" \
  --purpose "Payment" \
  --date "2026-01-31" \
  --data-disclosed "Diagnosis, treatment dates"
```

**Disclosure Log:**
```sql
CREATE TABLE phi_disclosures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id UUID REFERENCES patients(id),
    disclosed_at TIMESTAMPTZ DEFAULT NOW(),
    disclosed_to TEXT NOT NULL,
    recipient_type TEXT,  -- healthcare_provider, insurance, legal, patient, etc.
    purpose TEXT NOT NULL,
    data_disclosed TEXT[],
    authorization_id UUID,  -- If disclosed with patient authorization
    disclosure_method TEXT,
    disclosed_by UUID REFERENCES auth.users(id)
);
```

---

#### Right to Request Restrictions (45 CFR §164.522)

**Patient right to request restrictions on uses and disclosures.**

```bash
# Record restriction request
nself patient-rights restriction-request create \
  --patient <patient_id> \
  --restriction "Do not disclose mental health records to insurance"

# Approve restriction
nself patient-rights restriction-request approve <request_id>

# Implement restriction
nself patient-rights restriction enforce <patient_id> \
  --restriction-id <restriction_id>
```

---

## Breach Notification Rule

### Breach Determination

**Breach:** Impermissible use or disclosure that compromises security or privacy of PHI.

**4-Factor Risk Assessment:**

1. **Nature and extent of PHI involved**
2. **Unauthorized person who used/disclosed PHI**
3. **Was PHI actually acquired or viewed**
4. **Extent to which risk has been mitigated**

**Decision Tree:**
```bash
nself compliance breach-assessment \
  --incident-id SI-2026-001 \
  --phi-types "name,dob,ssn,diagnosis" \
  --unauthorized-person "external-attacker" \
  --data-accessed true \
  --mitigation-steps "Changed all passwords, blocked IP, notified law enforcement"

# Output: Breach determination (YES/NO)
```

---

### Notification Requirements

#### To Individuals (≤500 people)

**Timeline:** Within 60 days of discovery

```bash
# Generate and send breach notifications
nself compliance breach-notify-individuals \
  --incident-id SI-2026-001 \
  --template breach_notification.html \
  --delivery-method email
```

**Notification Must Include:**
- Brief description of the breach
- Types of PHI involved
- Steps individuals should take
- What organization is doing to investigate
- Contact information

---

#### To Media (>500 people in a state)

**Timeline:** Within 60 days of discovery

```bash
# Notify media outlets
nself compliance breach-notify-media \
  --incident-id SI-2026-001 \
  --affected-state "California" \
  --affected-count 750
```

---

#### To HHS

**Timeline:**
- >500 people: Within 60 days
- <500 people: Annual report (within 60 days of end of calendar year)

```bash
# Submit to HHS
nself compliance breach-notify-hhs \
  --incident-id SI-2026-001 \
  --affected-count 150

# Annual report for small breaches
nself compliance breach-annual-report --year 2026
```

---

## Business Associate Agreements (BAAs)

### When Required

You need a BAA with:
- Cloud hosting providers
- Database providers
- Email service providers
- SMS/messaging providers
- Backup service providers
- Security monitoring services
- Analytics providers (if processing PHI)

### nself Cloud Providers with BAAs

**Verified BAA Support:**
- AWS (Amazon Web Services) ✅
- Google Cloud Platform ✅
- Microsoft Azure ✅
- Oracle Cloud ✅

**BAA Not Available:**
- Most shared hosting providers ❌
- Most free-tier services ❌
- GitHub, GitLab (for PHI data) ❌

### BAA Tracking

```bash
# Record signed BAA
nself compliance baa record \
  --vendor "AWS" \
  --signed-date "2026-01-15" \
  --expiry-date "2027-01-15" \
  --document-url "s3://contracts/aws-baa-2026.pdf"

# BAA expiry reminders
nself compliance baa expiry-check --notify
```

---

## HIPAA Compliance Checklist for nself

### Initial Setup

#### Administrative
- [ ] Designate Privacy Officer
- [ ] Designate Security Officer
- [ ] Conduct initial risk assessment
- [ ] Develop policies and procedures
- [ ] Create incident response plan
- [ ] Create contingency plan
- [ ] Develop training program

#### Technical
- [ ] Enable HTTPS/TLS (minimum TLS 1.2)
- [ ] Configure strong authentication
- [ ] Enable MFA for all users
- [ ] Implement automatic session timeout
- [ ] Configure comprehensive audit logging
- [ ] Set up encrypted backups
- [ ] Implement role-based access control
- [ ] Configure security headers

#### Contracts
- [ ] Sign BAAs with all service providers
- [ ] Review and sign cloud provider agreements
- [ ] Document all third-party relationships

### Configuration

```bash
# Recommended HIPAA configuration
cat > .env.hipaa << 'EOF'
# HIPAA-Compliant Configuration

# Force HTTPS
FORCE_HTTPS=true
SSL_ENABLED=true
SSL_MINIMUM_VERSION=TLS1.2

# Strong authentication
MFA_REQUIRED=true
MFA_REQUIRED_FOR_PHI_ACCESS=true
PASSWORD_MIN_LENGTH=12
PASSWORD_MAX_AGE_DAYS=90

# Session security
SESSION_TIMEOUT_MINUTES=30
SESSION_IDLE_TIMEOUT_MINUTES=15

# Audit logging
AUDIT_LOGGING_ENABLED=true
AUDIT_LOG_LEVEL=detailed
AUDIT_LOG_PHI_ACCESS=true
AUDIT_LOG_RETENTION_DAYS=2555  # 7 years

# Encryption
ENCRYPTION_AT_REST_ENABLED=true
DATABASE_ENCRYPTION=true
BACKUP_ENCRYPTION=true

# Security headers
SECURITY_HEADERS_MODE=strict
HSTS_MAX_AGE=31536000
HSTS_INCLUDE_SUBDOMAINS=true

# Data retention
DATA_RETENTION_ENABLED=true
AUDIT_LOG_RETENTION_DAYS=2555  # 7 years minimum

# Backup
BACKUP_ENABLED=true
BACKUP_SCHEDULE="0 2 * * *"
BACKUP_RETENTION_DAYS=90
BACKUP_ENCRYPTION=true
EOF
```

### Ongoing Operations

#### Daily
- [ ] Review security alerts
- [ ] Monitor failed login attempts
- [ ] Check backup completion

#### Weekly
- [ ] Review audit logs for anomalies
- [ ] Check access control lists
- [ ] Verify backup integrity

#### Monthly
- [ ] Access control review
- [ ] Update risk assessment
- [ ] Review incident logs
- [ ] Training completion check

#### Quarterly
- [ ] Security evaluation
- [ ] Disaster recovery drill
- [ ] Policy review and updates
- [ ] Vendor BAA review

#### Annually
- [ ] Comprehensive risk assessment
- [ ] Staff HIPAA training
- [ ] Security certification renewal
- [ ] Full compliance audit

---

## nself HIPAA Features

### Compliance Commands

```bash
# HIPAA risk assessment
nself compliance risk-assessment --standard hipaa

# Generate compliance report
nself compliance report --standard hipaa --output hipaa_report.pdf

# Audit log export
nself audit export --start 2026-01-01 --end 2026-12-31

# Patient data export (right to access)
nself patient-rights export <patient_id>

# Breach notification
nself compliance breach-notify <incident_id>

# BAA management
nself compliance baa list
nself compliance baa expiry-check

# Training tracking
nself compliance training-report
```

---

## Resources

### Official HIPAA Resources
- [HHS HIPAA Website](https://www.hhs.gov/hipaa/)
- [OCR Security Rule Guidance](https://www.hhs.gov/hipaa/for-professionals/security/)
- [OCR Privacy Rule Guidance](https://www.hhs.gov/hipaa/for-professionals/privacy/)
- [Breach Notification Rule](https://www.hhs.gov/hipaa/for-professionals/breach-notification/)

### Enforcement
- [OCR Breach Portal](https://ocrportal.hhs.gov/ocr/breach/breach_report.jsf)
- Report breaches affecting >500 individuals

### Documentation Templates
- `docs/compliance/hipaa-policies-template.md`
- `docs/compliance/hipaa-risk-assessment-template.xlsx`
- `docs/compliance/baa-template.docx`
- `docs/compliance/breach-notification-template.html`

---

**Disclaimer:** This document provides technical guidance on configuring nself for HIPAA compliance. It is not legal advice. Consult with healthcare compliance experts, legal counsel, and conduct proper risk assessments for your specific use case. HIPAA compliance requires both technical and administrative safeguards beyond what nself provides.

**Last Updated:** 2026-01-31
**Next Review:** 2026-04-30
