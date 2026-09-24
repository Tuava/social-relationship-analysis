package qzone

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRequestLimiterUsesConfiguredJitterRange(t *testing.T) {
	var waited time.Duration
	limiter := requestLimiter{
		minimum: 1100 * time.Millisecond,
		maximum: 1800 * time.Millisecond,
		random:  func() float64 { return 0.5 },
		sleep: func(_ context.Context, delay time.Duration) error {
			waited = delay
			return nil
		},
	}
	if err := limiter.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if waited != 1450*time.Millisecond {
		t.Fatalf("waited %v, want 1.45s", waited)
	}
}

func TestRetryBackoffIsFiniteAndExponential(t *testing.T) {
	wants := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second}
	for attempt, want := range wants {
		if got := retryBackoff(attempt, 0); got != want {
			t.Fatalf("attempt %d = %v, want %v", attempt, got, want)
		}
	}
	if got := retryBackoff(50, 1); got != qzoneBackoffMax {
		t.Fatalf("capped backoff = %v", got)
	}
}

func TestAccountRequestGateRetriesTemporaryErrors(t *testing.T) {
	backoffs := []time.Duration{}
	gate := &accountRequestGate{
		token: make(chan struct{}, 1),
		limiter: requestLimiter{random: func() float64 { return 0 }, sleep: func(context.Context, time.Duration) error {
			return nil
		}},
		random: func() float64 { return 0 },
		sleep: func(_ context.Context, delay time.Duration) error {
			backoffs = append(backoffs, delay)
			return nil
		},
	}
	attempts := 0
	value, err := gate.call(context.Background(), func() ([]byte, error) {
		attempts++
		if attempts < 3 {
			return nil, &HTTPStatusError{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable"}
		}
		return []byte("ok"), nil
	}, nil)
	if err != nil || string(value) != "ok" || attempts != 3 {
		t.Fatalf("value=%q attempts=%d err=%v", value, attempts, err)
	}
	if len(backoffs) != 2 || backoffs[0] != 2*time.Second || backoffs[1] != 4*time.Second {
		t.Fatalf("backoffs = %v", backoffs)
	}
}

func TestAccountRequestGateDoesNotRetryPermanentError(t *testing.T) {
	gate := &accountRequestGate{
		token: make(chan struct{}, 1),
		limiter: requestLimiter{random: func() float64 { return 0 }, sleep: func(context.Context, time.Duration) error {
			return nil
		}},
		random: func() float64 { return 0 },
		sleep:  func(context.Context, time.Duration) error { return nil },
	}
	attempts := 0
	_, err := gate.call(context.Background(), func() ([]byte, error) {
		attempts++
		return nil, &APIError{Retcode: 1401, Message: "not logged in"}
	}, nil)
	if err == nil || attempts != 1 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestAccountRequestGateStopsAtMaximumAttempts(t *testing.T) {
	gate := &accountRequestGate{
		token: make(chan struct{}, 1),
		limiter: requestLimiter{random: func() float64 { return 0 }, sleep: func(context.Context, time.Duration) error {
			return nil
		}},
		random: func() float64 { return 0 },
		sleep:  func(context.Context, time.Duration) error { return nil },
	}
	attempts := 0
	_, err := gate.call(context.Background(), func() ([]byte, error) {
		attempts++
		return nil, &HTTPStatusError{StatusCode: 503, Status: "503"}
	}, nil)
	if err == nil || attempts != qzoneMaxAttempts {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestAccountRequestGateWaitIsCancelable(t *testing.T) {
	gate := &accountRequestGate{token: make(chan struct{}, 1)}
	gate.token <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gate.call(ctx, func() ([]byte, error) { return []byte("unexpected"), nil }, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestTemporaryQZoneErrorClassification(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{&HTTPStatusError{StatusCode: 429, Status: "429"}, true},
		{&HTTPStatusError{StatusCode: 502, Status: "502"}, true},
		{&HTTPStatusError{StatusCode: 400, Status: "400"}, false},
		{&APIError{Retcode: 1401, Message: "network busy | 0103-184"}, true},
		{&APIError{Retcode: 1401, Message: "not logged in"}, false},
		{context.DeadlineExceeded, false},
	}
	for _, test := range tests {
		if got := temporaryQZoneError(test.err); got != test.want {
			t.Errorf("temporaryQZoneError(%v) = %v, want %v", test.err, got, test.want)
		}
	}
}
