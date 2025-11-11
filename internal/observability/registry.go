package observability

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

// Registry manages a collection of observability hooks.
// It safely dispatches events to all registered hooks with error handling.
type Registry struct {
	paymentHooks  []PaymentHook
	webhookHooks  []WebhookHook
	refundHooks   []RefundHook
	cartHooks     []CartHook
	rpcHooks      []RPCHook
	databaseHooks []DatabaseHook
	logger        zerolog.Logger
	mu            sync.RWMutex
}

// NewRegistry creates a new hook registry.
func NewRegistry(logger zerolog.Logger) *Registry {
	return &Registry{
		logger: logger,
	}
}

// RegisterPaymentHook adds a payment hook to the registry.
func (r *Registry) RegisterPaymentHook(hook PaymentHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paymentHooks = append(r.paymentHooks, hook)
	r.logger.Info().Str("hook", hook.Name()).Msg("registered payment hook")
}

// RegisterWebhookHook adds a webhook hook to the registry.
func (r *Registry) RegisterWebhookHook(hook WebhookHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.webhookHooks = append(r.webhookHooks, hook)
	r.logger.Info().Str("hook", hook.Name()).Msg("registered webhook hook")
}

// RegisterRefundHook adds a refund hook to the registry.
func (r *Registry) RegisterRefundHook(hook RefundHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refundHooks = append(r.refundHooks, hook)
	r.logger.Info().Str("hook", hook.Name()).Msg("registered refund hook")
}

// RegisterCartHook adds a cart hook to the registry.
func (r *Registry) RegisterCartHook(hook CartHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cartHooks = append(r.cartHooks, hook)
	r.logger.Info().Str("hook", hook.Name()).Msg("registered cart hook")
}

// RegisterRPCHook adds an RPC hook to the registry.
func (r *Registry) RegisterRPCHook(hook RPCHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rpcHooks = append(r.rpcHooks, hook)
	r.logger.Info().Str("hook", hook.Name()).Msg("registered RPC hook")
}

// RegisterDatabaseHook adds a database hook to the registry.
func (r *Registry) RegisterDatabaseHook(hook DatabaseHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.databaseHooks = append(r.databaseHooks, hook)
	r.logger.Info().Str("hook", hook.Name()).Msg("registered database hook")
}

// ===============================================
// Payment Hook Dispatchers
// ===============================================

// EmitPaymentStarted dispatches the event to all payment hooks.
func (r *Registry) EmitPaymentStarted(ctx context.Context, event PaymentStartedEvent) {
	r.mu.RLock()
	hooks := r.paymentHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnPaymentStarted", hook.Name())
			hook.OnPaymentStarted(ctx, event)
		}()
	}
}

// EmitPaymentCompleted dispatches the event to all payment hooks.
func (r *Registry) EmitPaymentCompleted(ctx context.Context, event PaymentCompletedEvent) {
	r.mu.RLock()
	hooks := r.paymentHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnPaymentCompleted", hook.Name())
			hook.OnPaymentCompleted(ctx, event)
		}()
	}
}

// EmitPaymentSettled dispatches the event to all payment hooks.
func (r *Registry) EmitPaymentSettled(ctx context.Context, event PaymentSettledEvent) {
	r.mu.RLock()
	hooks := r.paymentHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnPaymentSettled", hook.Name())
			hook.OnPaymentSettled(ctx, event)
		}()
	}
}

// ===============================================
// Webhook Hook Dispatchers
// ===============================================

// EmitWebhookQueued dispatches the event to all webhook hooks.
func (r *Registry) EmitWebhookQueued(ctx context.Context, event WebhookQueuedEvent) {
	r.mu.RLock()
	hooks := r.webhookHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnWebhookQueued", hook.Name())
			hook.OnWebhookQueued(ctx, event)
		}()
	}
}

// EmitWebhookDelivered dispatches the event to all webhook hooks.
func (r *Registry) EmitWebhookDelivered(ctx context.Context, event WebhookDeliveredEvent) {
	r.mu.RLock()
	hooks := r.webhookHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnWebhookDelivered", hook.Name())
			hook.OnWebhookDelivered(ctx, event)
		}()
	}
}

// EmitWebhookFailed dispatches the event to all webhook hooks.
func (r *Registry) EmitWebhookFailed(ctx context.Context, event WebhookFailedEvent) {
	r.mu.RLock()
	hooks := r.webhookHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnWebhookFailed", hook.Name())
			hook.OnWebhookFailed(ctx, event)
		}()
	}
}

// EmitWebhookRetried dispatches the event to all webhook hooks.
func (r *Registry) EmitWebhookRetried(ctx context.Context, event WebhookRetriedEvent) {
	r.mu.RLock()
	hooks := r.webhookHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnWebhookRetried", hook.Name())
			hook.OnWebhookRetried(ctx, event)
		}()
	}
}

// ===============================================
// Refund Hook Dispatchers
// ===============================================

// EmitRefundRequested dispatches the event to all refund hooks.
func (r *Registry) EmitRefundRequested(ctx context.Context, event RefundRequestedEvent) {
	r.mu.RLock()
	hooks := r.refundHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnRefundRequested", hook.Name())
			hook.OnRefundRequested(ctx, event)
		}()
	}
}

// EmitRefundProcessed dispatches the event to all refund hooks.
func (r *Registry) EmitRefundProcessed(ctx context.Context, event RefundProcessedEvent) {
	r.mu.RLock()
	hooks := r.refundHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnRefundProcessed", hook.Name())
			hook.OnRefundProcessed(ctx, event)
		}()
	}
}

// ===============================================
// Cart Hook Dispatchers
// ===============================================

// EmitCartCreated dispatches the event to all cart hooks.
func (r *Registry) EmitCartCreated(ctx context.Context, event CartCreatedEvent) {
	r.mu.RLock()
	hooks := r.cartHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnCartCreated", hook.Name())
			hook.OnCartCreated(ctx, event)
		}()
	}
}

// EmitCartCheckout dispatches the event to all cart hooks.
func (r *Registry) EmitCartCheckout(ctx context.Context, event CartCheckoutEvent) {
	r.mu.RLock()
	hooks := r.cartHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnCartCheckout", hook.Name())
			hook.OnCartCheckout(ctx, event)
		}()
	}
}

// ===============================================
// RPC Hook Dispatchers
// ===============================================

// EmitRPCCall dispatches the event to all RPC hooks.
func (r *Registry) EmitRPCCall(ctx context.Context, event RPCCallEvent) {
	r.mu.RLock()
	hooks := r.rpcHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnRPCCall", hook.Name())
			hook.OnRPCCall(ctx, event)
		}()
	}
}

// ===============================================
// Database Hook Dispatchers
// ===============================================

// EmitDatabaseQuery dispatches the event to all database hooks.
func (r *Registry) EmitDatabaseQuery(ctx context.Context, event DatabaseQueryEvent) {
	r.mu.RLock()
	hooks := r.databaseHooks
	r.mu.RUnlock()

	for _, hook := range hooks {
		func() {
			defer r.recoverPanic("OnDatabaseQuery", hook.Name())
			hook.OnDatabaseQuery(ctx, event)
		}()
	}
}

// ===============================================
// Error Recovery
// ===============================================

// recoverPanic recovers from panics in hook implementations.
// This ensures one bad hook doesn't crash the entire system.
func (r *Registry) recoverPanic(method, hookName string) {
	if err := recover(); err != nil {
		r.logger.Error().
			Str("hook", hookName).
			Str("method", method).
			Interface("panic", err).
			Msg("observability hook panicked (recovered)")
	}
}
