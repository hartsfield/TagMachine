package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

func signin(w http.ResponseWriter, r *http.Request) {
	c, err := marshalCredentials(r)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "Invalid Credentials"})
		return
	}

	var ctx context.Context
	ctx = context.WithValue(r.Context(), ctxkey, &credentials{})
	hash, err := client.Get(ctx, c.Name).Result()
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "User doesn't exist"})
		return
	}
	doesMatch := checkPasswordHash(c.Password, hash)
	if doesMatch {
		setTokenCookie(w, r, c)
		ajaxResponse(w, map[string]string{"success": "true", "error": "false"})
		return
	}
	ajaxResponse(w, map[string]string{"success": "false", "error": "Bad Password"})
}

func signup(w http.ResponseWriter, r *http.Request) {
	c, err := marshalCredentials(r)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "Invalid Credentials"})
		return
	}

	match, err := regexp.MatchString("^[A-Za-z0-9]+(?:[ _-][A-Za-z0-9]+)*$", c.Name)
	if match && err == nil && (len(c.Name) < 25) && (len(c.Name) > 3) && (len(c.Password) > 7) {
		_, err = client.Get(context.Background(), c.Name).Result()
		if err != nil {
			fmt.Println(err)
			hash, err := hashPassword(c.Password)
			if err != nil {
				fmt.Println(err)
				ajaxResponse(w, map[string]string{"success": "false", "error": "Invalid Password"})
				return
			}

			client.Set(context.Background(), c.Name, hash, 0)
			setTokenCookie(w, r, c)
			_, err = client.ZAdd(ctx, "USERS", makeZmem(c.Name)).Result()
			if err != nil {
				fmt.Println(err)
				ajaxResponse(w, map[string]string{"success": "false", "error": "Invalid Password"})
				return
			}

			ajaxResponse(w, map[string]string{"success": "true", "error": "false"})
			return
		}
		ajaxResponse(w, map[string]string{"success": "false", "error": "User Exists"})
		return
	}
	ajaxResponse(w, map[string]string{"success": "false", "error": "Try a Different Username"})
}

