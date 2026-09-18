package policies

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrorLeakyBucketQueueFull = errors.New("leaky buket queue is full")

type LeakyBucketShaperConfig struct {
	QueueCapacity int
	LeakInterval  time.Duration
}

type queuedRequest struct {
	ctx   context.Context
	ready chan struct{}
}

type requestsQueue struct {
	requests chan queuedRequest
}

type LeakyBucketShaper struct {
	mu     sync.Mutex
	config LeakyBucketShaperConfig
	queues map[string]*requestsQueue
}

func (l *LeakyBucketShaper) runQueue(
	queue *requestsQueue,
) {
	ticker := time.NewTicker(l.config.LeakInterval)
	defer ticker.Stop()

	for range ticker.C {
		for {
			select {
			case request := <-queue.requests:
				if request.ctx.Err() != nil {
					continue
				}
				close(request.ready)
			default:
			}
			break
		}
	}
}

func (l *LeakyBucketShaper) Wait(ctx context.Context, key string) error {
	queue := l.getQueue(key)
	request := queuedRequest{
		ctx:   ctx,
		ready: make(chan struct{}),
	}
	select {
	case queue.requests <- request:
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrorLeakyBucketQueueFull
	}

	select {
	case <-request.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *LeakyBucketShaper) getQueue(key string) *requestsQueue {
	l.mu.Lock()
	defer l.mu.Unlock()

	queue, exists := l.queues[key]
	if exists {
		return queue
	}

	queue = &requestsQueue{
		requests: make(chan queuedRequest, l.config.QueueCapacity),
	}
	l.queues[key] = queue
	go l.runQueue(queue)
	return queue
}

func NewLeakyBucketShaper(cfg LeakyBucketShaperConfig) *LeakyBucketShaper {
	if cfg.QueueCapacity <= 0 {
		panic("queue capacity must be positive")
	}
	if cfg.LeakInterval <= 0 {
		panic("leaking interval must be positive")
	}
	return &LeakyBucketShaper{
		config: cfg,
		queues: make(map[string]*requestsQueue),
	}
}
