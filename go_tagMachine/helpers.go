package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	redis "github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func parseToken(tokenString string) (*credentials, error) {
	var claims *credentials
	token, err := jwt.ParseWithClaims(tokenString, &credentials{}, func(token *jwt.Token) (interface{}, error) {
		return hmacSampleSecret, nil
	})
	if err != nil {
		fmt.Println(err)
		cc := credentials{IsLoggedIn: false}
		return &cc, err
	}

	if claims, ok := token.Claims.(*credentials); ok && token.Valid {
		return claims, nil
	}
	return claims, err
}

func renewToken(w http.ResponseWriter, r *http.Request, claims *credentials) (ctx context.Context) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		fmt.Println(err)
	}

	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	client.Set(ctx, claims.Name+"token", ss, 0)
	ctx = context.WithValue(r.Context(), "credentials", claims)
	return
}

func setTokenCookie(w http.ResponseWriter, r *http.Request) (ctx context.Context) {
	claims := credentials{
		r.Form["username"][0],
		true,
		jwt.StandardClaims{
			// ExpiresAt: 15000,
			// Issuer:    "test",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(hmacSampleSecret)
	fmt.Printf("%v %v", ss, err)

	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	client.Set(ctx, r.Form["username"][0]+"token", ss, 0)
	ctx = context.WithValue(r.Context(), "credentials", claims)
	return
}

// func ajaxResponse(w http.ResponseWriter, res map[string]string) {
// 	w.Header().Set("Content-Type", "application/json")
// 	err := json.NewEncoder(w).Encode(res)
// 	if err != nil {
// 		log.Println(err)
// 	}
// }

// func marshallJSON(r *http.Request) (*credentials, error) {
// 	t := &credentials{}
// 	decoder := json.NewDecoder(r.Body)
// 	defer r.Body.Close()
// 	err := decoder.Decode(t)
// 	if err != nil {
// 		return t, err
// 	}
// 	return t, nil
// }
//

func init() {
	makeTags()
}

func makeTags() {
	for i := 0; i < 50; i++ {
		tags := makeTagsForPost()
		for _, tag := range tags {
			fmt.Println(tag)
			_, err := client.ZAdd(ctx, "TAGS", makeZmem(tag)).Result()
			if err != nil {
				log.Println(err)
			}

		}
		makePosts(tags)
	}
	genReplies()
	fmt.Println("done")
	// tagmem, _ := client.ZRevRangeByScoreWithScores("TAGS", redis.ZRangeBy{Max: "100000"}).Result()
}

func makeZmem(st string) *redis.Z {
	return &redis.Z{
		Member: st,
		Score:  0,
	}
}

type postData struct {
	Body   template.HTML `json:"body"`
	ID     string        `json:"ID"`
	Tags   string        `json:"tags"`
	TS     string        `json:"created"`
	Author string        `json:"author"`
	Title  string        `json:"title"`
	Parent string        `json:"parent"`
}

var ctx = context.Background()

func bubbleUp(parent string, newPostAuthor string) {
	author, err := client.HMGet(ctx, "OBJECT:"+parent, "author").Result()
	if err != nil {
		fmt.Println(err)
	}
	if a, ok := author[0].(string); ok && len(a) > 2 {
		grandParent, err := client.HMGet(ctx, "OBJECT:"+parent, "parent").Result()
		if err != nil {
			fmt.Println(err)
		}

		if g, ok := grandParent[0].(string); ok && len(g) > 2 {
			if a == newPostAuthor {
				bubbleUp(g, newPostAuthor)
				return
			}
			client.ZIncrBy(ctx, "USERS", 1, a)
			client.ZIncrBy(ctx, g+":CHILDREN", 1, parent)
			bubbleUp(g, newPostAuthor)
		} else {
			client.ZIncrBy(ctx, "USERS", 1, a)
			tags, err := client.HMGet(ctx, "OBJECT:"+parent, "tags").Result()
			if err != nil {
				fmt.Println(err)
			}
			var tagsm []string
			_ = json.Unmarshal([]byte(tags[0].(string)), &tagsm)
			for _, tag := range tagsm {
				_, err := client.ZIncrBy(ctx, "TAGS", 1, tag).Result()
				if err != nil {
					fmt.Println(err)
				}
				_, err = client.ZIncrBy(ctx, tag, 1, parent).Result()
				if err != nil {
					fmt.Println(err)
				}

			}
			_, err = client.ZIncrBy(ctx, "ALLPOSTS", 1, parent).Result()
			if err != nil {
				fmt.Println(err)
			}

		}
	}
}

var pBody = parseMentions(`Nasdaq, Inc. is an American multinational financial services corporation that owns and operates three stock exchanges in the United States: the namesake Nasdaq stock exchange, the Philadelphia Stock Exchange, and the Boston Stock Exchange, and seven European stock exchanges: Nasdaq Copenhagen, Nasdaq Helsinki, Nasdaq Iceland, Nasdaq Riga, Nasdaq Stockholm, Nasdaq Tallinn, and Nasdaq Vilnius. It is headquartered in New York City, and its president and chief executive officer is Adena Friedman.

Historically, the European operations have been known by @rememberme the company name OMX AB (Aktiebolaget Optionsmäklarna/Helsinki Stock Exchange), which was created in 2003 upon a merger between OM AB and HEX plc. The operations have been part of Nasdaq, Inc. (formerly known as Nasdaq OMX Group) since February 2008.[2] They are now known as Nasdaq Nordic, which provides financial services and operates marketplaces for securities in the Nordic and Baltic regions of Europe.`)

func makePosts(tags []string) {
	// fmt.Println(string(makeTagsForPost()))
	b_tags, err := json.Marshal(tags)
	if err != nil {
		fmt.Printf("Error: %s", err.Error())
	}
	postID := genPostID(15)
	post := postData{
		Title:  "This is a post title",
		Body:   template.HTML(pBody),
		ID:     postID,
		Tags:   string(b_tags),
		TS:     time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		Author: authors[rand.Intn(len(authors))],
	}
	var newMap map[string]interface{}
	data, _ := json.Marshal(post)
	json.Unmarshal(data, &newMap)
	client.HMSet(ctx, "OBJECT:"+postID, newMap)

	client.ZAdd(ctx, post.Author+":POSTS:", makeZmem(postID))
	client.ZAdd(ctx, "ALLPOSTS", makeZmem(postID))

	for _, tag := range tags {
		client.ZAdd(ctx, tag, makeZmem(postID))
	}
}

var authors = []string{"John", "Mathew", "Mark", "Luke", "Amanda", "Michelle"}

func makeReply(parent string, ptags []string) {
	postID := genPostID(15)
	post := postData{
		Title:  "This is a post title",
		Body:   template.HTML(pBody),
		ID:     postID,
		Parent: parent,
		TS:     time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		Author: authors[rand.Intn(len(authors))],
	}
	bubbleUp(parent, post.Author)
	var newMap map[string]interface{}
	data, _ := json.Marshal(post)
	json.Unmarshal(data, &newMap)
	client.HMSet(ctx, "OBJECT:"+postID, newMap)

	client.ZAdd(ctx, post.Author+":POSTS:", makeZmem(postID))
	client.ZAdd(ctx, parent+":CHILDREN", makeZmem(postID))
	if len(ptags) >= 1 {
		_, err := client.ZIncrBy(ctx, "ALLPOSTS", 1, parent).Result()
		if err != nil {
			fmt.Println(err)
			return
		}
		for _, tag := range ptags {
			_, err := client.ZIncrBy(ctx, tag, 1, parent).Result()
			if err != nil {
				fmt.Println(err)
				return
			}
		}
	}

}

func parseMentions(s string) string {
	s = html.EscapeString(s)
	w := strings.Split(s, " ")
	for i, e := range w {
		if e[0:1] == "@" {
			w[i] = `<div class="mention" onclick="viewUser('` + e[1:] + `')">` + e + "</div>"
		}
	}
	return strings.Join(w, " ")
}

func makeTagsForPost() []string {
	tags := []string{"politics", "stem", "arts", "other", "sports"}
	randomElement := rand.Intn(len(tags)) + 1
	a := make([]string, randomElement)
	for i := 0; i < len(a); i++ {
		b := rand.Intn(len(tags))
		if contains(a, tags[b]) {
			i--
			continue
		} else {
			a[i] = tags[b]
		}
	}
	return a
}

func contains(s []string, st string) bool {
	for _, v := range s {
		if v == st {
			return true
		}
	}
	return false
}

// genPostID generates a post ID
func genPostID(length int) (ID string) {
	symbols := "abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := 0; i <= length; i++ {
		s := rand.Intn(len(symbols))
		ID += symbols[s : s+1]
	}
	return
}

func genReplies() {
	tags := []string{"politics", "stem", "arts", "other", "sports"}
	for _, tag := range tags {
		postIDs, err := client.ZRange(ctx, tag, 0, -1).Result()
		if err != nil {
			fmt.Println(err)
		}
		for range postIDs {
			makeReply(postIDs[rand.Intn(len(postIDs)-1)], []string{tag})
		}
	}

}
