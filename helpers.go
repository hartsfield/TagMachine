package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

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

	// client.Set(claims.Name+"token", ss, 0)
	ctx = context.WithValue(r.Context(), "credentials", claims)
	return
}

func setTokenCookie(w http.ResponseWriter, r *http.Request, c *credentials) (ctx context.Context) {
	log.Println(c.Name, "89h9h98h98h98h9h")
	claims := credentials{
		c.Name,
		"",
		true,
		[]string{},
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

	// client.Set(c.Name+"token", ss, 0)
	ctx = context.WithValue(r.Context(), "credentials", claims)
	return
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

// func init() {
// 	posts, _ := client.ZRevRangeByScore("FRONTPAGE:", redis.ZRangeBy{Max: "100000"}).Result()
// 	for _, post := range posts {
// 		data, _ := client.HGetAll("POSTS:" + post).Result()
// 		Posts["FRONTPAGE"] = append(Posts["FRONTPAGE"], &postData{
// 			ID:     data["ID"],
// 			Body:   data["body"],
// 			Author: data["author"],
// 		})
// 	}
// }

func ajaxResponse(w http.ResponseWriter, res map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(res)
	if err != nil {
		log.Println(err)
	}
}

func marshalpostData(r *http.Request) (*postData, error) {
	t := &postData{}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(t)
	if err != nil {
		return t, err
	}
	return t, nil
}

func marshalCredentials(r *http.Request) (*credentials, error) {
	t := &credentials{}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(t)
	if err != nil {
		return t, err
	}
	return t, nil
}

// bubbleUp increments the scores of all the parents when a post is replied
// to. Also increments the reply count.
func bubbleUp(parent string) {
	pa := client.HMGet(parent, "ID").Val()
	client.ZIncrBy(pa[0].(string), 1, "Score")
	client.Incr("replyCount:" + parent)
	grandParent, err := client.HMGet(parent, "parent").Result()
	if err == nil && grandParent[0] != nil {
		bubbleUp(grandParent[0].(string))
		client.ZIncrBy(grandParent[0].(string), 1, parent)
	}
	return
}

func makePost(data map[string]string) *postData {
	var arr []string
	_ = json.Unmarshal([]byte(data["tags"]), &arr)
	return &postData{
		ID:       data["ID"],
		Title:    data["title"],
		Body:     data["body"],
		Children: getChildren(data["ID"]),
		Parent:   data["parent"],
		TS:       data["created"],
		Author:   data["author"],
		Tags:     arr,
		// Title:  data["title"],
		// Random: keyMap[ip].Key,
		// ReplyCount: client.Get("replyCount:" + data["ID"]).Val()
		// Tags:       strings.Split(data["tags"], " "),
	}
}
