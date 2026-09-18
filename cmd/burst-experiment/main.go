package main

import (
	"fmt"
	"rate-limiter/policies"
	"time"
)

type experiment struct {
	name    string
	limiter policies.IRateLimiterPolicer
}

func main() {
	experiments := []experiment{
		{
			name: "token_bucket",
			limiter: policies.NewTokenBucketLimiter(
				policies.TokenBucketConfig{
					Capacity:       10,
					RefillInterval: 100 * time.Millisecond,
				},
			),
		},
		{
			name: "leaky_bucket",
			limiter: policies.NewLeakyBucketLimiter(
				policies.LeakyBucketConfig{
					Capacity: 10,
					LeakRate: 10,
				},
			),
		},
		{
			name: "sliding_window",
			limiter: policies.NewSlidingWindow(
				policies.SlidingWindowConfig{
					Limit:  10,
					Window: time.Second,
				},
			),
		},
	}

	fmt.Println(
		"limiter,phase,request,elapsed_ms,allowed,retry_after_ms",
	)

	for _, exp := range experiments {
		runExperiment(exp)
	}
}

func runExperiment(exp experiment) {
	const key = "client"

	start := time.Now()

	for req := 1; req <= 20; req++ {
		allowed, retryAfter := exp.limiter.Allow(key)

		printResult(
			exp.name,
			"burst",
			req,
			time.Since(start),
			allowed,
			retryAfter,
		)
	}

	for req := 1; req <= 12; req++ {
		time.Sleep(100 * time.Millisecond)

		allowed, retryAfter := exp.limiter.Allow(key)

		printResult(
			exp.name,
			"recovery",
			req,
			time.Since(start),
			allowed,
			retryAfter,
		)
	}
}

func printResult(
	limiter string,
	phase string,
	request int,
	elapsed time.Duration,
	allowed bool,
	retryAfter time.Duration,
) {
	fmt.Printf(
		"%s,%s,%d,%d,%t,%d\n",
		limiter,
		phase,
		request,
		elapsed.Milliseconds(),
		allowed,
		retryAfter.Milliseconds(),
	)
}
