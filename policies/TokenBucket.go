package policies

import (
	"sync"
	"time"
)

type TokenBucketConfig struct {
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
	config  TokenBucketConfig
	buckets map[string]*Bucket
}

func createTokenBucket(capacity int) *Bucket {
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
		bucket = createTokenBucket(l.config.Capacity)
		l.buckets[key] = bucket
	}
	return bucket
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

func NewTokenBucketLimiter(cfg TokenBucketConfig) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		config:  cfg,
		buckets: make(map[string]*Bucket),
	}
}
