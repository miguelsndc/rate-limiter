package policies

import (
	"sync"
	"time"
)

type LeakyBucketConfig struct {
	Capacity       float64
	LeakRate       float64
}

type LeakyBucket struct {
	mu         sync.Mutex
	waterLevel float64
	lastUpdate time.Time
}

type LeakyBucketLimiter struct {
	mu      sync.Mutex
	config  LeakyBucketConfig
	buckets map[string]*LeakyBucket
}

func createLeakyBucket(capacity float64) *LeakyBucket {
	return &LeakyBucket{
		waterLevel: 0,
		lastUpdate: time.Now(),
	}
}

func (l *LeakyBucketLimiter) getBucket(key string) *LeakyBucket {
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, exists := l.buckets[key]
	if !exists {
		bucket = createLeakyBucket(l.config.Capacity)
		l.buckets[key] = bucket
	}
	return bucket
}

func (l *LeakyBucketLimiter) Allow(key string) (bool, time.Duration) {
	bucket := l.getBucket(key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	leakRate := l.config.LeakRate 
	capacity := l.config.Capacity

	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	bucket.waterLevel = max(0, bucket.waterLevel - elapsed * leakRate)
	bucket.lastUpdate = now
	if bucket.waterLevel + 1 > capacity {
	    required := bucket.waterLevel + 1 - l.config.Capacity
		waitSeconds := required / l.config.LeakRate
		return false, time.Duration(waitSeconds * float64(time.Second))
	}

	bucket.waterLevel += 1
	return true, 0
}

func NewLeakyBucketLimiter(cfg LeakyBucketConfig) *LeakyBucketLimiter {
	return &LeakyBucketLimiter{
		config:  cfg,
		buckets: make(map[string]*LeakyBucket),
	}
}

