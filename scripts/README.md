# Scripts Directory

Utility scripts for development, testing, and publishing.

---

## publish.sh

**Purpose:** Sync the private development repository to the public open-source repository.

### Usage

```bash
./scripts/publish.sh
```

**Publishes to:** `/Users/conorholdsworth/Workspace/published/cedrospay/server`

The script will:
1. Show what will be excluded from the public repository
2. Ask for confirmation before proceeding
3. Use `rsync` to copy files while excluding sensitive content
4. Display git status of the public repository

### What Gets Excluded

The following files/directories are **excluded** from the public repository:

**Audit Documents (Private):**
- `readiness-audit.md`
- `readiness-audit-impact.md`
- `AUDIT-RESPONSE.md`
- `AUDIT-RESPONSE-2.md`
- `FRONTEND_REFUND_INTEGRATION.md`

**Development Files:**
- `.env` (environment variables with secrets)
- `.env.*` (any environment-specific configs)
- `coverage.out` (test coverage reports)
- `*.test` (test binaries)
- `CLAUDE.md` (private AI development instructions)

**Build Artifacts:**
- `bin/`
- `dist/`
- `server` (compiled binary)
- `cmd/server/server`

**IDE/OS Files:**
- `.DS_Store`
- `.idea/`
- `.vscode/`
- `*.swp`, `*.swo`, `*~`

**Git Directory:**
- `.git/` (public repo has its own git history)

### What Gets Published

Everything else, including:
- ✅ Source code (`cmd/`, `internal/`, `pkg/`)
- ✅ Public documentation (`README.md`, `docs/`, `CONTRIBUTING.md`, `SECURITY.md`, etc.)
- ✅ Configuration templates (`.env.example`)
- ✅ Go modules (`go.mod`, `go.sum`)
- ✅ Build configuration (`Makefile`, `.dockerignore`)
- ✅ Migrations (`migrations/`)
- ✅ Agent documentation (`AGENTS.md`)
- ✅ License (`LICENSE`)

### First-Time Setup

If the public repository doesn't exist yet:

```bash
# 1. Run the publish script (it will create the directory)
./scripts/publish.sh

# 2. Initialize git in the public repo
cd /Users/conorholdsworth/Workspace/published/cedrospay-server
git init
git add .
git commit -m "Initial public release"

# 3. Add remote and push
git remote add origin https://github.com/your-org/cedrospay-server.git
git branch -M main
git push -u origin main
```

### Subsequent Releases

For updates to the public repository:

```bash
# 1. Run the publish script from private repo
./scripts/publish.sh

# 2. Review changes in public repo
cd /Users/conorholdsworth/Workspace/published/cedrospay-server
git status
git diff

# 3. Commit and push
git add .
git commit -m "Update: <describe changes>"
git push
```

### Workflow Recommendations

**Private Development:**
- Work in `/Users/conorholdsworth/Workspace/cedros/cedros-pay/server` (private)
- Commit audit documents, sensitive configs, private notes
- Use `.env` for real credentials

**Public Releases:**
- Run `./scripts/publish.sh` when ready to publish
- Review changes before pushing to public repo
- Keep `.env.example` updated with non-sensitive placeholders

### Customizing Exclusions

To exclude additional files, edit `scripts/publish.sh` and add patterns to the `EXCLUDE_PATTERNS` array:

```bash
EXCLUDE_PATTERNS=(
    # ... existing patterns ...
    "your-custom-file.txt"
    "private-notes/"
)
```

### Safety Features

- **Confirmation prompt** before syncing
- **Dry-run option** (not yet implemented)
- **Shows git status** after sync to review changes
- **Preserves public repo's `.git` directory**

### Troubleshooting

**Error: "Not in the correct repository root"**
- Run the script from the repository root: `./scripts/publish.sh`

**Public repo not created**
- The script will prompt to create it if it doesn't exist
- Or manually create: `mkdir -p /Users/conorholdsworth/Workspace/published/cedrospay-server`

**Git conflicts after sync**
- Review with `git status` in public repo
- Use `git diff` to see what changed
- Manually resolve if needed

### Advanced Usage

**Custom destination:**
Edit the `PUBLIC_REPO_ROOT` variable in `publish.sh`:

```bash
PUBLIC_REPO_ROOT="/path/to/your/public/repo"
```

**Preview changes without syncing:**
Use rsync's dry-run mode (modify script to add `-n` flag):

```bash
rsync -avn --delete ... # Add -n for dry-run
```

---

## populate-demo-data.sh

**Purpose:** Populate MongoDB and PostgreSQL databases with demo products and coupons from `configs/local.yaml`.

### Usage

```bash
# Populate both MongoDB and PostgreSQL
./scripts/populate-demo-data.sh

# Populate only MongoDB
./scripts/populate-demo-data.sh mongodb

# Populate only PostgreSQL
./scripts/populate-demo-data.sh postgres
```

### Environment Variables

```bash
# MongoDB connection string (default: mongodb://localhost:27017)
export MONGO_URI="mongodb://localhost:27017"

# PostgreSQL connection string (default: postgres://localhost:5432/cedros_pay?sslmode=disable)
export POSTGRES_URI="postgres://user:pass@localhost:5432/cedros_pay?sslmode=disable"
```

