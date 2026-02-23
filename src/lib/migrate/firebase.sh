#!/usr/bin/env bash

# firebase.sh - Migrate from Firebase to nself
# Mission: Help users escape vendor lock-in
# v0.4.8

set -euo pipefail

# Import utilities
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$SCRIPT_DIR/../utils/display.sh" 2>/dev/null || true

# Colors
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"

# Firebase service account validation
validate_firebase_credentials() {
  local service_account_path="$1"

  if [[ ! -f "$service_account_path" ]]; then
    log_error "Firebase service account file not found: $service_account_path"
    return 1
  fi

  # Validate JSON structure
  if ! command -v jq >/dev/null 2>&1; then
    log_warning "jq not found - skipping JSON validation"
    log_info "Install jq for better validation: brew install jq (macOS) or apt install jq (Linux)"
  else
    if ! jq empty "$service_account_path" 2>/dev/null; then
      log_error "Invalid JSON in service account file"
      return 1
    fi

    # Check required fields
    local project_id=$(jq -r '.project_id // empty' "$service_account_path")
    local private_key=$(jq -r '.private_key // empty' "$service_account_path")

    if [[ -z "$project_id" ]] || [[ -z "$private_key" ]]; then
      log_error "Service account missing required fields (project_id, private_key)"
      return 1
    fi

    log_success "Firebase credentials validated"
  fi

  return 0
}

# Install Firebase Admin SDK (Node.js)
setup_firebase_tools() {
  log_info "Setting up Firebase migration tools..."

  # Create temporary migration workspace
  local workspace
  workspace=$(mktemp -d /tmp/nself-firebase-migration.XXXXXX)
  trap "rm -rf '$workspace'" EXIT

  cd "$workspace"

  # Create package.json
  cat >package.json <<'EOF'
{
  "name": "nself-firebase-migration",
  "version": "1.0.0",
  "private": true,
  "dependencies": {
    "firebase-admin": "^12.0.0",
    "pg": "^8.11.0"
  }
}
EOF

  # Install dependencies
  if command -v npm >/dev/null 2>&1; then
    npm install --silent 2>&1 | grep -v "npm WARN" || true
    log_success "Firebase tools installed"
    echo "$workspace"
  else
    log_error "npm not found - required for Firebase migration"
    log_info "Install Node.js: https://nodejs.org/"
    rm -rf "$workspace"
    return 1
  fi
}

# Export Firestore collections to JSON
export_firestore_data() {
  local service_account="$1"
  local output_dir="$2"
  local collections="$3" # Comma-separated list or "all"

  log_info "Exporting Firestore data..."

  mkdir -p "$output_dir/firestore"

  # Create export script
  local export_script="$output_dir/export-firestore.js"

  cat >"$export_script" <<'EOFJS'
const admin = require('firebase-admin');
const fs = require('fs');
const path = require('path');

const serviceAccount = require(process.argv[2]);
const outputDir = process.argv[3];
const collectionsArg = process.argv[4] || 'all';

admin.initializeApp({
  credential: admin.credential.cert(serviceAccount)
});

const db = admin.firestore();

async function exportCollection(collectionName) {
  console.log(`Exporting collection: ${collectionName}`);

  const snapshot = await db.collection(collectionName).get();
  const data = [];

  snapshot.forEach(doc => {
    data.push({
      id: doc.id,
      ...doc.data()
    });
  });

  const outputPath = path.join(outputDir, 'firestore', `${collectionName}.json`);
  fs.writeFileSync(outputPath, JSON.stringify(data, null, 2));

  console.log(`✓ Exported ${data.length} documents from ${collectionName}`);
  return data.length;
}

async function listCollections() {
  const collections = await db.listCollections();
  return collections.map(col => col.id);
}

async function main() {
  try {
    let collections = [];

    if (collectionsArg === 'all') {
      collections = await listCollections();
      console.log(`Found ${collections.length} collections`);
    } else {
      collections = collectionsArg.split(',').map(c => c.trim());
    }

    let totalDocs = 0;
    for (const collection of collections) {
      const count = await exportCollection(collection);
      totalDocs += count;
    }

    console.log(`\nTotal: ${totalDocs} documents exported from ${collections.length} collections`);

    // Save collection metadata
    const metadata = {
      timestamp: new Date().toISOString(),
      collections: collections,
      totalDocuments: totalDocs
    };

    fs.writeFileSync(
      path.join(outputDir, 'firestore', '_metadata.json'),
      JSON.stringify(metadata, null, 2)
    );

  } catch (error) {
    console.error('Export failed:', error.message);
    process.exit(1);
  }
}

main();
EOFJS

  # Run export
  if node "$export_script" "$service_account" "$output_dir" "$collections" 2>&1; then
    log_success "Firestore data exported to $output_dir/firestore/"
    return 0
  else
    log_error "Firestore export failed"
    return 1
  fi
}

