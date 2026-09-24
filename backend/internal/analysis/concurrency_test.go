package analysis

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestModelQueueStrictConcurrencyLimit(t *testing.T) {
	mgr := &ModelQueueManager{
		queues:       make(map[string]*modelQueue),
		defaultLimit: 2,
		queueTimeout: 5 * time.Second,
	}

	modelKey := "test-model-concurrency"
	var currentActive int32
	var maxObservedActive int32
	var totalDone int32

	var wg sync.WaitGroup
	numWorkers := 8

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			release, err := mgr.Acquire(context.Background(), modelKey)
			if err != nil {
				t.Errorf("worker %d acquire error: %v", id, err)
				return
			}
			defer release()

			cur := atomic.AddInt32(&currentActive, 1)
			// Track max observed
			for {
				oldMax := atomic.LoadInt32(&maxObservedActive)
				if cur <= oldMax || atomic.CompareAndSwapInt32(&maxObservedActive, oldMax, cur) {
					break
				}
			}

			// Simulate work
			time.Sleep(30 * time.Millisecond)

			atomic.AddInt32(&currentActive, -1)
			atomic.AddInt32(&totalDone, 1)
		}(i)
	}

	wg.Wait()

	if totalDone != int32(numWorkers) {
		t.Fatalf("expected %d completed, got %d", numWorkers, totalDone)
	}

	if maxObservedActive > 2 {
		t.Fatalf("max concurrency exceeded: observed %d, limit was 2", maxObservedActive)
	}

	stats := mgr.getOrCreateQueue(modelKey).stats()
	if stats.TotalProcessed != int64(numWorkers) {
		t.Fatalf("expected %d total processed in stats, got %d", numWorkers, stats.TotalProcessed)
	}
}

func TestModelQueueFIFOOrdering(t *testing.T) {
	mgr := &ModelQueueManager{
		queues:       make(map[string]*modelQueue),
		defaultLimit: 1, // Single worker at a time for strict FIFO test
		queueTimeout: 5 * time.Second,
	}

	modelKey := "test-fifo-model"

	// Hold the first slot
	release1, err := mgr.Acquire(context.Background(), modelKey)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	var order []int
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 1; i <= 4; i++ {
		wg.Add(1)
		orderID := i
		go func() {
			defer wg.Done()
			rel, err := mgr.Acquire(context.Background(), modelKey)
			if err != nil {
				t.Errorf("worker %d acquire error: %v", orderID, err)
				return
			}
			defer rel()

			mu.Lock()
			order = append(order, orderID)
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}()
		time.Sleep(10 * time.Millisecond) // ensure queuing order
	}

	// Release first slot to allow queue to drain
	release1()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 4 {
		t.Fatalf("expected 4 items in order, got %d", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("expected position %d to be %d, got %d (full order: %v)", i, i+1, v, order)
		}
	}
}

func TestModelQueueContextCancellation(t *testing.T) {
	mgr := &ModelQueueManager{
		queues:       make(map[string]*modelQueue),
		defaultLimit: 1,
		queueTimeout: 5 * time.Second,
	}

	modelKey := "test-cancel-model"

	// Hold the only slot
	release, err := mgr.Acquire(context.Background(), modelKey)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after 20ms
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err2 := mgr.Acquire(ctx, modelKey)
	if err2 == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}

	stats := mgr.getOrCreateQueue(modelKey).stats()
	if stats.QueuedRequests != 0 {
		t.Fatalf("expected waiter to be removed after cancel, but queued is %d", stats.QueuedRequests)
	}
}

func TestModelQueuePacesSequentialRequests(t *testing.T) {
	mgr := &ModelQueueManager{
		queues:       make(map[string]*modelQueue),
		defaultLimit: 1,
		queueTimeout: time.Second,
	}
	modelKey := "test-pacing-model"
	mgr.ConfigureModel(modelKey, 35*time.Millisecond, 0)

	release, err := mgr.Acquire(context.Background(), modelKey)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if err := mgr.WaitForRequest(context.Background(), modelKey); err != nil {
		t.Fatalf("first pacing wait failed: %v", err)
	}
	release()

	started := time.Now()
	release, err = mgr.Acquire(context.Background(), modelKey)
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	defer release()
	if err := mgr.WaitForRequest(context.Background(), modelKey); err != nil {
		t.Fatalf("second pacing wait failed: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 25*time.Millisecond {
		t.Fatalf("second request was not paced: waited %v", elapsed)
	}
}

func TestModelQueueRateLimitCooldownIsContextAware(t *testing.T) {
	mgr := &ModelQueueManager{
		queues:       make(map[string]*modelQueue),
		defaultLimit: 1,
		queueTimeout: time.Second,
	}
	modelKey := "test-cooldown-model"
	mgr.ConfigureModel(modelKey, 0, 40*time.Millisecond)
	cooldown := mgr.NotifyRateLimit(modelKey, 0)
	if cooldown <= 0 {
		t.Fatal("expected configured cooldown")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := mgr.WaitForRequest(ctx, modelKey); err == nil {
		t.Fatal("expected pacing wait to honor context cancellation")
	}

	// Cancellation of the pacing wait must not consume the queue slot.
	release, err := mgr.Acquire(context.Background(), modelKey)
	if err != nil {
		t.Fatalf("queue slot leaked after pacing cancellation: %v", err)
	}
	release()
}

func TestModelQueueReturnsConcurrentlyGrantedCancelledSlot(t *testing.T) {
	mq := newModelQueue("handoff-cancel", 1)
	mq.active = 1
	waiter := &modelQueueWaiter{ready: make(chan struct{})}
	mq.waitQueue = append(mq.waitQueue, waiter)

	// The active request hands its slot to the waiter just as the waiter times
	// out. The timeout path must return that already-granted slot.
	mq.releaseSlot(true)
	if !mq.cancelWaiter(waiter) {
		t.Fatal("expected waiter to report a concurrently granted slot")
	}
	mq.releaseSlot(false)

	stats := mq.stats()
	if stats.ActiveRequests != 0 {
		t.Fatalf("active slot leaked after cancelled handoff: %d", stats.ActiveRequests)
	}
	if stats.QueuedRequests != 0 {
		t.Fatalf("waiter remained queued after cancelled handoff: %d", stats.QueuedRequests)
	}
	if stats.TotalProcessed != 1 {
		t.Fatalf("cancelled waiter must not count as processed, got %d", stats.TotalProcessed)
	}
}
