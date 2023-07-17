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
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

// hashPassword takes a password string and returns a hash
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// checkPasswordHash compares a password to a hash and returns true if they
// match
func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// parseToken takes a token string, checks its validity, and parses it into a
// set of credentials. If the token is invalid it returns an error
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

// renewToken renews a users token using existing claims, sets it as a cookie
// on the client, and adds it to the database.
// TODO: FIX EXPIRY
func renewToken(w http.ResponseWriter, r *http.Request, claims *credentials) (ctxx context.Context) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		fmt.Println(err)
	}

	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	rdb.Set(rdbctx, claims.Name+":token", ss, 0)
	ctxx = context.WithValue(r.Context(), ctxkey, claims)
	return
}

// newClaims creates a new set of claims using user credentials, and uses
// the claims to create a new token using renewToken()
func newClaims(w http.ResponseWriter, r *http.Request, c *credentials) (ctxx context.Context) {
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

	return renewToken(w, r, &claims)
	// token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// ss, err := token.SignedString(hmacSampleSecret)
	// if err != nil {
	// 	fmt.Println(err)
	// }

	// expire := time.Now().Add(10 * time.Minute)
	// cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	// http.SetCookie(w, &cookie)

	// rdb.Set(ctx, c.Name+":token", ss, -1)
	// ctxx = context.WithValue(r.Context(), ctxkey, claims)
	// return
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

// marshalpostData is used convert a request body into a postData{} struct
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

// type pageNum struct {
// 	Number   string `json:"lastPost,number"`
// 	PageName string `json:"pageName"`
// }

func marshalPageData(r *http.Request) (*pageData, error) {
	t := &pageData{}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(t)
	if err != nil {
		return t, err
	}
	return t, nil
}

// marshalCredentials is used convert a request body into a credentials{}
// struct
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

// bubbleUp increments the scores of all the parents when a post is replied to.
// The logic here is DELICATE.
func bubbleUp(parent string, newPostAuthor string) {
	// Get the parentAuthor of parent
	parentAuthor, err := rdb.HMGet(rdbctx, "OBJECT:"+parent, "author").Result()
	handleErr(err)

	if a, ok := parentAuthor[0].(string); ok && len(a) > 2 {
		// Get the parent of parent (the grandparent)
		grandParent, err := rdb.HMGet(rdbctx, "OBJECT:"+parent, "parent").Result()
		handleErr(err)

		// Don't increment the score of the post if its author is the
		// the same as newPostAuthor
		if g, ok := grandParent[0].(string); ok && len(g) > 2 {
			if a == newPostAuthor {
				bubbleUp(g, newPostAuthor)
				return
			}
			// Increment the users score
			rdb.ZIncrBy(rdbctx, "USERS", 1, a)
			// Increment the parent
			rdb.ZIncrBy(rdbctx, g+":CHILDREN", 1, parent)
			// Run this same function again (recursively) to
			// increment each comment north of the parent
			bubbleUp(g, newPostAuthor)
		} else {
			// We get here when we reach the top of the tree. This
			// should be the first post of a thread.
			if a != newPostAuthor {
				// increment the score if the author isn't
				// the same as the poster
				rdb.ZIncrBy(rdbctx, "USERS", 1, a)
				// We only need to do this once
				// TODO: Consider making "ALLPOSTS" just another tag
				_, err = rdb.ZIncrBy(rdbctx, "ALLPOSTS", 1, parent).Result()
				handleErr(err)

				// Get the tags from the post and increment their score
				tags, err := rdb.HMGet(rdbctx, "OBJECT:"+parent, "tags").Result()
				handleErr(err)

				var tagsm []string
				_ = json.Unmarshal([]byte(tags[0].(string)), &tagsm)
				for _, tag := range tagsm {
					_, err := rdb.ZIncrBy(rdbctx, "TAGS", 1, tag).Result()
					handleErr(err)
					// In this case, the parent is the original
					// poster, and the thread is posted on each
					// tag, so we increment "parent" for each tag.
					_, err = rdb.ZIncrBy(rdbctx, tag, 1, parent).Result()
					handleErr(err)
				}
			}
		}
	}
}

// trimHashTags trims everything before the last "#" on each tag and then
// removes any duplicates.
func trimHashTags(htags []string) []string {
	for k, tag := range htags {
		i := strings.LastIndex(tag, "#")
		if i != -1 {
			htags[k] = tag[i+1:]
		}
	}
	return removeDuplicateStr(htags)
}

// removeDuplicateStr removes duplicate strings from a slice of strings
// [0] https://stackoverflow.com/questions/66643946/how-to-remove-duplicates-strings-or-int-from-slice-in-go
func removeDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range strSlice {
		item = strings.ToLower(item)
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

// bytify is used to store tags in redis. Redis can't store a slice/array as
// a value to a key of a hash, and storing it as a JSON string comes out
// wonky(?), so we store it as a []byte and convert it to a slice using the
// JSON marshaler
func bytify(a any) ([]byte, error) {
	bTags, err := json.Marshal(a)
	if err != nil {
		fmt.Println(err)
		return bTags, err
	}

	return bTags, nil
}

// validateBody performs a sanity check on the post body. Currently it just
// makes sure the body is greater than two characters and less than 2500.
// TODO: Define the parameters of sanity (using regexp?)
func validateBody(s string) bool {
	l := len(s)
	if l > 2 && l < 2500 {
		return true
	}
	return false
}

// validateTags performs a sanity check on the tags. Currently it just makes
// sure the tag is greater than two characters and less than twenty.
// TODO: Define the parameters of sanity (using regexp?)
func validateTags(tags []string) bool {
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

// makePage returns a *pageData{} struct
func makePage() *pageData {
	return &pageData{
		Tags:        tags,
		DefaultTags: defaultTags,
		UserData:    &credentials{},
	}
}

// ajaxResponse is used to respond to ajax requests with arbitrary data in the
// format of map[string]string
func ajaxResponse(w http.ResponseWriter, res map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(res)
	if err != nil {
		log.Println(err)
	}
}

// exeTmpl is used to build and execute an html template.
func exeTmpl(w http.ResponseWriter, r *http.Request, page *pageData, tmpl string) {
	// Add the user data to the page if they're logged in.
	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		page.UserData = a

		err := templates.ExecuteTemplate(w, tmpl, page)
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	err := templates.ExecuteTemplate(w, tmpl, page)
	if err != nil {
		fmt.Println(err)
	}
}

// parseBody parses the post body for @mentions, and #hashtags and surrounds
// them with html for styling and functionality.
// TODO: Make sure the user exists?
func parseBody(s string) string {
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

// handleErr is used for handling errors. It needs to be more robust.
// TODO: Add robustness
func handleErr(e error) {
	if e != nil {
		fmt.Println(e)
	}
}

// Increment or add tags to the database
func processTags(incomingTags []string, postID string) error {
	zmem := makeZmem(postID)
	for _, tag := range incomingTags {
		// increment the tag. If it doesn't exist add it to the
		// database
		_, err := rdb.ZIncrBy(rdbctx, "TAGS", 1, tag).Result()
		if err != nil {
			fmt.Println(err)
			// add a new tag to "TAGS"
			rdb.ZAdd(rdbctx, "TAGS", makeZmem(tag))
		}

		// Add a reference to the postID as a Zmember to each
		// tag to retrieve posts by tag name
		// Ex. "zrannge "POLITICS" 0 -1" should return a list
		// postID's that are tagged with "POLITICS"
		_, err = rdb.ZAdd(rdbctx, tag, zmem).Result()
		if err != nil {
			return err
		}
	}
	return nil
}

func addPostToDB(post map[string]interface{}, authorName string, postID string) error {
	zmem := makeZmem(postID)
	// TODO: Create database pipeline/reversal
	// Add the post to redis with "OBJECT:postID" as the key
	_, err := rdb.HMSet(rdbctx, "OBJECT:"+postID, post).Result()
	if err != nil {
		return err
	}

	// Add a reference to the postID to "username:POSTS" to
	// retrieve posts by username
	// Ex. "zrange bobby:POSTS 0 -1" should return a list of
	// postIDs from user "bobby"
	_, err = rdb.ZAdd(rdbctx, authorName+":POSTS", zmem).Result()
	if err != nil {
		return err
	}

	if post["type"] == "thread" {
		// If it's a new thread:
		// Add a reference to the postID to "ALLPOSTS". This is used to
		// easily retrieve all threads at once.
		// Ex. "zrange ALLPOSTS 0 -1" should return a list containing
		// the postIDs of every threads
		_, err = rdb.ZAdd(rdbctx, "ALLPOSTS", zmem).Result()
		if err != nil {
			return err
		}
	} else {
		// If it's not a thread it's a reply to a thread
		// This is a reply, so we add it as a child to the parent post
		_, err = rdb.ZAdd(rdbctx, post["parent"].(string)+":CHILDREN", zmem).Result()
		if err != nil {
			return err
		}

		// bubbleUp increments the post count of all the parent posts,
		// all the way up to the original post, but will skip posts
		// made by the user (you can't upvote yourself)
		bubbleUp(post["parent"].(string), authorName)

	}
	// rebuild the cache
	beginCache()

	return nil
}

func getSecret() (testPass string) {
	testPass = os.Getenv("testPass")
	if len(testPass) < 10 {
		log.Panic(`Testing password rejected, please see README for 
instructions on running TagMachine.`)
	}
	return
}

// makeZmem returns a redis Z member for use in a ZSET. Score is set to zero
func makeZmem(st string) *redis.Z {
	return &redis.Z{
		Member: st,
		Score:  0,
	}
}

// isDefaultTag checks to see if a string matches a default tag and returns
// true if it does
func isDefaultTag(tag string) bool {
	for _, dtag := range defaultTags {
		if dtag == tag {
			return true
		}
	}
	return false
}

// makePost takes data in the form of a map[string]string, and returns a
// *postData{} struct. Use withChildren to specify whether or not to also get
// the children. If withChildren is true, getChildren() will be run, which is
// a recursive function, and should only be run when necessary.
func makePost(data map[string]string, withChildren bool) *postData {
	var sl []string
	_ = json.Unmarshal([]byte(data["tags"]), &sl)
	if withChildren {
		return &postData{
			ID:    data["ID"],
			Title: data["title"],
			Body:  template.HTML(data["body"]),
			// NOTE: Recursive
			Children: getChildren(data["ID"]),
			Parent:   data["parent"],
			TS:       data["created"],
			Author:   data["author"],
			Tags:     sl,
		}
	}
	return &postData{
		ID:       data["ID"],
		Title:    data["title"],
		Body:     template.HTML(data["body"]),
		Children: nil,
		Parent:   data["parent"],
		TS:       data["created"],
		Author:   data["author"],
		Tags:     sl,
	}
}

// getChildren takes a postID, retrieves the replies, and returns them as a
// slice
func getChildren(ID string) (childs []*postData) {
	// get the postIDs of the children
	children, err := rdb.ZRevRange(rdbctx, ID+":CHILDREN", 0, -1).Result()
	if err != nil {
		fmt.Println(err)
	}

	// look up each postID to get the post data for the children
	for _, child := range children {
		data, err := rdb.HGetAll(rdbctx, "OBJECT:"+child).Result()
		if err != nil {
			fmt.Println(err)
		}

		// append the child to the comment tree
		// NOTE: Recursive
		childs = append(childs, makePost(data, true))
	}
	return
}
