package solana

import (
	"container/list"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog/log"

	"github.com/CedrosPay/server/internal/logger"
	"github.com/CedrosPay/server/pkg/x402"
)

const (
	// QueuePollInterval is how frequently the worker checks for new transactions when queue is empty.
	QueuePollInterval = 50 * time.Millisecond

	// TxTimeout is the timeout for sending and confirming individual transactions.
	TxTimeout = 30 * time.Second

	// TxConfirmTimeout is the timeout for waiting for transaction confirmation.
	TxConfirmTimeout = 60 * time.Second

	// MaxTxRetries is the maximum number of times to retry a rate-limited transaction.
	MaxTxRetries = 3
)

// TransactionQueue is a simple queue that sends transactions with rate limiting.
// Rate-limited transactions go back to the TOP of the queue.
type TransactionQueue struct {
	queue          *list.List
	mu             sync.Mutex
	minTimeBetween time.Duration
	maxInFlight    int
	inFlight       int
	lastSendTime   time.Time
	rpcClient      *rpc.Client
	verifier       *SolanaVerifier
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

type queuedTx struct {
	id          string
	transaction *solana.Transaction
	opts        rpc.TransactionOpts
	requirement x402.Requirement
	retries     int
	priority    bool // true = rate limited, goes to front
}

// NewTransactionQueue creates the queue.
func NewTransactionQueue(rpcClient *rpc.Client, verifier *SolanaVerifier, minTimeBetween time.Duration, maxInFlight int) *TransactionQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransactionQueue{
		queue:          list.New(),
		minTimeBetween: minTimeBetween,
		maxInFlight:    maxInFlight,
		rpcClient:      rpcClient,
		verifier:       verifier,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins processing the queue.
func (q *TransactionQueue) Start() {
	q.wg.Add(1)
	go q.worker()
	log.Info().
		Dur("min_time_between", q.minTimeBetween).
		Int("max_in_flight", q.maxInFlight).
		Msg("transaction_queue.started")
}

// Enqueue adds a transaction to the queue.
func (q *TransactionQueue) Enqueue(id string, tx *solana.Transaction, opts rpc.TransactionOpts, req x402.Requirement) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.queue.PushBack(&queuedTx{
		id:          id,
		transaction: tx,
		opts:        opts,
		requirement: req,
		retries:     0,
		priority:    false,
	})
}

// EnqueuePriority adds a rate-limited transaction to the FRONT of the queue.
func (q *TransactionQueue) EnqueuePriority(qtx *queuedTx) {
	q.mu.Lock()
	defer q.mu.Unlock()

	qtx.priority = true
	q.queue.PushFront(qtx) // TOP of queue
}

// worker processes the queue.
func (q *TransactionQueue) worker() {
	defer q.wg.Done()

	ticker := time.NewTicker(QueuePollInterval)
	defer ticker.Stop()

	for {
		// Get next transaction
		qtx := q.dequeue()
		if qtx == nil {
			// Queue empty - wait for poll interval or context cancellation
			select {
			case <-q.ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}

		// Wait for rate limiting
		q.waitForRateLimit()

		// Mark as in-flight
		q.mu.Lock()
		q.inFlight++
		q.lastSendTime = time.Now()
		q.mu.Unlock()

		// Send transaction
		go q.process(qtx)

		// Check if context is cancelled
		select {
		case <-q.ctx.Done():
			return
		default:
		}
	}
}

// dequeue gets the next transaction, respecting max in-flight.
func (q *TransactionQueue) dequeue() *queuedTx {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check max in-flight
	if q.maxInFlight > 0 && q.inFlight >= q.maxInFlight {
		return nil
	}

	// Get from queue
	if q.queue.Len() == 0 {
		return nil
	}

	elem := q.queue.Front()
	q.queue.Remove(elem)
	return elem.Value.(*queuedTx)
}

// waitForRateLimit enforces minimum time between sends with context-aware timing.
func (q *TransactionQueue) waitForRateLimit() {
	if q.minTimeBetween == 0 {
		return
	}

	q.mu.Lock()
	timeSince := time.Since(q.lastSendTime)
	q.mu.Unlock()

	if timeSince < q.minTimeBetween {
		waitDuration := q.minTimeBetween - timeSince
		timer := time.NewTimer(waitDuration)
		defer timer.Stop()

		select {
		case <-q.ctx.Done():
			return
		case <-timer.C:
			// Rate limit satisfied
		}
	}
}

// process sends the transaction and handles result.
func (q *TransactionQueue) process(qtx *queuedTx) {
	defer func() {
		q.mu.Lock()
		q.inFlight--
		q.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(q.ctx, TxTimeout)
	defer cancel()

	// Send transaction
	sig, err := q.rpcClient.SendTransactionWithOpts(ctx, qtx.transaction, qtx.opts)

	if err != nil {
		// Check if rate limited
		if isRateLimitError(err) && qtx.retries < MaxTxRetries {
			qtx.retries++
			backoff := 500 * time.Millisecond * time.Duration(1<<uint(qtx.retries-1))

			log.Warn().
				Str("tx_id", qtx.id).
				Int("retry", qtx.retries).
				Int("max_retries", MaxTxRetries).
				Dur("backoff", backoff).
				Msg("transaction_queue.rate_limited")

			// Use context-aware timer instead of blocking sleep
			timer := time.NewTimer(backoff)
			defer timer.Stop()

			select {
			case <-q.ctx.Done():
				// Shutdown during backoff - don't retry
				return
			case <-timer.C:
				// Backoff complete - retry
			}

			// Put back at TOP of queue
			q.EnqueuePriority(qtx)
			return
		}

		log.Error().
			Err(err).
			Str("tx_id", qtx.id).
			Msg("transaction_queue.send_failed")
		return
	}

	// Wait for confirmation
	log.Debug().
		Str("tx_id", qtx.id).
		Str("signature", logger.TruncateAddress(sig.String())).
		Msg("transaction_queue.sent")

	// Wait for confirmation based on requirement
	// IMPORTANT: Use q.ctx (not ctx) to avoid timeout chain bug
	// ctx already has TxTimeout (30s), so using it as parent would cap confirmation at 30s
	confirmCtx, confirmCancel := context.WithTimeout(q.ctx, TxConfirmTimeout)
	defer confirmCancel()

	commitment := rpc.CommitmentConfirmed
	if qtx.opts.MaxRetries != nil && *qtx.opts.MaxRetries > 0 {
		// Use finalized for retries
		commitment = rpc.CommitmentFinalized
	}

	err = q.verifier.awaitConfirmation(confirmCtx, sig, commitment)
	if err != nil {
		log.Error().
			Err(err).
			Str("tx_id", qtx.id).
			Str("signature", logger.TruncateAddress(sig.String())).
			Msg("transaction_queue.confirmation_failed")
		return
	}

	log.Info().
		Str("tx_id", qtx.id).
		Str("signature", logger.TruncateAddress(sig.String())).
		Msg("transaction_queue.confirmed")
}

// isRateLimitError checks if error is a rate limit.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "throttle")
}

// Shutdown stops the queue.
func (q *TransactionQueue) Shutdown() {
	log.Info().Msg("transaction_queue.shutting_down")
	q.cancel()
	q.wg.Wait()
	log.Info().Msg("transaction_queue.shutdown_complete")
}

// Stats returns queue stats.
func (q *TransactionQueue) Stats() map[string]int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return map[string]int{
		"queued":    q.queue.Len(),
		"in_flight": q.inFlight,
	}
}
