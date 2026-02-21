# Documentation Consistency Fix Guide

**Purpose**: Practical guide to fix all identified consistency issues
**Related Docs**:
- [STYLE-GUIDE.md](STYLE-GUIDE.md) - Official style standards
- [CONSISTENCY-AUDIT-REPORT.md](CONSISTENCY-AUDIT-REPORT.md) - Detailed audit findings

---

## Quick Fix Scripts

### 1. Update Version References (High Priority)

**Problem**: 240 files reference v0.9.6 instead of v0.9.8

**Find affected files**:
```bash
cd /Users/admin/Sites/nself/docs

# Find all v0.9.6 references
grep -r "v0\.9\.6\|0\.9\.6" . --include="*.md" | wc -l
# Result: ~240 references

# See specific files
grep -r "v0\.9\.6\|0\.9\.6" . --include="*.md" -l
```

**Automated fix** (USE WITH CAUTION):
```bash
# Backup first!
cp -r docs docs.backup

# Update version badges
find docs -name "*.md" -exec sed -i '' 's/version-0\.9\.6/version-0.9.8/g' {} \;

# Update version in links
find docs -name "*.md" -exec sed -i '' 's|releases/v0\.9\.6\.md|releases/v0.9.8.md|g' {} \;
```

**Manual review required for**:
```bash
# These may be intentional historical references:
grep -r "v0\.9\.6" docs/releases/ --include="*.md"

# Version callouts - review context
grep -r "> \*\*v0\.9\.6" docs/ --include="*.md"
```

**Safe approach**:
1. Update badge versions: `version-0.9.6` → `version-0.9.8`
2. Update "current version" statements
3. Keep historical references in release notes
4. Change version callouts only where referring to current version

---

### 2. Fix Placeholder Inconsistencies (Medium Priority)

**Problem**: Mixed formats like `<project-name>`, `PROJECT_NAME`, `your-project`

**Standard format**:
- Commands: `<kebab-case>`
- Environment variables: `UPPERCASE_UNDERSCORES`
- Examples: Use concrete values (`myapp`, `acme`)

**Find mixed usage**:
```bash
# Find uppercase placeholders in commands
grep -r "nself.*[A-Z_]\{5,\}" docs/commands/ --include="*.md" | head -20

# Find angle brackets with underscores
grep -r "<[A-Z_]*>" docs/ --include="*.md" | head -20
```

**Manual fix required** - Context-dependent:

**Before** (inconsistent):
```bash
nself tenant create PROJECT_NAME
nself tenant create <PROJECT_NAME>
nself tenant create your-project-name
```

**After** (standardized):
```bash
# In reference docs - use placeholder
nself tenant create <project-name>

# In tutorials - use concrete example
nself tenant create myapp
```

**Recommendation**: Fix file-by-file in command reference docs first.

---

### 3. Standardize Command Casing (Medium Priority)

**Problem**: Rare cases of `NSELF`, `Nself`, or uppercase subcommands

**Find issues**:
```bash
# Find wrong command casing
grep -r "NSELF db\|NSELF tenant\|Nself " docs/ --include="*.md"

# Find uppercase subcommands
grep -r "nself [A-Z][A-Z]" docs/ --include="*.md"
```

**Expected results**: Should be very few or zero

**Fix**:
```bash
# All commands should be lowercase:
nself db migrate up          # ✅ Correct
nself tenant create          # ✅ Correct

# NOT:
NSELF db migrate up          # ❌ Wrong
nself DB migrate up          # ❌ Wrong
```

---

### 4. Add Missing Language Identifiers (Low Priority)

**Problem**: Some code blocks missing ` ```bash ` identifier

**Find blocks without language**:
```bash
# Find code blocks without language identifier
grep -rn "^\`\`\`$" docs/ --include="*.md" | head -20
```

**Fix template**:
````markdown
# Before:
```
nself start
```

# After:
```bash
nself start
```
````

**Supported languages**:
- `bash` - Shell commands
- `sql` - SQL queries
- `typescript`, `javascript`, `python`, `go` - Code
- `json`, `yaml` - Config files
- `dbml` - Database markup

---

### 5. Fix Link Formats (Low Priority)

**Problem**: Occasional absolute paths instead of relative

**Find absolute links**:
```bash
# Find absolute GitHub URLs
grep -r "https://github.com/nself-org/cli/blob/main/docs" docs/ --include="*.md"

