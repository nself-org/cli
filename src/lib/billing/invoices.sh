#!/usr/bin/env bash

#
# nself billing/invoices.sh - Invoice Generation and Management
# Part of nself v0.9.6 - Complete Billing System
#
# Comprehensive invoice generation, management, and PDF export functionality.
#
# Note: Library files should not use set -euo pipefail as it can cause
# issues when sourced by other scripts. Use explicit error checking instead.

# Prevent multiple sourcing
[[ -n "${NSELF_BILLING_INVOICES_LOADED:-}" ]] && return 0
NSELF_BILLING_INVOICES_LOADED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_BILLING_INVOICES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_BILLING_INVOICES_DIR}/core.sh"

# Invoice configuration
INVOICE_TEMPLATE_DIR="${NSELF_ROOT}/src/templates/invoices"
INVOICE_OUTPUT_DIR="${BILLING_EXPORT_DIR}/invoices"
INVOICE_NUMBER_PREFIX="${INVOICE_NUMBER_PREFIX:-INV}"
INVOICE_DUE_DAYS="${INVOICE_DUE_DAYS:-7}"

# Initialize invoice system
invoices_init() {
  mkdir -p "${INVOICE_OUTPUT_DIR}"
  mkdir -p "${INVOICE_TEMPLATE_DIR}"

  # Create default invoice template if not exists
  if [[ ! -f "${INVOICE_TEMPLATE_DIR}/default.html" ]]; then
    invoices_create_default_template
  fi

  return 0
}

