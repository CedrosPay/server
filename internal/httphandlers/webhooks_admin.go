package httphandlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/internal/storage"
	"github.com/go-chi/chi/v5"
)

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Log error but can't change response at this point
		return
	}
}

// WebhooksAdminHandler handles webhook queue management endpoints.
type WebhooksAdminHandler struct {
	store storage.Store
}

// NewWebhooksAdminHandler creates a new webhooks admin handler.
func NewWebhooksAdminHandler(store storage.Store) *WebhooksAdminHandler {
	return &WebhooksAdminHandler{
		store: store,
	}
}

// ListWebhooks returns a list of webhooks with optional status filter.
// GET /admin/webhooks?status=pending&limit=100
func (h *WebhooksAdminHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	statusStr := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")

	var status storage.WebhookStatus
	if statusStr != "" {
		status = storage.WebhookStatus(statusStr)
		// Validate status
		if status != storage.WebhookStatusPending &&
			status != storage.WebhookStatusProcessing &&
			status != storage.WebhookStatusFailed &&
			status != storage.WebhookStatusSuccess {
			errors.WriteSimpleError(w, errors.ErrCodeInvalidSignature, "Invalid status parameter. Must be: pending, processing, failed, or success")
			return
		}
	}

	limit := 100 // Default limit
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit < 1 || parsedLimit > 1000 {
			errors.WriteSimpleError(w, errors.ErrCodeInvalidSignature, "Invalid limit parameter. Must be between 1 and 1000")
			return
		}
		limit = parsedLimit
	}

	// Fetch webhooks
	webhooks, err := h.store.ListWebhooks(ctx, status, limit)
	if err != nil {
		errors.WriteErrorWithDetail(w, errors.ErrCodeDatabaseError, "Failed to list webhooks", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": webhooks,
		"count":    len(webhooks),
	})
}

// GetWebhook retrieves a specific webhook by ID.
// GET /admin/webhooks/:id
func (h *WebhooksAdminHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	webhookID := chi.URLParam(r, "id")

	if webhookID == "" {
		errors.WriteSimpleError(w, errors.ErrCodeInvalidSignature, "Webhook ID is required")
		return
	}

	webhook, err := h.store.GetWebhook(ctx, webhookID)
	if err != nil {
		if err == storage.ErrNotFound {
			errors.WriteSimpleError(w, errors.ErrCodeResourceNotFound, "Webhook not found")
			return
		}

		errors.WriteErrorWithDetail(w, errors.ErrCodeDatabaseError, "Failed to get webhook", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, webhook)
}

// RetryWebhook resets a webhook to pending state for manual retry.
// POST /admin/webhooks/:id/retry
func (h *WebhooksAdminHandler) RetryWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	webhookID := chi.URLParam(r, "id")

	if webhookID == "" {
		errors.WriteSimpleError(w, errors.ErrCodeInvalidSignature, "Webhook ID is required")
		return
	}

	if err := h.store.RetryWebhook(ctx, webhookID); err != nil {
		if err == storage.ErrNotFound {
			errors.WriteSimpleError(w, errors.ErrCodeResourceNotFound, "Webhook not found")
			return
		}

		errors.WriteErrorWithDetail(w, errors.ErrCodeDatabaseError, "Failed to retry webhook", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Webhook queued for retry",
		"webhookId": webhookID,
	})
}

// DeleteWebhook removes a webhook from the queue.
// DELETE /admin/webhooks/:id
func (h *WebhooksAdminHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	webhookID := chi.URLParam(r, "id")

	if webhookID == "" {
		errors.WriteSimpleError(w, errors.ErrCodeInvalidSignature, "Webhook ID is required")
		return
	}

	if err := h.store.DeleteWebhook(ctx, webhookID); err != nil {
		if err == storage.ErrNotFound {
			errors.WriteSimpleError(w, errors.ErrCodeResourceNotFound, "Webhook not found")
			return
		}

		errors.WriteErrorWithDetail(w, errors.ErrCodeDatabaseError, "Failed to delete webhook", "error", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