func logout(w http.ResponseWriter, r *http.Request) {
	token, err := r.Cookie("token")
	if err != nil {
		fmt.Println(err)
	}

	c, err := parseToken(token.Value)
	if err != nil {
		fmt.Println(err)
	}
	client.Set(ctx, c.Name+":token", "loggedout", 0)

	expire := time.Now()
	cookie := http.Cookie{Name: "token", Value: "loggedout", Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	ajaxResponse(w, map[string]string{"error": "false", "success": "true"})
}

func checkAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := credentials{IsLoggedIn: false}
		ctx := context.WithValue(r.Context(), ctxkey, user)
		token, err := r.Cookie("token")
		if err != nil {
			next.ServeHTTP(w, r.WithContext(ctx))
			fmt.Println(err)
			return
		}

		c, err := parseToken(token.Value)
		if err != nil {
			fmt.Println(err)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		tkn, _ := client.Get(ctx, c.Name+":token").Result()
		if tkn == token.Value {

			c.IsLoggedIn = true
			ctxx := renewToken(w, r, c)
			next.ServeHTTP(w, r.WithContext(ctxx))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func home(w http.ResponseWriter, r *http.Request) {
	page := makePage()
	page.Posts = frontpage["all"]
	exeTmpl(w, r, page)
}

func view(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		fmt.Println(err)
	}

	data, err := client.HGetAll(context.Background(), "OBJECT:"+r.Form["postNum"][0]).Result()
	if err != nil {
		fmt.Println(err)
	}
	var p *postData
	if data["body"] != "" && data["ID"] != "" {
		p = makePost(data)
	}

	childs := getChildren(r.Form["postNum"][0])
	d := &threadData{
		Thread:   p,
		Children: childs,
		Parent:   p.Parent,
	}
	page := makePage()
	page.Thread = d
	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		page.UserData = a

		err := templates.ExecuteTemplate(w, "thread.tmpl", page)
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	err = templates.ExecuteTemplate(w, "thread.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}

func userPosts(w http.ResponseWriter, r *http.Request) {
	name := strings.Split(r.URL.Path, "/")[2]
	dbposts, err := client.ZRevRange(context.Background(), name+":POSTS:", 0, -1).Result()
	if err != nil {
		fmt.Println(err)
		return
	}

	posts[name] = nil
	for _, post := range dbposts {
		data, err := client.HGetAll(context.Background(), "OBJECT:"+post).Result()
		if err != nil {
			fmt.Println(err)
		}

		posts[name] = append(posts[name], makePost(data))
	}

	page := makePage()
	page.Posts = posts[name]

	exeTmpl(w, r, page)
}

func newThread(w http.ResponseWriter, r *http.Request) {
	p, err := marshalpostData(r)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "Bad JSON sent to server"})
		return
	}

	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		if !verifyBody(string(p.Body)) || !verifyTags(p.Tags) {
			ajaxResponse(w, map[string]string{"success": "false", "error": "Text not allowed"})
			return
		}

		bTags, err := bytify(trimHashTags(p.Tags))
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Bad JSON in tags"})
			return
		}

		postID := genPostID(15)
		post := map[string]interface{}{
			"title":   p.Title,
			"body":    p.Body,
			"ID":      postID,
			"created": time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
			"author":  a.Name,
			"tags":    bTags,
		}

		_, err = client.HMSet(ctx, "OBJECT:"+postID, post).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
			return
		}

		zmem := makeZmem(postID)
		for _, tag := range p.Tags {
			_, err := client.ZIncrBy(ctx, "TAGS", 1, tag).Result()
			if err != nil {
				fmt.Println(err)
				client.ZAdd(ctx, "TAGS", makeZmem(tag))
				// ajaxResponse(w, map[string]string{"success": "false", "error": "Bad tag"})
				// return
			}

			_, err = client.ZAdd(ctx, tag, zmem).Result()
			if err != nil {
				fmt.Println(err)
				ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
				return
			}
		}
		_, err = client.ZAdd(ctx, a.Name+":POSTS:", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
			return
		}

		_, err = client.ZAdd(ctx, "ALLPOSTS", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
			return
		}

		ajaxResponse(w, map[string]string{"success": "true", "error": "nil", "postID": postID})
		beginCache()
		return
	}
	ajaxResponse(w, map[string]string{"success": "false", "error": "Error verifying data"})
}

func newReply(w http.ResponseWriter, r *http.Request) {
	p, err := marshalpostData(r)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "Bad JSON sent to server"})
		return
	}

	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		if !verifyBody(string(p.Body)) {
			ajaxResponse(w, map[string]string{"success": "false", "error": "Text not allowed"})
			return
		}
		postID := genPostID(15)
		post := map[string]interface{}{
			"body":    parseMentions(string(p.Body)),
			"ID":      postID,
			"created": time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
			"parent":  p.ID,
			"author":  a.Name,
		}
		_, err = client.HMSet(ctx, "OBJECT:"+postID, post).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
			return
		}

		zmem := makeZmem(postID)
		_, err = client.ZAdd(ctx, p.ID+":CHILDREN", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
			return
		}

		_, err = client.ZAdd(ctx, a.Name+":POSTS:", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
			return
		}

		bubbleUp(p.ID, a.Name)
		beginCache()
		ajaxResponse(w, map[string]string{"success": "true", "error": "nil"})
		return
	}
	ajaxResponse(w, map[string]string{"error": "not logged in", "success": "false"})
}

func getTags(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		fmt.Println(err)
	}
	urlTags := strings.Split(r.Form["tags"][0], ",")

	page := makePage()
	client.ZUnionStore(ctx, "tempstore", &redis.ZStore{Keys: urlTags}).Result()
	dbposts, _ := client.ZRevRange(ctx, "tempstore", 0, -1).Result()
	for _, dbpost := range dbposts {
		obj, _ := client.HGetAll(ctx, "OBJECT:"+dbpost).Result()
		page.Posts = append(page.Posts, makePost(obj))
	}

	exeTmpl(w, r, page)
}
