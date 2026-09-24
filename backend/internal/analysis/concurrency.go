package analysis

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ModelQueueStats represents runtime statistics for a specific model queue.
type ModelQueueStats struct {
	ModelKey       string    `json:"model_key"`
	ActiveRequests int       `json:"active_requests"`
	QueuedRequests int       `json:"queued_requests"`
	MaxConcurrency int       `json:"max_concurrency"`
	TotalProcessed int64     `json:"total_processed"`
	LastActiveAt   time.Time `json:"last_active_at"`
	NextRequestAt  time.Time `json:"next_request_at,omitempty"`
	CooldownUntil  time.Time `json:"cooldown_until,omitempty"`
	MinIntervalMS  int       `json:"min_interval_ms"`
}

type modelQueue struct {
	mu             sync.Mutex
	modelKey       string
	active         int
	maxConcurrency int
	waitQueue      []*modelQueueWaiter
	totalProcessed int64
	lastActiveAt   time.Time
	minInterval    time.Duration
	rateBackoff    time.Duration
	nextRequestAt  time.Time
	cooldownUntil  time.Time
}

type modelQueueWaiter struct {
	ready   chan struct{}
	granted bool
}

func newModelQueue(key string, maxConcurrency int) *modelQueue {
	if maxConcurrency <= 0 {
		maxConcurrency = 2
	}
	return &modelQueue{
		modelKey:       key,
		maxConcurrency: maxConcurrency,
		waitQueue:      make([]*modelQueueWaiter, 0),
		lastActiveAt:   time.Now(),
	}
}

func (mq *modelQueue) setMaxConcurrency(n int) {
	if n <= 0 {
		n = 2
	}
	mq.mu.Lock()
	defer mq.mu.Unlock()
	mq.maxConcurrency = n
	// If capacity increased, wake up available waiters
	for mq.active < mq.maxConcurrency && len(mq.waitQueue) > 0 {
		mq.active++
		next := mq.waitQueue[0]
		mq.waitQueue = mq.waitQueue[1:]
		next.granted = true
		close(next.ready)
	}
}

func (mq *modelQueue) setTiming(minInterval, rateBackoff time.Duration) {
	if minInterval < 0 {
		minInterval = 0
	}
	if rateBackoff < 0 {
		rateBackoff = 0
	}
	mq.mu.Lock()
	mq.minInterval = minInterval
	mq.rateBackoff = rateBackoff
	mq.mu.Unlock()
}

// waitForRequestTurn reserves the next send slot for this model. The wait is
// outside the mutex and is always context-aware, so cancelled callers do not
// retain active queue slots.
func (mq *modelQueue) waitForRequestTurn(ctx context.Context) error {
	for {
		mq.mu.Lock()
		now := time.Now()
		waitUntil := mq.nextRequestAt
		if mq.cooldownUntil.After(waitUntil) {
			waitUntil = mq.cooldownUntil
		}
		if !waitUntil.After(now) {
			if mq.minInterval > 0 {
				mq.nextRequestAt = now.Add(mq.minInterval)
			} else {
				mq.nextRequestAt = now
			}
			mq.mu.Unlock()
			return nil
		}
		wait := time.Until(waitUntil)
		mq.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		}
	}
}

func (mq *modelQueue) notifyRateLimit(cooldown time.Duration) time.Duration {
	if cooldown <= 0 {
		cooldown = mq.rateBackoff
	}
	if cooldown <= 0 {
		return 0
	}
	mq.mu.Lock()
	defer mq.mu.Unlock()
	until := time.Now().Add(cooldown)
	if until.After(mq.cooldownUntil) {
		mq.cooldownUntil = until
	}
	if mq.cooldownUntil.After(mq.nextRequestAt) {
		mq.nextRequestAt = mq.cooldownUntil
	}
	return time.Until(mq.cooldownUntil)
}

func (mq *modelQueue) acquire(ctx context.Context, timeout time.Duration) (func(), error) {
	mq.mu.Lock()
	if mq.active < mq.maxConcurrency && len(mq.waitQueue) == 0 {
		mq.active++
		mq.lastActiveAt = time.Now()
		mq.mu.Unlock()
		return mq.makeRelease(), nil
	}

	waiter := &modelQueueWaiter{ready: make(chan struct{})}
	mq.waitQueue = append(mq.waitQueue, waiter)
	mq.mu.Unlock()

	var cancelTimer <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		cancelTimer = timer.C
	}

	select {
	case <-waiter.ready:
		mq.mu.Lock()
		mq.lastActiveAt = time.Now()
		mq.mu.Unlock()
		return mq.makeRelease(), nil
	case <-ctx.Done():
		if mq.cancelWaiter(waiter) {
			mq.releaseSlot(false)
		}
		return nil, ctx.Err()
	case <-cancelTimer:
		if mq.cancelWaiter(waiter) {
			mq.releaseSlot(false)
		}
		return nil, fmt.Errorf("llm model queue timeout (%v exceeded for model %s)", timeout, mq.modelKey)
	}
}

