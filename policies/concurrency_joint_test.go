package policies

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimitersConcurrentBurst(t *testing.T) {
	const (
		limit    = 10
		requests = 100
	)

	testCases := []struct {
		name    string
		limiter IRateLimiter
	}{

		{
			name: "token bucket",
			limiter: NewTokenBucketLimiter(TokenBucketConfig{
				Capacity:       limit,
				RefillInterval: time.Hour,
			}),
		},
		{
			name: "leaky bucket",
			limiter: NewLeakyBucketLimiter(LeakyBucketConfig{
				Capacity: limit,
				LeakRate: 0.001,
			}),
		},
		{
			name: "sliding window",
			limiter: NewSlidingWindow(SlidingWindowConfig{
				Limit:  limit,
				Window: time.Hour,
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var accepted atomic.Int64
			var wg sync.WaitGroup

			wg.Add(requests)
			for range requests {
				go func() {
					defer wg.Done()
					allowed, _ := tc.limiter.Allow("client")
					if allowed {
						accepted.Add(1)
					}
				}()
			}

			wg.Wait()

			if got := accepted.Load(); got != limit {
				t.Fatalf("expected %d requests accepeted, got %d", limit, got)
			}
		})
	}
}
