#!/usr/bin/env bash
# test-migration-workflow.sh - Database migration workflow integration test
#
# Tests: initial migrations → verify schema → create migration → run → rollback → fresh

set -euo pipefail

# Load test utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Test configuration
readonly TEST_NAME="migration-workflow"
TEST_PROJECT_DIR=""
CLEANUP_ON_EXIT=true
MIGRATION_DIR=""

# ============================================================================
# Cleanup Handler
# ============================================================================

cleanup() {
  if [[ "$CLEANUP_ON_EXIT" == "true" ]] && [[ -n "$TEST_PROJECT_DIR" ]]; then
    printf "\nCleaning up test environment...\n"
    cleanup_test_project "$TEST_PROJECT_DIR"
  fi
}

trap cleanup EXIT INT TERM

# ============================================================================
# Helper Functions
# ============================================================================

run_nself_command() {
  "$NSELF_ROOT/bin/nself" "$@"
}

db_exec() {
  local query="$1"
  docker-compose exec -T postgres psql -U postgres -d "$POSTGRES_DB" -t -c "$query" 2>/dev/null | xargs
}

table_exists() {
  local table_name="$1"
  local exists
  exists=$(db_exec "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name='$table_name');")
  [[ "$exists" == "t" ]]
}

column_exists() {
  local table_name="$1"
  local column_name="$2"
  local exists
  exists=$(db_exec "SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name='$table_name' AND column_name='$column_name');")
  [[ "$exists" == "t" ]]
}

# ============================================================================
# Test Functions
# ============================================================================

test_01_setup() {
  describe "Test 1: Setup test environment"

  # Create test environment
  TEST_PROJECT_DIR=$(setup_test_project)
  cd "$TEST_PROJECT_DIR"

  # Initialize project
  run_nself_command init --quiet

  # Build and start
  run_nself_command build
  run_nself_command start

  # Wait for services
  sleep 10
  wait_for_service_healthy "postgres" 60 || fail "postgres not healthy"

  # Source .env
  source .env

  # Set migration directory
  MIGRATION_DIR="$TEST_PROJECT_DIR/migrations"
  mkdir -p "$MIGRATION_DIR"

  pass "Test environment setup complete"
}

test_02_initial_migrations() {
  describe "Test 2: Run initial migrations"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Run initial migrations
  printf "Running initial migrations...\n"
  run_nself_command db migrate run

  # Verify migrations table exists
  if ! table_exists "schema_migrations"; then
    fail "schema_migrations table not created"
  fi

  printf "✓ schema_migrations table exists\n"

  pass "Initial migrations completed"
}

test_03_verify_initial_schema() {
  describe "Test 3: Verify initial schema"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Verify core tables exist
  local tables=("auth.users" "auth.refresh_tokens" "tenants" "tenant_members")

  for table in "${tables[@]}"; do
    if table_exists "$table"; then
      printf "✓ Table exists: %s\n" "$table"
    else
      fail "Required table missing: $table"
    fi
  done

  pass "Initial schema verified"
}

