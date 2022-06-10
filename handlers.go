package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-redis/redis"
)

func signin(w http.ResponseWriter, r *http.Request) {
	c, err := marshalCredentials(r)
	if err != nil {
		log.Println(err)
	}

	var ctx context.Context
	ctx = context.WithValue(r.Context(), "credentials", &credentials{})
	hash, err := client.Get(c.Name).Result()
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "User doesn't exist"})
	} else {
		doesMatch := CheckPasswordHash(c.Password, hash)
		if doesMatch {
			log.Println("setting cokie")
			setTokenCookie(w, r, c)
			ajaxResponse(w, map[string]string{"success": "true", "error": "false"})
		} else {
			ajaxResponse(w, map[string]string{"success": "false", "error": "Bad Password"})
			fmt.Println(ctx)
			return
		}
	}
}

func signup(w http.ResponseWriter, r *http.Request) {
	c, err := marshalCredentials(r)
	if err != nil {
		log.Println(err)
	}

	_, err = client.Get(c.Name).Result()
	if err != nil {
		fmt.Println(err)
		hash, err := HashPassword(c.Password)
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Invalid Password"})
			return
		}

		match, err := regexp.MatchString("^[A-Za-z0-9]+(?:[ _-][A-Za-z0-9]+)*$", c.Name)
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Invalid Username"})
			return
		}
		if match && (len(c.Name) < 25) {
			client.Set(c.Name, hash, 0)
			setTokenCookie(w, r, c)
			ajaxResponse(w, map[string]string{"success": "true", "error": "false"})
		}
		return
	}
	ajaxResponse(w, map[string]string{"success": "false", "error": "User Exists"})
}

func logout(w http.ResponseWriter, r *http.Request) {
	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: "loggedout", Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	ajaxResponse(w, map[string]string{"error": "false", "success": "true"})
}

func home(w http.ResponseWriter, r *http.Request) {
	page := &pageData{}
	cr := &credentials{}
	for key := range Posts {
		page.Posts = append(page.Posts, Posts[key]...)
	}
	page.Tags = tags

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		page.UserData = a

		err := templates.ExecuteTemplate(w, "home.tmpl", page)
		if err != nil {
			fmt.Println(err)
			return
		}
		return
	}
	page.UserData = cr
	err := templates.ExecuteTemplate(w, "home.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}

func checkAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := credentials{Name: "nouser", IsLoggedIn: false}
		ctx := context.WithValue(r.Context(), "credentials", user)
		token, err := r.Cookie("token")
		if err != nil {
			next.ServeHTTP(w, r.WithContext(ctx))
			fmt.Println(err)
			return
		}

		// check for token in database

		c, err := parseToken(token.Value)
		if err != nil {
			fmt.Println(err)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		c.IsLoggedIn = true
		ctx = renewToken(w, r, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func view(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	page := &pageData{}
	cr := &credentials{}
	var p *postData

	data := client.HGetAll(r.Form["postNum"][0]).Val()
	childs := getChildren(r.Form["postNum"][0])
	if data["body"] != "" && data["ID"] != "" {
		p = makePost(data)
	}

	d := &threadData{
		Thread:   p,
		Children: childs,
		Parent:   p.Parent,
	}
	page.Thread = d

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		page.UserData = a

		err := templates.ExecuteTemplate(w, "thread.tmpl", page)
		if err != nil {
			fmt.Println(err)
			return
		}
		return
	}
	page.UserData = cr

	err := templates.ExecuteTemplate(w, "thread.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}

func userPosts(w http.ResponseWriter, r *http.Request) {
	name := strings.Split(r.URL.Path, "/")[2]
	posts, _ := client.ZRevRangeByScore(name+":POSTS:", redis.ZRangeBy{Max: "100000"}).Result()
	Posts[name] = nil
	for _, post := range posts {
		data, _ := client.HGetAll(post).Result()
		Posts[name] = append(Posts[name], makePost(data))
	}

	page := &pageData{}
	page.Posts = Posts[name]

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		page.UserData = a

		err := templates.ExecuteTemplate(w, "home.tmpl", page)
		if err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	page.UserData = &credentials{}
	err := templates.ExecuteTemplate(w, "home.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}

func newThread(w http.ResponseWriter, r *http.Request) {
	p, err := marshalpostData(r)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"error": "true"})
		return
	}

	b_tags, err := json.Marshal(p.Tags)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"error": "true"})
		return
	}

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		postID := genPostID(15)
		post := map[string]interface{}{
			"title":   p.Title,
			"body":    p.Body,
			"ID":      postID,
			"created": time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
			// "parent":  "FRONTPAGE:",
			"author": a.Name,
			"tags":   b_tags,
		}

		_, err := client.HMSet(postID, post).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"error": "true"})
			return
		}

		zmem := makeZmem(postID)
		for _, tag := range p.Tags {
			zmem2 := makeZmem(tag)
			client.ZAdd("TAGS", zmem2).Result()
			client.ZAdd(tag, zmem)
		}
		client.ZAdd(a.Name+":POSTS:", zmem)

		ajaxResponse(w, map[string]string{"error": "false"})
	}
}

func newReply(w http.ResponseWriter, r *http.Request) {
	fmt.Println("reply")
	p, err := marshalpostData(r)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(p.ID)
	bubbleUp(p.ID)

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		postID := genPostID(15)
		post := map[string]interface{}{
			"body":    p.Body,
			"ID":      postID,
			"created": time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
			"parent":  p.ID,
			"author":  a.Name,
		}
		client.HMSet(postID, post)

		zmem := makeZmem(postID)
		client.ZAdd(p.ID+":CHILDREN", zmem)

		ajaxResponse(w, map[string]string{"error": "nil", "ID": postID})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func getTags(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	page := &pageData{}
	url_tags := strings.Split(r.Form["tags"][0], ",")
	fmt.Println(url_tags)

	// TODO  *IMPORTANT*
	// someone could send unknown tags in a xhr request, need to check if
	// tag is default

	for _, val := range url_tags {
		page.Posts = append(page.Posts, Posts[val]...)
	}
	page.Tags = tags

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		page.UserData = a

		err := templates.ExecuteTemplate(w, "home.tmpl", page)
		if err != nil {
			fmt.Println(err)
			return
		}
		return
	}
	page.UserData = &credentials{}
	err := templates.ExecuteTemplate(w, "home.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}
