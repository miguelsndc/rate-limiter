package policies

import (
	"context"
	"time"
)

type RateLimiter interface {
    Allow(key string) (bool, time.Duration)
    Wait(ctx context.Context, key string) error
    ShouldWait() bool
}