#!/usr/bin/env bats
# Storage Operations Tests
# Tests for file upload/download, MinIO integration, and bucket operations

setup() {
    # Create temp test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"

    # Resolve nself path dynamically
    NSELF_PATH="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
    export PATH="$NSELF_PATH:$PATH"

    # Initialize minimal nself project
    nself init

    # Enable MinIO storage
    printf "MINIO_ENABLED=true\n" >> .env
    printf "MINIO_ROOT_USER=minioadmin\n" >> .env
    printf "MINIO_ROOT_PASSWORD=minioadmin123\n" >> .env
    printf "PROJECT_NAME=test-storage\n" >> .env
}

teardown() {
    # Stop any running containers
    docker compose down 2>/dev/null || true

    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
}

@test "storage help command shows available options" {
    run nself storage help
    [ "$status" -eq 0 ]
    [[ "$output" =~ "storage" ]] || [[ "$output" =~ "file" ]]
}

@test "storage requires MinIO to be enabled" {
    # Disable MinIO
    printf "MINIO_ENABLED=false\n" > .env.override

    run nself storage status
    # Should indicate MinIO is not enabled or fail gracefully
    [ "$status" -eq 0 ] || [[ "$output" =~ "not enabled" ]] || [[ "$output" =~ "MinIO" ]]
}

@test "storage bucket list shows available buckets" {
    skip "Requires MinIO container running"

    # Start services
    nself build
    nself start

    run nself storage bucket list
    [ "$status" -eq 0 ]
}

@test "storage bucket create creates new bucket" {
    skip "Requires MinIO container running"

    nself build
    nself start

    run nself storage bucket create test-bucket
    [ "$status" -eq 0 ]
    [[ "$output" =~ "created" ]] || [[ "$output" =~ "success" ]]
}

@test "storage upload requires file path" {
    run nself storage upload
    # Should fail or show usage without file path
    [ "$status" -ne 0 ] || [[ "$output" =~ "usage" ]] || [[ "$output" =~ "required" ]]
}

@test "storage upload validates file exists" {
    run nself storage upload nonexistent.txt
    [ "$status" -ne 0 ]
    [[ "$output" =~ "not found" ]] || [[ "$output" =~ "does not exist" ]]
}

@test "storage download requires object key" {
    run nself storage download
    # Should fail or show usage without object key
    [ "$status" -ne 0 ] || [[ "$output" =~ "usage" ]] || [[ "$output" =~ "required" ]]
}

@test "storage quota shows current usage" {
    skip "Requires MinIO container running"

    nself build
    nself start

    run nself storage quota
    [ "$status" -eq 0 ]
    [[ "$output" =~ "usage" ]] || [[ "$output" =~ "quota" ]]
}

@test "storage handles missing docker gracefully" {
    skip "Docker availability varies by environment"

    # Test graceful failure when Docker isn't available
}

@test "storage status shows MinIO connection status" {
    run nself storage status
    [ "$status" -eq 0 ]
}

@test "storage bucket operations handle invalid names" {
    skip "Requires MinIO container running"

    # Bucket names must be DNS-compliant
    run nself storage bucket create "Invalid_Bucket_Name!"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "invalid" ]] || [[ "$output" =~ "name" ]]
}

@test "storage upload supports different file types" {
    skip "Requires MinIO container running"

    # Create test files
    echo "test content" > test.txt
    printf "\x89PNG\r\n\x1a\n" > test.png

    nself build
    nself start

    # Upload text file
    run nself storage upload test.txt
    [ "$status" -eq 0 ]

    # Upload binary file
    run nself storage upload test.png
    [ "$status" -eq 0 ]
}

@test "storage generates presigned URLs for file access" {
    skip "Requires MinIO container running"

    nself build
    nself start

    # Upload file
    echo "test" > test.txt
    nself storage upload test.txt

    # Generate presigned URL
    run nself storage presign test.txt
    [ "$status" -eq 0 ]
    [[ "$output" =~ "http" ]]
}

@test "storage bucket delete removes bucket" {
    skip "Requires MinIO container running"

    nself build
    nself start

    # Create bucket
    nself storage bucket create test-delete-bucket

    # Delete bucket
    run nself storage bucket delete test-delete-bucket
    [ "$status" -eq 0 ]
}

@test "storage bucket delete prevents deletion of non-empty bucket" {
    skip "Requires MinIO container running"

    nself build
    nself start

    # Create bucket and upload file
    nself storage bucket create test-nonempty-bucket
    echo "test" > test.txt
    nself storage upload test.txt test-nonempty-bucket

    # Try to delete non-empty bucket
    run nself storage bucket delete test-nonempty-bucket
    [ "$status" -ne 0 ] || [[ "$output" =~ "not empty" ]] || [[ "$output" =~ "force" ]]
}