# Export Firebase Auth users
export_firebase_auth() {
  local service_account="$1"
  local output_dir="$2"

  log_info "Exporting Firebase Auth users..."

  mkdir -p "$output_dir/auth"

  # Create export script
  local export_script="$output_dir/export-auth.js"

  cat >"$export_script" <<'EOFJS'
const admin = require('firebase-admin');
const fs = require('fs');
const path = require('path');

const serviceAccount = require(process.argv[2]);
const outputDir = process.argv[3];

admin.initializeApp({
  credential: admin.credential.cert(serviceAccount)
});

async function exportUsers() {
  console.log('Exporting users...');

  const users = [];
  let nextPageToken;

  do {
    const listUsersResult = await admin.auth().listUsers(1000, nextPageToken);

    listUsersResult.users.forEach(userRecord => {
      users.push({
        uid: userRecord.uid,
        email: userRecord.email,
        emailVerified: userRecord.emailVerified,
        displayName: userRecord.displayName,
        photoURL: userRecord.photoURL,
        disabled: userRecord.disabled,
        metadata: {
          creationTime: userRecord.metadata.creationTime,
          lastSignInTime: userRecord.metadata.lastSignInTime
        },
        providerData: userRecord.providerData,
        customClaims: userRecord.customClaims || {}
      });
    });

    nextPageToken = listUsersResult.pageToken;
  } while (nextPageToken);

  const outputPath = path.join(outputDir, 'auth', 'users.json');
  fs.writeFileSync(outputPath, JSON.stringify(users, null, 2));

  console.log(`✓ Exported ${users.length} users`);

  // Save metadata
  const metadata = {
    timestamp: new Date().toISOString(),
    totalUsers: users.length,
    exportedFields: ['uid', 'email', 'displayName', 'metadata', 'customClaims']
  };

  fs.writeFileSync(
    path.join(outputDir, 'auth', '_metadata.json'),
    JSON.stringify(metadata, null, 2)
  );

  return users.length;
}

async function main() {
  try {
    const count = await exportUsers();
    console.log(`\nTotal: ${count} users exported`);
  } catch (error) {
    console.error('Export failed:', error.message);
    process.exit(1);
  }
}

main();
EOFJS

  # Run export
  if node "$export_script" "$service_account" "$output_dir" 2>&1; then
    log_success "Auth users exported to $output_dir/auth/"
    return 0
  else
    log_error "Auth export failed"
    return 1
  fi
}

# Export Firebase Storage to MinIO
export_firebase_storage() {
  local service_account="$1"
  local output_dir="$2"
  local bucket_name="$3"

  log_info "Exporting Firebase Storage..."

  mkdir -p "$output_dir/storage"

  # Create export script
  local export_script="$output_dir/export-storage.js"

  cat >"$export_script" <<'EOFJS'
const admin = require('firebase-admin');
const fs = require('fs');
const path = require('path');

const serviceAccount = require(process.argv[2]);
const outputDir = process.argv[3];
const bucketName = process.argv[4] || '';

admin.initializeApp({
  credential: admin.credential.cert(serviceAccount),
  storageBucket: bucketName
});

async function downloadFile(file, destPath) {
  const options = {
    destination: destPath,
  };

  await file.download(options);
}

async function exportStorage() {
  const bucket = admin.storage().bucket();
  console.log(`Exporting from bucket: ${bucket.name}`);

  const [files] = await bucket.getFiles();

  console.log(`Found ${files.length} files`);

  const fileList = [];
  let downloaded = 0;

  for (const file of files) {
    const filePath = file.name;
    const destPath = path.join(outputDir, 'storage', filePath);
    const destDir = path.dirname(destPath);

    // Create directory structure
    if (!fs.existsSync(destDir)) {
      fs.mkdirSync(destDir, { recursive: true });
    }

    try {
      await downloadFile(file, destPath);
      downloaded++;

      fileList.push({
        name: filePath,
        size: file.metadata.size,
        contentType: file.metadata.contentType,
        created: file.metadata.timeCreated,
        updated: file.metadata.updated
      });

      if (downloaded % 10 === 0) {
        console.log(`Downloaded ${downloaded}/${files.length} files...`);
      }
    } catch (error) {
      console.error(`Failed to download ${filePath}:`, error.message);
    }
  }

  console.log(`✓ Downloaded ${downloaded} files`);

  // Save file manifest
  fs.writeFileSync(
    path.join(outputDir, 'storage', '_manifest.json'),
    JSON.stringify(fileList, null, 2)
  );

  return downloaded;
}

async function main() {
  try {
    const count = await exportStorage();
    console.log(`\nTotal: ${count} files exported`);
  } catch (error) {
    console.error('Export failed:', error.message);
    process.exit(1);
  }
}

main();
EOFJS

  # Run export
  if node "$export_script" "$service_account" "$output_dir" "$bucket_name" 2>&1; then
    log_success "Storage exported to $output_dir/storage/"
    return 0
  else
    log_error "Storage export failed"
    return 1
  fi
}

