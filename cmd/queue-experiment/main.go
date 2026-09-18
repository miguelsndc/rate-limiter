package main

import (
	"context"
	"fmt"
	"rate-limiter/policies"
	"sort"
	"sync"
	"time"
)

type result struct {
	requestID int
	elapsed   time.Duration
	err       error
}

func main() {
	const (
		key      = "client"
		limit    = 10
		interval = 100 * time.Millisecond
	)
	lim := policies.NewLeakyBucketQueue(policies.LeakyBucketQueueConfig{
		QueueCapacity: limit,
		LeakInterval:  interval,
	})
	var wg sync.WaitGroup
	wg.Add(limit)
	results := make(chan result, limit)
	start := time.Now()
	for i := range limit {
		go func(i int) {
			defer wg.Done()
			err := lim.Wait(context.Background(), key)
			results <- result{
				requestID: i,
				elapsed:   time.Since(start),
				err:       err,
			}
		}(i)
	}
	wg.Wait()
	close(results)

	completions := make([]result, 0, limit)
	for result := range results {
		completions = append(completions, result)
	}

	sort.Slice(completions, func(i, j int) bool {
		return completions[i].elapsed < completions[j].elapsed
	})

	fmt.Println(
		"position,request_id,completed_ms,gap_ms,error",
	)

	var previous time.Duration

	for position, result := range completions {
		gap := result.elapsed - previous

		fmt.Printf(
			"%d,%d,%d,%d,%v\n",
			position+1,
			result.requestID,
			result.elapsed.Milliseconds(),
			gap.Milliseconds(),
			result.err,
		)

		previous = result.elapsed
	}
}