// cancelWaiter returns true when a slot was concurrently granted and must be
// returned by the caller. This closes the timeout/handoff race that could
// otherwise leak one active slot permanently.
func (mq *modelQueue) cancelWaiter(waiter *modelQueueWaiter) bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	for i, w := range mq.waitQueue {
		if w == waiter {
			mq.waitQueue = append(mq.waitQueue[:i], mq.waitQueue[i+1:]...)
			return false
		}
	}
	return waiter.granted
}

func (mq *modelQueue) makeRelease() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			mq.releaseSlot(true)
		})
	}
}

func (mq *modelQueue) releaseSlot(processed bool) {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if processed {
		mq.totalProcessed++
	}
	if len(mq.waitQueue) > 0 {
		next := mq.waitQueue[0]
		mq.waitQueue = mq.waitQueue[1:]
		next.granted = true
		close(next.ready)
		return
	}
	if mq.active > 0 {
		mq.active--
	}
}

func (mq *modelQueue) stats() ModelQueueStats {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return ModelQueueStats{
		ModelKey:       mq.modelKey,
		ActiveRequests: mq.active,
		QueuedRequests: len(mq.waitQueue),
		MaxConcurrency: mq.maxConcurrency,
		TotalProcessed: mq.totalProcessed,
		LastActiveAt:   mq.lastActiveAt,
		NextRequestAt:  mq.nextRequestAt,
		CooldownUntil:  mq.cooldownUntil,
		MinIntervalMS:  int(mq.minInterval / time.Millisecond),
	}
}

// ModelQueueManager manages per-model concurrency queues globally.
type ModelQueueManager struct {
	mu           sync.RWMutex
	queues       map[string]*modelQueue
	defaultLimit int
	queueTimeout time.Duration
}

var globalQueueManager = &ModelQueueManager{
	queues:       make(map[string]*modelQueue),
	defaultLimit: 2,
	queueTimeout: 180 * time.Second,
}

// GetGlobalQueueManager returns the singleton ModelQueueManager.
func GetGlobalQueueManager() *ModelQueueManager {
	return globalQueueManager
}

// SetGlobalMaxConcurrency dynamically updates the max concurrency for all and default queues.
func (m *ModelQueueManager) SetGlobalMaxConcurrency(limit int) {
	if limit <= 0 {
		limit = 2
	}
	m.mu.Lock()
	m.defaultLimit = limit
	for _, q := range m.queues {
		q.setMaxConcurrency(limit)
	}
	m.mu.Unlock()
}

// SetModelMaxConcurrency sets the max concurrency for a specific model key.
func (m *ModelQueueManager) SetModelMaxConcurrency(modelKey string, limit int) {
	m.getOrCreateQueue(modelKey).setMaxConcurrency(limit)
}

// ConfigureModel updates pacing without changing the selected model or its
// concurrency. Values come from system_configs and may be changed at runtime.
func (m *ModelQueueManager) ConfigureModel(modelKey string, minInterval, rateBackoff time.Duration) {
	m.getOrCreateQueue(modelKey).setTiming(minInterval, rateBackoff)
}

func (m *ModelQueueManager) WaitForRequest(ctx context.Context, modelKey string) error {
	return m.getOrCreateQueue(modelKey).waitForRequestTurn(ctx)
}

func (m *ModelQueueManager) NotifyRateLimit(modelKey string, cooldown time.Duration) time.Duration {
	return m.getOrCreateQueue(modelKey).notifyRateLimit(cooldown)
}

func (m *ModelQueueManager) getOrCreateQueue(modelKey string) *modelQueue {
	if modelKey == "" {
		modelKey = "default"
	}
	m.mu.RLock()
	q, ok := m.queues[modelKey]
	limit := m.defaultLimit
	m.mu.RUnlock()

	if ok {
		return q
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if q, ok := m.queues[modelKey]; ok {
		return q
	}
	q = newModelQueue(modelKey, limit)
	m.queues[modelKey] = q
	return q
}

// Acquire requests a concurrency slot for the given model with FIFO queuing.
func (m *ModelQueueManager) Acquire(ctx context.Context, modelKey string) (func(), error) {
	return m.AcquireWithTimeout(ctx, modelKey, 0)
}

func (m *ModelQueueManager) AcquireWithTimeout(ctx context.Context, modelKey string, timeout time.Duration) (func(), error) {
	q := m.getOrCreateQueue(modelKey)
	if timeout <= 0 {
		timeout = m.queueTimeout
	}
	return q.acquire(ctx, timeout)
}

// Stats returns the real-time queue metrics across all active models.
func (m *ModelQueueManager) Stats() []ModelQueueStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := make([]ModelQueueStats, 0, len(m.queues))
	for _, q := range m.queues {
		stats = append(stats, q.stats())
	}
	return stats
}
