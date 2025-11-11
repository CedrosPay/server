# Cedros Pay Server Configuration

1. Copy `configs/config.example.yaml` to `configs/local.yaml` and update it with your keys, wallet, and paywall definitions.
2. Start the server without flags – it automatically loads `configs/local.yaml` when present.
3. Override any setting at runtime using environment variables such as `STRIPE_SECRET_KEY`, `SOLANA_RPC_URL`, or `CALLBACK_PAYMENT_SUCCESS_URL`.

Key sections:
- `stripe`: API keys plus checkout redirect URLs. The defaults land on backend-hosted success/cancel pages (`/stripe/success`, `/stripe/cancel`) so Storybook or other static shells can finish the flow without their own routing. The webhook endpoint (`/webhook/stripe`) now serves a helpful GET page for local setup while still accepting POSTs from Stripe. **These pages are for local testing only**—override all three URLs with your own app routes and HTTPS webhook endpoint before shipping. Pair it with the Stripe CLI (`stripe listen --forward-to localhost:8080/webhook/stripe`) to forward events to your machine. For the sandbox, keep `secret_key`/`publishable_key` in their `sk_test_`/`pk_test_` forms; swap to the live pair only when deploying. Optional fields like `tax_rate_id` apply a Stripe Tax Rate when you generate ad-hoc prices (leave blank when supplying a `stripe_price_id` that already encapsulates tax).
- `server.cors_allowed_origins`: Add the origins (e.g. Storybook at `http://localhost:6006`) that should be allowed to call the Cedros server during development. Leave the list empty in production and terminate CORS at your own proxy/app unless you explicitly need cross-origin access.
- `x402`: Solana wallet details. Populate `payment_address`, `token_mint`, and point `rpc_url`/`ws_url` at your provider before deploying. Adjust `skip_preflight` and `commitment` if you need different Solana confirmation semantics.
- `paywall`: Resource catalogue used by both Stripe and x402 flows. Put shared metadata (e.g. `package_id`, `credits`) under `metadata`; it is merged into Stripe sessions and the callback payload. Per-request metadata such as `user_id` is accepted from the frontend and merged automatically when you call `/paywall/v1/stripe-session`.
- `state`: The server keeps paywall state in-memory; embed Cedros and call `cedros.WithStore` if you need a persistent backend.
- `callbacks`: Optional webhook you control; the server POSTs payment details so you can update your own database.
  Set `CALLBACK_PAYMENT_SUCCESS_URL` to override the URL at runtime and `CALLBACK_HEADER_<NAME>` to append custom headers. Leave `body` empty to receive the full JSON event, or supply `body_template` (Go `text/template`) to render custom payloads—handy for Discord’s `content` field or Slack blocks.
