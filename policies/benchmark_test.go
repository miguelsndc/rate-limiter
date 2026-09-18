package policies

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkSaturatedSequential(b *testing.B) {
	const (
		limit = 1000
		key   = "client"
	)
	testCases := []struct {
		name       string
		newLimiter func() IRateLimiterPolicer
	}{
		{
			name: "token_bucket",
			newLimiter: func() IRateLimiterPolicer {
				return NewTokenBucketLimiter(
					TokenBucketConfig{
						Capacity:       limit,
						RefillInterval: time.Hour,
					},
				)
			},
		},
		{
			name: "leaky_bucket",
			newLimiter: func() IRateLimiterPolicer {
				return NewLeakyBucketLimiter(
					LeakyBucketConfig{
						Capacity: limit,
						LeakRate: 0.000001,
					},
				)
			},
		},
		{
			name: "sliding_window",
			newLimiter: func() IRateLimiterPolicer {
				return NewSlidingWindow(
					SlidingWindowConfig{
						Limit:  limit,
						Window: time.Hour,
					},
				)
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			limiter := tc.newLimiter()
			for range limit {
				allowed, _ := limiter.Allow(key)
				if !allowed {
					b.Fatal("failed to fill limiter")
				}
			}
			allowed, _ := limiter.Allow(key)
			if allowed {
				b.Fatal("saturated limiter should reject")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				limiter.Allow(key)
			}
		})
	}
}

func BenchmarkSaturatedConcurrent(b *testing.B) {
	testCases := []struct {
		name       string
		newLimiter func() IRateLimiterPolicer
	}{
		{
			name: "token_bucket",
			newLimiter: func() IRateLimiterPolicer {
				return NewTokenBucketLimiter(
					TokenBucketConfig{
						Capacity:       1,
						RefillInterval: time.Hour,
					},
				)
			},
		},
		{
			name: "leaky_bucket",
			newLimiter: func() IRateLimiterPolicer {
				return NewLeakyBucketLimiter(
					LeakyBucketConfig{
						Capacity: 1,
						LeakRate: 0.000001,
					},
				)
			},
		},
		{
			name: "sliding_window",
			newLimiter: func() IRateLimiterPolicer {
				return NewSlidingWindow(
					SlidingWindowConfig{
						Limit:  1,
						Window: time.Hour,
					},
				)
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func (b *testing.B) {
			b.Run("same_key", func(b *testing.B) {
				benchmarkConcurrentSameKey(
					b, tc.newLimiter,
				)
			})
			b.Run("many_keys", func(b*testing.B) {
				benchmarkConcurrentManyKeys(
					b, tc.newLimiter,
				)
			})
		})
	}
}

func benchmarkConcurrentSameKey(b *testing.B, newLimiter func() IRateLimiterPolicer){
	const key = "client"
	limiter := newLimiter()
	allowed, _ := limiter.Allow(key)
	if !allowed {
		b.Fatal("failed to saturate limiter")
	}
	allowed, _ = limiter.Allow(key)
	if allowed {
		b.Fatal("saturated limiter should reject")
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb * testing.PB) {
		for pb.Next() {
			limiter.Allow(key)
		}
	})
} 

func benchmarkConcurrentManyKeys(b *testing.B, newLimiter func() IRateLimiterPolicer) {
	const keyCount = 128
	limiter := newLimiter()
	keys := make([]string, keyCount)
	for i := range keyCount {
		keys[i] = "client-" + strconv.Itoa(i)
		allowed, _ := limiter.Allow(keys[i])
		if !allowed {
			b.Fatal("failed to saturate limiter")
		}
	}

	var nextWorker atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func (pb *testing.PB) {
		workerID := nextWorker.Add(1) - 1
		key := keys[int(workerID)%len(keys)]
		for pb.Next() {
			limiter.Allow(key)
		}
	})
} 
