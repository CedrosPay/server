# Security Policy

## Reporting a Vulnerability

**DO NOT** open public issues for security vulnerabilities.

Instead, email: **conorholds@gmail.com** (or create a private security advisory on GitHub)

Include:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We'll respond within 48 hours and work with you on a fix.

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| < 1.0   | :x:                |

## Security Best Practices

### For Operators

1. **Secrets Management**

   - Never commit `.env` files
   - Rotate Stripe keys and Solana wallets regularly
   - Use environment variables for all sensitive config
   - Enable webhook signature validation

2. **Network Security**

   - Always use HTTPS in production
   - Restrict CORS origins to your domains
   - Use firewall rules for RPC endpoints
   - Enable rate limiting

3. **Wallet Security**

   - Keep server wallet private keys encrypted at rest
   - Use hardware wallets for high-value recipient addresses
   - Monitor wallet balances (use built-in monitoring)
   - Limit server wallet funds (refill as needed)

4. **Stripe Security**

   - Validate webhook signatures
   - Use Stripe test mode during development
   - Never log full payment card numbers
   - Enable Stripe Radar for fraud detection

5. **Solana Security**
   - Use dedicated RPC endpoints (not public)
   - Enable transaction memo validation
   - Set appropriate commitment levels (`finalized` for production)
   - Verify token mints match expected values

### For Contributors

1. **Code Review**

   - Check for injection vulnerabilities (SQL, command, XSS)
   - Validate all user inputs
   - Use parameterized queries
   - Avoid `eval()` or `exec()`

2. **Dependencies**

   - Keep dependencies up to date
   - Run `go list -m -u all` regularly
   - Review dependency changes for security issues
   - Use Go's built-in `go.sum` for integrity

3. **Error Handling**
   - Don't leak sensitive data in error messages
   - Log errors server-side, return generic messages to users
   - Sanitize logs (remove tokens, keys, signatures)

## Known Security Considerations

### Payment Verification

- **Transaction Replay:** Each payment memo includes a nonce to prevent replay attacks
- **Amount Validation:** Server verifies exact payment amounts on-chain
- **Token Mint Validation:** Ensures correct token type is used
- **Recipient Validation:** Confirms payment went to correct account

### Rate Limiting

- Configure `tx_queue_min_time_between` to prevent RPC abuse
- Use `tx_queue_max_in_flight` to limit concurrent requests
- Frontend should implement client-side debouncing

### Webhook Security

- Stripe webhooks use signature validation (always enabled)
- Payment success callbacks should be authenticated
- Use HTTPS for all webhook endpoints
- Verify callback source (IP allowlisting if possible)

## Security Features

✅ **Built-in protections:**

- Webhook signature validation (Stripe)
- Memo-based idempotency (Solana)
- On-chain transaction verification
- User-friendly error messages (no leaks)
- Configurable CORS
- Automatic retry with backoff
- Transaction amount validation

## Disclosure Policy

- We follow responsible disclosure
- Security patches released ASAP
- Credit given to reporters (unless requested otherwise)
- CVE IDs assigned for critical issues

## Audit Status

This project has not undergone a formal security audit. Use at your own risk in production.

Consider:

- Internal security review before production deployment
- Third-party audit for high-value deployments
- Penetration testing
- Bug bounty program

## Contact

Security Team: **security@cedros.dev**

For general questions, use GitHub discussions.
