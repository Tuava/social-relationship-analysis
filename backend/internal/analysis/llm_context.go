package analysis

import (
	"context"
	"time"
)

type llmQueueTimeoutKey struct{}

func withLLMQueueTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, llmQueueTimeoutKey{}, timeout)
}

func llmQueueTimeout(ctx context.Context) time.Duration {
	if timeout, ok := ctx.Value(llmQueueTimeoutKey{}).(time.Duration); ok && timeout > 0 {
		return timeout
	}
	return 0
}