# Create default HTML invoice template
invoices_create_default_template() {
  local template_file="${INVOICE_TEMPLATE_DIR}/default.html"

  cat > "${template_file}" <<'EOF'
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Invoice {{INVOICE_NUMBER}}</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 40px; }
    .header { margin-bottom: 30px; }
    .company { font-size: 24px; font-weight: bold; }
    .invoice-details { margin: 20px 0; }
    .line-items { width: 100%; border-collapse: collapse; margin: 20px 0; }
    .line-items th, .line-items td { border: 1px solid #ddd; padding: 8px; text-align: left; }
    .line-items th { background-color: #f4f4f4; }
    .totals { text-align: right; margin-top: 20px; }
    .total-row { font-weight: bold; font-size: 18px; }
  </style>
</head>
<body>
  <div class="header">
    <div class="company">{{COMPANY_NAME}}</div>
    <div>{{COMPANY_ADDRESS}}</div>
    <div>{{COMPANY_EMAIL}}</div>
  </div>

  <div class="invoice-details">
    <h2>Invoice {{INVOICE_NUMBER}}</h2>
    <p><strong>Date:</strong> {{INVOICE_DATE}}</p>
    <p><strong>Due Date:</strong> {{DUE_DATE}}</p>
    <p><strong>Customer:</strong> {{CUSTOMER_NAME}}</p>
    <p><strong>Email:</strong> {{CUSTOMER_EMAIL}}</p>
  </div>

  <table class="line-items">
    <thead>
      <tr>
        <th>Description</th>
        <th>Quantity</th>
        <th>Unit Price</th>
        <th>Amount</th>
      </tr>
    </thead>
    <tbody>
      {{LINE_ITEMS}}
    </tbody>
  </table>

  <div class="totals">
    <p>Subtotal: ${{SUBTOTAL}}</p>
    <p>Tax ({{TAX_RATE}}%): ${{TAX_AMOUNT}}</p>
    <p class="total-row">Total: ${{TOTAL_AMOUNT}}</p>
  </div>

  <div style="margin-top: 40px; font-size: 12px; color: #666;">
    <p>Payment Terms: Net {{PAYMENT_TERMS}} days</p>
    <p>Thank you for your business!</p>
  </div>
</body>
</html>
EOF

  chmod 644 "${template_file}"
}

# Generate invoice for customer
# Args: customer_id, period_start, period_end, [template]
invoice_generate() {
  local customer_id="$1"
  local period_start="$2"
  local period_end="$3"
  local template="${4:-default}"

  # Validate inputs
  if [[ -z "$customer_id" ]] || [[ -z "$period_start" ]] || [[ -z "$period_end" ]]; then
    error "Customer ID, period start, and period end are required"
    return 1
  fi

  # Generate invoice ID
  local invoice_id
  invoice_id="${INVOICE_NUMBER_PREFIX}_$(date +%Y%m%d)_$(openssl rand -hex 4 2>/dev/null || printf '%08x' $RANDOM)"

  # Get customer details
  local customer_data
  customer_data=$(billing_get_customer "$customer_id")

  if [[ -z "$customer_data" ]]; then
    error "Customer not found: ${customer_id}"
    return 1
  fi

  # Extract customer info (pipe-delimited)
  local customer_name customer_email customer_company
  IFS='|' read -r _ _ customer_email customer_name customer_company _ _ _ <<< "$customer_data"

  # Trim whitespace
  customer_email=$(printf "%s" "$customer_email" | tr -d ' ')
  customer_name=$(printf "%s" "$customer_name" | tr -d ' ')
  customer_company=$(printf "%s" "$customer_company" | tr -d ' ')

  # Get usage for billing period
  local usage_data
  usage_data=$(invoice_get_usage_for_period "$customer_id" "$period_start" "$period_end")

  # Calculate invoice totals
  local subtotal tax_amount total_amount
  read -r subtotal tax_amount total_amount <<< "$(invoice_calculate_totals "$usage_data")"

  # Insert invoice into database
  local invoice_date
  invoice_date=$(date -u +"%Y-%m-%d %H:%M:%S")

  local due_date
  if date -v+${INVOICE_DUE_DAYS}d >/dev/null 2>&1; then
    # BSD date (macOS)
    due_date=$(date -u -v+${INVOICE_DUE_DAYS}d +"%Y-%m-%d %H:%M:%S")
  else
    # GNU date (Linux)
    due_date=$(date -u -d "+${INVOICE_DUE_DAYS} days" +"%Y-%m-%d %H:%M:%S")
  fi

  billing_db_query "
    INSERT INTO billing_invoices (
      invoice_id,
      customer_id,
      period_start,
      period_end,
      subtotal,
      tax_amount,
      total_amount,
      status,
      invoice_date,
      due_date,
      created_at
    ) VALUES (
      :'invoice_id',
      :'customer_id',
      :'period_start',
      :'period_end',
      :'subtotal',
      :'tax_amount',
      :'total_amount',
      'draft',
      :'invoice_date',
      :'due_date',
      NOW()
    );
  " "tuples" \
    "invoice_id" "$invoice_id" \
    "customer_id" "$customer_id" \
    "period_start" "$period_start" \
    "period_end" "$period_end" \
    "subtotal" "$subtotal" \
    "tax_amount" "$tax_amount" \
    "total_amount" "$total_amount" \
    "invoice_date" "$invoice_date" \
    "due_date" "$due_date" >/dev/null

  # Add line items
  invoice_add_line_items "$invoice_id" "$usage_data"

  # Mark invoice as finalized
  invoice_finalize "$invoice_id"

  billing_log "INVOICE" "generated" "$invoice_id" "customer=${customer_id},total=${total_amount}"

  success "Invoice generated: ${invoice_id}"
  printf "%s" "$invoice_id"
}

# Get usage data for billing period
invoice_get_usage_for_period() {
  local customer_id="$1"
  local period_start="$2"
  local period_end="$3"

  billing_db_query "
    SELECT
      service_name,
      SUM(quantity) as total_quantity,
      unit_cost,
      SUM(quantity * unit_cost) as total_cost
    FROM billing_usage_records
    WHERE customer_id = :'customer_id'
    AND recorded_at >= :'period_start'
    AND recorded_at <= :'period_end'
    GROUP BY service_name, unit_cost;
  " "tuples" \
    "customer_id" "$customer_id" \
    "period_start" "$period_start" \
    "period_end" "$period_end"
}

# Calculate invoice totals
invoice_calculate_totals() {
  local usage_data="$1"
  local subtotal=0
  local tax_rate="${BILLING_TAX_RATE:-0}"

  # Sum all line item costs
  while IFS='|' read -r service quantity unit_cost total_cost; do
    # Trim whitespace
    total_cost=$(printf "%s" "$total_cost" | tr -d ' ')

    if [[ -n "$total_cost" ]]; then
      subtotal=$(awk "BEGIN {printf \"%.2f\", $subtotal + $total_cost}")
    fi
  done <<< "$usage_data"

  # Calculate tax
  local tax_amount
  tax_amount=$(awk "BEGIN {printf \"%.2f\", $subtotal * ($tax_rate / 100)}")

  # Calculate total
  local total_amount
  total_amount=$(awk "BEGIN {printf \"%.2f\", $subtotal + $tax_amount}")

  printf "%s %s %s" "$subtotal" "$tax_amount" "$total_amount"
}

# Add line items to invoice
invoice_add_line_items() {
  local invoice_id="$1"
  local usage_data="$2"

  local line_number=0

  while IFS='|' read -r service quantity unit_cost total_cost; do
    # Trim whitespace
    service=$(printf "%s" "$service" | tr -d ' ')
    quantity=$(printf "%s" "$quantity" | tr -d ' ')
    unit_cost=$(printf "%s" "$unit_cost" | tr -d ' ')
    total_cost=$(printf "%s" "$total_cost" | tr -d ' ')

    if [[ -z "$service" ]]; then
      continue
    fi

    ((line_number++))

    # Create description based on service
    local description
    case "$service" in
      api) description="API Requests" ;;
      storage) description="Storage (GB-hours)" ;;
      bandwidth) description="Bandwidth (GB)" ;;
      compute) description="Compute (CPU-hours)" ;;
      database) description="Database Queries" ;;
      functions) description="Function Invocations" ;;
      *) description="$service" ;;
    esac

    billing_db_query "
      INSERT INTO billing_invoice_items (
        invoice_id,
        line_number,
        description,
        service_name,
        quantity,
        unit_price,
        amount
      ) VALUES (
        :'invoice_id',
        :'line_number',
        :'description',
        :'service_name',
        :'quantity',
        :'unit_price',
        :'amount'
      );
    " "tuples" \
      "invoice_id" "$invoice_id" \
      "line_number" "$line_number" \
      "description" "$description" \
      "service_name" "$service" \
      "quantity" "$quantity" \
      "unit_price" "$unit_cost" \
      "amount" "$total_cost" >/dev/null
  done <<< "$usage_data"
}

