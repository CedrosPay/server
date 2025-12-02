#!/bin/bash

# Cedros Pay Server - Public Repository Publishing Script
#
# This script copies the source code to a public repository while excluding
# sensitive configuration, audit documents, and private development files.

set -e  # Exit on error

# Configuration
PRIVATE_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PUBLIC_REPO_ROOT="/Users/conorholdsworth/Workspace/published/cedrospay/server"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Excluded files and directories (private/sensitive content)
EXCLUDE_PATTERNS=(
    # Audit documents (keep private - internal review process)
    "AUDIT_FINDINGS_1.md"
    "AUDIT_FINDINGS_2.md"
    "AUDIT_FINDINGS_3.md"
    "AUDIT_RESPONSE_1.md"
    "AUDIT_RESPONSE_2.md"
    "DOCUMENTATION_AUDIT.md"
    "readiness-audit.md"
    "readiness-audit-impact.md"
    "AUDIT-RESPONSE.md"
    "AUDIT-RESPONSE-2.md"

    # Sensitive configuration files (CRITICAL: prevents secret leaks)
    "configs/local.yaml"
    "configs/*.local.yaml"
    "configs/dev.yaml"
    "configs/staging.yaml"
    "configs/production.yaml"
    "configs/deploy.yaml"

    # GitHub directory (private deployment automation, issue templates, etc.)
    ".github/"

    # Database files containing PII and transaction history (CRITICAL)
    "data/"
    "data/*.db"
    "data/*.sqlite"
    "data/*.sqlite3"
    "*.db"
    "*.sqlite"
    "*.sqlite3"

    # Environment files (.env but NOT .env.example)
    ".env"
    ".env.local"
    ".env.production"
    ".env.staging"
    ".env.development"
    ".env.test"

    # Test coverage and artifacts
    "coverage.out"
    "coverage.html"
    "*.test"
    "*.prof"

    # Build artifacts
    "bin/"
    "dist/"
    "server"
    "cmd/server/server"
    "cedros-pay-server"

    # Go module cache (added for audit - not needed in public repo)
    ".gomodcache/"

    # IDE and OS files
    ".DS_Store"
    ".idea/"
    ".vscode/"
    "*.swp"
    "*.swo"
    "*~"
    ".project"
    ".classpath"
    ".settings/"

    # Git directory (will be separate in public repo)
    ".git/"

    # Claude Code project-specific instructions (keep private)
    "CLAUDE.md"

    # Temporary files
    "tmp/"
    "temp/"
    "*.tmp"
    "*.log"
)

# Print banner
echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Cedros Pay Server - Public Repository Publisher      ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
echo ""

# Verify we're in the correct directory
if [ ! -f "$PRIVATE_REPO_ROOT/go.mod" ]; then
    echo -e "${RED}Error: Not in the correct repository root${NC}"
    echo "Expected to find go.mod in: $PRIVATE_REPO_ROOT"
    exit 1
fi

echo -e "${BLUE}Private repo:${NC} $PRIVATE_REPO_ROOT"
echo -e "${BLUE}Public repo:${NC}  $PUBLIC_REPO_ROOT"
echo ""

# Create public repo directory if it doesn't exist
if [ ! -d "$PUBLIC_REPO_ROOT" ]; then
    echo -e "${YELLOW}Public repository directory does not exist.${NC}"
    read -p "Create $PUBLIC_REPO_ROOT? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        mkdir -p "$PUBLIC_REPO_ROOT"
        echo -e "${GREEN}✓${NC} Created public repository directory"
    else
        echo -e "${RED}Aborted.${NC}"
        exit 1
    fi
fi

# Build rsync exclusion arguments
RSYNC_EXCLUDE_ARGS=()
for pattern in "${EXCLUDE_PATTERNS[@]}"; do
    RSYNC_EXCLUDE_ARGS+=(--exclude="$pattern")
done

# Show what will be excluded
echo -e "${YELLOW}Excluding the following patterns:${NC}"
for pattern in "${EXCLUDE_PATTERNS[@]}"; do
    echo "  - $pattern"
done
echo ""

# Security check: Warn if sensitive files exist
echo -e "${YELLOW}Security check...${NC}"
SENSITIVE_FILES_FOUND=false

# Check for .env files (except .env.example)
if find "$PRIVATE_REPO_ROOT" -maxdepth 1 -name ".env" -not -name ".env.example" | grep -q .; then
    echo -e "${RED}⚠ WARNING: .env file found (will be excluded)${NC}"
    SENSITIVE_FILES_FOUND=true
fi