test_04_create_migration() {
  describe "Test 4: Create new migration"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create migration for a new table
  printf "Creating migration for 'products' table...\n"
  run_nself_command db migrate create \
    --name "create_products_table" \
    --path "$MIGRATION_DIR"

  # Find the created migration file
  local migration_file
  migration_file=$(ls -t "$MIGRATION_DIR"/*create_products_table*.sql 2>/dev/null | head -1)

  if [[ ! -f "$migration_file" ]]; then
    fail "Migration file not created"
  fi

  printf "Migration file created: %s\n" "$migration_file"

  # Add migration content
  cat > "$migration_file" <<'EOF'
-- Migration: Create products table

-- Up Migration
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price DECIMAL(10,2),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_products_tenant_id ON products(tenant_id);

-- Down Migration (for rollback)
-- DROP TABLE IF EXISTS products;
EOF

  printf "Migration content added\n"

  pass "Migration created successfully"
}

test_05_run_migration() {
  describe "Test 5: Run new migration"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Run migrations
  printf "Running migrations...\n"
  run_nself_command db migrate run --path "$MIGRATION_DIR"

  # Verify products table created
  if ! table_exists "products"; then
    fail "products table not created"
  fi

  printf "✓ products table created\n"

  # Verify columns
  local columns=("id" "name" "description" "price" "tenant_id" "created_at" "updated_at")

  for column in "${columns[@]}"; do
    if column_exists "products" "$column"; then
      printf "✓ Column exists: %s\n" "$column"
    else
      fail "Required column missing: $column"
    fi
  done

  # Verify index created
  local index_exists
  index_exists=$(db_exec "SELECT EXISTS (SELECT FROM pg_indexes WHERE tablename='products' AND indexname='idx_products_tenant_id');")

  if [[ "$index_exists" == "t" ]]; then
    printf "✓ Index idx_products_tenant_id created\n"
  else
    fail "Index not created"
  fi

  pass "Migration executed successfully"
}

test_06_migration_status() {
  describe "Test 6: Check migration status"

  cd "$TEST_PROJECT_DIR"

  # Get migration status
  printf "Checking migration status...\n"
  local status_output
  status_output=$(run_nself_command db migrate status --path "$MIGRATION_DIR" 2>&1)

  printf "Migration status:\n%s\n" "$status_output"

  # Verify output shows applied migrations
  echo "$status_output" | grep -q "create_products_table" || fail "Migration not in status"

  pass "Migration status verified"
}

test_07_insert_test_data() {
  describe "Test 7: Insert test data into new table"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create a test tenant first
  db_exec "INSERT INTO tenants (id, name, slug) VALUES ('11111111-1111-1111-1111-111111111111', 'Test Tenant', 'test-tenant');"

  # Insert product data
  printf "Inserting test products...\n"
  for i in {1..10}; do
    db_exec "INSERT INTO products (name, price, tenant_id) VALUES ('Product $i', $i.99, '11111111-1111-1111-1111-111111111111');"
  done

  # Verify data count
  local count
  count=$(db_exec "SELECT COUNT(*) FROM products;")

  if [[ $count -ne 10 ]]; then
    fail "Expected 10 products, got $count"
  fi

  printf "Created %s test products\n" "$count"

  pass "Test data inserted successfully"
}

test_08_create_alter_migration() {
  describe "Test 8: Create migration to alter table"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create migration to add column
  printf "Creating migration to add stock_quantity column...\n"
  run_nself_command db migrate create \
    --name "add_stock_quantity_to_products" \
    --path "$MIGRATION_DIR"

  # Find the migration file
  local migration_file
  migration_file=$(ls -t "$MIGRATION_DIR"/*add_stock_quantity*.sql 2>/dev/null | head -1)

  if [[ ! -f "$migration_file" ]]; then
    fail "Migration file not created"
  fi

  # Add migration content
  cat > "$migration_file" <<'EOF'
-- Migration: Add stock_quantity to products

-- Up Migration
ALTER TABLE products ADD COLUMN stock_quantity INTEGER DEFAULT 0;

-- Down Migration
-- ALTER TABLE products DROP COLUMN stock_quantity;
EOF

  # Run migration
  printf "Running migration...\n"
  run_nself_command db migrate run --path "$MIGRATION_DIR"

  # Verify column added
  if column_exists "products" "stock_quantity"; then
    printf "✓ stock_quantity column added\n"
  else
    fail "stock_quantity column not added"
  fi

  pass "Alter migration executed successfully"
}

test_09_rollback_migration() {
  describe "Test 9: Rollback last migration"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Rollback migration
  printf "Rolling back last migration...\n"
  run_nself_command db migrate rollback --path "$MIGRATION_DIR"

  # Verify column removed
  if column_exists "products" "stock_quantity"; then
    fail "stock_quantity column still exists after rollback"
  fi

  printf "✓ stock_quantity column removed\n"

  # Verify data still exists
  local count
  count=$(db_exec "SELECT COUNT(*) FROM products;")

  if [[ $count -ne 10 ]]; then
    fail "Product data lost during rollback: expected 10, got $count"
  fi

  printf "✓ Product data preserved (%s records)\n" "$count"

  pass "Migration rollback successful"
}

test_10_fresh_migrations() {
  describe "Test 10: Test fresh migrations (drop all and re-run)"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Run fresh migrations
  printf "Running fresh migrations...\n"
  run_nself_command db migrate fresh \
    --path "$MIGRATION_DIR" \
    --confirm

  # Verify tables recreated
  if table_exists "products"; then
    printf "✓ products table recreated\n"
  else
    fail "products table not recreated"
  fi

  # Verify data is gone (fresh = clean slate)
  local count
  count=$(db_exec "SELECT COUNT(*) FROM products;")

  if [[ $count -ne 0 ]]; then
    fail "Data should be empty after fresh migrations, got $count records"
  fi

  printf "✓ Database reset to clean state\n"

  pass "Fresh migrations successful"
}

test_11_migration_locking() {
  describe "Test 11: Test migration locking (prevent concurrent runs)"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Start migration in background
  printf "Starting migration in background...\n"
  run_nself_command db migrate run --path "$MIGRATION_DIR" &
  local bg_pid=$!

  # Wait a moment
  sleep 2

  # Try to run another migration (should be blocked)
  printf "Attempting concurrent migration...\n"
  if run_nself_command db migrate run --path "$MIGRATION_DIR" 2>&1 | grep -q "locked\|running"; then
    printf "✓ Concurrent migration blocked\n"
    kill $bg_pid 2>/dev/null || true
    wait $bg_pid 2>/dev/null || true
  else
    kill $bg_pid 2>/dev/null || true
    wait $bg_pid 2>/dev/null || true
    fail "Concurrent migration not blocked"
  fi

  pass "Migration locking verified"
}

# ============================================================================
# Main Test Runner
# ============================================================================

main() {
  start_suite "Database Migration Workflow Integration Test"

  # Skip gracefully when Docker or nself is not available (requires live stack in CI)
  if ! docker ps >/dev/null 2>&1; then
    printf "⚠ Docker not available - skipping workflow tests\n"
    exit 0
  fi
  if [[ -z "${NSELF_ROOT:-}" ]] || [[ ! -x "${NSELF_ROOT}/bin/nself" ]]; then
    printf "⚠ NSELF_ROOT not set or nself not found - skipping workflow tests\n"
    exit 0
  fi

  printf "\n=================================================================\n"
  printf "Database Migration Workflow Integration Test\n"
  printf "=================================================================\n\n"

  # Run all tests in sequence
  test_01_setup
  test_02_initial_migrations
  test_03_verify_initial_schema
  test_04_create_migration
  test_05_run_migration
  test_06_migration_status
  test_07_insert_test_data
  test_08_create_alter_migration
  test_09_rollback_migration
  test_10_fresh_migrations
  test_11_migration_locking

  # Print summary
  printf "\n=================================================================\n"
  printf "Test Summary\n"
  printf "=================================================================\n"
  printf "Total Tests: %d\n" "$TESTS_RUN"
  printf "Passed: %d\n" "$TESTS_PASSED"
  printf "Failed: %d\n" "$TESTS_FAILED"
  printf "Skipped: %d\n" "$TESTS_SKIPPED"
  printf "=================================================================\n\n"

  # Exit with proper code
  if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
  else
    exit 0
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
