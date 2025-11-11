# API Versioning

This package implements comprehensive API versioning for Cedros Pay Server using **content negotiation** while keeping URL routes stable.

## Design Philosophy

- **URLs stay constant** - No `/v2/` prefixes in routes
- **Header-based negotiation** - Clients specify version via headers
- **Graceful deprecation** - Old versions receive warning headers with sunset dates
- **Default to latest stable** - New clients automatically use the current version

## How to Specify API Version (Client Side)

Clients can request a specific API version using any of these methods (in priority order):

### Method 1: X-API-Version Header (Recommended)

```bash
curl -H "X-API-Version: v2" https://api.cedrospay.com/paywall/v1/quote
```

### Method 2: Vendor-Specific Media Type

```bash
curl -H "Accept: application/vnd.cedros.v2+json" https://api.cedrospay.com/paywall/v1/quote
```

### Method 3: Version Parameter in Accept Header

```bash
curl -H "Accept: application/json; version=2" https://api.cedrospay.com/paywall/v1/quote
```

### Default Behavior

If no version is specified, the server defaults to **v1** (current stable version).

## Response Headers

All API responses include these headers to inform clients about versioning:

- `X-API-Version: v1` - The version used to process the request
- `Vary: Accept, X-API-Version` - Cache control for proxies/CDNs

### Deprecated Version Warnings

When using a deprecated API version, you'll receive additional headers:

```
Deprecation: true
Sunset: 2025-12-31T23:59:59Z
Warning: 299 - "Deprecated API Version: Please upgrade to v2 by Dec 31, 2025"
```

## Server-Side Usage

### Accessing Version in Handlers

```go
func (h *handlers) someHandler(w http.ResponseWriter, r *http.Request) {
    version := versioning.FromContext(r.Context())

    switch version {
    case versioning.V1:
        // Handle v1 response format
        json.NewEncoder(w).Encode(ResponseV1{...})
    case versioning.V2:
        // Handle v2 response format (breaking changes)
        json.NewEncoder(w).Encode(ResponseV2{...})
    }
}
```

### Adding Deprecation Warnings

When you're ready to sunset v1:

```go
// In server.go ConfigureRouter
deprecation := versioning.NewDeprecationWarning(
    versioning.V1,
    "2025-12-31T23:59:59Z",  // Sunset date (RFC 3339)
    "Please upgrade to v2 by Dec 31, 2025. See docs.cedrospay.com/migration",
)
router.Use(deprecation.Middleware)
```

## Version History

### v1 (Current Default)
- Initial stable API
- All existing functionality
- Default when no version specified

### v2 (Reserved for Future)
- Breaking changes TBD
- Migration guide: TBD

## Migration Strategy

When introducing breaking changes:

1. **Announce deprecation** - Add deprecation middleware with 6+ month sunset date
2. **Release v2** - Deploy alongside v1 (both supported)
3. **Monitor adoption** - Track `X-API-Version` header distribution
4. **Provide migration path** - Clear docs showing v1 → v2 changes
5. **Sunset v1** - After sunset date, reject v1 requests or redirect to v2

## Testing

Run version negotiation tests:

```bash
go test ./internal/versioning -v
```

## Standards Compliance

This implementation follows:
- **RFC 8594** - Sunset HTTP Header
- **RFC 7234** - HTTP Caching (Vary header)
- **RFC 7231** - Warning HTTP Header
