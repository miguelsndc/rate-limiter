package policies

import (
	"sync"
	"time"
)

type Bucket struct {
	mu sync.Mutex
	tokens     int
	lastRefill time.Time
}

type Limiter struct {
	mu sync.Mutex
	capacity       int
	refillInterval time.Duration
	buckets        map[string]*Bucket
}

func createBucket(capacity int) *Bucket {
	return &Bucket{
		tokens:     capacity,
		lastRefill: time.Now(),
	}
}

func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	bucket, exists := l.buckets[key]
	if !exists {
		bucket = createBucket(l.capacity)
		l.buckets[key] = bucket
	}
	l.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	refilled := int(elapsed / l.refillInterval)
	bucket.tokens = min(l.capacity, bucket.tokens+refilled)
	if refilled > 0 {
		bucket.lastRefill = bucket.lastRefill.Add(time.Duration(refilled) * l.refillInterval)
	}
	if bucket.tokens == 0 {
		retryAfter := bucket.lastRefill.Add(l.refillInterval).Sub(now)
		return false, retryAfter
	}
	bucket.tokens--
	return true, 0
}

func NewLimiter(capacity int, refillInterval time.Duration) *Limiter {
	return &Limiter{
		capacity:       capacity,
		refillInterval: refillInterval,
		buckets: make(map[string]*Bucket),
	}
}
