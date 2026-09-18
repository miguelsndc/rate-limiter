package policies

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrorLeakyBucketQueueFull = errors.New("leaky buket queue is full")

type LeakyBucketQueueConfig struct {
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

type LeakyBucketQueue struct {
	mu     sync.Mutex
	config LeakyBucketQueueConfig
	queues map[string]*requestsQueue
}

func (l *LeakyBucketQueue) runQueue(
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

func (l *LeakyBucketQueue) Wait(ctx context.Context, key string) error {
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
	case <- ctx.Done():
		return ctx.Err()
	}
}

func (l *LeakyBucketQueue) getQueue(key string) *requestsQueue {
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

func NewLeakyBucketQueue(cfg LeakyBucketQueueConfig) *LeakyBucketQueue {
	if cfg.QueueCapacity <= 0 {
		panic("queue capacity must be positive")
	}
	if cfg.LeakInterval <= 0 {
		panic("leaking interval must be positive")
	}
	return &LeakyBucketQueue{
		config: cfg,
		queues: make(map[string]*requestsQueue),
	}
}