# Import Firestore data to PostgreSQL
import_firestore_to_postgres() {
  local data_dir="$1"
  local db_host="${2:-localhost}"
  local db_port="${3:-5432}"
  local db_name="${4:-nhost}"
  local db_user="${5:-postgres}"
  local db_pass="${6:-}"

  log_info "Importing Firestore data to PostgreSQL..."

  # Create import script
  local import_script="$data_dir/import-to-postgres.js"

  cat >"$import_script" <<'EOFJS'
const fs = require('fs');
const path = require('path');
const { Pool } = require('pg');

const dataDir = process.argv[2];
const dbConfig = {
  host: process.argv[3] || 'localhost',
  port: parseInt(process.argv[4]) || 5432,
  database: process.argv[5] || 'nhost',
  user: process.argv[6] || 'postgres',
  password: process.argv[7] || ''
};

const pool = new Pool(dbConfig);

function sanitizeColumnName(name) {
  return name.replace(/[^a-zA-Z0-9_]/g, '_').toLowerCase();
}

async function createTableFromData(collectionName, data) {
  if (data.length === 0) {
    console.log(`Skipping empty collection: ${collectionName}`);
    return 0;
  }

  // Analyze first document to determine schema
  const sample = data[0];
  const columns = [];

  for (const [key, value] of Object.entries(sample)) {
    const colName = sanitizeColumnName(key);
    let colType = 'TEXT';

    if (typeof value === 'number') {
      colType = Number.isInteger(value) ? 'INTEGER' : 'NUMERIC';
    } else if (typeof value === 'boolean') {
      colType = 'BOOLEAN';
    } else if (value instanceof Date || /^\d{4}-\d{2}-\d{2}/.test(value)) {
      colType = 'TIMESTAMP';
    } else if (typeof value === 'object' && value !== null) {
      colType = 'JSONB';
    }

    columns.push(`${colName} ${colType}`);
  }

  // Create table
  const tableName = sanitizeColumnName(collectionName);
  const createTableSQL = `
    CREATE TABLE IF NOT EXISTS ${tableName} (
      ${columns.join(',\n      ')},
      created_at TIMESTAMP DEFAULT NOW(),
      updated_at TIMESTAMP DEFAULT NOW()
    );
  `;

  await pool.query(createTableSQL);
  console.log(`✓ Created table: ${tableName}`);

  // Insert data
  let inserted = 0;
  for (const doc of data) {
    const colNames = Object.keys(doc).map(sanitizeColumnName);
    const values = Object.values(doc).map(v => {
      if (typeof v === 'object' && v !== null) {
        return JSON.stringify(v);
      }
      return v;
    });

    const placeholders = values.map((_, i) => `$${i + 1}`).join(', ');
    const insertSQL = `
      INSERT INTO ${tableName} (${colNames.join(', ')})
      VALUES (${placeholders})
      ON CONFLICT DO NOTHING;
    `;

    try {
      await pool.query(insertSQL, values);
      inserted++;
    } catch (error) {
      console.error(`Failed to insert into ${tableName}:`, error.message);
    }
  }

  console.log(`✓ Inserted ${inserted} rows into ${tableName}`);
  return inserted;
}

async function importCollections() {
  const firestoreDir = path.join(dataDir, 'firestore');

  if (!fs.existsSync(firestoreDir)) {
    console.error('Firestore data directory not found');
    process.exit(1);
  }

  const files = fs.readdirSync(firestoreDir)
    .filter(f => f.endsWith('.json') && !f.startsWith('_'));

  console.log(`Found ${files.length} collections to import`);

  let totalRows = 0;
  for (const file of files) {
    const collectionName = path.basename(file, '.json');
    const data = JSON.parse(fs.readFileSync(path.join(firestoreDir, file), 'utf8'));

    const rows = await createTableFromData(collectionName, data);
    totalRows += rows;
  }

  console.log(`\nTotal: ${totalRows} rows imported from ${files.length} collections`);
}

async function main() {
  try {
    console.log('Connecting to PostgreSQL...');
    await pool.query('SELECT NOW()');
    console.log('✓ Connected');

    await importCollections();

    await pool.end();
  } catch (error) {
    console.error('Import failed:', error.message);
    process.exit(1);
  }
}

main();
EOFJS

  # Run import
  if node "$import_script" "$data_dir" "$db_host" "$db_port" "$db_name" "$db_user" "$db_pass" 2>&1; then
    log_success "Firestore data imported to PostgreSQL"
    return 0
  else
    log_error "PostgreSQL import failed"
    return 1
  fi
}