# Finalize invoice (change status from draft)
invoice_finalize() {
  local invoice_id="$1"

  billing_db_query "
    UPDATE billing_invoices
    SET status = 'open',
        finalized_at = NOW()
    WHERE invoice_id = :'invoice_id';
  " "tuples" "invoice_id" "$invoice_id" >/dev/null

  billing_log "INVOICE" "finalized" "$invoice_id" "status=open"
}

# Get invoice details
invoice_get() {
  local invoice_id="$1"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  billing_db_query "
    SELECT
      invoice_id,
      customer_id,
      period_start,
      period_end,
      subtotal,
      tax_amount,
      total_amount,
      status,
      invoice_date,
      due_date,
      paid_at,
      created_at
    FROM billing_invoices
    WHERE invoice_id = :'invoice_id';
  " "tuples" "invoice_id" "$invoice_id"
}

# List invoices for customer
invoice_list() {
  local customer_id="${1:-}"
  local status="${2:-}"
  local limit="${3:-100}"
  local offset="${4:-0}"

  local where_clause="1=1"
  local params=()

  if [[ -n "$customer_id" ]]; then
    where_clause+=" AND customer_id = :'customer_id'"
    params+=("customer_id" "$customer_id")
  fi

  if [[ -n "$status" ]]; then
    where_clause+=" AND status = :'status'"
    params+=("status" "$status")
  fi

  billing_db_query "
    SELECT
      invoice_id,
      customer_id,
      total_amount,
      status,
      invoice_date,
      due_date
    FROM billing_invoices
    WHERE ${where_clause}
    ORDER BY invoice_date DESC
    LIMIT :'limit' OFFSET :'offset';
  " "tuples" "${params[@]}" "limit" "$limit" "offset" "$offset"
}

# Mark invoice as paid
invoice_mark_paid() {
  local invoice_id="$1"
  local payment_method="${2:-}"
  local transaction_id="${3:-}"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  local paid_at
  paid_at=$(date -u +"%Y-%m-%d %H:%M:%S")

  billing_db_query "
    UPDATE billing_invoices
    SET status = 'paid',
        paid_at = :'paid_at',
        payment_method = :'payment_method',
        transaction_id = :'transaction_id'
    WHERE invoice_id = :'invoice_id';
  " "tuples" \
    "invoice_id" "$invoice_id" \
    "paid_at" "$paid_at" \
    "payment_method" "$payment_method" \
    "transaction_id" "$transaction_id" >/dev/null

  billing_log "INVOICE" "paid" "$invoice_id" "method=${payment_method},txn=${transaction_id}"

  success "Invoice marked as paid: ${invoice_id}"
}

# Void invoice
invoice_void() {
  local invoice_id="$1"
  local reason="${2:-}"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  billing_db_query "
    UPDATE billing_invoices
    SET status = 'void',
        void_reason = :'reason',
        voided_at = NOW()
    WHERE invoice_id = :'invoice_id';
  " "tuples" \
    "invoice_id" "$invoice_id" \
    "reason" "$reason" >/dev/null

  billing_log "INVOICE" "voided" "$invoice_id" "reason=${reason}"

  success "Invoice voided: ${invoice_id}"
}

