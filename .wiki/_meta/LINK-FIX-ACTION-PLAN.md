# Documentation Link Fix - Action Plan

**Status**: Ready for Execution
**Priority**: High - Needed for wiki migration
**Current Broken Links**: 588 / 3,671 total (16.0%)

## Priority 1: Critical Missing Files

These files are referenced frequently but don't exist. Create them immediately:

### Getting Started Section

1. **`getting-started/Quick-Start.md`** ✓ EXISTS
   - Already exists, verify links point to correct name

2. **`getting-started/FIRST-PROJECT.md`** - CREATE
   - Referenced 3+ times
   - Should guide users through first project setup
   - Content: Step-by-step tutorial for creating first nself project

3. **`getting-started/CONCEPTS.md`** - CREATE
   - Referenced 2+ times
   - Should explain core concepts: services, tenants, RLS, etc.
   - Content: Foundational concepts every user should understand

### Core Guides

4. **`guides/AUTHENTICATION.md`** - CREATE
   - Referenced 3+ times
   - Critical for user onboarding
   - Content: Complete authentication setup guide
   - Should cover: JWT, OAuth, MFA, user management

### Configuration

5. **`configuration/BUILD-CONFIG.md`** - CREATE
   - Should explain build process configuration
   - Content: How `nself build` works, customization options

6. **`configuration/CASCADING-OVERRIDES.md`** - CREATE
   - Should explain .env file hierarchy
   - Content: .env.dev → .env.local → .env.staging → .env.prod → .secrets

### Security

7. **`security/SSL-SETUP.md`** - CREATE OR REDIRECT
   - Referenced 4+ times
   - Content: SSL/TLS certificate setup for production
   - May already be covered in `configuration/SSL.md`

## Priority 2: Fix Path Issues

### Example Projects

The examples exist but at different paths. Decision needed:

**Option A: Move Files to Match Links**
```bash
# Current:
docs/examples/projects/01-simple-blog/
docs/examples/projects/02-saas-starter/

# Move to:
docs/examples/01-simple-blog/
docs/examples/02-saas-starter/
```

**Option B: Update All Links**
Update ~10 links to point to `examples/projects/01-simple-blog/` instead of `examples/01-simple-blog/`

**Recommendation**: Option B (update links) - preserves better organization structure.

### Command Documentation

Fix these path mismatches:

1. **INFRA command documentation**
   - Links point to: `commands/INFRA.md`
   - Actually at: `migrations/INFRA-CONSOLIDATION.md`
   - **Action**: Create redirect or stub at `commands/INFRA.md` pointing to correct location

2. **Command reorganization docs**
   - Many links to `../../COMMAND-REORGANIZATION-SUMMARY.md`
   - **Action**: Verify this file location and update references

## Priority 3: API Reference Organization

Multiple links point to non-existent API docs:

1. **`../reference/api/graphql-schema.md`** - CREATE
   - Should document GraphQL schema
   - Content: Complete GraphQL API reference

2. **`../reference/PLUGIN-API.md`** - CREATE
   - Should document plugin development API
   - Content: How to create plugins, API reference

## Priority 4: Clean Up Sidebar

Update `_Sidebar.md` to remove broken links and verify structure:

```bash
python3 -c "
import re
with open('docs/_Sidebar.md', 'r') as f:
    content = f.read()
    links = re.findall(r'\[([^\]]+)\]\(([^\)]+)\)', content)
    for text, url in links:
        print(f'{text} -> {url}')
" | while read line; do
    # Check each link
    echo "Checking: $line"
done
```

## Priority 5: Monitoring Bundle Links

8 broken references to `MONITORING_BUNDLE` - these seem to be anchor links.

**Action**: Verify `services/MONITORING-BUNDLE.md` exists and has proper anchors.

## Quick Wins (Can Be Done in Parallel)

### Create Redirect/Stub Files

For files that exist elsewhere, create redirects:

1. **`commands/INFRA.md`**
```markdown
# Infrastructure Commands

This documentation has been reorganized. See:

- `[Infrastructure Consolidation](../migrations/INFRA-CONSOLIDATION.md)`
- `[infra Command Reference](../commands/DEPLOY.md#infra-subcommands)`
```

2. **`security/SSL-SETUP.md`**
```markdown
# SSL/TLS Setup

See the complete SSL configuration guide:

- `[SSL Configuration](../configuration/SSL.md)`
- `[Production Security Checklist](../guides/PRODUCTION-SECURITY-CHECKLIST.md)`
```

### Fix Example Path Links

Update all example links in one batch:

```bash
# Find and replace
find docs -name "*.md" -exec sed -i.bak \
  's|examples/01-simple-blog/|examples/projects/01-simple-blog/|g' {} \;

find docs -name "*.md" -exec sed -i.bak \
  's|examples/02-saas-starter/|examples/projects/02-saas-starter/|g' {} \;

# Clean up backups
find docs -name "*.bak" -delete
```

## Execution Plan

### Week 1: Create Critical Files

**Day 1-2**:
- Create `getting-started/FIRST-PROJECT.md`
- Create `getting-started/CONCEPTS.md`
- Create `guides/AUTHENTICATION.md`

**Day 3-4**:
- Create `configuration/BUILD-CONFIG.md`
- Create `configuration/CASCADING-OVERRIDES.md`
- Create redirect stubs (INFRA, SSL-SETUP)

**Day 5**:
- Update example path links (batch operation)
- Create `reference/api/graphql-schema.md`
- Create `reference/PLUGIN-API.md`

### Week 2: Validate and Polish

**Day 1**:
- Re-run link analyzer
- Target: < 100 broken links

**Day 2-3**:
- Manual review of remaining broken links
- Fix or remove invalid references

**Day 4**:
- Update `_Sidebar.md`
- Verify navigation works

**Day 5**:
- Final link audit
- Target: < 50 broken links
- Prepare for wiki migration

## Success Criteria

- [ ] All Priority 1 files created
- [ ] Broken links < 100 (from 588)
- [ ] All example paths corrected
- [ ] _Sidebar.md has no broken links
- [ ] Health score > 90% (from 54.1%)

## Scripts to Use

### 1. Check Progress
```bash
python3 /Users/admin/Sites/nself/scripts/analyze-links.py
```

### 2. Batch Fix Links (Dry Run)
```bash
DRY_RUN=true bash /Users/admin/Sites/nself/scripts/fix-doc-links.sh
```

### 3. Validate Specific File
```bash
# Check if a file exists
file="docs/getting-started/FIRST-PROJECT.md"
if [ -f "$file" ]; then
    echo "✓ $file exists"
else
    echo "✗ $file missing"
fi
```

### 4. Find All References to a File
```bash
# Find all links to a specific file
grep -r "FIRST-PROJECT" docs/ | grep -o '\[.*\]`(.*FIRST-PROJECT.*)`'
```

## Notes

- **Backup Created**: `/tmp/nself-docs-backup-20260131-171429`
- **Files Modified So Far**: 41 files, 254 fixes
- **DO NOT PUSH YET**: Keep changes local until validation complete

## Questions to Resolve

1. Should we move example projects or update links?
   - **Recommendation**: Update links to `examples/projects/`

2. Create full docs for missing files or just stubs?
   - **Recommendation**: Full docs for getting-started, stubs for others initially

3. Remove outdated references or create redirects?
   - **Recommendation**: Redirects for recently moved files, remove for ancient refs

---

**Next Action**: Start with Priority 1, creating critical missing files.