# Import Firebase Auth users to nHost Auth
import_firebase_auth_to_nhost() {
  local data_dir="$1"
  local db_host="${2:-localhost}"
  local db_port="${3:-5432}"
  local db_name="${4:-nhost}"
  local db_user="${5:-postgres}"
  local db_pass="${6:-}"

  log_info "Importing Firebase Auth users to nHost..."

  # Create import script
  local import_script="$data_dir/import-auth.js"

  cat >"$import_script" <<'EOFJS'
const fs = require('fs');
const path = require('path');
const { Pool } = require('pg');
const crypto = require('crypto');

const dataDir = process.argv[2];
const dbConfig = {
  host: process.argv[3] || 'localhost',
  port: parseInt(process.argv[4]) || 5432,
  database: process.argv[5] || 'nhost',
  user: process.argv[6] || 'postgres',
  password: process.argv[7] || ''
};

const pool = new Pool(dbConfig);

async function importUsers() {
  const authFile = path.join(dataDir, 'auth', 'users.json');

  if (!fs.existsSync(authFile)) {
    console.error('Auth users file not found');
    process.exit(1);
  }

  const users = JSON.parse(fs.readFileSync(authFile, 'utf8'));
  console.log(`Importing ${users.length} users...`);

  let imported = 0;
  for (const user of users) {
    try {
      // Insert into auth.users table (nHost schema)
      const insertSQL = `
        INSERT INTO auth.users (
          id,
          email,
          email_verified,
          display_name,
          avatar_url,
          disabled,
          created_at,
          updated_at,
          metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (id) DO NOTHING;
      `;

      await pool.query(insertSQL, [
        user.uid,
        user.email,
        user.emailVerified || false,
        user.displayName || null,
        user.photoURL || null,
        user.disabled || false,
        user.metadata?.creationTime || new Date(),
        user.metadata?.lastSignInTime || new Date(),
        JSON.stringify(user.customClaims || {})
      ]);

      imported++;

      if (imported % 100 === 0) {
        console.log(`Imported ${imported}/${users.length} users...`);
      }
    } catch (error) {
      console.error(`Failed to import user ${user.uid}:`, error.message);
    }
  }

  console.log(`✓ Imported ${imported} users`);
  return imported;
}

async function main() {
  try {
    console.log('Connecting to PostgreSQL...');
    await pool.query('SELECT NOW()');
    console.log('✓ Connected');

    // Check if auth schema exists
    const schemaCheck = await pool.query(`
      SELECT schema_name FROM information_schema.schemata WHERE schema_name = 'auth';
    `);

    if (schemaCheck.rows.length === 0) {
      console.log('Creating auth schema...');
      await pool.query('CREATE SCHEMA IF NOT EXISTS auth;');

      // Create users table
      await pool.query(`
        CREATE TABLE IF NOT EXISTS auth.users (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          email TEXT UNIQUE NOT NULL,
          email_verified BOOLEAN DEFAULT FALSE,
          display_name TEXT,
          avatar_url TEXT,
          disabled BOOLEAN DEFAULT FALSE,
          created_at TIMESTAMP DEFAULT NOW(),
          updated_at TIMESTAMP DEFAULT NOW(),
          metadata JSONB
        );
      `);
      console.log('✓ Auth schema created');
    }

    await importUsers();

    await pool.end();
  } catch (error) {
    console.error('Import failed:', error.message);
    process.exit(1);
  }
}

main();
EOFJS

  # Run import
  if node "$import_script" "$data_dir" "$db_host" "$db_port" "$db_name" "$db_user" "$db_pass" 2>&1; then
    log_success "Firebase Auth users imported"
    log_warning "Note: Password hashes cannot be migrated. Users will need to reset passwords."
    return 0
  else
    log_error "Auth import failed"
    return 1
  fi
}

