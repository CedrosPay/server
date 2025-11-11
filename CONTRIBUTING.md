# Contributing to Cedros Pay

We welcome contributions! This guide assumes you're a competent developer.

## Quick Start

```bash
git clone https://github.com/cedros-pay/server.git
cd server
go mod download
cp configs/local.example.yaml configs/local.yaml
# Edit configs/local.yaml with your keys
go run ./cmd/server
```

## Development Workflow

1. **Fork & Branch**
   ```bash
   git checkout -b feat/your-feature
   ```

2. **Code**
   - Follow standard Go conventions (`gofmt`, `go vet`)
   - Keep functions under 60 lines
   - Files under 500 lines
   - See [AGENTS.md](./AGENTS.md) for detailed guidelines

3. **Test**
   ```bash
   go test ./...
   go vet ./...
   ```

4. **Commit**
   - Use [Conventional Commits](https://www.conventionalcommits.org/)
   - `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`
   - Keep subjects under 72 chars

5. **PR**
   - Describe what and why
   - Include test output
   - Update docs if needed

## Project Structure

```
cmd/server/          # Main executable
internal/            # Private packages
  ├── config/        # YAML config loading
  ├── httpserver/    # HTTP handlers
  ├── paywall/       # Payment logic
  ├── stripe/        # Stripe integration
  ├── x402/          # Solana x402 verification
  ├── monitoring/    # Wallet balance monitoring
  └── callbacks/     # Webhook notifications
pkg/cedros/          # Public embedding API
configs/             # Example configs
```

## Testing

- **Unit tests:** `go test ./internal/...`
- **Integration tests:** Require env vars, skip with `-short`
- **Coverage target:** ≥80% for new packages

```bash
# Run with coverage
go test -cover ./...

# Run integration tests
STRIPE_SECRET_KEY=sk_test_... SOLANA_RPC_URL=https://... go test ./...
```

## Adding Features

### New Payment Provider

1. Create `internal/providers/yourprovider/`
2. Implement `Verifier` interface
3. Add config struct to `internal/config/config.go`
4. Wire up in `cmd/server/main.go`
5. Add tests in `yourprovider_test.go`
6. Update README

### New Endpoint

1. Add handler to `internal/httpserver/`
2. Register route in `ConfigureRouter()`
3. Add to API docs in README
4. Write handler test

### New Configuration

1. Add field to config struct in `internal/config/config.go`
2. Add default in `defaultConfig()`
3. Add env override in `applyEnvOverrides()`
4. Add validation in `validate()`
5. Update `configs/local.example.yaml`
6. Document in README

## Code Style

- **Formatting:** Run `go fmt ./...`
- **Naming:** `PascalCase` exports, `camelCase` internal
- **Errors:** Wrap with context, return don't panic
- **Comments:** Document exported symbols
- **Imports:** Standard library, then external, then internal

## What We Look For

✅ **Good PRs:**
- Focused on one thing
- Include tests
- Update docs
- Clean commit history
- Pass CI

❌ **Avoid:**
- Reformatting existing code
- Multiple unrelated changes
- Breaking changes without discussion
- Secrets in code or commits
- TODO comments without issues

## Release Process

Maintainers handle releases. Versioning follows [SemVer](https://semver.org/).

## Getting Help

- **Questions:** Open a discussion
- **Bugs:** Open an issue with repro steps
- **Security:** See [SECURITY.md](./SECURITY.md)

## License

By contributing, you agree your code is licensed under [MIT](./LICENSE).
