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
	children, err := client.ZRevRange(ID+":CHILDREN", 0, -1).Result()
	if err != nil {
		fmt.Println(err)
	}

	for _, child := range children {
		data, err := client.HGetAll("OBJECT:" + child).Result()
		if err != nil {
			fmt.Println(err)
		}

		childs = append(childs, makePost(data))
	}
	return
}

// getData gets the board data from redis
func getData() {
	Posts = make(map[string][]*postData)
	tagmem, err := client.ZRevRange("TAGS", 0, -1).Result()
	if err != nil {
		fmt.Println(err)
	}

	for _, tag := range tagmem {
		// fmt.Println("0 ", tag, tagmem)
		posts, err := client.ZRevRange(tag, 0, -1).Result()
		if err != nil {
			fmt.Println(err)
		}

		// fmt.Println("1 ", posts)
		for _, post := range posts {
			data, err := client.HGetAll("OBJECT:" + post).Result()
			if err != nil {
				fmt.Println(err)
			}

			Posts[tag] = append(Posts[tag], makePost(data))
		}
		// fmt.Println("2 ", tag)
	}

	Frontpage = make(map[string][]*postData)
	dbPosts, err := client.ZRevRange("ALLPOSTS", 0, -1).Result()
	if err != nil {
		fmt.Println(err)
	}
	// fmt.Println("DBP: ", dbPosts)

	for _, dbPost := range dbPosts {
		data, err := client.HGetAll("OBJECT:" + dbPost).Result()
		if err != nil {
			fmt.Println(err)
		}

		Frontpage["all"] = append(Frontpage["all"], makePost(data))
	}

}

func init() {
	makeTags()
}

func makeTags() {
	tags, _ = client.ZRevRange("TAGS", 0, -1).Result()
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
