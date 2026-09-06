package main

import (
	"fmt"
	"rate-limiter/policies"
	"time"
	"math/rand/v2"
)

func getKey() string {
	keys := []string {"miguel", "samuel", "gustavo", "dyers"}
	index := rand.IntN(len(keys))
	return keys[index]
}

func main() {
	lim := policies.NewLimiter(5, 500 * time.Millisecond)
    for {
		key := getKey()
        noOfRequests := rand.IntN(20)
		for range noOfRequests {
			if lim.Allow(key) {
				fmt.Println("key ", key, " allowed at ", time.Now())
			} else {
				fmt.Println("key ", key, " denied at ", time.Now())
			}
			time.Sleep(time.Duration(rand.IntN(200)) * time.Millisecond)
		}
		time.Sleep(time.Duration(rand.IntN(200)) * time.Millisecond)
	}    
}