# Export invoice to HTML
invoice_export_html() {
  local invoice_id="$1"
  local output_file="${2:-}"
  local template="${3:-default}"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  # Default output filename
  if [[ -z "$output_file" ]]; then
    output_file="${INVOICE_OUTPUT_DIR}/${invoice_id}.html"
  fi

  # Get invoice data
  local invoice_data
  invoice_data=$(invoice_get "$invoice_id")

  if [[ -z "$invoice_data" ]]; then
    error "Invoice not found: ${invoice_id}"
    return 1
  fi

  # Parse invoice data
  local cust_id period_start period_end subtotal tax_amount total_amount status invoice_date due_date
  IFS='|' read -r _ cust_id period_start period_end subtotal tax_amount total_amount status invoice_date due_date _ _ <<< "$invoice_data"

  # Get customer data
  local customer_data
  customer_data=$(billing_get_customer "$(printf '%s' "$cust_id" | tr -d ' ')")

  local customer_name customer_email customer_company
  IFS='|' read -r _ _ customer_email customer_name customer_company _ _ _ <<< "$customer_data"

  # Get line items
  local line_items
  line_items=$(invoice_get_line_items "$invoice_id")

  # Load template
  local template_file="${INVOICE_TEMPLATE_DIR}/${template}.html"
  if [[ ! -f "$template_file" ]]; then
    warn "Template not found: ${template}, using default"
    template_file="${INVOICE_TEMPLATE_DIR}/default.html"
  fi

  # Read template
  local html_content
  html_content=$(cat "$template_file")

  # Replace placeholders
  html_content="${html_content//\{\{INVOICE_NUMBER\}\}/$invoice_id}"
  html_content="${html_content//\{\{COMPANY_NAME\}\}/${BILLING_COMPANY_NAME:-nself}}"
  html_content="${html_content//\{\{COMPANY_ADDRESS\}\}/${BILLING_COMPANY_ADDRESS:-}}"
  html_content="${html_content//\{\{COMPANY_EMAIL\}\}/${BILLING_COMPANY_EMAIL:-}}"
  html_content="${html_content//\{\{CUSTOMER_NAME\}\}/$(printf '%s' "$customer_name" | tr -d ' ')}"
  html_content="${html_content//\{\{CUSTOMER_EMAIL\}\}/$(printf '%s' "$customer_email" | tr -d ' ')}"
  html_content="${html_content//\{\{INVOICE_DATE\}\}/$(printf '%s' "$invoice_date" | tr -d ' ')}"
  html_content="${html_content//\{\{DUE_DATE\}\}/$(printf '%s' "$due_date" | tr -d ' ')}"
  html_content="${html_content//\{\{SUBTOTAL\}\}/$(printf '%s' "$subtotal" | tr -d ' ')}"
  html_content="${html_content//\{\{TAX_RATE\}\}/${BILLING_TAX_RATE:-0}}"
  html_content="${html_content//\{\{TAX_AMOUNT\}\}/$(printf '%s' "$tax_amount" | tr -d ' ')}"
  html_content="${html_content//\{\{TOTAL_AMOUNT\}\}/$(printf '%s' "$total_amount" | tr -d ' ')}"
  html_content="${html_content//\{\{PAYMENT_TERMS\}\}/${INVOICE_DUE_DAYS}}"

  # Generate line items HTML
  local line_items_html=""
  while IFS='|' read -r description quantity unit_price amount; do
    description=$(printf '%s' "$description" | tr -d ' ')
    quantity=$(printf '%s' "$quantity" | tr -d ' ')
    unit_price=$(printf '%s' "$unit_price" | tr -d ' ')
    amount=$(printf '%s' "$amount" | tr -d ' ')

    line_items_html+="<tr><td>$description</td><td>$quantity</td><td>\$$unit_price</td><td>\$$amount</td></tr>"
  done <<< "$line_items"

  html_content="${html_content//\{\{LINE_ITEMS\}\}/$line_items_html}"

  # Write to output file
  mkdir -p "$(dirname "$output_file")"
  printf "%s" "$html_content" > "$output_file"

  success "Invoice exported to HTML: ${output_file}"
  printf "%s" "$output_file"
}

# Get invoice line items
invoice_get_line_items() {
  local invoice_id="$1"

  billing_db_query "
    SELECT
      description,
      quantity,
      unit_price,
      amount
    FROM billing_invoice_items
    WHERE invoice_id = :'invoice_id'
    ORDER BY line_number;
  " "tuples" "invoice_id" "$invoice_id"
}

