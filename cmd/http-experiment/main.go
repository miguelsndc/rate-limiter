package main

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"rate-limiter/policies"
	"sort"
	"strconv"
	"sync"
	"time"
)

type requestResult struct {
	status  int
	latency time.Duration
	err     error
}

type summary struct {
	elapsed    time.Duration
	latencies  []time.Duration
	status200  int
	status429  int
	errorCount int
}

type experiment struct {
	name       string
	newHandler func() http.Handler
}

func main() {
	const (
		total       = 5000
		concurrency = 32
		runs        = 5
		limit       = total / 2
	)

	experiments := []experiment{
		{
			name: "baseline",
			newHandler: func() http.Handler {
				return http.HandlerFunc(baseHandler)
			},
		},
		{
			name: "token_bucket",
			newHandler: func() http.Handler {
				limiter := policies.NewTokenBucketLimiter(
					policies.TokenBucketConfig{
						Capacity:       limit,
						RefillInterval: time.Hour,
					},
				)

				return policerMiddleware(
					limiter,
					http.HandlerFunc(baseHandler),
				)
			},
		},
		{
			name: "leaky_bucket",
			newHandler: func() http.Handler {
				limiter := policies.NewLeakyBucketLimiter(
					policies.LeakyBucketConfig{
						Capacity: limit,
						LeakRate: 0.000001,
					},
				)

				return policerMiddleware(
					limiter,
					http.HandlerFunc(baseHandler),
				)
			},
		},
		{
			name: "sliding_window",
			newHandler: func() http.Handler {
				limiter := policies.NewSlidingWindow(
					policies.SlidingWindowConfig{
						Limit:  limit,
						Window: time.Hour,
					},
				)

				return policerMiddleware(
					limiter,
					http.HandlerFunc(baseHandler),
				)
			},
		},
	}

	fmt.Println(
		"limiter,run,throughput_rps,p50_us,p95_us,p99_us,status_200,status_429,errors",
	)

	for _, exp := range experiments {
		for run := 1; run <= runs; run++ {
			server := httptest.NewServer(exp.newHandler())

			result := runLoad(
				server.URL,
				total,
				concurrency,
			)

			server.Close()

			throughput := float64(total) /
				result.elapsed.Seconds()

			fmt.Printf(
				"%s,%d,%.2f,%d,%d,%d,%d,%d,%d\n",
				exp.name,
				run,
				throughput,
				percentile(result.latencies, 0.50).Microseconds(),
				percentile(result.latencies, 0.95).Microseconds(),
				percentile(result.latencies, 0.99).Microseconds(),
				result.status200,
				result.status429,
				result.errorCount,
			)
		}
	}
}

func baseHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func policerMiddleware(
	limiter policies.IRateLimiterPolicer,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				http.Error(
					w,
					"invalid remote address",
					http.StatusInternalServerError,
				)
				return
			}

			allowed, retryAfter := limiter.Allow(key)
			if !allowed {
				seconds := int(math.Ceil(
					retryAfter.Seconds(),
				))

				w.Header().Set(
					"Retry-After",
					strconv.Itoa(seconds),
				)

				http.Error(
					w,
					"too many requests",
					http.StatusTooManyRequests,
				)
				return
			}

			next.ServeHTTP(w, r)
		},
	)
}

func runLoad(
	url string,
	total int,
	concurrency int,
) summary {
	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxIdleConnsPerHost: concurrency,
		MaxConnsPerHost:     concurrency,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	defer transport.CloseIdleConnections()

	jobs := make(chan struct{})
	results := make(chan requestResult, total)

	var wg sync.WaitGroup
	wg.Add(concurrency)

	start := time.Now()

	for range concurrency {
		go func() {
			defer wg.Done()

			for range jobs {
				requestStart := time.Now()

				response, err := client.Get(url)
				latency := time.Since(requestStart)

				if err != nil {
					results <- requestResult{
						latency: latency,
						err:     err,
					}
					continue
				}

				_, _ = io.Copy(
					io.Discard,
					response.Body,
				)
				response.Body.Close()

				results <- requestResult{
					status:  response.StatusCode,
					latency: latency,
				}
			}
		}()
	}

	go func() {
		for range total {
			jobs <- struct{}{}
		}

		close(jobs)
	}()

	wg.Wait()
	elapsed := time.Since(start)
	close(results)

	result := summary{
		elapsed: elapsed,
	}

	for request := range results {
		if request.err != nil {
			result.errorCount++
			continue
		}

		result.latencies = append(
			result.latencies,
			request.latency,
		)

		switch request.status {
		case http.StatusOK:
			result.status200++

		case http.StatusTooManyRequests:
			result.status429++
		}
	}

	sort.Slice(
		result.latencies,
		func(i, j int) bool {
			return result.latencies[i] <
				result.latencies[j]
		},
	)

	return result
}

func percentile(
	values []time.Duration,
	p float64,
) time.Duration {
	if len(values) == 0 {
		return 0
	}

	index := int(math.Ceil(
		p*float64(len(values)),
	)) - 1

	if index < 0 {
		index = 0
	}

	return values[index]
}
