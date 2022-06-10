package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/go-redis/redis"
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

	client.Set(claims.Name+"token", ss, 0)
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

	client.Set(r.Form["username"][0]+"token", ss, 0)
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
	for i := 0; i < 150; i++ {
		tags := makeTagsForPost()
		for _, tag := range tags {
			fmt.Println(tag)
			_, err := client.ZAdd("TAGS", makeZmem(tag)).Result()
			if err != nil {
				log.Println(err)
			}

		}
		makePosts(tags)
	}
	// tagmem, _ := client.ZRevRangeByScoreWithScores("TAGS", redis.ZRangeBy{Max: "100000"}).Result()
}

func makeZmem(st string) redis.Z {
	return redis.Z{
		Member: st,
		Score:  0,
	}
}

type postData struct {
	Body   string `json:"body"`
	ID     string `json:"ID"`
	Tags   string `json:"tags"`
	TS     string `json:"created"`
	Author string `json:"author"`
	Title  string `json:"title"`
}

var pBody = `Nasdaq, Inc. is an American multinational financial services corporation that owns and operates three stock exchanges in the United States: the namesake Nasdaq stock exchange, the Philadelphia Stock Exchange, and the Boston Stock Exchange, and seven European stock exchanges: Nasdaq Copenhagen, Nasdaq Helsinki, Nasdaq Iceland, Nasdaq Riga, Nasdaq Stockholm, Nasdaq Tallinn, and Nasdaq Vilnius. It is headquartered in New York City, and its president and chief executive officer is Adena Friedman.

Historically, the European operations have been known by the company name OMX AB (Aktiebolaget Optionsmäklarna/Helsinki Stock Exchange), which was created in 2003 upon a merger between OM AB and HEX plc. The operations have been part of Nasdaq, Inc. (formerly known as Nasdaq OMX Group) since February 2008.[2] They are now known as Nasdaq Nordic, which provides financial services and operates marketplaces for securities in the Nordic and Baltic regions of Europe.`

func makePosts(tags []string) {
	// fmt.Println(string(makeTagsForPost()))
	b_tags, err := json.Marshal(tags)
	if err != nil {
		fmt.Printf("Error: %s", err.Error())
	}
	postID := genPostID(15)
	post := postData{
		Title:  "This is a post title",
		Body:   pBody,
		ID:     postID,
		Tags:   string(b_tags),
		TS:     time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		Author: "JOHN",
	}
	var newMap map[string]interface{}
	data, _ := json.Marshal(post)
	json.Unmarshal(data, &newMap)
	client.HMSet(postID, newMap)

	client.ZAdd("JOHN:POSTS:", makeZmem(postID))

	for _, tag := range tags {
		client.ZAdd(tag, makeZmem(postID))
	}
}

func makeTagsForPost() []string {
	tags := []string{"politics", "stem", "arts", "other", "sports"}
	a := make([]string, rand.Intn(len(tags)))
	for i := range a {
		a[i] = tags[rand.Intn(len(tags))]
	}
	return a
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
