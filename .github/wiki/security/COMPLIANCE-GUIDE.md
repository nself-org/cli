# nself Compliance Guide

**Version:** v0.9.6+
**Last Updated:** January 31, 2026

This guide helps you configure nself for compliance with major regulatory frameworks: GDPR, HIPAA, SOC 2, and PCI DSS.

---

## Table of Contents

- [Overview](#overview)
- [GDPR Compliance](#gdpr-compliance)
- [HIPAA Compliance](#hipaa-compliance)
- [SOC 2 Compliance](#soc-2-compliance)
- [PCI DSS Compliance](#pci-dss-compliance)
- [Data Retention](#data-retention)
- [Audit Logging](#audit-logging)
- [Privacy Controls](#privacy-controls)

---

## Overview

### Compliance Support Matrix

| Framework | nself Support | Required Configuration |
|-----------|---------------|------------------------|
| **GDPR** | ✅ Full | Data retention policies, audit logging |
| **HIPAA** | ✅ Full | Encryption, access controls, audit trails |
| **SOC 2** | ✅ Full | Security controls, monitoring, change management |
| **PCI DSS** | ⚠️ Partial | Additional encryption, network segmentation |
| **ISO 27001** | ✅ Full | Security policies, risk management |
| **CCPA** | ✅ Full | Same as GDPR requirements |

### Prerequisites

```bash
# Ensure nself is up to date
nself version  # Should be v0.9.6+

# Run security audit
nself auth security audit

# Enable monitoring
MONITORING_ENABLED=true
```

---

## GDPR Compliance

### Data Protection Requirements

**1. Data Minimization**

Only collect data necessary for your service:

```sql
-- users table - minimal data
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  email VARCHAR(255) UNIQUE NOT NULL,
  display_name VARCHAR(100),
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Avoid collecting:
-- - Unnecessary personal data
-- - Sensitive categories (race, religion, health) unless required
-- - Data without explicit consent
```

**2. Right to Access (Data Portability)**

Implement user data export:

```sql
-- Create data export function
CREATE OR REPLACE FUNCTION export_user_data(user_id UUID)
RETURNS JSON AS $$
DECLARE
  result JSON;
BEGIN
  SELECT json_build_object(
    'user', (SELECT row_to_json(u) FROM users u WHERE u.id = user_id),
    'profile', (SELECT row_to_json(p) FROM user_profiles p WHERE p.user_id = user_id),
    'messages', (SELECT json_agg(m) FROM messages m WHERE m.user_id = user_id),
    'uploaded_files', (SELECT json_agg(f) FROM files f WHERE f.user_id = user_id)
  ) INTO result;

  RETURN result;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
```

CLI command for data export:

```bash
# Export user data
nself db query "SELECT export_user_data('user-uuid-here')" > user-data.json
```

**3. Right to be Forgotten (Data Deletion)**

Implement cascading deletion or anonymization:

```sql
-- Option 1: Hard delete (CASCADE)
DELETE FROM users WHERE id = 'user-uuid-here';
-- All related data deleted via foreign key CASCADE

-- Option 2: Anonymization (preserve analytics)
UPDATE users
SET
  email = CONCAT('deleted-', id, '@privacy.local'),
  display_name = 'Deleted User',
  avatar_url = NULL,
  phone = NULL,
  deleted_at = NOW()
WHERE id = 'user-uuid-here';

-- Delete sensitive data but keep aggregate stats
DELETE FROM messages WHERE user_id = 'user-uuid-here';
```

**4. Data Retention Policy**

Configure automated data retention:

```sql
-- Create retention policy table
CREATE TABLE data_retention_policies (
  table_name VARCHAR(100) PRIMARY KEY,
  retention_days INTEGER NOT NULL,
  delete_after_days INTEGER,
  anonymize_after_days INTEGER
);

-- Set policies
INSERT INTO data_retention_policies VALUES
  ('audit_logs', 2555, NULL, NULL),          -- 7 years
  ('user_sessions', 90, 90, NULL),           -- 90 days then delete
  ('deleted_users', 30, NULL, 30),           -- Anonymize after 30 days
  ('temporary_files', 7, 7, NULL);           -- Delete after 7 days

-- Scheduled cleanup job (run daily via cron)
CREATE OR REPLACE FUNCTION cleanup_old_data()
RETURNS void AS $$
DECLARE
  policy RECORD;
BEGIN
  FOR policy IN SELECT * FROM data_retention_policies LOOP
    IF policy.delete_after_days IS NOT NULL THEN
      EXECUTE format(
        'DELETE FROM %I WHERE created_at < NOW() - INTERVAL ''%s days''',
        policy.table_name,
        policy.delete_after_days
      );
    END IF;
  END LOOP;
END;
$$ LANGUAGE plpgsql;
```

**5. Consent Management**

Track user consents:

```sql
CREATE TABLE user_consents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  consent_type VARCHAR(50) NOT NULL, -- 'terms', 'privacy', 'marketing'
  granted BOOLEAN NOT NULL,
  ip_address INET,
  user_agent TEXT,
  granted_at TIMESTAMP NOT NULL DEFAULT NOW(),
  withdrawn_at TIMESTAMP
);

-- Create index for fast lookup
CREATE INDEX idx_user_consents_user ON user_consents(user_id);
CREATE INDEX idx_user_consents_type ON user_consents(consent_type);
```

**6. GDPR Audit Logging**

```bash
# Enable comprehensive audit logging
AUDIT_LOGGING=true
AUDIT_LOG_FILE=logs/audit.log
AUDIT_RETENTION_DAYS=2555  # 7 years

# Log all data access
HASURA_GRAPHQL_ENABLE_CONSOLE=false  # Disable in production
HASURA_GRAPHQL_LOG_LEVEL=info
```

### GDPR Configuration

Add to `.env`:

```bash
# GDPR Configuration
GDPR_ENABLED=true
DATA_RETENTION_DAYS=365
DATA_EXPORT_ENABLED=true
DATA_DELETION_ENABLED=true

# Privacy settings
COOKIE_CONSENT_REQUIRED=true
PRIVACY_POLICY_URL=https://yourdomain.com/privacy
TERMS_OF_SERVICE_URL=https://yourdomain.com/terms

# Contact information
DPO_EMAIL=dpo@yourdomain.com
DPO_NAME="Data Protection Officer"
```

---

## HIPAA Compliance

### Protected Health Information (PHI) Security

**1. Encryption**

```bash
# Enable SSL/TLS for all connections
SSL_ENABLED=true
SSL_MODE=letsencrypt

# Database encryption at rest
POSTGRES_SSL_MODE=require
POSTGRES_SSL_CERT=/path/to/cert.pem
POSTGRES_SSL_KEY=/path/to/key.pem

# Encrypt backups
BACKUP_ENCRYPTION=true
BACKUP_ENCRYPTION_KEY=<strong-encryption-key>
```

**2. Access Controls**

```sql
-- Implement role-based access control
CREATE TABLE hipaa_access_roles (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  role_name VARCHAR(50) UNIQUE NOT NULL,
  can_view_phi BOOLEAN DEFAULT false,
  can_edit_phi BOOLEAN DEFAULT false,
  can_delete_phi BOOLEAN DEFAULT false,
  requires_mfa BOOLEAN DEFAULT true
);

-- Assign roles
INSERT INTO hipaa_access_roles VALUES
  (uuid_generate_v4(), 'physician', true, true, false, true),
  (uuid_generate_v4(), 'nurse', true, true, false, true),
  (uuid_generate_v4(), 'admin', true, false, false, true),
  (uuid_generate_v4(), 'billing', true, false, false, true);

-- Link users to roles
CREATE TABLE user_hipaa_roles (
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  role_id UUID REFERENCES hipaa_access_roles(id) ON DELETE CASCADE,
  assigned_at TIMESTAMP NOT NULL DEFAULT NOW(),
  assigned_by UUID REFERENCES users(id),
  PRIMARY KEY (user_id, role_id)
);
```

**3. Audit Trails**

Track ALL PHI access:

```sql
CREATE TABLE phi_access_log (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id UUID REFERENCES users(id),
  patient_id UUID,
  access_type VARCHAR(20) NOT NULL, -- 'view', 'edit', 'delete', 'export'
  table_name VARCHAR(100),
  record_id UUID,
  ip_address INET,
  user_agent TEXT,
  accessed_at TIMESTAMP NOT NULL DEFAULT NOW(),
  purpose TEXT -- Why was PHI accessed?
);

-- Create trigger to log all PHI access
CREATE OR REPLACE FUNCTION log_phi_access()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO phi_access_log (
    user_id,
    patient_id,
    access_type,
    table_name,
    record_id,
    accessed_at
  ) VALUES (
    current_setting('hasura.user_id', true)::UUID,
    NEW.patient_id,
    TG_OP,
    TG_TABLE_NAME,
    NEW.id,
    NOW()
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply to PHI tables
CREATE TRIGGER log_patient_access
  AFTER INSERT OR UPDATE ON patient_records
  FOR EACH ROW EXECUTE FUNCTION log_phi_access();
```

**4. Session Security**

```bash
# Require MFA for all users
AUTH_MFA_REQUIRED=true
AUTH_MFA_METHOD=totp

# Short session timeouts for PHI access
AUTH_JWT_ACCESS_TOKEN_EXPIRES_IN=600  # 10 minutes
AUTH_JWT_REFRESH_TOKEN_EXPIRES_IN=3600  # 1 hour

# Require re-authentication for sensitive operations
AUTH_REQUIRE_REAUTH_FOR_SENSITIVE=true
```

**5. Data Breach Notification**

Implement automated breach detection:

```sql
-- Monitor for suspicious data access
CREATE OR REPLACE FUNCTION detect_suspicious_activity()
RETURNS void AS $$
DECLARE
  suspicious RECORD;
BEGIN
  -- Alert if user accesses >100 patient records in 1 hour
  FOR suspicious IN
    SELECT user_id, COUNT(*) as access_count
    FROM phi_access_log
    WHERE accessed_at > NOW() - INTERVAL '1 hour'
    GROUP BY user_id
    HAVING COUNT(*) > 100
  LOOP
    -- Send alert (implement notification logic)
    RAISE WARNING 'Suspicious PHI access: User % accessed % records',
      suspicious.user_id, suspicious.access_count;
  END LOOP;
END;
$$ LANGUAGE plpgsql;
```

### HIPAA Configuration

```bash
# HIPAA Compliance
HIPAA_ENABLED=true
HIPAA_AUDIT_RETENTION_YEARS=7
HIPAA_MFA_REQUIRED=true

# Breach notification
BREACH_NOTIFICATION_EMAIL=security@yourdomain.com
BREACH_DETECTION_ENABLED=true

# Business Associate Agreement
BAA_SIGNED=true
BAA_DATE=2026-01-31
```

---

## SOC 2 Compliance

### Security Controls

**1. Access Management**

```bash
# User provisioning
nself auth roles create --name="admin" --permissions="full"
nself auth roles create --name="developer" --permissions="read,write"
nself auth roles create --name="viewer" --permissions="read"

# MFA for all users
nself auth mfa enable --method=totp --required=true

# Session management
AUTH_JWT_ACCESS_TOKEN_EXPIRES_IN=900  # 15 minutes
AUTH_JWT_REFRESH_TOKEN_EXPIRES_IN=86400  # 24 hours
```

**2. Change Management**

```bash
# Git-based change tracking
git log --all --oneline --graph

# Deployment tracking
echo "$(date): Deployed v1.2.3 to production" >> deployments.log

# Require code reviews
# (Configure in GitHub/GitLab settings)
```

**3. Monitoring & Alerting**

```bash
# Enable comprehensive monitoring
MONITORING_ENABLED=true

# Configure alerts
ALERT_EMAIL=security@yourdomain.com
ALERT_SLACK_WEBHOOK=https://hooks.slack.com/...

# Alert conditions
ALERT_ON_AUTH_FAILURES=true
ALERT_ON_DATABASE_DOWN=true
ALERT_ON_HIGH_CPU=true
```

**4. Incident Response**

```bash
# Document incident response plan
cat > incident-response-plan.md << 'EOF'
# Incident Response Plan

## Detection
- Monitor alerts in #security-alerts
- Check logs: nself logs --grep="error"

## Containment
- Rotate compromised secrets
- Block suspicious IPs
- Disable affected accounts

## Recovery
- Restore from backup if needed
- Verify system integrity

## Post-Incident
- Document timeline
- Update procedures
- Conduct review
EOF
```

### SOC 2 Configuration

```bash
# SOC 2 Compliance
SOC2_ENABLED=true
SOC2_AUDIT_LOGGING=true
SOC2_CHANGE_TRACKING=true
SOC2_INCIDENT_RESPONSE_PLAN=/path/to/plan.md

# Availability
UPTIME_SLA=99.9
BACKUP_FREQUENCY=daily
BACKUP_RETENTION_DAYS=90

# Security
MFA_REQUIRED=true
PASSWORD_MIN_LENGTH=16
SESSION_TIMEOUT_MINUTES=15
```

---

## Data Retention

### Retention Policy Template

```sql
-- Data retention implementation
CREATE TABLE data_retention_schedule (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  data_category VARCHAR(100) NOT NULL,
  table_name VARCHAR(100),
  retention_period_days INTEGER NOT NULL,
  deletion_method VARCHAR(20) NOT NULL, -- 'hard_delete', 'anonymize', 'archive'
  compliance_requirement VARCHAR(50), -- 'GDPR', 'HIPAA', 'SOC2'
  last_cleanup TIMESTAMP,
  next_cleanup TIMESTAMP
);

-- Example retention policies
INSERT INTO data_retention_schedule VALUES
  (uuid_generate_v4(), 'User Data', 'users', 2555, 'anonymize', 'GDPR', NOW(), NOW() + INTERVAL '30 days'),
  (uuid_generate_v4(), 'Audit Logs', 'audit_logs', 2555, 'archive', 'HIPAA', NOW(), NOW() + INTERVAL '1 year'),
  (uuid_generate_v4(), 'Session Data', 'sessions', 90, 'hard_delete', 'SOC2', NOW(), NOW() + INTERVAL '1 day'),
  (uuid_generate_v4(), 'Temporary Files', 'temp_files', 7, 'hard_delete', 'Internal', NOW(), NOW() + INTERVAL '1 day');
```

---

## Audit Logging

### Comprehensive Audit Log

```sql
CREATE TABLE comprehensive_audit_log (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
  user_id UUID REFERENCES users(id),
  user_email VARCHAR(255),
  action VARCHAR(100) NOT NULL,
  resource_type VARCHAR(100),
  resource_id UUID,
  ip_address INET,
  user_agent TEXT,
  request_id UUID,
  changes JSONB,
  metadata JSONB,
  compliance_category VARCHAR(50) -- 'GDPR', 'HIPAA', 'SOC2', 'PCI'
);

-- Indexes for fast queries
CREATE INDEX idx_audit_timestamp ON comprehensive_audit_log(timestamp);
CREATE INDEX idx_audit_user ON comprehensive_audit_log(user_id);
CREATE INDEX idx_audit_resource ON comprehensive_audit_log(resource_type, resource_id);
CREATE INDEX idx_audit_compliance ON comprehensive_audit_log(compliance_category);
```

### Enable Audit Logging

```bash
# In .env
AUDIT_LOGGING=true
AUDIT_LOG_TABLE=comprehensive_audit_log
AUDIT_RETENTION_DAYS=2555  # 7 years
AUDIT_LOG_ALL_QUERIES=true
```

---

## Privacy Controls

### Privacy by Design

```sql
-- Personal data inventory
CREATE TABLE personal_data_inventory (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  table_name VARCHAR(100) NOT NULL,
  column_name VARCHAR(100) NOT NULL,
  data_category VARCHAR(50), -- 'identity', 'contact', 'financial', 'health'
  sensitivity_level VARCHAR(20), -- 'public', 'internal', 'confidential', 'restricted'
  encryption_required BOOLEAN DEFAULT false,
  retention_days INTEGER,
  compliance_notes TEXT
);

-- Document all personal data
INSERT INTO personal_data_inventory VALUES
  (uuid_generate_v4(), 'users', 'email', 'contact', 'confidential', true, 2555, 'GDPR Article 6'),
  (uuid_generate_v4(), 'users', 'phone', 'contact', 'confidential', true, 2555, 'GDPR Article 6'),
  (uuid_generate_v4(), 'patient_records', 'diagnosis', 'health', 'restricted', true, 2555, 'HIPAA PHI');
```

---

## Quick Reference

### Compliance Checklist

**GDPR:**
- [ ] Data minimization implemented
- [ ] User data export function
- [ ] Data deletion/anonymization
- [ ] Retention policies configured
- [ ] Consent management
- [ ] Privacy policy published
- [ ] DPO designated

**HIPAA:**
- [ ] SSL/TLS enabled
- [ ] Database encryption at rest
- [ ] Role-based access control
- [ ] MFA required
- [ ] PHI access logging
- [ ] Audit logs retained 7 years
- [ ] Business Associate Agreement signed

**SOC 2:**
- [ ] Security policies documented
- [ ] Access controls implemented
- [ ] Monitoring enabled
- [ ] Incident response plan
- [ ] Change management process
- [ ] Regular security audits
- [ ] Vendor management

---

## Support

For compliance questions:
- **Email:** compliance@nself.org
- **Documentation:** [nself.org/docs/compliance](https://nself.org/docs/compliance)