# Upload Firebase Storage to MinIO
import_firebase_storage_to_minio() {
  local data_dir="$1"
  local minio_endpoint="${2:-http://localhost:9000}"
  local minio_access_key="${3:-minioadmin}"
  local minio_secret_key="${4:-minioadmin}"
  local bucket_name="${5:-firebase-storage}"

  log_info "Uploading files to MinIO..."

  # Check if mc (MinIO Client) is available
  if ! command -v mc >/dev/null 2>&1; then
    log_warning "MinIO Client (mc) not found"
    log_info "Install: brew install minio/stable/mc (macOS) or curl https://dl.min.io/client/mc/release/linux-amd64/mc"
    log_info "Alternatively, use the MinIO Console to upload files manually"
    return 1
  fi

  # Configure MinIO client
  local alias_name="nself-migration"
  mc alias set "$alias_name" "$minio_endpoint" "$minio_access_key" "$minio_secret_key" >/dev/null 2>&1

  # Create bucket
  if ! mc ls "$alias_name/$bucket_name" >/dev/null 2>&1; then
    mc mb "$alias_name/$bucket_name" 2>&1 | grep -v "Bucket created" || true
    log_success "Created bucket: $bucket_name"
  fi

  # Upload files
  local storage_dir="$data_dir/storage"
  if [[ -d "$storage_dir" ]]; then
    log_info "Uploading files..."
    mc cp --recursive "$storage_dir/" "$alias_name/$bucket_name/" 2>&1 | grep -E "^(Total|✓)" || true
    log_success "Files uploaded to MinIO bucket: $bucket_name"
  else
    log_warning "Storage directory not found: $storage_dir"
  fi

  # Remove alias
  mc alias remove "$alias_name" >/dev/null 2>&1

  return 0
}

# Main migration orchestrator
migrate_from_firebase() {
  local service_account="$1"
  local output_dir="${2:-./firebase-migration-$(date +%Y%m%d-%H%M%S)}"
  local collections="${3:-all}"
  local storage_bucket="${4:-}"

  printf "${COLOR_CYAN}╔════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_CYAN}║   Firebase → nself Migration Tool     ║${COLOR_RESET}\n"
  printf "${COLOR_CYAN}╚════════════════════════════════════════╝${COLOR_RESET}\n"
  echo ""

  log_info "Mission: Help you escape vendor lock-in"
  echo ""

  # Validate credentials
  if ! validate_firebase_credentials "$service_account"; then
    return 1
  fi

  # Setup tools
  local workspace=$(setup_firebase_tools)
  if [[ -z "$workspace" ]]; then
    return 1
  fi

  cd "$workspace"

  mkdir -p "$output_dir"

  # Export data
  printf "${COLOR_CYAN}➞ Step 1: Export Firebase Data${COLOR_RESET}\n"
  echo ""

  export_firestore_data "$service_account" "$output_dir" "$collections"
  echo ""

  export_firebase_auth "$service_account" "$output_dir"
  echo ""

  if [[ -n "$storage_bucket" ]]; then
    export_firebase_storage "$service_account" "$output_dir" "$storage_bucket"
    echo ""
  fi

  # Import to nself
  printf "${COLOR_CYAN}➞ Step 2: Import to nself${COLOR_RESET}\n"
  echo ""

  log_info "To complete migration, run these commands:"
  echo ""
  echo "  # Import Firestore data to PostgreSQL"
  echo "  nself migrate from firebase import-data \"$output_dir\""
  echo ""
  echo "  # Import Auth users"
  echo "  nself migrate from firebase import-auth \"$output_dir\""
  echo ""
  if [[ -n "$storage_bucket" ]]; then
    echo "  # Import Storage to MinIO"
    echo "  nself migrate from firebase import-storage \"$output_dir\""
    echo ""
  fi

  log_success "Firebase data exported to: $output_dir"
  log_info "Migration workspace: $workspace"
  echo ""
  log_warning "Important: Password hashes cannot be migrated from Firebase"
  log_info "Users will need to reset their passwords after migration"

  return 0
}

# Export functions
export -f validate_firebase_credentials
export -f setup_firebase_tools
export -f export_firestore_data
export -f export_firebase_auth
export -f export_firebase_storage
export -f import_firestore_to_postgres
export -f import_firebase_auth_to_nhost
export -f import_firebase_storage_to_minio
export -f migrate_from_firebase
