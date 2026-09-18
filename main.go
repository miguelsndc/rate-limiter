package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"rate-limiter/policies"
	"strconv"
	"sync"
	"time"
)

func WaiterMiddleware(lim policies.IRateLimiterWaiter, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Invalid remote address", http.StatusInternalServerError)
			return
		}
		err = lim.Wait(r.Context(), ip)
		if errors.Is(err, policies.ErrorLeakyBucketQueueFull) {
			http.Error(
				w,
				"Rate limit queue is full.",
				http.StatusTooManyRequests,
			)
			return
		}

		if errors.Is(err, context.Canceled) {
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(
				w,
				"Request timed out.",
				http.StatusRequestTimeout,
			)
			return
		}

		if err != nil {
			http.Error(
				w,
				"Internal rate limiter error.",
				http.StatusInternalServerError,
			)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func PolicerMiddleware(lim policies.IRateLimiterPolicer, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Invalid remote address", http.StatusInternalServerError)
			return
		}
		key := ip
		allowed, retryAfter := lim.Allow(key)
		if !allowed {
			retryAfterInSeconds := strconv.Itoa(int(math.Ceil(retryAfter.Seconds())))
			w.Header().Set("Retry-After", retryAfterInSeconds)
			http.Error(w, "Too many requests.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func baseHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hit\n"))
}

const PORT = 3000

func SetupServer(port int, done chan struct{}) {
	cfg := policies.LeakyBucketShaperConfig{
		LeakInterval:  time.Second,
		QueueCapacity: 10,
	}
	lim := policies.NewLeakyBucketShaper(cfg)
	mux := http.NewServeMux()
	mux.Handle("/", WaiterMiddleware(lim, http.HandlerFunc(baseHandler)))
	actualPort := ":" + strconv.Itoa(port)
	listener, err := net.Listen("tcp", actualPort)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Listening at port: ", port)
	close(done)

	err = http.Serve(listener, mux)
	log.Fatal(err)
}

func burst(n int) {
	var wg sync.WaitGroup
	url := fmt.Sprintf("http://localhost:%d", PORT)
	start := time.Now()
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			resp, err := http.Get(url)
			if err != nil {
				log.Println(err)
				return
			}
			defer resp.Body.Close()
			fmt.Printf("%02d -> %d | (%v)\n", i, resp.StatusCode, time.Since(start))
		}(i)
	}
	wg.Wait()
}

func main() {
	done := make(chan struct{})
	go SetupServer(PORT, done)
	<-done
	burst(15)
}
