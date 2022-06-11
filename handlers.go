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
	// TODO: remove cookie from db

	ajaxResponse(w, map[string]string{"error": "false", "success": "true"})
}

func home(w http.ResponseWriter, r *http.Request) {
	page := &pageData{}
	cr := &credentials{}
	// for key := range Posts {
	// 	page.Posts = append(page.Posts, Posts[key]...)
	// }
	page.Posts = Frontpage["all"]
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
	err := r.ParseForm()
	if err != nil {
		fmt.Println(err)
	}

	page := &pageData{}
	page.Tags = tags
	cr := &credentials{}
	var p *postData

	data, err := client.HGetAll("OBJECT:" + r.Form["postNum"][0]).Result()
	if err != nil {
		fmt.Println(err)
	}

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

	err = templates.ExecuteTemplate(w, "thread.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}

func userPosts(w http.ResponseWriter, r *http.Request) {
	name := strings.Split(r.URL.Path, "/")[2]
	posts, err := client.ZRevRange(name+":POSTS:", 0, -1).Result()
	if err != nil {
		fmt.Println(err)
	}

	Posts[name] = nil
	for _, post := range posts {
		data, err := client.HGetAll("OBJECT:" + post).Result()
		if err != nil {
			fmt.Println(err)
		}

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
	err = templates.ExecuteTemplate(w, "home.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}

func newThread(w http.ResponseWriter, r *http.Request) {
	p, err := marshalpostData(r)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "Bad JSON sent to server"})
		return
	}

	b_tags, err := json.Marshal(p.Tags)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "Bad JSON in tags"})
		return
	}

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn && len(p.Body) > 8 {
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

		_, err := client.HMSet("OBJECT:"+postID, post).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
			return
		}

		zmem := makeZmem(postID)
		for _, tag := range p.Tags {
			_, err := client.ZIncrBy("TAGS", 1, tag).Result()
			if err != nil {
				fmt.Println(err)
				ajaxResponse(w, map[string]string{"success": "false", "error": "Bad tag"})
				return
			}

			_, err = client.ZAdd(tag, zmem).Result()
			if err != nil {
				fmt.Println(err)
				ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
				return
			}

		}
		_, err = client.ZAdd(a.Name+":POSTS:", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
			return
		}

		_, err = client.ZAdd("ALLPOSTS", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "Error setting database object"})
			return
		}

		ajaxResponse(w, map[string]string{"success": "true", "error": "nil"})
	}
}

func newReply(w http.ResponseWriter, r *http.Request) {
	fmt.Println("reply")
	p, err := marshalpostData(r)
	if err != nil {
		fmt.Println(err)
		ajaxResponse(w, map[string]string{"success": "false", "error": "Bad data"})
		return
	}

	postID := genPostID(15)
	bubbleUp(p.ID, postID, p.Tags)

	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		post := map[string]interface{}{
			"body":    p.Body,
			"ID":      postID,
			"created": time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
			"parent":  p.ID,
			"author":  a.Name,
		}
		_, err = client.HMSet("OBJECT:"+postID, post).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
			return
		}

		zmem := makeZmem(postID)
		_, err = client.ZAdd(p.ID+":CHILDREN", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
			return
		}

		_, err := client.ZAdd(a.Name+":POSTS:", zmem).Result()
		if err != nil {
			fmt.Println(err)
			ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
			return
		}

		if len(p.Tags) >= 1 {
			_, err := client.ZIncrBy("ALLPOSTS", 1, p.ID).Result()
			if err != nil {
				fmt.Println(err)
				ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
				return
			}
			for _, tag := range p.Tags {
				_, err := client.ZIncrBy(tag, 1, p.ID).Result()
				if err != nil {
					fmt.Println(err)
					ajaxResponse(w, map[string]string{"success": "false", "error": "db error"})
					return
				}
			}
		}

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

	page := &pageData{}
	url_tags := strings.Split(r.Form["tags"][0], ",")

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
	err = templates.ExecuteTemplate(w, "home.tmpl", page)
	if err != nil {
		fmt.Println(err)
	}
}
