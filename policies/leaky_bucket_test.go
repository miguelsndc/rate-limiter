package policies

import (
	"testing"
	"time"
)

func TestLeakyBucketBurst(t *testing.T) {
	const limit = 10
	const key = "client"

	limiter := NewLeakyBucketLimiter(LeakyBucketConfig{
		Capacity: limit,
		LeakRate: 0.001,
	})

	for i := range limit {
		allowed, _ := limiter.Allow(key)
		if !allowed {
			t.Fatalf("request %d should have been accepted", i)
		}
	}

	allowed, retryAfter := limiter.Allow(key)
	if allowed {
		t.Fatal("request above the limit should have been rejected")
	}

	if retryAfter <= 0 {
		t.Fatalf("expected positive retry-after, got %v", retryAfter)
	}
}

func TestLeakyBucketIndependentKeys(t *testing.T) {
	limiter := NewLeakyBucketLimiter(LeakyBucketConfig{
		Capacity: 1,
		LeakRate: 0.001,
	})

	allowed, _ := limiter.Allow("client-a")
	if !allowed {
		t.Fatal("first request from client-a should be accepted")
	}

	allowed, _ = limiter.Allow("client-a")
	if allowed {
		t.Fatal("second request from client-a should be rejected")
	}

	allowed, _ = limiter.Allow("client-b")
	if !allowed {
		t.Fatal("client-b should have an independent bucket")
	}
}

func TestLeakyBucketExpiration(t *testing.T) {
	const interval = 50 * time.Millisecond

	limiter := NewLeakyBucketLimiter(LeakyBucketConfig{
		Capacity: 1,
		LeakRate: 1 / interval.Seconds(),
	})

	allowed, _ := limiter.Allow("client")
	if !allowed {
		t.Fatal("first request should be accepted")
	}

	allowed, retryAfter := limiter.Allow("client")
	if allowed {
		t.Fatal("request before leakage should be rejected")
	}

	if retryAfter <= 0 {
		t.Fatalf("expected positive retry-after, got %v", retryAfter)
	}

	time.Sleep(interval + 20*time.Millisecond)

	allowed, _ = limiter.Allow("client")
	if !allowed {
		t.Fatal("request should be accepted after enough water has leaked")
	}
}
