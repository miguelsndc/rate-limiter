package main

import (
	"log"
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
		if !lim.Allow(key) {
			http.Error(w, "Too many requests.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func baseHandler (w http.ResponseWriter, r* http.Request) {
	w.Write([]byte("Hit\n"))
}

const PORT = 3000
func main () {
	lim := policies.NewLimiter(8, 300 * time.Millisecond)
	mux := http.NewServeMux()
	mux.Handle("/", RateLimiterMiddleware(lim, http.HandlerFunc(baseHandler)))
	log.Println("Listening at port: ", PORT)
	actualPort := ":" + strconv.Itoa(PORT)
	err := http.ListenAndServe(actualPort, mux)
	log.Fatal(err)
}