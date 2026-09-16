package policies

import (
	"rate-limiter/utils"
	"sync"
	"time"
)

type window struct {
	mu       sync.Mutex
	requests *utils.Deque[time.Time]
}

func createWindow(limit int) *window {
	return &window{
		requests: utils.NewDeque[time.Time](limit),
	}
}

type SlidingWindowLimiter struct {
	mu      sync.Mutex
	config  SlidingWindowConfig
	windows map[string]*window
}

func (sw *SlidingWindowLimiter) getWindow(key string) *window {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	window, exists := sw.windows[key]
	if !exists {
		window = createWindow(sw.config.Limit)
		sw.windows[key] = window
	}
	return window
}

type SlidingWindowConfig struct {
	Limit  int
	Window time.Duration
}

func (sw *SlidingWindowLimiter) Allow(key string) (bool, time.Duration) {
	window := sw.getWindow(key)
	window.mu.Lock()
	defer window.mu.Unlock()
	now := time.Now()
	for !window.requests.Empty() && now.Sub(window.requests.Front()) >= sw.config.Window {
		window.requests.PopFront()
	}
	if window.requests.Full() {
		retryAfter := sw.config.Window - now.Sub(window.requests.Front())
		return false, retryAfter
	}
	window.requests.PushBack(now)
	return true, 0
}

func NewSlidingWindow(cfg SlidingWindowConfig) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		config:  cfg,
		windows: make(map[string]*window),
	}
}
