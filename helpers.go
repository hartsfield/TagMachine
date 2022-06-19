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

	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
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

func renewToken(w http.ResponseWriter, r *http.Request, claims *credentials) (ctxx context.Context) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		fmt.Println(err)
	}

	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	client.Set(ctx, claims.Name+":token", ss, 0)
	ctxx = context.WithValue(r.Context(), ctxkey, claims)
	return
}

func setTokenCookie(w http.ResponseWriter, r *http.Request, c *credentials) (ctxx context.Context) {
	claims := credentials{
		c.Name,
		"",
		true,
		[]string{},
		0,
		jwt.StandardClaims{
			// ExpiresAt: 15000,
			// Issuer:    "test",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		fmt.Println(err)
	}

	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	client.Set(ctx, c.Name+":token", ss, -1)
	ctxx = context.WithValue(r.Context(), ctxkey, claims)
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
			if a != newPostAuthor {
				client.ZIncrBy(ctx, "USERS", 1, a)
			}
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

func makePost(data map[string]string) *postData {
	var arr []string
	_ = json.Unmarshal([]byte(data["tags"]), &arr)
	return &postData{
		ID:       data["ID"],
		Title:    data["title"],
		Body:     template.HTML(data["body"]),
		Children: getChildren(data["ID"]),
		Parent:   data["parent"],
		TS:       data["created"],
		Author:   data["author"],
		Tags:     arr,
	}
}

func trimHashTags(htags []string) []string {
	for k, tag := range htags {
		i := strings.Index(tag, "#")
		if i != -1 {
			htags[k] = tag[i+1:]
		}
	}
	return htags
}

func bytify(a any) ([]byte, error) {
	bTags, err := json.Marshal(a)
	if err != nil {
		fmt.Println(err)
		return bTags, err
	}

	return bTags, nil
}

func verifyBody(s string) bool {
	l := len(s)
	if l > 2 && l < 2500 {
		return true
	}
	return false
}

func verifyTags(tags []string) bool {
	for _, t := range tags {
		// b, _ := regexp.MatchString("^[a-zA-Z0-9_]*$", t)
		// if b {
		l := len(t)
		if l > 2 && l < 20 {
			return true
		}
		// }
	}
	return false
}

func makePage() *pageData {
	return &pageData{
		Tags:        tags,
		DefaultTags: defaultTags,
		UserData:    &credentials{},
	}
}

func exeTmpl(w http.ResponseWriter, r *http.Request, page *pageData) {
	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		page.UserData = a

		err := templates.ExecuteTemplate(w, "home.tmpl", page)
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	err := templates.ExecuteTemplate(w, "home.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}

func parseMentions(s string) string {
	s = html.EscapeString(s)
	w := strings.Split(s, " ")
	for i, e := range w {
		if len(e) > 0 && e[0:1] == "@" {
			w[i] = `<div class="mention" onclick="viewUser('` + e[1:] + `')">` + e + "</div>"
		}
		if len(e) > 0 && e[0:1] == "#" {
			w[i] = `<div class="bodyTag" onclick="setTag('` + e[1:] + `')">` + e + "</div>"
		}

	}
	return strings.Join(w, " ")
}