### What Gets Populated

**Products:**
- `demo-item-id-1` - Demo protected content ($1.00)
- `demo-item-id-2` - Test product 2 ($2.22)

**Coupons:**
- `SAVE20` - 20% off site-wide (manual)
- `NEWUSER10` - $10 off site-wide (manual)
- `SITE10` - 10% off site-wide (auto-apply)
- `CRYPTO5AUTO` - 5% off crypto payments (auto-apply)
- `FIXED5` - $0.50 off site-wide (auto-apply)

**Data Source Indicator:**
- MongoDB data includes `"source": "mongodb"` in metadata
- PostgreSQL data includes `"source": "postgres"` in metadata
- Descriptions are updated to indicate data source (e.g., "Demo protected content (from MongoDB)")

### Prerequisites

**For MongoDB:**
```bash
# Install MongoDB Shell
brew install mongosh

# Start MongoDB (if using local instance)
brew services start mongodb-community
```

**For PostgreSQL:**
```bash
# Install PostgreSQL client
brew install postgresql

# Create database (if it doesn't exist)
createdb cedros_pay

# Or using psql
psql postgres -c "CREATE DATABASE cedros_pay;"
```

### Database Schema

**Products Table (PostgreSQL):**
```sql
CREATE TABLE products (
    id TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    fiat_amount DOUBLE PRECISION NOT NULL,
    fiat_currency TEXT NOT NULL,
    stripe_price_id TEXT NOT NULL,
    crypto_amount DOUBLE PRECISION NOT NULL,
    crypto_token TEXT NOT NULL,
    crypto_account TEXT NOT NULL,
    memo_template TEXT NOT NULL,
    metadata JSONB NOT NULL,
    active BOOLEAN NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

**Coupons Table (PostgreSQL):**
```sql
CREATE TABLE coupons (
    code TEXT PRIMARY KEY,
    discount_type TEXT NOT NULL,
    discount_value DOUBLE PRECISION NOT NULL,
    currency TEXT NOT NULL,
    scope TEXT NOT NULL,
    product_ids JSONB NOT NULL,
    payment_method TEXT NOT NULL,
    auto_apply BOOLEAN NOT NULL,
    usage_limit INTEGER,
    usage_count INTEGER NOT NULL,
    starts_at TIMESTAMP,
    expires_at TIMESTAMP,
    active BOOLEAN NOT NULL,
    metadata JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

### Updating Your Config

After populating, update `configs/local.yaml` to use the database:

**For MongoDB:**
```yaml
paywall:
  product_source: "mongodb"
  mongodb_url: "mongodb://localhost:27017"
  mongodb_database: "cedros_pay"
  mongodb_collection: "products"

coupons:
  coupon_source: "mongodb"
  mongodb_url: "mongodb://localhost:27017"
  mongodb_database: "cedros_pay"
  mongodb_collection: "coupons"
```

**For PostgreSQL:**
```yaml
paywall:
  product_source: "postgres"
  postgres_url: "postgres://localhost:5432/cedros_pay?sslmode=disable"

coupons:
  coupon_source: "postgres"
  postgres_url: "postgres://localhost:5432/cedros_pay?sslmode=disable"
```

### Testing the Integration

**1. Populate the databases:**
```bash
./scripts/populate-demo-data.sh
```

**2. Start the server with MongoDB:**
```bash
# Edit configs/local.yaml to use product_source: "mongodb"
go run cmd/server/main.go -config configs/local.yaml
```

**3. Test product retrieval:**
```bash
# Get product list
curl http://localhost:8080/products

# Get specific product
curl http://localhost:8080/paywall/demo-item-id-1
```

**4. Verify data source:**
Check that product descriptions include "(from MongoDB)" or "(from PostgreSQL)"

**5. Test with PostgreSQL:**
```bash
# Edit configs/local.yaml to use product_source: "postgres"
go run cmd/server/main.go -config configs/local.yaml
```

### Troubleshooting

**MongoDB connection failed:**
```bash
# Check if MongoDB is running
brew services list | grep mongodb

# Start MongoDB
brew services start mongodb-community

# Test connection manually
mongosh mongodb://localhost:27017
```

**PostgreSQL connection failed:**
```bash
# Check if PostgreSQL is running
brew services list | grep postgresql

# Start PostgreSQL
brew services start postgresql

# Test connection manually
psql postgres://localhost:5432/cedros_pay
```

**Database doesn't exist:**
```bash
# For PostgreSQL
createdb cedros_pay

# For MongoDB (created automatically on first insert)
```

**Permission denied:**
```bash
# Make script executable
chmod +x scripts/populate-demo-data.sh
```

### Cleaning Up

**Drop MongoDB collections:**
```bash
mongosh mongodb://localhost:27017/cedros_pay --eval "db.products.drop(); db.coupons.drop();"
```

**Drop PostgreSQL tables:**
```bash
psql postgres://localhost:5432/cedros_pay -c "DROP TABLE IF EXISTS products, coupons CASCADE;"
```

---

## Future Scripts

Placeholder for additional scripts:
- `test.sh` - Run full test suite
- `lint.sh` - Run linters and code quality checks
- `build.sh` - Build for multiple platforms
- `deploy.sh` - Deploy to production environments