# Check for config files with secrets
if find "$PRIVATE_REPO_ROOT/configs" -name "*.yaml" 2>/dev/null | grep -E "(local|dev|staging|production)" | grep -q .; then
    echo -e "${RED}⚠ WARNING: Sensitive config files found (will be excluded)${NC}"
    SENSITIVE_FILES_FOUND=true
fi

# Check for database files
if find "$PRIVATE_REPO_ROOT" -name "*.db" -o -name "*.sqlite" -o -name "*.sqlite3" 2>/dev/null | grep -q .; then
    echo -e "${RED}⚠ WARNING: Database files found (will be excluded)${NC}"
    SENSITIVE_FILES_FOUND=true
fi

if [ "$SENSITIVE_FILES_FOUND" = true ]; then
    echo -e "${GREEN}✓${NC} Sensitive files will be excluded from publish"
else
    echo -e "${GREEN}✓${NC} No sensitive files detected"
fi
echo ""

# Confirm before proceeding
read -p "Proceed with publishing? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${RED}Aborted.${NC}"
    exit 1
fi

# Perform the sync
echo ""
echo -e "${BLUE}Syncing files...${NC}"
rsync -av --delete \
    "${RSYNC_EXCLUDE_ARGS[@]}" \
    --exclude='.git' \
    --exclude='node_modules/' \
    --exclude='vendor/' \
    "$PRIVATE_REPO_ROOT/" \
    "$PUBLIC_REPO_ROOT/"

if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}✓ Successfully published to public repository${NC}"
    echo ""

    # Post-publish verification
    echo -e "${BLUE}Post-publish verification...${NC}"

    # Check that .env.example exists (should be published)
    if [ -f "$PUBLIC_REPO_ROOT/.env.example" ]; then
        echo -e "${GREEN}✓${NC} .env.example is present"
    else
        echo -e "${YELLOW}⚠${NC} .env.example not found (consider adding one)"
    fi

    # Check that CLAUDE.md is NOT published
    if [ ! -f "$PUBLIC_REPO_ROOT/CLAUDE.md" ]; then
        echo -e "${GREEN}✓${NC} CLAUDE.md excluded (private instructions)"
    else
        echo -e "${RED}✗${NC} WARNING: CLAUDE.md should not be published!"
    fi

    # Check that audit files are NOT published
    if [ ! -f "$PUBLIC_REPO_ROOT/AUDIT_FINDINGS_1.md" ]; then
        echo -e "${GREEN}✓${NC} Audit files excluded (internal documents)"
    else
        echo -e "${RED}✗${NC} WARNING: Audit files should not be published!"
    fi

    # Check that spec docs ARE published
    if [ -d "$PUBLIC_REPO_ROOT/docs/specs" ] && [ -f "$PUBLIC_REPO_ROOT/docs/specs/README.md" ]; then
        SPEC_COUNT=$(ls -1 "$PUBLIC_REPO_ROOT/docs/specs/"*.md 2>/dev/null | wc -l | tr -d ' ')
        echo -e "${GREEN}✓${NC} Spec documentation published ($SPEC_COUNT files)"
    else
        echo -e "${YELLOW}⚠${NC} Spec documentation not found (docs/specs/)"
    fi

    # Check that API reference is published
    if [ -f "$PUBLIC_REPO_ROOT/docs/API_REFERENCE.md" ]; then
        echo -e "${GREEN}✓${NC} API reference published"
    else
        echo -e "${YELLOW}⚠${NC} API reference not found (docs/API_REFERENCE.md)"
    fi

    echo ""

    # Show summary
    echo -e "${BLUE}Next steps:${NC}"
    echo "  1. cd $PUBLIC_REPO_ROOT"
    echo "  2. Review changes: git status"
    echo "  3. Verify no secrets: git diff"
    echo "  4. Commit and push: git add . && git commit -m 'Update from private repo' && git push"
    echo ""

    # Check if public repo is a git repository
    if [ -d "$PUBLIC_REPO_ROOT/.git" ]; then
        echo -e "${YELLOW}Public repository git status:${NC}"
        cd "$PUBLIC_REPO_ROOT"
        git status --short | head -20

        # Count changes
        NUM_CHANGED=$(git status --short | wc -l | tr -d ' ')
        if [ "$NUM_CHANGED" -gt 0 ]; then
            echo ""
            echo -e "${YELLOW}Total files changed: $NUM_CHANGED${NC}"
        fi
    else
        echo -e "${YELLOW}Note: Public repository is not yet a git repository.${NC}"
        echo "Initialize it with: cd $PUBLIC_REPO_ROOT && git init"
    fi
else
    echo -e "${RED}✗ Failed to publish${NC}"
    exit 1
fi
