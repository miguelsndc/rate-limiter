package policies

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestLeakyBucketQueueSpacesRequest(t *testing.T) {
	const requests = 3
	const interval = 30 * time.Millisecond

	shaper := NewLeakyBucketShaper(
		LeakyBucketShaperConfig{
			QueueCapacity: requests,
			LeakInterval:  interval,
		},
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	start := time.Now()
	completions := make(chan time.Duration, requests)
	errors := make(chan error, requests)

	for range requests {
		go func() {
			err := shaper.Wait(ctx, "client")
			errors <- err
			completions <- time.Since(start)
		}()
	}

	times := make([]time.Duration, 0, requests)

	for range requests {
		if err := <-errors; err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		times = append(times, <-completions)
	}
	slices.Sort(times)
	for i := 1; i < len(times); i++ {
		distance := times[i] - times[i-1]
		// tolerancia
		if distance < interval/2 {
			t.Fatalf("requests were ran too close together %v", times)
		}
	}
}

func TestLeakyBucketQueueRejectsWhenQueueFull(t *testing.T) {
	const key = "client"
	lim := NewLeakyBucketShaper(
		LeakyBucketShaperConfig{
			QueueCapacity: 1,
			LeakInterval:  time.Hour,
		},
	)
	queue := lim.getQueue(key)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- lim.Wait(ctx, key)
	}()

	deadline := time.Now().Add(100 * time.Millisecond)
	for len(queue.requests) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(queue.requests) != 1 {
		t.Fatalf("first request didnt enter queue")
	}
	err := lim.Wait(context.Background(), key)

	if !errors.Is(err, ErrorLeakyBucketQueueFull) {
		t.Fatalf("expected queue full error at %v", err)
	}
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected first request to be canceled, got %v",
			err,
		)
	}
}

func TestLeakyBucketShaperRespectsCancellation(
	t *testing.T,
) {
	lim := NewLeakyBucketShaper(
		LeakyBucketShaperConfig{
			QueueCapacity: 1,
			LeakInterval:  time.Second,
		},
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Millisecond,
	)
	defer cancel()

	err := lim.Wait(ctx, "client")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"expected deadline exceeded, got %v",
			err,
		)
	}
}
