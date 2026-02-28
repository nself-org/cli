# Billing Troubleshooting Guide

> Comprehensive guide to diagnosing and resolving billing system issues in nself

**Last Updated**: January 2026
**Applies To**: nself v0.5.0+

---

## Table of Contents

1. [Diagnostic Workflow](#diagnostic-workflow)
2. [Stripe Connection Issues](#1-stripe-connection-issues)
3. [Usage Tracking Issues](#2-usage-tracking-issues)
4. [Subscription Problems](#3-subscription-problems)
5. [Invoice Issues](#4-invoice-issues)
6. [Quota Enforcement](#5-quota-enforcement)
7. [Webhook Problems](#6-webhook-problems)
8. [Data Sync Issues](#7-data-sync-issues)
9. [Performance Issues](#8-performance-issues)
10. [Testing & Development](#9-testing--development)
11. [Common Error Messages](#10-common-error-messages)
12. [Emergency Procedures](#emergency-procedures)

---

## Diagnostic Workflow

### General Troubleshooting Steps

```bash
# 1. Check billing service status
nself billing status

# 2. View recent billing logs
nself logs billing --tail 100

# 3. Verify Stripe connection
nself billing stripe test-connection

# 4. Check database connectivity
nself db status

# 5. View error summary
nself billing errors --last 24h
```

### Log Levels

```bash
# Enable debug logging
export BILLING_LOG_LEVEL=debug
nself restart billing

# View debug logs
nself logs billing --level debug

# Disable debug logging after troubleshooting
export BILLING_LOG_LEVEL=info
nself restart billing
```

### Health Check Commands

```bash
# Complete system health check
nself billing health

# Component-specific checks
nself billing health --component stripe
nself billing health --component usage
nself billing health --component webhooks
nself billing health --component quotas
```

---

## 1. Stripe Connection Issues

### Issue 1.1: API Key Not Working

**Symptoms:**
- Error: "Invalid API key provided"
- HTTP 401 errors in logs
- Failed Stripe API calls

**Diagnosis:**
```bash
# Check current API key configuration
nself billing stripe show-config

# Test API key validity
nself billing stripe test-key

# View recent authentication errors
nself logs billing | grep "authentication failed"
```

**Root Causes:**
1. Wrong API key (test vs live)
2. Revoked or deleted key
3. Insufficient permissions
4. Whitespace in environment variable

**Resolution:**

```bash
# 1. Verify environment variable
echo "$STRIPE_SECRET_KEY"
# Should start with sk_test_PLACEHOLDER or sk_live_

# 2. Check for whitespace
printf "%q\n" "$STRIPE_SECRET_KEY"

# 3. Get new API key from Stripe Dashboard
# https://dashboard.stripe.com/apikeys

# 4. Update environment
nself env set STRIPE_SECRET_KEY="sk_test_PLACEHOLDER_new_key"

# 5. Restart billing service
nself restart billing

# 6. Verify connection
nself billing stripe test-connection
```

**Prevention:**
- Store keys in `.secrets` file (gitignored)
- Use separate keys for dev/staging/prod
- Set up key rotation schedule
- Enable Stripe Dashboard alerts

---

### Issue 1.2: Webhook Signature Verification Failures

**Symptoms:**
- Error: "No signatures found matching the expected signature"
- Webhooks rejected with 400 status
- Events not processing

**Diagnosis:**
```bash
# Check webhook secret
nself billing webhook show-secret

# View recent webhook failures
nself billing webhook failures --last 100

# Test webhook signature
nself billing webhook test-signature

# Check webhook logs
nself logs billing | grep "webhook signature"
```

**Root Causes:**
1. Wrong webhook secret
2. Multiple webhook endpoints configured
3. Clock skew between servers
4. Secret not properly escaped

**Resolution:**

```bash
# 1. Get webhook secret from Stripe
# Dashboard → Developers → Webhooks → [Your endpoint] → Signing secret

# 2. Update secret
nself env set STRIPE_WEBHOOK_SECRET="whsec_your_secret"

# 3. Verify no duplicate endpoints
nself billing webhook list-endpoints

# 4. Check server time
date -u
# Should be within 5 minutes of actual UTC time

# 5. Sync system time (if needed)
sudo ntpdate -s time.nist.gov

# 6. Restart billing
nself restart billing

# 7. Test with Stripe CLI
stripe listen --forward-to localhost:8080/webhooks/stripe
stripe trigger payment_intent.succeeded
```

**Prevention:**
- Use single webhook endpoint per environment
- Monitor webhook delivery in Stripe Dashboard
- Set up alerts for signature failures
- Keep system time synchronized

---

### Issue 1.3: Rate Limiting Errors

**Symptoms:**
- Error: "Rate limit exceeded"
- HTTP 429 responses
- Intermittent API failures

**Diagnosis:**
```bash
# Check rate limit status
nself billing stripe rate-limits

# View rate limit errors
nself logs billing | grep "rate_limit"

# Check request volume
nself billing metrics requests --last 1h
```

**Root Causes:**
1. Too many API calls in short period
2. No retry logic with backoff
3. Batch operations not used
4. Inefficient queries

**Resolution:**

```bash
# 1. Check current request rate
nself billing stripe show-rate

# 2. Enable request caching
nself env set STRIPE_ENABLE_CACHE=true
nself env set STRIPE_CACHE_TTL=300  # 5 minutes

# 3. Configure retry logic
nself env set STRIPE_MAX_RETRIES=3
nself env set STRIPE_RETRY_BACKOFF=exponential

# 4. Optimize batch operations
# Edit: services/billing/config/stripe.yml
batch_size: 100  # Process in batches
rate_limit_buffer: 0.8  # Use 80% of limit

# 5. Restart billing
nself restart billing
```

**Prevention:**
- Implement request caching
- Use batch APIs when possible
- Add exponential backoff
- Monitor request volumes

---

### Issue 1.4: Network Connectivity Problems

**Symptoms:**
- Error: "Connection timeout"
- Error: "Network is unreachable"
- Intermittent failures

**Diagnosis:**
```bash
# Test network connectivity
curl -I https://api.stripe.com

# Test DNS resolution
nslookup api.stripe.com

# Check firewall rules
sudo iptables -L | grep STRIPE

# Test from billing container
docker exec billing-service curl -I https://api.stripe.com

# Check proxy settings
echo $HTTP_PROXY
echo $HTTPS_PROXY
```

**Root Causes:**
1. Firewall blocking outbound HTTPS
2. DNS resolution issues
3. Proxy misconfiguration
4. Network outage

**Resolution:**

```bash
# 1. Allow Stripe API access
sudo iptables -A OUTPUT -p tcp -d api.stripe.com --dport 443 -j ACCEPT

# 2. Configure proxy (if needed)
nself env set HTTP_PROXY="http://proxy.company.com:8080"
nself env set HTTPS_PROXY="http://proxy.company.com:8080"
nself env set NO_PROXY="localhost,127.0.0.1,.local"

# 3. Update DNS servers (if needed)
# Edit: /etc/resolv.conf
nameserver 8.8.8.8
nameserver 8.8.4.4

# 4. Test connectivity
nself billing stripe test-connection

# 5. Restart billing
nself restart billing
```

**Prevention:**
- Whitelist Stripe IPs in firewall
- Use reliable DNS servers
- Monitor network health
- Set up redundant network paths

---

## 2. Usage Tracking Issues

### Issue 2.1: Metrics Not Recording

**Symptoms:**
- Zero usage reported
- Missing data in dashboard
- Stale usage numbers

**Diagnosis:**
```bash
# Check usage recording
nself billing usage show --user user123

# Verify usage collector status
nself billing usage collector-status

# View usage logs
nself logs billing | grep "usage_recorded"

# Check database
nself db query "SELECT COUNT(*) FROM billing_usage WHERE created_at > NOW() - INTERVAL '1 hour'"
```

**Root Causes:**
1. Usage collector not running
2. Database connection failed
3. User ID not found
4. Metric type not configured

**Resolution:**

```bash
# 1. Check collector status
nself status | grep usage-collector

# 2. Restart if stopped
nself restart usage-collector

# 3. Verify database connection
nself db test-connection

# 4. Check user exists
nself db query "SELECT id, email FROM auth.users WHERE id = 'user123'"

# 5. Verify metric configuration
nself billing usage list-metrics

# 6. Test recording manually
nself billing usage record \
  --user user123 \
  --metric api_calls \
  --value 1

# 7. Check if recorded
nself billing usage show --user user123 --metric api_calls
```

**Prevention:**
- Monitor collector uptime
- Set up usage recording alerts
- Validate user IDs before recording
- Regular database health checks

---

### Issue 2.2: Usage Aggregation Delays

**Symptoms:**
- Dashboard shows old data
- Usage numbers lag behind reality
- Billing calculations delayed

**Diagnosis:**
```bash
# Check aggregation job status
nself billing usage aggregation-status

# View aggregation lag
nself billing usage show-lag

# Check job queue
nself billing jobs list | grep aggregation

# View aggregation logs
nself logs billing | grep "aggregation"
```

**Root Causes:**
1. Aggregation job not scheduled
2. High volume overwhelming aggregator
3. Database query performance
4. Lock contention

**Resolution:**

```bash
# 1. Check cron schedule
nself billing usage show-schedule

# 2. Run aggregation manually
nself billing usage aggregate --force

# 3. Check for stuck jobs
nself billing jobs list --status stuck

# 4. Clear stuck jobs
nself billing jobs clear --status stuck

# 5. Optimize aggregation interval
nself env set USAGE_AGGREGATION_INTERVAL=5m  # Every 5 minutes

# 6. Enable parallel aggregation
nself env set USAGE_AGGREGATION_PARALLEL=true
nself env set USAGE_AGGREGATION_WORKERS=4

# 7. Restart billing
nself restart billing

# 8. Monitor lag
watch -n 5 'nself billing usage show-lag'
```

**Prevention:**
- Use appropriate aggregation intervals
- Enable parallel processing for high volume
- Optimize database queries
- Monitor aggregation lag

---

### Issue 2.3: Incorrect Usage Calculations

**Symptoms:**
- Usage numbers don't match actual
- Discrepancies between logs and billing
- Over/under reporting

**Diagnosis:**
```bash
# Compare raw vs aggregated
nself billing usage compare \
  --user user123 \
  --start "2026-01-01" \
  --end "2026-01-31"

# Check for duplicate records
nself db query "
  SELECT metric_type, COUNT(*), user_id
  FROM billing_usage
  WHERE user_id = 'user123'
  GROUP BY metric_type, user_id, created_at
  HAVING COUNT(*) > 1
"

# Verify calculation logic
nself billing usage show-formula --metric api_calls

# Audit trail
nself billing usage audit --user user123 --date 2026-01-15
```

**Root Causes:**
1. Duplicate recordings
2. Wrong aggregation formula
3. Timezone issues
4. Race conditions

**Resolution:**

```bash
# 1. Remove duplicates
nself billing usage deduplicate \
  --user user123 \
  --start "2026-01-01" \
  --end "2026-01-31"

# 2. Verify formulas
cat services/billing/config/usage-metrics.yml

# 3. Check timezone configuration
echo $TZ
nself env get BILLING_TIMEZONE

# 4. Recalculate usage
nself billing usage recalculate \
  --user user123 \
  --month 2026-01

# 5. Verify results
nself billing usage show --user user123 --month 2026-01
```

**Prevention:**
- Use idempotency keys for recordings
- Validate formulas with test cases
- Standardize on UTC timezone
- Implement deduplication logic

---

### Issue 2.4: Missing Data Points

**Symptoms:**
- Gaps in usage timeline
- Incomplete usage history
- Missing metrics for certain periods

**Diagnosis:**
```bash
# Find gaps in timeline
nself billing usage find-gaps \
  --user user123 \
  --start "2026-01-01" \
  --end "2026-01-31"

# Check for missing records
nself db query "
  SELECT DATE(created_at) as date, COUNT(*)
  FROM billing_usage
  WHERE user_id = 'user123'
    AND created_at BETWEEN '2026-01-01' AND '2026-01-31'
  GROUP BY DATE(created_at)
  ORDER BY date
"

# Correlate with service restarts
nself logs billing --grep "restart" --since "2026-01-01"
```

**Root Causes:**
1. Service downtime during critical periods
2. Failed batch recordings
3. Database connection loss
4. Buffer overflow

**Resolution:**

```bash
# 1. Check service uptime
nself billing uptime --start "2026-01-01"

# 2. Restore from backups (if available)
nself billing usage restore \
  --user user123 \
  --date 2026-01-15

# 3. Reconstruct from logs
nself billing usage reconstruct \
  --user user123 \
  --from-logs \
  --date 2026-01-15

# 4. Enable persistent buffering
nself env set USAGE_ENABLE_BUFFER=true
nself env set USAGE_BUFFER_SIZE=10000

# 5. Restart billing
nself restart billing
```

**Prevention:**
- Enable usage buffering
- Implement backup/restore for usage data
- Monitor for gaps regularly
- Set up redundant collectors

---

## 3. Subscription Problems

### Issue 3.1: Subscription Creation Failures

**Symptoms:**
- Error: "Unable to create subscription"
- Subscription stuck in "incomplete"
- Payment method not attached

**Diagnosis:**
```bash
# Check subscription status
nself billing subscription show sub_123

# View creation logs
nself logs billing | grep "subscription_create" | tail -20

# Check Stripe Dashboard
# Dashboard → Subscriptions → [Filter by incomplete]

# Verify customer and payment method
nself billing customer show cus_123
```

**Root Causes:**
1. Missing payment method
2. Invalid price ID
3. Customer not found
4. Insufficient permissions

**Resolution:**

```bash
# 1. Verify customer exists
nself billing customer show cus_123

# 2. Create if missing
nself billing customer create \
  --user user123 \
  --email user@example.com

# 3. Attach payment method
nself billing payment-method attach \
  --customer cus_123 \
  --payment-method pm_123

# 4. Verify price ID
nself billing price show price_123

# 5. Retry subscription creation
nself billing subscription create \
  --customer cus_123 \
  --price price_123 \
  --payment-method pm_123

# 6. Force complete if needed
nself billing subscription complete sub_123 --force
```

**Prevention:**
- Validate customer before subscription
- Verify payment method attached
- Use valid price IDs
- Implement retry logic with idempotency

---

### Issue 3.2: Payment Method Issues

**Symptoms:**
- Error: "Your card was declined"
- Error: "Payment method not found"
- Unable to update payment method

**Diagnosis:**
```bash
# Check payment method status
nself billing payment-method show pm_123

# View recent payment failures
nself billing payments failures --customer cus_123

# Check card details
nself billing payment-method details pm_123

# View Stripe logs
stripe logs list --limit 20
```

**Root Causes:**
1. Card declined by issuer
2. Expired card
3. Insufficient funds
4. 3D Secure required

**Resolution:**

```bash
# 1. Get decline reason
nself billing payment-method decline-reason pm_123

# 2. Notify customer to update
nself billing customer notify \
  --customer cus_123 \
  --template payment_failed

# 3. Retry with different card
nself billing payment-method create \
  --customer cus_123 \
  --card-number "4242424242424242" \
  --exp-month 12 \
  --exp-year 2027 \
  --cvc "123"

# 4. Update subscription
nself billing subscription update sub_123 \
  --payment-method pm_new

# 5. Retry payment
nself billing invoice pay in_123 --retry
```

**Prevention:**
- Send card expiration reminders
- Enable automatic retry logic
- Support multiple payment methods
- Implement 3D Secure

---

### Issue 3.3: Proration Calculation Errors

**Symptoms:**
- Incorrect proration amounts
- Unexpected charges
- Wrong billing dates

**Diagnosis:**
```bash
# View proration details
nself billing subscription prorations sub_123

# Check upcoming invoice
nself billing invoice upcoming --customer cus_123

# Compare expected vs actual
nself billing subscription simulate-change \
  --subscription sub_123 \
  --new-price price_456

# View proration logs
nself logs billing | grep "proration"
```

**Root Causes:**
1. Wrong proration_behavior setting
2. Timezone issues with billing dates
3. Incorrect price amount
4. Trial period overlap

**Resolution:**

```bash
# 1. Check proration behavior
nself billing subscription show-config sub_123

# 2. Set correct proration mode
nself env set STRIPE_PRORATION_BEHAVIOR=create_prorations
# Options: create_prorations, none, always_invoice

# 3. Verify timezone
nself env get BILLING_TIMEZONE

# 4. Recalculate prorations
nself billing subscription recalculate sub_123

# 5. Preview before applying
nself billing subscription preview-change \
  --subscription sub_123 \
  --new-price price_456 \
  --proration-date $(date +%s)

# 6. Apply change
nself billing subscription update sub_123 \
  --price price_456 \
  --proration-date $(date +%s)
```

**Prevention:**
- Use consistent proration_behavior
- Test proration calculations
- Preview changes before applying
- Document proration policy

---

### Issue 3.4: Trial Period Not Working

**Symptoms:**
- Trial not applied
- Immediate charges during trial
- Wrong trial end date

**Diagnosis:**
```bash
# Check trial configuration
nself billing subscription show sub_123 | grep trial

# View trial settings in Stripe
nself billing price show price_123 | grep trial

# Check subscription timeline
nself billing subscription timeline sub_123

# Verify trial eligibility
nself billing customer trial-eligibility cus_123
```

**Root Causes:**
1. Trial not configured on price
2. Customer already used trial
3. Wrong trial period length
4. Trial override not working

**Resolution:**

```bash
# 1. Check if customer eligible
nself billing customer show cus_123 | grep trial_used

# 2. Configure trial period
nself billing subscription create \
  --customer cus_123 \
  --price price_123 \
  --trial-days 14

# 3. Or set specific trial end
nself billing subscription create \
  --customer cus_123 \
  --price price_123 \
  --trial-end "2026-02-14"

# 4. Update existing subscription
nself billing subscription update sub_123 \
  --trial-end "2026-02-14"

# 5. Verify trial applied
nself billing subscription show sub_123 | grep trial_end
```

**Prevention:**
- Configure trials at price level
- Track trial usage per customer
- Validate trial eligibility before creation
- Test trial workflows

---

### Issue 3.5: Cancellation Not Processing

**Symptoms:**
- Subscription still active after cancel
- User still has access
- Billing continues

**Diagnosis:**
```bash
# Check cancellation status
nself billing subscription show sub_123 | grep cancel

# View cancellation request
nself logs billing | grep "cancel.*sub_123"

# Check webhook delivery
nself billing webhook events | grep "customer.subscription.deleted"

# Verify in Stripe Dashboard
stripe subscriptions retrieve sub_123
```

**Root Causes:**
1. Cancel at period end (not immediate)
2. Webhook not processed
3. Cache not invalidated
4. Database update failed

**Resolution:**

```bash
# 1. Check cancellation mode
nself billing subscription show sub_123 | grep cancel_at

# 2. Cancel immediately if needed
nself billing subscription cancel sub_123 --immediate

# 3. Force webhook processing
nself billing webhook replay evt_123

# 4. Clear cache
nself cache clear billing:subscription:sub_123

# 5. Force database sync
nself billing sync subscription sub_123

# 6. Revoke access
nself billing access revoke --user user123

# 7. Verify cancellation
nself billing subscription show sub_123
```

**Prevention:**
- Clearly document cancel behavior
- Process webhooks reliably
- Invalidate caches on cancellation
- Set up cancellation alerts

---

## 4. Invoice Issues

### Issue 4.1: Invoice Generation Failures

**Symptoms:**
- Error: "Unable to create invoice"
- Missing invoices
- Draft invoices not finalizing

**Diagnosis:**
```bash
# Check invoice status
nself billing invoice show in_123

# View generation logs
nself logs billing | grep "invoice_generation"

# List draft invoices
nself billing invoice list --status draft

# Check for errors
nself billing invoice errors --last 24h
```

**Root Causes:**
1. No subscription items
2. Zero amount invoice
3. Missing customer email
4. Tax calculation failure

**Resolution:**

```bash
# 1. Verify subscription has items
nself billing subscription show sub_123 | grep items

# 2. Check invoice line items
nself billing invoice show in_123 --show-lines

# 3. Add items if missing
nself billing invoice add-item in_123 \
  --price price_123 \
  --quantity 1

# 4. Set customer email
nself billing customer update cus_123 \
  --email user@example.com

# 5. Finalize invoice
nself billing invoice finalize in_123

# 6. Or regenerate
nself billing invoice regenerate in_123
```

**Prevention:**
- Validate subscription items
- Check for zero amounts
- Require customer email
- Test invoice generation

---

### Issue 4.2: Incorrect Line Items

**Symptoms:**
- Wrong prices on invoice
- Duplicate line items
- Missing expected charges

**Diagnosis:**
```bash
# View invoice details
nself billing invoice show in_123 --verbose

# Compare with subscription
nself billing subscription show sub_123

# Check usage records
nself billing usage show --user user123 --period "2026-01"

# Audit line items
nself billing invoice audit in_123
```

**Root Causes:**
1. Wrong price attached
2. Metered billing not aggregated
3. Proration errors
4. Manual adjustments

**Resolution:**

```bash
# 1. Verify line items
nself billing invoice show-lines in_123

# 2. Remove incorrect items
nself billing invoice remove-item in_123 --line-item li_123

# 3. Add correct items
nself billing invoice add-item in_123 \
  --price price_correct \
  --quantity 5

# 4. Recalculate usage
nself billing usage recalculate --user user123 --period "2026-01"

# 5. Update invoice
nself billing invoice update in_123

# 6. Preview before finalizing
nself billing invoice preview in_123
```

**Prevention:**
- Validate prices before invoicing
- Test usage aggregation
- Preview invoices before finalizing
- Audit line items regularly

---

### Issue 4.3: Tax Calculation Errors

**Symptoms:**
- Wrong tax amount
- Missing tax
- Tax rate not applied

**Diagnosis:**
```bash
# Check tax configuration
nself billing tax show-config

# View customer tax info
nself billing customer show cus_123 | grep tax

# Check tax rates
nself billing tax list-rates

# View invoice tax details
nself billing invoice show in_123 --show-tax
```

**Root Causes:**
1. Tax calculation disabled
2. Missing customer address
3. Wrong tax ID
4. Invalid tax rate

**Resolution:**

```bash
# 1. Enable tax calculation
nself env set STRIPE_TAX_CALCULATION=automatic

# 2. Update customer address
nself billing customer update cus_123 \
  --address-line1 "123 Main St" \
  --address-city "San Francisco" \
  --address-state "CA" \
  --address-postal-code "94102" \
  --address-country "US"

# 3. Set tax ID
nself billing customer set-tax-id cus_123 \
  --type us_ein \
  --value "12-3456789"

# 4. Apply tax rate
nself billing invoice add-tax in_123 \
  --tax-rate txr_123

# 5. Recalculate
nself billing invoice recalculate in_123

# 6. Restart billing (to apply config)
nself restart billing
```

**Prevention:**
- Enable automatic tax calculation
- Require customer address
- Validate tax IDs
- Test tax calculations

---

### Issue 4.4: PDF Generation Problems

**Symptoms:**
- PDF not generating
- Broken PDF links
- Missing invoice details in PDF

**Diagnosis:**
```bash
# Check PDF generation status
nself billing invoice pdf-status in_123

# View generation logs
nself logs billing | grep "pdf_generation"

# Test PDF generation
nself billing invoice generate-pdf in_123 --test

# Check PDF service
nself status | grep pdf-generator
```

**Root Causes:**
1. PDF service not running
2. Template rendering error
3. Missing fonts or assets
4. Memory limits

**Resolution:**

```bash
# 1. Check PDF service
nself status pdf-generator

# 2. Restart if stopped
nself restart pdf-generator

# 3. Regenerate PDF
nself billing invoice regenerate-pdf in_123

# 4. Check template
nself billing invoice show-template

# 5. Increase memory (if needed)
nself env set PDF_MEMORY_LIMIT=512M

# 6. Restart services
nself restart billing pdf-generator

# 7. Test generation
nself billing invoice generate-pdf in_123
```

**Prevention:**
- Monitor PDF service health
- Test templates regularly
- Set appropriate memory limits
- Cache generated PDFs

---

### Issue 4.5: Email Delivery Failures

**Symptoms:**
- Invoices not sent to customers
- Email errors in logs
- Customers not receiving receipts

**Diagnosis:**
```bash
# Check email configuration
nself billing email show-config

# View sent emails
nself billing email list --type invoice

# Check delivery status
nself billing email status msg_123

# View email logs
nself logs mail | grep "invoice"
```

**Root Causes:**
1. Email service not configured
2. Invalid customer email
3. Email bounced
4. Rate limiting

**Resolution:**

```bash
# 1. Verify email configuration
nself env get MAIL_ENABLED
nself env get MAIL_FROM

# 2. Enable email service
nself env set MAIL_ENABLED=true
nself env set MAIL_FROM="billing@example.com"

# 3. Update customer email
nself billing customer update cus_123 \
  --email valid@example.com

# 4. Resend invoice
nself billing invoice send in_123

# 5. Check delivery
nself billing email track msg_123

# 6. Restart mail service
nself restart mail
```

**Prevention:**
- Validate customer emails
- Monitor email delivery rates
- Handle bounces properly
- Use reliable email service

---

## 5. Quota Enforcement

### Issue 5.1: Quotas Not Enforcing

**Symptoms:**
- Users exceeding limits
- No quota warnings
- Overage charges not applied

**Diagnosis:**
```bash
# Check quota configuration
nself billing quota show --user user123

# View current usage vs limits
nself billing quota status --user user123

# Check enforcement logs
nself logs billing | grep "quota_enforcement"

# Verify enforcement enabled
nself env get QUOTA_ENFORCEMENT_ENABLED
```

**Root Causes:**
1. Quota enforcement disabled
2. Wrong quota limits
3. Usage not tracked
4. Race conditions

**Resolution:**

```bash
# 1. Enable quota enforcement
nself env set QUOTA_ENFORCEMENT_ENABLED=true

# 2. Set quota limits
nself billing quota set \
  --user user123 \
  --metric api_calls \
  --limit 1000 \
  --period month

# 3. Verify usage tracking
nself billing usage show --user user123

# 4. Force quota check
nself billing quota check --user user123

# 5. Restart billing
nself restart billing

# 6. Test enforcement
nself billing quota test-enforce --user user123
```

**Prevention:**
- Enable quota enforcement
- Set appropriate limits
- Monitor quota usage
- Test enforcement logic

---

### Issue 5.2: False Quota Violations

**Symptoms:**
- Users blocked incorrectly
- Quota shows exceeded but shouldn't
- Usage numbers don't match logs

**Diagnosis:**
```bash
# Compare quota vs actual usage
nself billing quota compare --user user123

# Check for double-counting
nself billing usage audit --user user123 --check-duplicates

# View quota calculation
nself billing quota show-calculation --user user123

# Check reset timing
nself billing quota show-resets --user user123
```

**Root Causes:**
1. Duplicate usage records
2. Wrong quota period
3. Timezone issues
4. Cache not cleared after reset

**Resolution:**

```bash
# 1. Remove duplicate records
nself billing usage deduplicate --user user123

# 2. Verify quota period
nself billing quota show --user user123 | grep period

# 3. Check timezone
nself env get BILLING_TIMEZONE

# 4. Clear quota cache
nself cache clear billing:quota:user123

# 5. Recalculate usage
nself billing usage recalculate --user user123

# 6. Reset quota if needed
nself billing quota reset --user user123

# 7. Unblock user
nself billing quota unblock --user user123
```

**Prevention:**
- Prevent duplicate recordings
- Use consistent timezones
- Clear cache on reset
- Regular usage audits

---

### Issue 5.3: Quota Reset Not Working

**Symptoms:**
- Quotas not resetting at period end
- Users still blocked after reset
- Wrong reset timestamps

**Diagnosis:**
```bash
# Check reset schedule
nself billing quota show-schedule

# View last reset times
nself billing quota last-reset --user user123

# Check reset job status
nself billing jobs list | grep quota_reset

# View reset logs
nself logs billing | grep "quota_reset"
```

**Root Causes:**
1. Reset job not scheduled
2. Job failed to run
3. Wrong reset timing
4. Database lock

**Resolution:**

```bash
# 1. Check cron schedule
nself billing quota show-cron

# 2. Run reset manually
nself billing quota reset-all --force

# 3. Verify reset for specific user
nself billing quota reset --user user123

# 4. Clear failed jobs
nself billing jobs clear --type quota_reset --status failed

# 5. Reschedule job
nself billing quota schedule-reset

# 6. Restart billing
nself restart billing

# 7. Monitor next reset
watch -n 60 'nself billing quota next-reset'
```

**Prevention:**
- Monitor reset job execution
- Set up alerts for failed resets
- Test reset logic
- Use reliable job scheduler

---

### Issue 5.4: Overage Calculation Incorrect

**Symptoms:**
- Wrong overage charges
- No overage charges when expected
- Incorrect overage amounts

**Diagnosis:**
```bash
# View overage calculation
nself billing quota show-overage --user user123

# Check overage pricing
nself billing price show --type overage

# Compare expected vs actual
nself billing quota calculate-overage \
  --user user123 \
  --actual-usage 1500 \
  --quota-limit 1000

# Audit overage records
nself billing overage audit --user user123
```

**Root Causes:**
1. Wrong overage rate
2. Incorrect usage aggregation
3. Tiered pricing not applied
4. Proration errors

**Resolution:**

```bash
# 1. Verify overage pricing
nself billing price show price_overage_123

# 2. Check tiered pricing config
cat services/billing/config/overage-tiers.yml

# 3. Recalculate overage
nself billing overage recalculate --user user123

# 4. Update if needed
nself billing overage update \
  --user user123 \
  --usage 1500 \
  --quota 1000 \
  --rate 0.10

# 5. Verify invoice
nself billing invoice show --user user123 --show-overage
```

**Prevention:**
- Test overage calculations
- Document pricing tiers
- Validate aggregation logic
- Preview overages before invoicing

---

## 6. Webhook Problems

### Issue 6.1: Webhooks Not Being Received

**Symptoms:**
- Events not processing
- Subscription updates not reflected
- Payment status not updating

**Diagnosis:**
```bash
# Check webhook endpoint status
nself billing webhook status

# View recent webhook deliveries in Stripe
# Dashboard → Developers → Webhooks → [Endpoint] → Attempts

# Test webhook endpoint
curl -X POST https://yourapp.com/webhooks/stripe \
  -H "Content-Type: application/json" \
  -d '{"type":"ping"}'

# Check firewall rules
sudo iptables -L | grep 443

# View webhook logs
nself logs billing | grep "webhook_received"
```

**Root Causes:**
1. Endpoint unreachable
2. Firewall blocking Stripe IPs
3. SSL certificate invalid
4. Endpoint disabled in Stripe

**Resolution:**

```bash
# 1. Verify endpoint accessible
curl -I https://yourapp.com/webhooks/stripe

# 2. Allow Stripe IPs
sudo iptables -A INPUT -p tcp --dport 443 -s 54.187.174.169 -j ACCEPT
# See full list: https://stripe.com/docs/ips

# 3. Check SSL certificate
echo | openssl s_client -connect yourapp.com:443 -servername yourapp.com

# 4. Enable endpoint in Stripe Dashboard
# Dashboard → Developers → Webhooks → [Endpoint] → Enable

# 5. Update endpoint URL if needed
nself billing webhook update-url https://yourapp.com/webhooks/stripe

# 6. Test with Stripe CLI
stripe listen --forward-to localhost:8080/webhooks/stripe

# 7. Restart billing
nself restart billing
```

**Prevention:**
- Monitor webhook delivery rates
- Whitelist Stripe IPs
- Keep SSL certificates valid
- Set up webhook alerts

---

### Issue 6.2: Duplicate Webhook Processing

**Symptoms:**
- Same event processed multiple times
- Duplicate charges
- Multiple emails sent

**Diagnosis:**
```bash
# Check for duplicate events
nself db query "
  SELECT event_id, COUNT(*)
  FROM billing_webhook_events
  GROUP BY event_id
  HAVING COUNT(*) > 1
"

# View event processing logs
nself logs billing | grep "evt_123"

# Check idempotency keys
nself billing webhook show-idempotency evt_123
```

**Root Causes:**
1. No idempotency key handling
2. Webhook retries processed
3. Database transaction not used
4. Race conditions

**Resolution:**

```bash
# 1. Enable idempotency tracking
nself env set WEBHOOK_IDEMPOTENCY_ENABLED=true

# 2. Clean up duplicates
nself billing webhook deduplicate --dry-run
nself billing webhook deduplicate --execute

# 3. Mark duplicates as processed
nself db query "
  UPDATE billing_webhook_events
  SET status = 'duplicate'
  WHERE event_id = 'evt_123' AND id != (
    SELECT MIN(id) FROM billing_webhook_events WHERE event_id = 'evt_123'
  )
"

# 4. Restart billing
nself restart billing

# 5. Verify no more duplicates
nself billing webhook check-duplicates
```

**Prevention:**
- Implement idempotency key tracking
- Use database transactions
- Handle webhook retries properly
- Test duplicate scenarios

---

### Issue 6.3: Webhook Timeout Errors

**Symptoms:**
- Stripe shows timeout errors
- Webhooks marked as failed
- Retries exhausted

**Diagnosis:**
```bash
# Check webhook processing time
nself billing webhook show-timing

# View slow webhooks
nself billing webhook slow --threshold 5s

# Check webhook handler performance
nself logs billing | grep "webhook_processed" | grep "duration"

# Monitor webhook queue
nself billing webhook queue-status
```

**Root Causes:**
1. Slow database queries
2. Blocking operations in handler
3. External API calls
4. Not using background jobs

**Resolution:**

```bash
# 1. Enable async webhook processing
nself env set WEBHOOK_ASYNC_PROCESSING=true

# 2. Configure timeout
nself env set WEBHOOK_TIMEOUT=10s

# 3. Use job queue for slow operations
nself env set WEBHOOK_USE_QUEUE=true

# 4. Optimize database queries
# Review and index: services/billing/src/webhooks/handlers.js

# 5. Restart billing
nself restart billing

# 6. Replay failed webhooks
nself billing webhook replay --status failed --since 24h

# 7. Monitor processing times
watch -n 5 'nself billing webhook show-timing'
```

**Prevention:**
- Process webhooks asynchronously
- Use background jobs for slow tasks
- Optimize database queries
- Monitor processing times

---

### Issue 6.4: Event Ordering Issues

**Symptoms:**
- Events processed out of order
- State inconsistencies
- Subscription status wrong

**Diagnosis:**
```bash
# Check event order
nself billing webhook show-order --object sub_123

# View event timeline
nself billing webhook timeline sub_123

# Check for race conditions
nself logs billing | grep "concurrent.*sub_123"

# Verify current state
nself billing subscription show sub_123 --compare-stripe
```

**Root Causes:**
1. Concurrent webhook processing
2. No event version tracking
3. Retry interference
4. Async processing without ordering

**Resolution:**

```bash
# 1. Enable sequential processing per object
nself env set WEBHOOK_SEQUENTIAL_PER_OBJECT=true

# 2. Enable event versioning
nself env set WEBHOOK_TRACK_VERSIONS=true

# 3. Add processing lock
nself env set WEBHOOK_USE_LOCKS=true

# 4. Rebuild state from events
nself billing subscription rebuild-state sub_123

# 5. Sync with Stripe
nself billing sync subscription sub_123

# 6. Restart billing
nself restart billing
```

**Prevention:**
- Process events sequentially per object
- Track event versions
- Use locks for critical updates
- Test concurrent scenarios

---

## 7. Data Sync Issues

### Issue 7.1: Database and Stripe Out of Sync

**Symptoms:**
- Data mismatch between systems
- Subscription shows different status
- Customer info inconsistent

**Diagnosis:**
```bash
# Compare database vs Stripe
nself billing sync compare

# Check specific subscription
nself billing subscription compare sub_123

# Find all inconsistencies
nself billing sync find-inconsistencies

# View sync logs
nself logs billing | grep "sync"
```

**Root Causes:**
1. Webhook processing failed
2. Manual changes in Stripe
3. Database transaction rollback
4. Network failure during sync

**Resolution:**

```bash
# 1. Check last sync time
nself billing sync status

# 2. Sync specific object
nself billing sync subscription sub_123

# 3. Sync customer
nself billing sync customer cus_123

# 4. Full sync (careful!)
nself billing sync all --dry-run
nself billing sync all --execute

# 5. Enable auto-sync
nself env set BILLING_AUTO_SYNC=true
nself env set BILLING_SYNC_INTERVAL=5m

# 6. Restart billing
nself restart billing
```

**Prevention:**
- Enable automatic sync
- Monitor sync status
- Process webhooks reliably
- Use database transactions

---

### Issue 7.2: Missing Customers or Subscriptions

**Symptoms:**
- Customer exists in Stripe but not database
- Subscription not found locally
- User can't access billing info

**Diagnosis:**
```bash
# Check if customer exists in both
nself billing customer show cus_123 --compare

# Find orphaned records
nself billing sync find-orphaned

# Check webhook history
nself billing webhook events --object cus_123

# View customer creation logs
nself logs billing | grep "customer_create.*cus_123"
```

**Root Causes:**
1. Webhook not delivered
2. Initial sync incomplete
3. Database write failed
4. Record deleted manually

**Resolution:**

```bash
# 1. Import from Stripe
nself billing customer import cus_123

# 2. Or create from scratch
nself billing customer create \
  --user user123 \
  --stripe-customer cus_123

# 3. Import subscription
nself billing subscription import sub_123

# 4. Link to user
nself billing customer link \
  --customer cus_123 \
  --user user123

# 5. Verify imported
nself billing customer show cus_123
nself billing subscription show sub_123
```

**Prevention:**
- Run initial sync on setup
- Monitor for orphaned records
- Enable auto-import for new webhooks
- Regular reconciliation

---

### Issue 7.3: Reconciliation Failures

**Symptoms:**
- Reconciliation job failing
- Too many discrepancies
- Sync taking too long

**Diagnosis:**
```bash
# Check reconciliation status
nself billing reconcile status

# View discrepancies
nself billing reconcile show-diff

# Check job logs
nself logs billing | grep "reconciliation"

# Count records to sync
nself billing reconcile count
```

**Root Causes:**
1. Too many records to process
2. API rate limits hit
3. Database connection pool exhausted
4. Memory limits

**Resolution:**

```bash
# 1. Reconcile in batches
nself billing reconcile --batch-size 100

# 2. Filter by date range
nself billing reconcile \
  --start "2026-01-01" \
  --end "2026-01-31"

# 3. Increase resources
nself env set BILLING_MEMORY_LIMIT=2G
nself env set BILLING_DB_POOL_SIZE=20

# 4. Enable caching
nself env set BILLING_CACHE_ENABLED=true

# 5. Restart billing
nself restart billing

# 6. Run reconciliation
nself billing reconcile --resume

# 7. Monitor progress
watch -n 10 'nself billing reconcile status'
```

**Prevention:**
- Run reconciliation regularly
- Use appropriate batch sizes
- Monitor resource usage
- Cache frequently accessed data

---

## 8. Performance Issues

### Issue 8.1: Slow Billing Operations

**Symptoms:**
- API requests timing out
- Webhook processing slow
- Dashboard loading slowly

**Diagnosis:**
```bash
# Check service performance
nself billing performance

# View slow queries
nself db slow-queries --limit 10

# Check CPU/memory usage
docker stats billing-service

# Profile billing operations
nself billing profile --duration 60s

# View performance metrics
nself billing metrics --show-percentiles
```

**Root Causes:**
1. Unoptimized database queries
2. Missing indexes
3. Too many API calls
4. Insufficient resources

**Resolution:**

```bash
# 1. Add database indexes
nself db query "
  CREATE INDEX idx_billing_usage_user_date
  ON billing_usage(user_id, created_at);

  CREATE INDEX idx_billing_subscriptions_customer
  ON billing_subscriptions(customer_id, status);
"

# 2. Enable query caching
nself env set BILLING_CACHE_ENABLED=true
nself env set BILLING_CACHE_TTL=300

# 3. Use connection pooling
nself env set BILLING_DB_POOL_SIZE=10
nself env set BILLING_DB_POOL_TIMEOUT=30s

# 4. Increase resources
docker update billing-service --cpus="2" --memory="2g"

# 5. Enable request batching
nself env set STRIPE_BATCH_REQUESTS=true

# 6. Restart billing
nself restart billing

# 7. Monitor improvements
nself billing performance --compare
```

**Prevention:**
- Regular performance testing
- Monitor slow queries
- Proper indexing strategy
- Cache frequently accessed data

---

### Issue 8.2: Database Query Performance

**Symptoms:**
- Queries taking seconds
- High database CPU
- Blocked transactions

**Diagnosis:**
```bash
# Show slow queries
nself db slow-queries --min-time 1s

# Check table sizes
nself db query "
  SELECT
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
  FROM pg_tables
  WHERE schemaname = 'billing'
  ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
"

# Check for missing indexes
nself db missing-indexes

# View query plans
nself db explain "
  SELECT * FROM billing_usage
  WHERE user_id = 'user123'
  AND created_at > NOW() - INTERVAL '1 month'
"
```

**Root Causes:**
1. Missing indexes
2. Full table scans
3. Inefficient queries
4. Large table sizes

**Resolution:**

```bash
# 1. Add recommended indexes
nself db add-index billing_usage \
  --columns "user_id,created_at" \
  --name idx_usage_user_date

# 2. Optimize queries
# Edit: services/billing/src/db/queries.js
# Use: SELECT specific columns, not SELECT *
# Add: WHERE clauses with indexed columns
# Avoid: N+1 queries, use JOIN instead

# 3. Partition large tables
nself db partition billing_usage \
  --by-month \
  --keep-months 12

# 4. Vacuum and analyze
nself db vacuum billing_usage
nself db analyze billing_usage

# 5. Archive old data
nself billing archive-usage \
  --before "2025-01-01" \
  --to-s3

# 6. Monitor query performance
watch -n 5 'nself db slow-queries --min-time 1s'
```

**Prevention:**
- Index foreign keys and date columns
- Regular vacuum and analyze
- Archive old data
- Monitor query patterns

---

### Issue 8.3: High Memory Usage

**Symptoms:**
- Out of memory errors
- Container restarts
- Slow performance
- Swap usage high

**Diagnosis:**
```bash
# Check memory usage
docker stats billing-service

# View memory by process
docker exec billing-service ps aux --sort=-%mem | head -n 10

# Check for memory leaks
nself billing memory-profile --duration 300s

# View heap snapshot
nself billing heap-snapshot
```

**Root Causes:**
1. Memory leaks
2. Large data sets in memory
3. Insufficient memory limits
4. Not releasing resources

**Resolution:**

```bash
# 1. Increase memory limit
docker update billing-service --memory="4g"

# Or in docker-compose.yml:
# mem_limit: 4g
# memswap_limit: 4g

# 2. Enable garbage collection
nself env set NODE_OPTIONS="--max-old-space-size=3072"

# 3. Implement pagination
# Edit: services/billing/src/api/routes.js
# Add: ?limit=100&offset=0 to queries

# 4. Stream large responses
# Use: res.write() instead of res.json() for large datasets

# 5. Clear caches periodically
nself cache clear billing:* --ttl-expired

# 6. Restart billing
nself restart billing

# 7. Monitor memory
watch -n 5 'docker stats billing-service --no-stream'
```

**Prevention:**
- Set appropriate memory limits
- Use streaming for large data
- Implement pagination
- Monitor memory trends

---

### Issue 8.4: Timeout Issues

**Symptoms:**
- Request timeout errors
- Gateway timeout (504)
- Long-running operations failing

**Diagnosis:**
```bash
# Check timeout configuration
nself billing show-timeouts

# View slow operations
nself logs billing | grep "timeout\|took.*ms" | tail -20

# Check nginx timeout
nself exec nginx cat /etc/nginx/nginx.conf | grep timeout

# Test operation timing
time nself billing invoice generate in_123
```

**Root Causes:**
1. Low timeout settings
2. Slow operations
3. Network latency
4. Blocking operations

**Resolution:**

```bash
# 1. Increase API timeout
nself env set API_TIMEOUT=60s

# 2. Increase nginx timeout
# Edit: nginx/nginx.conf
proxy_read_timeout 60s;
proxy_connect_timeout 60s;
proxy_send_timeout 60s;

# 3. Increase database timeout
nself env set DB_QUERY_TIMEOUT=30s

# 4. Use async operations
nself env set BILLING_ASYNC_OPERATIONS=true

# 5. Implement operation queuing
nself env set BILLING_USE_QUEUE=true

# 6. Reload nginx
nself exec nginx nginx -s reload

# 7. Restart billing
nself restart billing

# 8. Test timeout resolution
time nself billing invoice generate in_123
```

**Prevention:**
- Set reasonable timeouts
- Use async for long operations
- Optimize slow operations
- Monitor operation durations

---

## 9. Testing & Development

### Issue 9.1: Test Mode Not Working

**Symptoms:**
- Test charges creating real payments
- Can't switch to test mode
- Test data in production

**Diagnosis:**
```bash
# Check current mode
nself env get STRIPE_MODE

# Verify API key type
echo $STRIPE_SECRET_KEY | grep -o "^sk_[^_]*"
# Should show: sk_test (not sk_live)

# Check test mode in Stripe
nself billing stripe show-mode

# View recent transactions
nself billing transactions list --limit 5
```

**Root Causes:**
1. Using live API key
2. Wrong environment variable
3. Mode not set
4. Key leaked to production

**Resolution:**

```bash
# 1. Set test mode explicitly
nself env set STRIPE_MODE=test

# 2. Use test API key
nself env set STRIPE_SECRET_KEY="sk_test_PLACEHOLDER_test_key"

# 3. Set test publishable key
nself env set STRIPE_PUBLISHABLE_KEY="pk_test_your_test_key"

# 4. Clear any production data
nself db query "TRUNCATE TABLE billing_transactions CASCADE"

# 5. Restart billing
nself restart billing

# 6. Verify test mode
nself billing stripe show-mode

# 7. Test with test card
nself billing test-payment \
  --card "4242424242424242" \
  --amount 1000
```

**Prevention:**
- Use separate environments
- Store keys in `.env.local` (gitignored)
- Label test environments clearly
- Never commit real API keys

---

### Issue 9.2: Sandbox Data Issues

**Symptoms:**
- Test customers not appearing
- Sandbox data not seeding
- Sample data errors

**Diagnosis:**
```bash
# Check seed data status
nself billing seed status

# List test customers
nself billing customer list --test-mode

# View seed logs
nself logs billing | grep "seed"

# Check sample data files
ls -la services/billing/data/seeds/
```

**Root Causes:**
1. Seed script not run
2. Test mode not enabled
3. Database constraints failing
4. Duplicate data

**Resolution:**

```bash
# 1. Enable test mode
nself env set STRIPE_MODE=test

# 2. Clear existing test data
nself billing seed clear --confirm

# 3. Run seed script
nself billing seed run

# 4. Or seed specific data
nself billing seed customers
nself billing seed subscriptions
nself billing seed invoices

# 5. Verify seeded data
nself billing customer list --test-mode
nself billing subscription list --test-mode

# 6. Create custom test data
nself billing test-data create \
  --customers 10 \
  --subscriptions 5 \
  --invoices 20
```

**Prevention:**
- Document seed process
- Automate test data creation
- Use consistent test data
- Clear data between tests

---

### Issue 9.3: Mock Stripe Problems

**Symptoms:**
- Mock server not responding
- Test webhooks not firing
- Invalid mock responses

**Diagnosis:**
```bash
# Check mock server status
nself status | grep stripe-mock

# Test mock endpoint
curl http://localhost:12111/v1/customers

# View mock logs
nself logs stripe-mock

# Check configuration
nself env get STRIPE_MOCK_ENABLED
```

**Root Causes:**
1. Mock server not running
2. Wrong port configuration
3. Mock not in test mode
4. Missing mock fixtures

**Resolution:**

```bash
# 1. Start mock server
nself start stripe-mock

# Or with Docker:
docker run -d --name stripe-mock \
  -p 12111:12111 \
  stripemock/stripe-mock:latest

# 2. Configure billing to use mock
nself env set STRIPE_MOCK_ENABLED=true
nself env set STRIPE_API_BASE=http://localhost:12111

# 3. Add mock fixtures
mkdir -p services/billing/test/fixtures
# Add: customers.json, subscriptions.json, etc.

# 4. Restart billing
nself restart billing

# 5. Test mock
nself billing test --use-mock
```

**Prevention:**
- Use stripe-mock for local development
- Document mock setup
- Maintain fixture files
- Test with mock regularly

---

## 10. Common Error Messages

### Error 10.1: "Invalid API key provided"

**Full Error:**
```
Error: Invalid API key provided: sk_test_PLACEHOLDER***
```

**Meaning:** Stripe rejected the API key

**Quick Fix:**
```bash
# 1. Verify key format
echo $STRIPE_SECRET_KEY

# 2. Get new key from Stripe Dashboard
# https://dashboard.stripe.com/apikeys

# 3. Update environment
nself env set STRIPE_SECRET_KEY="sk_test_PLACEHOLDER_key"

# 4. Restart
nself restart billing
```

**Related Errors:**
- "This API key is expired"
- "This API key was deleted"
- "Insufficient permissions"

---

### Error 10.2: "No such customer"

**Full Error:**
```
Error: No such customer: cus_123
```

**Meaning:** Customer ID doesn't exist in Stripe

**Quick Fix:**
```bash
# 1. Check if customer exists
stripe customers retrieve cus_123

# 2. Create if missing
nself billing customer create \
  --user user123 \
  --email user@example.com

# 3. Or import from Stripe
nself billing customer import cus_123

# 4. Link to user
nself billing customer link --customer cus_123 --user user123
```

**Related Errors:**
- "No such subscription"
- "No such invoice"
- "No such payment method"

---

### Error 10.3: "Amount must be at least $0.50"

**Full Error:**
```
Error: Amount must be at least $0.50 usd
```

**Meaning:** Stripe minimum charge amount not met

**Quick Fix:**
```bash
# 1. Check invoice amount
nself billing invoice show in_123

# 2. Add minimum charge line item
nself billing invoice add-item in_123 \
  --description "Minimum charge" \
  --amount 50

# 3. Or configure minimum
nself env set BILLING_MINIMUM_CHARGE=50

# 4. Update invoice
nself billing invoice update in_123
```

**Prevention:**
- Enforce minimum charges
- Accumulate small charges
- Waive charges below minimum

---

### Error 10.4: "Your card was declined"

**Full Error:**
```
Error: Your card was declined. Your request was in test mode, but used a non test card.
```

**Meaning:** Using real card in test mode or card declined

**Quick Fix:**
```bash
# Test mode - use test card
nself billing test-payment \
  --card "4242424242424242" \
  --exp "12/27" \
  --cvc "123"

# Production - contact customer
nself billing customer notify \
  --customer cus_123 \
  --template payment_failed
```

**Test Cards:**
- Success: `4242424242424242`
- Declined: `4000000000000002`
- Insufficient funds: `4000000000009995`
- 3D Secure: `4000002500003155`

**Related Errors:**
- "Insufficient funds"
- "Card expired"
- "Incorrect CVC"

---

### Error 10.5: "Rate limit exceeded"

**Full Error:**
```
Error: Rate limit exceeded. Too many requests hit the API too quickly.
```

**Meaning:** Stripe API rate limit hit

**Quick Fix:**
```bash
# 1. Enable retry with backoff
nself env set STRIPE_MAX_RETRIES=3
nself env set STRIPE_RETRY_BACKOFF=exponential

# 2. Enable caching
nself env set STRIPE_ENABLE_CACHE=true

# 3. Reduce request rate
nself env set STRIPE_RATE_LIMIT_BUFFER=0.8

# 4. Restart
nself restart billing
```

**Prevention:**
- Implement exponential backoff
- Cache API responses
- Batch operations
- Monitor rate limits

---

### Error 10.6: "idempotency_key_in_use"

**Full Error:**
```
Error: Keys for idempotent requests can only be used with the same parameters they were first used with.
```

**Meaning:** Reusing idempotency key with different parameters

**Quick Fix:**
```bash
# 1. Generate new idempotency key
uuidgen

# 2. Retry with new key
nself billing payment create \
  --amount 1000 \
  --customer cus_123 \
  --idempotency-key "$(uuidgen)"

# 3. Clear old keys (dev only)
nself cache clear billing:idempotency:*
```

**Prevention:**
- Generate unique keys per request
- Don't retry with same key if parameters changed
- Expire old keys

---

### Error 10.7: "resource_missing"

**Full Error:**
```
Error: No such price: price_123
```

**Meaning:** Referenced resource doesn't exist

**Quick Fix:**
```bash
# 1. List available prices
nself billing price list

# 2. Create if missing
nself billing price create \
  --product prod_123 \
  --amount 999 \
  --currency usd \
  --interval month

# 3. Update references
nself billing subscription update sub_123 \
  --price price_new
```

**Related Errors:**
- "No such product"
- "No such coupon"
- "No such tax rate"

---

### Error 10.8: "parameter_invalid_integer"

**Full Error:**
```
Error: Invalid integer: abc
```

**Meaning:** Non-integer value where integer expected

**Quick Fix:**
```bash
# Wrong
nself billing payment create --amount "10.00"

# Right (amount in cents)
nself billing payment create --amount 1000

# Wrong
nself billing usage record --value "1.5"

# Right (integer value)
nself billing usage record --value 2
```

**Prevention:**
- Use integers for amounts (cents)
- Validate input types
- Read API documentation

---

### Error 10.9: "subscription_payment_intent_requires_action"

**Full Error:**
```
Error: This subscription requires customer action to authenticate payment.
```

**Meaning:** 3D Secure authentication required

**Quick Fix:**
```bash
# 1. Get payment intent
nself billing subscription show sub_123 | grep payment_intent

# 2. Get client secret
nself billing payment-intent show pi_123 --show-secret

# 3. Complete on frontend with Stripe.js
# Use: stripe.confirmCardPayment(clientSecret)

# 4. Or simulate in test mode
stripe payment_intents confirm pi_123
```

**Prevention:**
- Implement 3D Secure handling
- Use Stripe.js for frontend
- Test with 3DS test cards

---

### Error 10.10: "webhook_signature_verification_failed"

**Full Error:**
```
Error: No signatures found matching the expected signature for payload
```

**Meaning:** Webhook signature doesn't match

**Quick Fix:**
```bash
# 1. Get webhook secret from Stripe
# Dashboard → Developers → Webhooks → [Endpoint] → Signing secret

# 2. Update secret
nself env set STRIPE_WEBHOOK_SECRET="whsec_new_secret"

# 3. Check system time
date -u

# 4. Sync time if needed
sudo ntpdate -s time.nist.gov

# 5. Restart
nself restart billing

# 6. Test webhook
stripe trigger payment_intent.succeeded
```

**Prevention:**
- Keep webhook secret secure
- Sync system time
- Validate signatures always

---

## Emergency Procedures

### Emergency 1: Complete Billing System Failure

**Symptoms:**
- All billing operations failing
- Multiple services down
- Critical errors in logs

**Immediate Actions:**

```bash
# 1. Stop all billing services
nself stop billing

# 2. Check system resources
df -h  # Disk space
free -h  # Memory
top  # CPU usage

# 3. View all billing logs
nself logs billing --tail 1000 > /tmp/billing-emergency.log

# 4. Check database connectivity
nself db ping

# 5. Check Stripe status
curl -I https://status.stripe.com

# 6. Start in safe mode (minimal features)
nself env set BILLING_SAFE_MODE=true
nself start billing

# 7. Notify team
nself alert send --severity critical --message "Billing system failure"

# 8. Enable maintenance mode
nself maintenance on --message "Billing system under maintenance"
```

**Recovery Process:**
1. Identify root cause from logs
2. Fix immediate issue
3. Restart services incrementally
4. Verify each component
5. Disable safe mode
6. Disable maintenance mode
7. Monitor closely

---

### Emergency 2: Mass Overcharging Incident

**Symptoms:**
- Multiple customers reporting overcharges
- Duplicate charges appearing
- Wrong amounts billed

**Immediate Actions:**

```bash
# 1. STOP billing immediately
nself stop billing

# 2. Disable auto-payments
nself env set BILLING_AUTO_PAYMENT=false

# 3. Identify affected customers
nself billing audit charges \
  --since "2026-01-30 00:00:00" \
  --status suspicious

# 4. Export for analysis
nself billing export charges \
  --since "2026-01-30 00:00:00" \
  --format csv \
  --output /tmp/charges.csv

# 5. Issue immediate refunds
nself billing refund batch \
  --input /tmp/affected-customers.csv \
  --reason "billing_error" \
  --notify

# 6. Send notification
nself billing notify-customers \
  --template billing_error_apology \
  --customers /tmp/affected-customers.csv

# 7. Document incident
nself billing incident create \
  --type overcharge \
  --severity critical \
  --description "Mass overcharging on 2026-01-30"
```

**Recovery:**
1. Fix billing logic
2. Test thoroughly
3. Review all recent charges
4. Enable billing cautiously
5. Monitor closely
6. Follow up with customers

---

### Emergency 3: Data Breach - API Keys Exposed

**Symptoms:**
- API keys leaked in logs/code
- Unauthorized API usage
- Suspicious transactions

**Immediate Actions:**

```bash
# 1. Revoke compromised keys immediately
# Stripe Dashboard → Developers → API keys → [Key] → Delete

# 2. Generate new keys
# Dashboard → Developers → API keys → Create secret key

# 3. Update application
nself env set STRIPE_SECRET_KEY="sk_live_NEW_KEY"
nself env set STRIPE_PUBLISHABLE_KEY="pk_live_NEW_KEY"

# 4. Rotate webhook secrets
nself billing webhook rotate-secret

# 5. Audit all transactions
nself billing audit transactions \
  --since "EXPOSURE_TIME" \
  --output /tmp/audit.csv

# 6. Contact Stripe support
# https://support.stripe.com

# 7. Review all customers
nself billing customers audit --check-fraud

# 8. Enable enhanced security
nself env set STRIPE_ENABLE_IP_WHITELIST=true
nself env set STRIPE_REQUIRE_2FA=true

# 9. Restart with new keys
nself restart billing

# 10. Document incident
echo "API key exposure on $(date)" > /tmp/security-incident.txt
```

**Follow-Up:**
1. Investigate how keys were exposed
2. Fix security holes
3. Review access controls
4. Update security policies
5. Train team on key management
6. Set up key rotation schedule

---

### Emergency 4: Payment Processing Completely Down

**Symptoms:**
- All payments failing
- Stripe API unreachable
- Critical business impact

**Immediate Actions:**

```bash
# 1. Check Stripe status
curl https://status.stripe.com/api/v2/status.json

# 2. If Stripe is down - wait and monitor
watch -n 30 'curl -s https://status.stripe.com/api/v2/status.json'

# 3. If Stripe is up - check connectivity
nself billing stripe test-connection

# 4. Enable payment queue
nself env set BILLING_QUEUE_PAYMENTS=true

# 5. Notify customers
nself billing notify-customers \
  --template payment_processing_delayed \
  --all-active

# 6. Enable graceful degradation
nself env set BILLING_GRACEFUL_DEGRADATION=true

# 7. Queue failed payments for retry
nself billing queue failures --since 1h

# 8. Monitor Stripe status
# Subscribe to: https://status.stripe.com

# 9. Process queued payments when resolved
nself billing process-queue --batch-size 100

# 10. Verify all payments processed
nself billing verify-queue-empty
```

**Recovery:**
1. Wait for Stripe service restoration
2. Process queued payments
3. Verify all transactions
4. Disable queue mode
5. Monitor for issues

---

## Additional Resources

### Log Locations

```bash
# Billing service logs
/var/log/nself/billing.log

# Webhook logs
/var/log/nself/billing-webhooks.log

# Database logs
/var/log/postgresql/postgresql-*.log

# Nginx logs
/var/log/nginx/billing-access.log
/var/log/nginx/billing-error.log

# Docker logs
docker logs billing-service
docker logs postgres
docker logs redis
```

### Useful Commands Reference

```bash
# Status and Health
nself billing status
nself billing health
nself billing metrics

# Customer Management
nself billing customer list
nself billing customer show <id>
nself billing customer create
nself billing customer update <id>

# Subscription Management
nself billing subscription list
nself billing subscription show <id>
nself billing subscription create
nself billing subscription update <id>
nself billing subscription cancel <id>

# Invoice Management
nself billing invoice list
nself billing invoice show <id>
nself billing invoice create
nself billing invoice finalize <id>
nself billing invoice pay <id>

# Usage Tracking
nself billing usage show --user <id>
nself billing usage record
nself billing usage aggregate

# Quota Management
nself billing quota show --user <id>
nself billing quota set
nself billing quota reset

# Webhook Management
nself billing webhook status
nself billing webhook failures
nself billing webhook replay <event_id>

# Sync Operations
nself billing sync status
nself billing sync subscription <id>
nself billing sync customer <id>

# Testing
nself billing test-connection
nself billing test-payment
nself billing seed run
```

### Stripe Dashboard Links

- **Home**: https://dashboard.stripe.com
- **Customers**: https://dashboard.stripe.com/customers
- **Subscriptions**: https://dashboard.stripe.com/subscriptions
- **Invoices**: https://dashboard.stripe.com/invoices
- **Payments**: https://dashboard.stripe.com/payments
- **API Keys**: https://dashboard.stripe.com/apikeys
- **Webhooks**: https://dashboard.stripe.com/webhooks
- **Logs**: https://dashboard.stripe.com/logs
- **Status**: https://status.stripe.com

### Documentation

- **nself Billing Docs**: `/docs/billing/README.md`
- **Stripe API Docs**: https://stripe.com/docs/api
- **Stripe Webhooks**: https://stripe.com/docs/webhooks
- **Testing**: https://stripe.com/docs/testing

### Support Contacts

```bash
# nself Support
support@nself.org

# Stripe Support
https://support.stripe.com

# Emergency Contact
emergency@nself.org
+1-XXX-XXX-XXXX
```

---

## Document Information

**Version**: 1.0.0
**Last Updated**: January 30, 2026
**Maintainer**: nself Core Team
**Status**: Active

### Version History

- **1.0.0** (2026-01-30): Initial comprehensive troubleshooting guide

### Contributing

Found an issue not covered here? Submit updates:

```bash
# Create issue
gh issue create \
  --title "Billing Troubleshooting: [Issue]" \
  --label "documentation,billing"

# Or submit PR
git checkout -b docs/billing-troubleshooting-update
# Edit this file
git commit -m "docs: add troubleshooting for [issue]"
git push origin docs/billing-troubleshooting-update
gh pr create
```

---

**Remember**: When in doubt, check logs first, test in dev/staging, and never make emergency changes directly in production without a rollback plan.
