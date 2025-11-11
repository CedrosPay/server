package httpserver

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/internal/rpcutil"
	"github.com/CedrosPay/server/pkg/responders"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// rpcProxyHandlers provides cached RPC endpoints for frontend transaction building.
type rpcProxyHandlers struct {
	cfg       *config.Config
	rpcClient *rpc.Client

	// Blockhash cache
	mu              sync.RWMutex
	cachedBlockhash *cachedBlockhash
}

type cachedBlockhash struct {
	blockhash string
	expiresAt time.Time
}

// NewRPCProxyHandlers creates handlers for RPC proxy endpoints.
func NewRPCProxyHandlers(cfg *config.Config) *rpcProxyHandlers {
	return &rpcProxyHandlers{
		cfg:       cfg,
		rpcClient: rpc.New(cfg.X402.RPCURL),
	}
}

// deriveTokenAccountRequest contains parameters for deriving a token account.
type deriveTokenAccountRequest struct {
	Owner string `json:"owner"` // Wallet address
	Mint  string `json:"mint"`  // Token mint address
}

// deriveTokenAccountResponse contains the derived associated token account.
type deriveTokenAccountResponse struct {
	TokenAccount string `json:"tokenAccount"` // Derived ATA address
	Owner        string `json:"owner"`        // Owner address (echoed back)
	Mint         string `json:"mint"`         // Mint address (echoed back)
}

// getCachedBlockhash returns a cached blockhash for use in transaction building.
// Returns the blockhash and whether it's valid. If not valid or not cached, returns empty string and false.
func (h *rpcProxyHandlers) getCachedBlockhash() (solana.Hash, bool) {
	// Cache time.Now() to avoid syscall under lock
	now := time.Now()

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.cachedBlockhash == nil {
		return solana.Hash{}, false
	}

	if now.After(h.cachedBlockhash.expiresAt) {
		return solana.Hash{}, false
	}

	// Parse the cached string back to Hash
	hash, err := solana.HashFromBase58(h.cachedBlockhash.blockhash)
	if err != nil {
		return solana.Hash{}, false
	}

	return hash, true
}

// fetchAndCacheBlockhash fetches a fresh blockhash from RPC and caches it.
func (h *rpcProxyHandlers) fetchAndCacheBlockhash(ctx context.Context) (solana.Hash, error) {
	result, err := rpcutil.WithRetry(ctx, func() (*rpc.GetLatestBlockhashResult, error) {
		return h.rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	})
	if err != nil {
		return solana.Hash{}, err
	}

	blockhash := result.Value.Blockhash
	blockhashStr := blockhash.String()

	// Cache for 1 second
	h.mu.Lock()
	h.cachedBlockhash = &cachedBlockhash{
		blockhash: blockhashStr,
		expiresAt: time.Now().Add(1 * time.Second),
	}
	h.mu.Unlock()

	return blockhash, nil
}

// deriveTokenAccount derives the associated token account for a wallet and mint.
// POST /paywall/v1/derive-token-account
func (h *rpcProxyHandlers) deriveTokenAccount(w http.ResponseWriter, r *http.Request) {
	var req deriveTokenAccountRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		responders.JSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid request format",
		})
		return
	}

	if req.Owner == "" || req.Mint == "" {
		responders.JSON(w, http.StatusBadRequest, map[string]any{
			"error": "owner and mint are required",
		})
		return
	}

	// Parse owner public key
	ownerKey, err := solana.PublicKeyFromBase58(req.Owner)
	if err != nil {
		responders.JSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid owner address",
		})
		return
	}

	// Parse mint public key
	mintKey, err := solana.PublicKeyFromBase58(req.Mint)
	if err != nil {
		responders.JSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid mint address",
		})
		return
	}

	// Derive associated token account
	ata, _, err := solana.FindAssociatedTokenAddress(ownerKey, mintKey)
	if err != nil {
		responders.JSON(w, http.StatusInternalServerError, map[string]any{
			"error": "failed to derive token account",
		})
		return
	}

	responders.JSON(w, http.StatusOK, deriveTokenAccountResponse{
		TokenAccount: ata.String(),
		Owner:        req.Owner,
		Mint:         req.Mint,
	})
}
