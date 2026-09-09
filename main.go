package main

import (
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"rate-limiter/policies"
	"strconv"
	"time"
)

func RateLimiterMiddleware(lim policies.RateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Invalid remote address", http.StatusInternalServerError)
			return
		}
		key := ip
		if lim.ShouldWait() {
			err = lim.Wait(r.Context(), ip)
			if err != nil {
				http.Error(w, "Request canceled or timed out", http.StatusRequestTimeout)
				return
			}
			next.ServeHTTP(w, r)
		} else {
			allowed, retryAfter := lim.Allow(key)
			if !allowed {
				retryAfterInSeconds := strconv.Itoa(int(math.Ceil(retryAfter.Seconds())))
				w.Header().Set("Retry-After", retryAfterInSeconds)
				http.Error(w, "Too many requests.", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

func baseHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hit\n"))
	time.Sleep(200 * time.Millisecond)
}

const PORT = 3000
const RATE_LIMITER_CAPACITY = 5
const RATE_LIMITER_REFILL_INTERVAL_MS = 2000

func SetupServer(port int, done chan struct{}) {
	cfg := policies.Config{
		Capacity:       RATE_LIMITER_CAPACITY,
		RefillInterval: RATE_LIMITER_REFILL_INTERVAL_MS * time.Millisecond,
		Wait:           true,
	}
	lim := policies.NewTokenBucketLimiter(cfg)
	mux := http.NewServeMux()
	mux.Handle("/", RateLimiterMiddleware(lim, http.HandlerFunc(baseHandler)))
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

func request(url string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println("--- Response Headers ---")
	fmt.Println(resp.StatusCode)
	for key, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("%s: %s\n", key, value)
		}
	}
}

func main() {
	done := make(chan struct{})
	go SetupServer(PORT, done)
	<-done
	for range 10 {
		request("http://localhost:3000")
	}
}