# Find absolute /docs paths
grep -r "\](/" docs/ --include="*.md"
```

**Fix pattern**:
```markdown
# Wrong (absolute):
[Quick Start](getting-started/Quick-Start.md)
[Quick Start](https://github.com/.../do../getting-started/Quick-Start.md)

# Correct (relative):
# From /docs/README.md:
[Quick Start](getting-started/Quick-Start.md)

# From /docs/guides/DEPLOYMENT.md:
[Quick Start](getting-started/Quick-Start.md)
```

---

### 6. Standardize Headers (Low Priority)

**Problem**: Inconsistent capitalization (Title Case vs sentence case)

**Standard**: Use **sentence case**

```markdown
✅ Correct:
# Database workflow guide
## Creating your first migration
### Migration file format

❌ Incorrect:
# Database Workflow Guide
## Creating Your First Migration
### Migration File Format
```

**Exceptions**:
- Brand names: `nself`, `PostgreSQL`, `Docker`
- Acronyms: `SQL`, `API`, `CLI`
- Proper nouns: `GitHub`, `Hasura`

**Find title case headers**:
```bash
# Find headers with multiple capital words
grep -rn "^## [A-Z][a-z]* [A-Z]" docs/ --include="*.md" | head -30
```

**Manual fix required** - Review context for proper nouns.

---

## Validation Scripts

### Check Documentation Quality

Create `/scripts/check-docs-consistency.sh`:

```bash
#!/bin/bash

echo "=== Documentation Consistency Check ==="
echo

# 1. Check current version
CURRENT_VERSION=$(cat src/VERSION)
echo "Current version: $CURRENT_VERSION"

# 2. Find outdated version references
echo
echo "Checking for outdated version references..."
OUTDATED=$(grep -r "v0\.9\.[0-6]" docs/ --include="*.md" -l | wc -l)
echo "Files with old versions: $OUTDATED"

# 3. Check for missing language identifiers
echo
echo "Checking for code blocks without language..."
MISSING_LANG=$(grep -rn "^\`\`\`$" docs/ --include="*.md" | wc -l)
echo "Code blocks without language: $MISSING_LANG"

# 4. Check for absolute links
echo
echo "Checking for absolute paths in links..."
ABS_LINKS=$(grep -r "\](/" docs/ --include="*.md" | wc -l)
echo "Absolute link paths: $ABS_LINKS"

# 5. Summary
echo
echo "=== Summary ==="
if [[ $OUTDATED -gt 0 ]] || [[ $MISSING_LANG -gt 0 ]] || [[ $ABS_LINKS -gt 0 ]]; then
  echo "⚠️  Issues found. Run individual checks above for details."
  exit 1
else
  echo "✅ All checks passed!"
  exit 0
fi
```

**Usage**:
```bash
chmod +x scripts/check-docs-consistency.sh
./scripts/check-docs-consistency.sh
```

---

## Pre-Commit Hook

Add to `.git/hooks/pre-commit`:

```bash
#!/bin/bash

# Check if any .md files are being committed
MD_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.md$')

if [ -n "$MD_FILES" ]; then
  echo "Checking documentation consistency..."

  # Check for common issues
  for file in $MD_FILES; do
    # Check for code blocks without language
    if grep -q "^\`\`\`$" "$file"; then
      echo "⚠️  Warning: $file has code blocks without language identifier"
    fi

    # Check for absolute paths
    if grep -q "\](/" "$file"; then
      echo "⚠️  Warning: $file contains absolute /docs/ paths"
    fi
  done

  echo "✅ Documentation checks complete"
fi
```

---

## CI/CD Integration

Add to `.github/workflows/docs-check.yml`:

```yaml
name: Documentation Checks

on:
  pull_request:
    paths:
      - 'docs/**/*.md'

jobs:
  check-consistency:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Check documentation consistency
        run: |
          # Check for outdated versions
          CURRENT_VERSION=$(cat src/VERSION)
          echo "Current version: $CURRENT_VERSION"

          # Find old version references (may be historical - warn only)
          OLD_REFS=$(grep -r "v0\.9\.[0-6]" docs/ --include="*.md" -l | wc -l)
          if [ $OLD_REFS -gt 0 ]; then
            echo "::warning::Found $OLD_REFS files with older version references"
          fi

          # Check for missing language identifiers
          MISSING=$(grep -rn "^\`\`\`$" docs/ --include="*.md" | wc -l)
          if [ $MISSING -gt 0 ]; then
            echo "::error::Found $MISSING code blocks without language identifier"
            exit 1
          fi

          echo "✅ Documentation checks passed"
```

---

## Safe Update Workflow

### For Major Version Updates

```bash
# 1. Backup
cp -r docs docs.backup.$(date +%Y%m%d)

# 2. Update version file
echo "0.9.8" > src/VERSION

# 3. Update badges in main docs
find docs -name "README.md" -o -name "INDEX.md" | while read file; do
  sed -i '' 's/version-0\.9\.6/version-0.9.8/g' "$file"
done

# 4. Update version tables
# (Manual - in docs/README.md and release notes)

# 5. Verify
./scripts/check-docs-consistency.sh

# 6. Review changes
git diff docs/

# 7. Commit
git add docs/
git commit -m "docs: update to version 0.9.8"
```

---

## Priority Action Plan

### Week 1 - Critical Updates

1. ✅ **DONE**: Updated main README.md to v0.9.8
2. ✅ **DONE**: Updated Quick-Start.md version notes
3. **TODO**: Review and update all INDEX.md files
4. **TODO**: Update version badges in key docs
5. **TODO**: Create check script

### Week 2 - Standardization

6. **TODO**: Fix placeholder inconsistencies in command docs
7. **TODO**: Add missing language identifiers
8. **TODO**: Standardize header capitalization

### Week 3 - Automation

9. **TODO**: Add pre-commit hook
10. **TODO**: Add CI/CD checks
11. **TODO**: Create contributor guidelines

---

## Files Already Updated

### ✅ Completed (January 31, 2026)

1. `/docs/README.md`
   - Version badge: 0.9.6 → 0.9.8
   - Version callout: v0.9.6 → v0.9.8
   - Version history table: Added v0.9.7 and v0.9.8
   - Footer date: Updated to January 31, 2026

2. `/do../getting-started/Quick-Start.md`
   - Version note: v0.9.6 → v0.9.7+ (for historical accuracy)

3. **NEW FILES CREATED**:
   - `/docs/STYLE-GUIDE.md` - Official style standards
   - `/docs/CONSISTENCY-AUDIT-REPORT.md` - Detailed audit
   - `/docs/CONSISTENCY-FIX-GUIDE.md` - This file

---

## Remaining High-Priority Files

### Need Review (Version Updates)

Check these files for version references:

```bash
# Find all files with version references
grep -rl "v0\.9\.6" docs/ --include="*.md" | sort

# Prioritize:
# 1. INDEX.md files
# 2. README.md files
# 3. Getting started guides
# 4. Release notes (keep historical references)
```

---

## Notes

1. **Historical References**: Don't change version numbers in release notes or historical documentation - those are intentionally dated.

2. **Version Callouts**: Only update `> **v0.9.6:**` notes if they refer to "current version". If they're historical ("In v0.9.6, we added..."), keep them.

3. **Concrete Examples**: When standardizing placeholders, prefer concrete examples in tutorials and guides. Save `<placeholders>` for reference documentation.

4. **Test Before Commit**: Always review changes with `git diff` before committing mass updates.

5. **Gradual Updates**: Don't feel pressured to fix everything at once. Prioritize:
   - High: Version updates, broken links
   - Medium: Placeholder consistency, command formatting
   - Low: Header capitalization, code block languages

---

## Quick Reference

**Current Version**: 0.9.8 (as of February 16, 2026)

**Style Standards**: See [STYLE-GUIDE.md](STYLE-GUIDE.md)

**Audit Report**: See [CONSISTENCY-AUDIT-REPORT.md](CONSISTENCY-AUDIT-REPORT.md)

---

<div align="center">

**Documentation Consistency Fix Guide**

*Practical steps to standardize 406 markdown files*

</div>