# Export invoice to PDF (requires wkhtmltopdf)
invoice_export_pdf() {
  local invoice_id="$1"
  local output_file="${2:-}"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  # Default output filename
  if [[ -z "$output_file" ]]; then
    output_file="${INVOICE_OUTPUT_DIR}/${invoice_id}.pdf"
  fi

  # First export to HTML
  local html_file="${INVOICE_OUTPUT_DIR}/${invoice_id}.tmp.html"
  invoice_export_html "$invoice_id" "$html_file" >/dev/null

  # Convert HTML to PDF if wkhtmltopdf is available
  if command -v wkhtmltopdf >/dev/null 2>&1; then
    wkhtmltopdf "$html_file" "$output_file" 2>/dev/null
    rm -f "$html_file"
    success "Invoice exported to PDF: ${output_file}"
    printf "%s" "$output_file"
  else
    warn "wkhtmltopdf not installed. HTML version saved instead."
    mv "$html_file" "${output_file%.pdf}.html"
    printf "%s" "${output_file%.pdf}.html"
  fi
}

# Send invoice by email (placeholder - requires email service)
invoice_send_email() {
  local invoice_id="$1"
  local recipient="${2:-}"

  if [[ -z "$invoice_id" ]]; then
    error "Invoice ID required"
    return 1
  fi

  # Get customer email if not provided
  if [[ -z "$recipient" ]]; then
    local invoice_data
    invoice_data=$(invoice_get "$invoice_id")
    local customer_id
    customer_id=$(printf '%s' "$invoice_data" | cut -d'|' -f2 | tr -d ' ')

    local customer_data
    customer_data=$(billing_get_customer "$customer_id")
    recipient=$(printf '%s' "$customer_data" | cut -d'|' -f3 | tr -d ' ')
  fi

  # Export invoice to PDF
  local pdf_file
  pdf_file=$(invoice_export_pdf "$invoice_id")

  # Log email send attempt
  billing_log "INVOICE" "email" "$invoice_id" "recipient=${recipient}"

  info "Invoice ready to send to: ${recipient}"
  info "PDF: ${pdf_file}"

  # TODO (v1.0): Integrate with email service (SMTP, SendGrid, etc.)
  # See: .ai/roadmap/v1.0/deferred-features.md (BILLING-001)
  warn "Email sending not yet implemented. PDF generated at: ${pdf_file}"

  printf "%s" "$pdf_file"
}

# Get invoice summary statistics
invoice_get_summary() {
  local customer_id="${1:-}"
  local start_date="${2:-}"
  local end_date="${3:-}"

  local where_clause="1=1"
  local params=()

  if [[ -n "$customer_id" ]]; then
    where_clause+=" AND customer_id = :'customer_id'"
    params+=("customer_id" "$customer_id")
  fi

  if [[ -n "$start_date" ]]; then
    where_clause+=" AND invoice_date >= :'start_date'"
    params+=("start_date" "$start_date")
  fi

  if [[ -n "$end_date" ]]; then
    where_clause+=" AND invoice_date <= :'end_date'"
    params+=("end_date" "$end_date")
  fi

  billing_db_query "
    SELECT
      COUNT(*) as total_invoices,
      COUNT(CASE WHEN status = 'paid' THEN 1 END) as paid_invoices,
      COUNT(CASE WHEN status = 'open' THEN 1 END) as open_invoices,
      COUNT(CASE WHEN status = 'void' THEN 1 END) as void_invoices,
      COALESCE(SUM(total_amount), 0) as total_amount,
      COALESCE(SUM(CASE WHEN status = 'paid' THEN total_amount ELSE 0 END), 0) as total_paid,
      COALESCE(SUM(CASE WHEN status = 'open' THEN total_amount ELSE 0 END), 0) as total_outstanding
    FROM billing_invoices
    WHERE ${where_clause};
  " "tuples" "${params[@]}"
}

# Export functions
export -f invoices_init
export -f invoices_create_default_template
export -f invoice_generate
export -f invoice_get_usage_for_period
export -f invoice_calculate_totals
export -f invoice_add_line_items
export -f invoice_finalize
export -f invoice_get
export -f invoice_list
export -f invoice_mark_paid
export -f invoice_void
export -f invoice_export_html
export -f invoice_export_pdf
export -f invoice_get_line_items
export -f invoice_send_email
export -f invoice_get_summary
