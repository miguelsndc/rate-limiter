package main

import (
	"fmt"
	"math/rand/v2"
	"rate-limiter/policies"
	"sync"
	"time"
)

func getKey() string {
	keys := []string{"miguel", "samuel", "gustavo", "dyers", "wesley"}
	index := rand.IntN(len(keys))
	return keys[index]
}

func worker(id int, key string, lim *policies.Limiter) {
	for range 10 {
		if lim.Allow(key) {
			fmt.Printf("%d: worker with key %s allowed at %v\n", id, key, time.Now())
		} else {
			fmt.Printf("%d: worker with key %s denied at %v\n", id, key, time.Now())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func main() {
	lim := policies.NewLimiter(5, 400*time.Millisecond)
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			worker(i, getKey(), lim)
		})
	}
	wg.Wait()
}
