package policies

import (
	"testing"
	"time"
)

func TestSlidingWindowBurst(t * testing.T) {
	const limit = 10
	const key = "client"
	limiter := NewSlidingWindow(SlidingWindowConfig{
		Limit: limit,
		Window: time.Hour,
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

func TestSlidingWindowIndependentKeys(t *testing.T) {
	limiter := NewSlidingWindow(SlidingWindowConfig{
		Limit:  1,
		Window: time.Hour,
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
		t.Fatal("client-b should have an independent window")
	}
}

func TestSlidingWindowExpiration(t *testing.T) {
	const window = 50 * time.Millisecond

	limiter := NewSlidingWindow(SlidingWindowConfig{
		Limit:  1,
		Window: window,
	})

	allowed, _ := limiter.Allow("client")
	if !allowed {
		t.Fatal("first request should be accepted")
	}

	allowed, _ = limiter.Allow("client")
	if allowed {
		t.Fatal("request inside the window should be rejected")
	}

	time.Sleep(window + 20*time.Millisecond)

	allowed, _ = limiter.Allow("client")
	if !allowed {
		t.Fatal("request should be accepted after expiration")
	}
}