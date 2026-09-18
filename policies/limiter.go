package policies

import (
	"context"
	"time"
)

type IRateLimiterPolicer interface {
	Allow(key string) (bool, time.Duration)
}

type IRateLimiterWaiter interface {
	Wait(ctx context.Context, key string) error
}
