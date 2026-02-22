#!/usr/bin/env bash
# test-utils.sh - Shared test utilities for nself unit tests
# Provides common color definitions and helper stubs

# Color codes (only define if not already set)
: ${RED:='\033[0;31m'}
: ${GREEN:='\033[0;32m'}
: ${YELLOW:='\033[1;33m'}
: ${BLUE:='\033[0;34m'}
: ${NC:='\033[0m'}
