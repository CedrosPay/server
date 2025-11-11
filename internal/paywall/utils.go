package paywall

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/gagliardetto/solana-go"
)

// InterpolateMemo replaces template placeholders with actual values.
func InterpolateMemo(template, resourceID string) string {
	if template == "" {
		template = "{{resource}}:{{nonce}}"
	}
	nonce := generateNonce()
	replacer := strings.NewReplacer(
		"{{resource}}", resourceID,
		"{{nonce}}", nonce,
	)
	return replacer.Replace(template)
}

// generateNonce creates a random base64-encoded string for unique memos.
func generateNonce() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "nonce"
	}
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

// cloneMap creates a shallow copy of a string map.
func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// mergeMetadata combines multiple metadata maps, with later maps overwriting earlier ones.
func mergeMetadata(maps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}

// deriveTokenAccountSafe derives an associated token account address.
// Returns empty string on any error to avoid panics.
func deriveTokenAccountSafe(owner, mint string) string {
	if owner == "" || mint == "" {
		return ""
	}
	ownerKey, err := solana.PublicKeyFromBase58(owner)
	if err != nil {
		return ""
	}
	mintKey, err := solana.PublicKeyFromBase58(mint)
	if err != nil {
		return ""
	}
	account, _, err := solana.FindAssociatedTokenAddress(ownerKey, mintKey)
	if err != nil {
		return ""
	}
	return account.String()
}

// hashResourceID creates a SHA256 hash of a resource ID for safe logging.
// This prevents resource ID leakage in server logs while maintaining debuggability.
func hashResourceID(resourceID string) string {
	if resourceID == "" {
		return "empty"
	}
	hash := sha256.Sum256([]byte(resourceID))
	return hex.EncodeToString(hash[:])[:16] // First 16 chars (64 bits)
}

// formatCouponCodes joins coupon codes with commas.
// Returns empty string if no codes provided. Safe to call with nil or empty slice.
func formatCouponCodes(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	return strings.Join(codes, ",")
}
