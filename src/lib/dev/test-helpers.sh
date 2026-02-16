#!/usr/bin/env bash
# test-helpers.sh - Testing utilities and helpers
# Part of nself v0.7.0 - Sprint 19: Developer Experience Tools


# Initialize test environment
init_test_environment() {

set -euo pipefail

  local test_dir="${1:-.nself/test}"

  printf "Initializing test environment...\n"

  mkdir -p "$test_dir"/{fixtures,factories,integration,unit}

  # Create test configuration
  cat >"$test_dir/config.json" <<EOF
{
  "database": {
    "host": "localhost",
    "port": 5432,
    "database": "test_db",
    "user": "postgres",
    "password": "postgres"
  },
  "graphql": {
    "endpoint": "http://localhost:8080/v1/graphql",
    "adminSecret": "test-admin-secret"
  },
  "timeout": 30000,
  "verbose": true
}
EOF

  # Create test utilities
  cat >"$test_dir/utils.js" <<'EOF'
// Test utilities for JavaScript/TypeScript
const { NselfClient } = require('@nself/sdk');

class TestHelper {
  constructor(config) {
    this.config = config;
    this.client = new NselfClient({
      endpoint: config.graphql.endpoint,
      adminSecret: config.graphql.adminSecret,
    });
  }

  async clearDatabase() {
    // Clear test data from database
    await this.client.mutate(`
      mutation ClearTestData {
        delete_users(where: {email: {_like: "%test.com"}}) {
          affected_rows
        }
      }
    `);
  }

  async seedDatabase(fixtures) {
    // Seed database with test data
    for (const [table, data] of Object.entries(fixtures)) {
      await this.client.mutate(`
        mutation SeedData($objects: [${table}_insert_input!]!) {
          insert_${table}(objects: $objects) {
            affected_rows
          }
        }
      `, { objects: data });
    }
  }

  async waitForCondition(fn, timeout = 5000) {
    const start = Date.now();
    while (Date.now() - start < timeout) {
      if (await fn()) {
        return true;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    throw new Error('Condition not met within timeout');
  }
}

module.exports = { TestHelper };
EOF

  cat >"$test_dir/utils.py" <<'EOF'
"""Test utilities for Python."""
import time
from typing import Any, Callable, Dict
from nself import NselfClient


class TestHelper:
    """Helper class for testing."""

    def __init__(self, config: Dict[str, Any]):
        self.config = config
        self.client = NselfClient(
            endpoint=config['graphql']['endpoint'],
            admin_secret=config['graphql']['adminSecret']
        )

    def clear_database(self) -> None:
        """Clear test data from database."""
        self.client.mutate("""
            mutation ClearTestData {
                delete_users(where: {email: {_like: "%test.com"}}) {
                    affected_rows
                }
            }
        """)

    def seed_database(self, fixtures: Dict[str, list]) -> None:
        """Seed database with test data."""
        for table, data in fixtures.items():
            self.client.mutate(f"""
                mutation SeedData($objects: [{table}_insert_input!]!) {{
                    insert_{table}(objects: $objects) {{
                        affected_rows
                    }}
                }}
            """, {'objects': data})

    def wait_for_condition(
        self,
        fn: Callable[[], bool],
        timeout: int = 5000
    ) -> bool:
        """Wait for condition to be true."""
        start = time.time()
        while (time.time() - start) * 1000 < timeout:
            if fn():
                return True
            time.sleep(0.1)
        raise TimeoutError('Condition not met within timeout')
EOF

  # Create example test files
  cat >"$test_dir/integration/example.test.js" <<'EOF'
const { TestHelper } = require('../utils');
const config = require('../config.json');

describe('User Integration Tests', () => {
  let helper;

  beforeAll(async () => {
    helper = new TestHelper(config);
    await helper.clearDatabase();
  });

  afterAll(async () => {
    await helper.clearDatabase();
  });

  test('should create user', async () => {
    const result = await helper.client.mutate(`
      mutation CreateUser($email: String!, $displayName: String!) {
        insert_users_one(object: {email: $email, displayName: $displayName}) {
          id
          email
          displayName
        }
      }
    `, {
      email: 'test@test.com',
      displayName: 'Test User'
    });

    expect(result.insert_users_one).toBeDefined();
    expect(result.insert_users_one.email).toBe('test@test.com');
  });

  test('should query users', async () => {
    const result = await helper.client.query(`
      query GetUsers {
        users {
          id
          email
        }
      }
    `);

    expect(result.users).toBeDefined();
    expect(Array.isArray(result.users)).toBe(true);
  });
});
EOF

  cat >"$test_dir/integration/test_example.py" <<'EOF'
"""Example integration test."""
import unittest
import sys
import os
import json

sys.path.append(os.path.dirname(os.path.dirname(__file__)))
from utils import TestHelper


class TestUsers(unittest.TestCase):
    """User integration tests."""

    @classmethod
    def setUpClass(cls):
        """Set up test class."""
        with open(os.path.join(os.path.dirname(__file__), '../config.json')) as f:
            config = json.load(f)
        cls.helper = TestHelper(config)
        cls.helper.clear_database()

    @classmethod
    def tearDownClass(cls):
        """Tear down test class."""
        cls.helper.clear_database()

    def test_create_user(self):
        """Test creating a user."""
        result = self.helper.client.mutate("""
            mutation CreateUser($email: String!, $displayName: String!) {
                insert_users_one(object: {email: $email, displayName: $displayName}) {
                    id
                    email
                    displayName
                }
            }
        """, {
            'email': 'test@test.com',
            'displayName': 'Test User'
        })

        self.assertIsNotNone(result['insert_users_one'])
        self.assertEqual(result['insert_users_one']['email'], 'test@test.com')

    def test_query_users(self):
        """Test querying users."""
        result = self.helper.client.query("""
            query GetUsers {
                users {
                    id
                    email
                }
            }
        """)

        self.assertIsNotNone(result['users'])
        self.assertIsInstance(result['users'], list)


if __name__ == '__main__':
    unittest.main()
EOF

  printf "Test environment initialized at: %s\n" "$test_dir"
  printf "\nCreated:\n"
  printf "  - config.json - Test configuration\n"
  printf "  - utils.js - JavaScript test utilities\n"
  printf "  - utils.py - Python test utilities\n"
  printf "  - integration/example.test.js - Example JS test\n"
  printf "  - integration/test_example.py - Example Python test\n"
}

# Generate mock data factory
generate_mock_factory() {
  local entity="${1:-users}"
  local output_dir="${2:-.nself/test/factories}"

  printf "Generating mock factory for %s...\n" "$entity"

  mkdir -p "$output_dir"

  # JavaScript/TypeScript factory
  cat >"$output_dir/${entity}.factory.js" <<'EOF'
const { faker } = require('@faker-js/faker');

class UsersFactory {
  static create(overrides = {}) {
    return {
      id: faker.string.uuid(),
      email: faker.internet.email(),
      displayName: faker.person.fullName(),
      avatarUrl: faker.image.avatar(),
      createdAt: faker.date.past().toISOString(),
      updatedAt: new Date().toISOString(),
      ...overrides
    };
  }

  static createMany(count, overrides = {}) {
    return Array.from({ length: count }, () => this.create(overrides));
  }

  static createBatch(specs) {
    return specs.map(spec => this.create(spec));
  }
}

module.exports = { UsersFactory };
EOF

  # Python factory
  cat >"$output_dir/${entity}_factory.py" <<'EOF'
"""Mock factory for users."""
from datetime import datetime
from typing import Any, Dict, List
from faker import Faker

fake = Faker()


class UsersFactory:
    """Factory for generating mock user data."""

    @staticmethod
    def create(overrides: Dict[str, Any] = None) -> Dict[str, Any]:
        """Create a single mock user."""
        user = {
            'id': fake.uuid4(),
            'email': fake.email(),
            'displayName': fake.name(),
            'avatarUrl': fake.image_url(),
            'createdAt': fake.past_datetime().isoformat(),
            'updatedAt': datetime.now().isoformat()
        }

        if overrides:
            user.update(overrides)

        return user

    @classmethod
    def create_many(cls, count: int, overrides: Dict[str, Any] = None) -> List[Dict[str, Any]]:
        """Create multiple mock users."""
        return [cls.create(overrides) for _ in range(count)]

    @classmethod
    def create_batch(cls, specs: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Create users from specifications."""
        return [cls.create(spec) for spec in specs]
EOF

  printf "Mock factory generated at: %s\n" "$output_dir"
}

# Generate test fixtures
generate_fixtures() {
  local entity="${1:-users}"
  local count="${2:-10}"
  local output_file="${3:-.nself/test/fixtures/${entity}.json}"

  printf "Generating %s test fixtures for %s...\n" "$count" "$entity"

  mkdir -p "$(dirname "$output_file")"

  # Use faker to generate realistic data
  if command -v node >/dev/null 2>&1; then
    # Generate with Node.js if available
    node -e "
      const { faker } = require('@faker-js/faker');
      const fixtures = [];
      for (let i = 0; i < $count; i++) {
        fixtures.push({
          id: faker.string.uuid(),
          email: faker.internet.email(),
          displayName: faker.person.fullName(),
          avatarUrl: faker.image.avatar(),
          createdAt: faker.date.past().toISOString()
        });
      }
      console.log(JSON.stringify(fixtures, null, 2));
    " >"$output_file" 2>/dev/null || generate_simple_fixtures "$entity" "$count" "$output_file"
  else
    generate_simple_fixtures "$entity" "$count" "$output_file"
  fi

  printf "Fixtures generated at: %s\n" "$output_file"
}

# Generate simple fixtures without external dependencies
generate_simple_fixtures() {
  local entity="$1"
  local count="$2"
  local output_file="$3"

  local fixtures="["
  for i in $(seq 1 "$count"); do
    if [[ $i -gt 1 ]]; then
      fixtures+=","
    fi
    fixtures+="
  {
    \"id\": \"$(uuidgen | tr '[:upper:]' '[:lower:]')\",
    \"email\": \"user${i}@test.com\",
    \"displayName\": \"Test User ${i}\",
    \"createdAt\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
  }"
  done
  fixtures+="
]"

  printf '%s' "$fixtures" >"$output_file"
}

# Create database snapshot for testing
create_test_snapshot() {
  local snapshot_name="${1:-test-snapshot}"

  printf "Creating test database snapshot: %s\n" "$snapshot_name"

  # Source environment
  if [[ -f .env ]]; then
    set -a
    source .env
    set +a
  fi

  local db_name="${POSTGRES_DB:-nself}"
  local db_user="${POSTGRES_USER:-postgres}"
  local snapshot_file=".nself/test/snapshots/${snapshot_name}.sql"

  mkdir -p "$(dirname "$snapshot_file")"

  # Create snapshot using pg_dump
  if command -v docker >/dev/null 2>&1; then
    docker exec "${PROJECT_NAME:-nself}_postgres" \
      pg_dump -U "$db_user" "$db_name" >"$snapshot_file"
    printf "Snapshot created: %s\n" "$snapshot_file"
  else
    printf "Docker not found. Cannot create snapshot.\n" >&2
    return 1
  fi
}

# Restore database from snapshot
restore_test_snapshot() {
  local snapshot_name="${1:-test-snapshot}"
  local snapshot_file=".nself/test/snapshots/${snapshot_name}.sql"

  if [[ ! -f "$snapshot_file" ]]; then
    printf "Snapshot not found: %s\n" "$snapshot_file" >&2
    return 1
  fi

  printf "Restoring test database from: %s\n" "$snapshot_name"

  # Source environment
  if [[ -f .env ]]; then
    set -a
    source .env
    set +a
  fi

  local db_name="${POSTGRES_DB:-nself}"
  local db_user="${POSTGRES_USER:-postgres}"

  if command -v docker >/dev/null 2>&1; then
    docker exec -i "${PROJECT_NAME:-nself}_postgres" \
      psql -U "$db_user" "$db_name" <"$snapshot_file"
    printf "Snapshot restored successfully\n"
  else
    printf "Docker not found. Cannot restore snapshot.\n" >&2
    return 1
  fi
}

# Run integration tests
run_integration_tests() {
  local test_dir="${1:-.nself/test/integration}"

  if [[ ! -d "$test_dir" ]]; then
    printf "Test directory not found: %s\n" "$test_dir" >&2
    return 1
  fi

  printf "Running integration tests...\n"

  # Run JavaScript tests if found
  if [[ -f "$test_dir/package.json" ]] || [[ -n "$(find "$test_dir" -name "*.test.js" 2>/dev/null)" ]]; then
    if command -v npm >/dev/null 2>&1; then
      printf "\nRunning JavaScript tests...\n"
      (cd "$test_dir" && npm test)
    fi
  fi

  # Run Python tests if found
  if [[ -n "$(find "$test_dir" -name "test_*.py" 2>/dev/null)" ]]; then
    if command -v python3 >/dev/null 2>&1; then
      printf "\nRunning Python tests...\n"
      python3 -m pytest "$test_dir" -v
    fi
  fi
}

export -f init_test_environment generate_mock_factory generate_fixtures
export -f generate_simple_fixtures create_test_snapshot restore_test_snapshot
export -f run_integration_tests
