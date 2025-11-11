# Repository Guidelines

## Project Structure & Module Organization
Cedros Pay server code lives under `cmd/` and `internal/`. Place executable entrypoints in `cmd/server/` (the HTTP service) and keep reusable packages in `internal/`, e.g. `internal/stripe`, `internal/x402`, `internal/paywall`, and `internal/config`. Shared utilities that could be consumed by other repos belong in `pkg/`. Use `testdata/` for fixture payloads and `deploy/` for Docker or infra manifests. Keep documentation with the feature in `docs/` or extend `plan-server.md` when the architecture changes.

## Build, Test, and Development Commands
Run `go mod tidy` after adding dependencies. Build the service with `go build ./cmd/server`. Start a local instance using `go run ./cmd/server --config configs/dev.yaml`. Execute the full test suite with `go test ./...`; append `-run TestStripe` or `-run TestX402` for focused suites. `go vet ./...` and `staticcheck ./...` should be clean before opening a PR.

## Coding Style & Naming Conventions
Follow standard Go formatting (`gofmt` or `go fmt ./...`); do not hand-format. Use tabs for indentation and keep lines under 100 characters. Exported symbols should use PascalCase, internal helpers camelCase, and errors named `errXYZ`. Group handlers by domain (`paywallHandler`, `stripeWebhookHandler`) and keep file names snake_case (`paywall_handler.go`). Document non-obvious behaviour with GoDoc comments.

## Testing Guidelines
Write table-driven unit tests with `_test.go` suffix beside the implementation. Mock external APIs through interfaces and provide golden responses in `testdata/`. Integration tests that hit Stripe or Solana RPC must be guarded by `testing.Short()` and credentials pulled from env vars (`STRIPE_SECRET_KEY`, `SOLANA_RPC_URL`). Target ≥80% coverage for new packages and include regression tests for every bug fix. Capture expected agent flows in scenario tests under `internal/agents/`.

## Commit & Pull Request Guidelines
Use Conventional Commits (`feat:`, `fix:`, `chore:`) and keep subject lines under 72 characters. Each commit should be scoped to one logical change and include any synced config or docs. PRs need: a concise summary, linked issue or RFC, verification notes (`go test ./...` output), and screenshots or curl transcripts for API-affecting changes. Update `README.md` or `plan-server.md` when behaviour or architecture shifts.

## Security & Configuration Tips
Never commit secrets; store them in `.env.local` and update `configs/example.env` instead. Rotate Stripe keys and Solana private keys regularly. Validate incoming webhooks with Stripe signatures and enforce HTTPS when deploying. Keep dependencies current and run `go list -m -u all` periodically to surface upgrades.
