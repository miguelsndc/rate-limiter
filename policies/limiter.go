package policies

import (
	"context"
	"time"
)

type RateLimiterWaiter interface {
    Wait(ctx context.Context, key string) error
}

type RateLimiterPolicer interface {
    Allow(key string) (bool, time.Duration)
}