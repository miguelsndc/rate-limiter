package policies

import (
	"context"
	"sync"
	"time"
)

type Config struct {
	Capacity       int
	RefillInterval time.Duration
	Wait           bool
}

type Bucket struct {
	mu         sync.Mutex
	tokens     int
	lastRefill time.Time
}

type TokenBucketLimiter struct {
	mu      sync.Mutex
	config  Config
	buckets map[string]*Bucket
}

func createBucket(capacity int) *Bucket {
	return &Bucket{
		tokens:     capacity,
		lastRefill: time.Now(),
	}
}

func (l *TokenBucketLimiter) getBucket(key string) *Bucket {
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, exists := l.buckets[key]
	if !exists {
		bucket = createBucket(l.config.Capacity)
		l.buckets[key] = bucket
	}
	return bucket
}

func (l *TokenBucketLimiter) Wait(ctx context.Context, key string) error {
	bucket := l.getBucket(key)
	bucket.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	refilled := int(elapsed / l.config.RefillInterval)
	bucket.tokens = min(l.config.Capacity, bucket.tokens+refilled)
	if refilled > 0 {
		bucket.lastRefill = bucket.lastRefill.Add(time.Duration(refilled) * l.config.RefillInterval)
	}

	var waitTime time.Duration
	if bucket.tokens > 0 {
		bucket.tokens--
	} else {
		nextAvailable := bucket.lastRefill.Add(l.config.RefillInterval)
		waitTime = nextAvailable.Sub(now)
		bucket.lastRefill = nextAvailable
	}
	bucket.mu.Unlock()

	if waitTime == 0 {
		return nil
	}

	timer := time.NewTimer(waitTime)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *TokenBucketLimiter) Allow(key string) (bool, time.Duration) {
	bucket := l.getBucket(key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	refilled := int(elapsed / l.config.RefillInterval)
	bucket.tokens = min(l.config.Capacity, bucket.tokens+refilled)
	if refilled > 0 {
		bucket.lastRefill = bucket.lastRefill.Add(time.Duration(refilled) * l.config.RefillInterval)
	}
	if bucket.tokens == 0 {
		retryAfter := bucket.lastRefill.Add(l.config.RefillInterval).Sub(now)
		return false, retryAfter
	}
	bucket.tokens--
	return true, 0
}

func NewTokenBucketLimiter(cfg Config) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		config:  cfg,
		buckets: make(map[string]*Bucket),
	}
}

func (l *TokenBucketLimiter) ShouldWait() bool {
	return l.config.Wait
}
