package main

import (
	"os"

	"github.com/go-redis/redis"
)

func main() {
	redisIP := os.Getenv("redisIP")
	client := redis.NewClient(&redis.Options{
		Addr:     redisIP + ":6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	client.FlushAll()
	seedRun(client)
}

func seedRun(client *redis.Client) {
	boards := []string{"POLITICS", "STEM", "ARTS", "BUSINESS", "OTHER"}

	for _, v := range boards {
		mem := redis.Z{
			Member: "BOARD:" + v,
			Score:  0,
		}
		client.ZAdd("BOARD", mem)
	}
}
