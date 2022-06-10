package main

import (
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis"
)

// cache the database every 3 seconds.
func beginCache() {
	tick := time.NewTicker(3 * time.Second)
	go func() {
		for range tick.C {
			getData()
		}
	}()
}

// getChildren loads the child replies recursively
func getChildren(ID string) (childs []*postData) {
	children, _ := client.ZRevRangeByScore(ID+":CHILDREN", redis.ZRangeBy{Max: "100000"}).Result()
	for _, child := range children {
		data, _ := client.HGetAll(child).Result()
		childs = append(childs, makePost(data))
	}
	return
}

// getData gets the board data from redis
func getData() {
	Posts = make(map[string][]*postData)
	tagmem, _ := client.ZRevRangeByScore("TAGS", redis.ZRangeBy{Max: "100000"}).Result()
	for _, tag := range tagmem {
		posts, _ := client.ZRevRangeByScore(tag, redis.ZRangeBy{Max: "100000"}).Result()
		for _, post := range posts {
			data, _ := client.HGetAll(post).Result()
			Posts[tag] = append(Posts[tag], makePost(data))
		}
	}
}

func init() {
	makeTags()
}

func makeTags() {
	tags, _ = client.ZRevRangeByScore("TAGS", redis.ZRangeBy{Max: "100000"}).Result()
	for _, tag := range tags {
		fmt.Println(tag)
		_, err := client.ZAdd("TAGS", makeZmem(tag)).Result()
		if err != nil {
			log.Println(err)
		}
	}
}

func makeZmem(st string) redis.Z {
	return redis.Z{
		Member: st,
		Score:  0,
	}
}
