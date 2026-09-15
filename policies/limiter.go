package policies

import (
	"time"
)

type IRateLimiter interface {
	Allow(key string) (bool, time.Duration)
}
