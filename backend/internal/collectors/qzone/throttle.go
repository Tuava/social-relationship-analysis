package qzone

import (
	"context"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	qzoneRequestMinDelay = 1100 * time.Millisecond
	qzoneRequestMaxDelay = 1800 * time.Millisecond
	qzoneMaxAttempts     = 4
	qzoneBackoffBase     = 2 * time.Second
	qzoneBackoffMax      = 15 * time.Second
)

type contextSleeper func(context.Context, time.Duration) error

type requestLimiter struct {
	minimum time.Duration
	maximum time.Duration
	random  func() float64
	sleep   contextSleeper
}

type accountRequestGate struct {
	token   chan struct{}
	limiter requestLimiter
	random  func() float64
	sleep   contextSleeper
}

var qzoneAccountGates sync.Map

func requestGateForAccount(accountID string) *accountRequestGate {
	created := newAccountRequestGate()
	actual, _ := qzoneAccountGates.LoadOrStore(accountID, created)
	return actual.(*accountRequestGate)
}

var (
	dynamicIntervalSec float64 = 1.5
	dynamicIntervalMu  sync.RWMutex
)

// SetDynamicInterval dynamically updates the crawler base interval in seconds.
func SetDynamicInterval(sec float64) {
	dynamicIntervalMu.Lock()
	defer dynamicIntervalMu.Unlock()
	if sec >= 0.2 && sec <= 10.0 {
		dynamicIntervalSec = sec
	}
}

func getDynamicDelays() (time.Duration, time.Duration) {
	dynamicIntervalMu.RLock()
	sec := dynamicIntervalSec
	dynamicIntervalMu.RUnlock()
	minD := time.Duration(sec*800) * time.Millisecond
	maxD := time.Duration(sec*1200) * time.Millisecond
	return minD, maxD
}

func newAccountRequestGate() *accountRequestGate {
	random := rand.Float64
	minD, maxD := getDynamicDelays()
	return &accountRequestGate{
		token: make(chan struct{}, 1),
		limiter: requestLimiter{
			minimum: minD,
			maximum: maxD,
			random:  random,
			sleep:   sleepWithContext,
		},
		random: random,
		sleep:  sleepWithContext,
	}
}

func (g *accountRequestGate) call(ctx context.Context, fn func() ([]byte, error), onRetry func(int, time.Duration, error)) ([]byte, error) {
	select {
	case g.token <- struct{}{}:
		defer func() { <-g.token }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	var lastErr error
	for attempt := 0; attempt < qzoneMaxAttempts; attempt++ {
		if err := g.limiter.wait(ctx); err != nil {
			return nil, err
		}
		value, err := fn()
		if err == nil {
			return value, nil
		}
		lastErr = err
		if attempt == qzoneMaxAttempts-1 || !temporaryQZoneError(err) {
			return nil, err
		}
		delay := retryBackoff(attempt, g.random())
		if onRetry != nil {
			onRetry(attempt+1, delay, err)
		}
		if err := g.sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (l requestLimiter) wait(ctx context.Context) error {
	minD, maxD := l.minimum, l.maximum
	if minD == 0 && maxD == 0 {
		minD, maxD = getDynamicDelays()
	}
	return l.sleep(ctx, jitterDuration(minD, maxD, l.random()))
}

func jitterDuration(minimum, maximum time.Duration, sample float64) time.Duration {
	if minimum < 0 {
		minimum = 0
	}
	if maximum <= minimum {
		return minimum
	}
	sample = math.Max(0, math.Min(1, sample))
	return minimum + time.Duration(float64(maximum-minimum)*sample)
}

func retryBackoff(attempt int, sample float64) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	base := qzoneBackoffBase * time.Duration(1<<minInt(attempt, 10))
	if base > qzoneBackoffMax {
		base = qzoneBackoffMax
	}
	jitter := jitterDuration(0, time.Second, sample)
	if base+jitter > qzoneBackoffMax {
		return qzoneBackoffMax
	}
	return base + jitter
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func temporaryQZoneError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) {
		return temporary.Temporary()
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return networkError.Timeout() || networkError.Temporary()
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"network busy", "connection reset", "broken pipe", "connection refused", "temporarily unavailable", "timeout"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
