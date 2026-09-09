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

func RateLimiterMiddleware(lim *policies.Limiter, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
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

func baseHandler (w http.ResponseWriter, r* http.Request) {
	w.Write([]byte("Hit\n"))
}

const REFILL_INTERVAL_MS = 300
const BUCKET_CAPACITY = 1
func SetupServer(port int, done chan struct{}) {
	lim := policies.NewLimiter(BUCKET_CAPACITY, REFILL_INTERVAL_MS * time.Millisecond)
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

func main () {
	done := make(chan struct{})
    go SetupServer(3000, done)
	<-done
	request("http://localhost:3000")
	request("http://localhost:3000")
	request("http://localhost:3000")
}