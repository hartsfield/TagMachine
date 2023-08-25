package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

///////////////////////////////////////////////////////////////////////////////
// Auth Routes ////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////////

// signin signs a user in. It's a response to an XMLHttpRequest (AJAX request)
// containing the user credentials. It responds with a map[string]string that
// can be converted to JSON by the client. The client expects a boolean
// indicating success or error, and a possible error string.
func signin(w http.ResponseWriter, r *http.Request) {
	// Marshal the Credentials into a credentials struct
	c, err := marshalCredentials(r)
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Invalid Credentials",
		})
		return
	}

	// Get the passwords hash from the database by looking up the users
	// name
	hash, err := rdb.Get(rdbctx, c.Name).Result()
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "User doesn't exist",
		})
		return
	}

	// Check if password matches by hashing it and comparing the hashes
	doesMatch := checkPasswordHash(c.Password, hash)
	if doesMatch {
		newClaims(w, r, c)
		ajaxResponse(w, map[string]string{
			"success": "true",
			"error":   "false",
		})
		return
	}
	ajaxResponse(w, map[string]string{"success": "false", "error": "Bad Password"})
}

// signup signs a user up. It's a response to an XMLHttpRequest (AJAX request)
// containing new user credentials. It responds with a map[string]string that
// can be converted to JSON. The client expects a boolean indicating success or
// error, and a possible error string.
func signup(w http.ResponseWriter, r *http.Request) {
	// Marshal the Credentials into a credentials struct
	c, err := marshalCredentials(r)
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Invalid Credentials",
		})
		return
	}

	// Make sure the username doesn't contain forbidden symbols
	match, err := regexp.MatchString("^[A-Za-z0-9]+(?:[ _-][A-Za-z0-9]+)*$", c.Name)
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Invalid Username",
		})
		return
	}

	// Make sure the username is longer than 3 characters and shorter than
	// 25, and the password is longer than 7.
	if match && (len(c.Name) < 25) && (len(c.Name) > 3) && (len(c.Password) > 7) {
		// Check if user already exists
		_, err = rdb.Get(rdbctx, c.Name).Result()
		if err != nil {
			// If username is unique and valid, we attempt to hash
			// the password
			hash, err := hashPassword(c.Password)
			if err != nil {
				log.Println(err)
				ajaxResponse(w, map[string]string{
					"success": "false",
					"error":   "Invalid Password",
				})
				return
			}

			// Add the user the the USERS set in redis. This
			// associates a score with the user that can be
			// incremented or decremented
			_, err = rdb.ZAdd(rdbctx, "USERS", makeZmem(c.Name)).Result()
			if err != nil {
				log.Println(err)
				ajaxResponse(w, map[string]string{
					"success": "false",
					"error":   "Error ",
				})
				return
			}

			// If the password is hashable, and we were able to add
			// the user to the redis ZSET, we store the hash in the
			// database with the username as the key and the hash
			// as the value thats returned by the key.
			_, err = rdb.Set(rdbctx, c.Name, hash, 0).Result()
			if err != nil {
				log.Println(err)
				ajaxResponse(w, map[string]string{
					"success": "false",
					"error":   "Error ",
				})
				return
			}

			// Set user token/credentials
			newClaims(w, r, c)

			// success response
			ajaxResponse(w, map[string]string{
				"success": "true",
				"error":   "false",
			})
			return
		}
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "User Exists",
		})
		return
	}
	ajaxResponse(w, map[string]string{
		"success": "false",
		"error":   "Invalid Username",
	})
}

// logout logs the user out by overwriting the token. It must first validate
// the existing token to get the username to overwrite the old token in the
// database
func logout(w http.ResponseWriter, r *http.Request) {
	token, err := r.Cookie("token")
	if err != nil {
		log.Println(err)
	}

	c, err := parseToken(token.Value)
	if err != nil {
		log.Println(err)
	}
	rdb.Set(rdbctx, c.Name+":token", "loggedout", 0)

	expire := time.Now()
	cookie := http.Cookie{
		Name:    "token",
		Value:   "loggedout",
		Path:    "/",
		Expires: expire,
		MaxAge:  0,
	}
	http.SetCookie(w, &cookie)

	ajaxResponse(w, map[string]string{"error": "false", "success": "true"})
}

// checkAuth parses and renews the authentication token, and adds it to the
// context. checkAuth is used as a middleware function for routes that allow or
// require authentication.
func checkAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// create a generic user object that not signed in to be used
		// as a placeholder until credentials are verified.
		user := credentials{IsLoggedIn: false}
		// ctx is a user who isn't logged in
		ctx := context.WithValue(r.Context(), ctxkey, user)

		// get the "token" cookie
		token, err := r.Cookie("token")
		if err != nil {
			log.Println(err)
			// anonSignin(w, r)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// parse the "token" cookie, making sure it's valid, and
		// obtaining user credentials if it is
		c, err := parseToken(token.Value)
		if err != nil {
			log.Println(err)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// check if "token" cookie matches the token stored in the
		// database
		tkn, err := rdb.Get(ctx, c.Name+":token").Result()
		if err != nil {
			log.Println(err)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// if the tokens match we renew the token and mark the user as
		// logged in
		if tkn == token.Value {
			c.IsLoggedIn = true
			ctxx := renewToken(w, r, c)
			next.ServeHTTP(w, r.WithContext(ctxx))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

///////////////////////////////////////////////////////////////////////////////
// Page Views /////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////////

// home serves the home page, which is a collection of all posts/tags
func home(w http.ResponseWriter, r *http.Request) {
	page := makePage()
	if len(frontpage["all"]) > 0 {
		page.Posts = frontpage["all"][0:len(frontpage["all"])]
	} else {
		page.Posts = frontpage["all"]
	}
	page.PageName = "frontpage"
	page.PageNumber = 1
	exeTmpl(w, r, page, "home.tmpl")
}

// view is a single thread or comment view
// Ex. tagmachine.spicy/view/?postNum=tstaxyVnacn02Iu6
func view(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Println(err)
	}

	// look up the postNum/postID, and retrieve the post data if it's valid
	data, err := rdb.HGetAll(context.Background(), "OBJECT:"+r.Form["postNum"][0]).Result()
	if err != nil {
		log.Println(err)
	}

	// Make sure the post has content, (if for some reason redis should
	// return an empty object), then we create a postData{} struct so we
	// can start passing it around
	if data["body"] != "" && data["ID"] != "" {
		var p *postData
		p = makePost(data, true)
		// create the thread data by getting the children of the post
		childs := getChildren(r.Form["postNum"][0])
		d := &threadData{
			Thread:   p,
			Children: childs,
			Parent:   p.Parent,
		}
		page := makePage()
		page.Thread = d
		page.PageName = "thread"
		exeTmpl(w, r, page, "thread.tmpl")

	} else {
		w.Write([]byte("you broke it"))
	}

}

// userPosts returns a list of posts by a specified user to the client
// Ex. tagmachine.com/user/username
func userPosts(w http.ResponseWriter, r *http.Request) {
	// parse the username from the path. Here, we split the string after
	// each "/". The path must match tagmachine.org/user/USERNAME
	//
	// Could a more elegant solution be implemented? Of course.
	name := strings.Split(r.URL.Path, "/")[2]

	// retrieve a list of postIDs of posts by the user
	dbposts, err := rdb.ZRevRange(context.Background(), name+":POSTS", 0, -1).Result()
	if err != nil {
		log.Println(err)
		return
	}

	// uses the global "posts" (map[string][]*postData) to store the users
	// posts (Ex. posts["john"])
	posts[name] = nil
	for _, post := range dbposts {
		// look up each postID in dbposts to retrieve individual post
		// data
		data, err := rdb.HGetAll(context.Background(), "OBJECT:"+post).Result()
		if err != nil {
			log.Println(err)
		}

		// append the posts to the map that will be used to for the
		// page data
		posts[name] = append(posts[name], makePost(data, false))
	}

	// build and serve the page
	page := makePage()

	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		_, err := rdb.ZScore(rdbctx, a.Name+":following", name).Result()
		if err != nil {
			// not following
			page.UserView.IsFriend = false
		} else {
			// following
			page.UserView.IsFriend = true
		}
	}

	page.Posts = posts[name]
	page.PageName = "user"
	page.UserView.Name = name
	exeTmpl(w, r, page, "user.tmpl")
}

// getTags retrieves the requested tags from the database
// Ex. tagmachine.com/tag/?tags=stem,nicolas-louis
// should return posts tagged with both "stem", and "nicolas-louis"
func getTags(w http.ResponseWriter, r *http.Request) {
	// parse the tags into the slice `urlTags`
	err := r.ParseForm()
	if err != nil {
		log.Println(err)
	}
	urlTags := strings.Split(r.Form["tags"][0], ",")

	// create the page
	page := makePage()

	// serve the page with the posts
	exeTmpl(w, r, tagsUnion(urlTags, page), "home.tmpl")
}

// tagsUnion is used to retrieve the users selected tags from the database.
// It's the union of all tags in urlTags.
func tagsUnion(uTags []string, p *pageData) *pageData {
	// This function gets every value associated with each tag/key, and
	// stores them in tempstore. These are the postIDs. This is the UNION
	// of all tags in urlTags.
	rdb.ZUnionStore(rdbctx, "tempstore", &redis.ZStore{Keys: uTags}).Result()
	// Get all the postIDs from the temp store
	dbposts, _ := rdb.ZRevRange(rdbctx, "tempstore", 0, 15).Result()
	// Get each post from the database and append it to the page
	for _, dbpost := range dbposts {
		obj, _ := rdb.HGetAll(rdbctx, "OBJECT:"+dbpost).Result()
		p.Posts = append(p.Posts, makePost(obj, false))
	}
	return p
}

func rules(w http.ResponseWriter, r *http.Request) {
	page := makePage()
	page.PageName = "rules"
	exeTmpl(w, r, page, "rulesPage.tmpl")
}

func donate(w http.ResponseWriter, r *http.Request) {
	page := makePage()
	page.PageName = "donate"
	exeTmpl(w, r, page, "donatePage.tmpl")
}

func nextPage(w http.ResponseWriter, r *http.Request) {
	page, err := marshalPageData(r)
	if err != nil {
		log.Println(err)
	}

	log.Println(page.PageName)

	switch page.PageName {
	case "hasTags":
		log.Println("tags")
	case "frontpage":
		log.Println("fp")
	case "user":
		log.Println("user")
	default:
		log.Println("Linux.")
	}

	num, _ := strconv.Atoi(page.Number)
	log.Println((num*5)+1, (num*5)+5)
	page.Posts = frontpage["all"][(num*5)+1 : (num*5)+5]
	page.PageNumber = num + 1
	page.PageName = "frontpage"
	var b bytes.Buffer
	err = templates.ExecuteTemplate(&b, "nextPage.tmpl", page)
	if err != nil {
		log.Println(err)
	}
	ajaxResponse(w, map[string]string{
		"success":    "true",
		"error":      "false",
		"template":   b.String(),
		"pageNumber": log.Sprint(page.PageNumber),
	})
}

///////////////////////////////////////////////////////////////////////////////
// API End Points /////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////////

// newthread adds a thread to the database, rebuilds the cache, and sends the
// user to the post they just submitted
// Ex. tagmachine.com/api/newthread
func newThread(w http.ResponseWriter, r *http.Request) {
	// Convert the JSON sent from the client to a postData{} struct
	p, err := marshalpostData(r)
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Bad JSON sent to server",
		})
		return
	}

	// Check if the user is logged in. You can't post wothout being logged
	// in unless you're a test bot. For testing TagMachine, we'll use
	// another program called TagBot. (see: README.md).
	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn || p.Testing == testPass {
		if p.Testing == testPass {
			a = &credentials{Name: p.Author}
		}
		// Validate the data
		if !validateBody(string(p.Body)) || !validateTags(p.Tags) {
			ajaxResponse(w, map[string]string{
				"success": "false",
				"error":   "Text not allowed",
			})
			return
		}

		// Trim extra stuff before the #hashtag (if it makes it this
		// far), remove duplicate strings, and bytify for storage in
		// redis.
		bTags, err := bytify(trimHashTags(p.Tags))
		if err != nil {
			log.Println(err)
			ajaxResponse(w, map[string]string{
				"success": "false",
				"error":   "Bad JSON in tags",
			})
			return
		}

		// Create the post.
		postID := genPostID(15)
		post := map[string]interface{}{
			"title":   p.Title,
			"body":    parseBody(string(p.Body)),
			"ID":      postID,
			"created": time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
			"author":  a.Name,
			"tags":    bTags,
			"type":    "thread",
		}

		// Increment or add tags to the database
		err = processTags(p.Tags, postID)
		if err != nil {
			log.Println(err)
			ajaxResponse(w, map[string]string{
				"success": "false",
				"error":   "Error setting database object",
			})
			return
		}

		err = addPostToDB(post, a.Name, postID)
		if err != nil {
			log.Println(err)
			ajaxResponse(w, map[string]string{
				"success": "false",
				"error":   "Error setting database object",
			})
		}

		// Respond with the postID so that the user can be redirected
		// to the new post.
		ajaxResponse(w, map[string]string{
			"success": "true",
			"error":   "nil",
			"postID":  postID,
		})
		return
	}
	// If we can't validate credentials
	ajaxResponse(w, map[string]string{
		"success": "false",
		"error":   "Not Logged In",
	})
}

// newReply adds a reply to a thread, rebuilds the cache, and sends the
// user to the post they just submitted
// TODO: Possibly combine newReply and newThread
// Ex. tagmachine.com/api/reply
func newReply(w http.ResponseWriter, r *http.Request) {
	// Convert the JSON sent from the client to a postData{} struct
	p, err := marshalpostData(r)
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Bad JSON sent to server",
		})
		return
	}

	// Check if the user is logged in. You can't post wothout being logged
	// in.
	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn || p.Testing == testPass {
		if p.Testing == testPass {
			a = &credentials{Name: p.Author}
		}
		// validate the data
		if !validateBody(string(p.Body)) {
			ajaxResponse(w, map[string]string{
				"success": "false",
				"error":   "Text not allowed",
			})
			return
		}

		// create the post
		postID := genPostID(15)
		post := map[string]interface{}{
			"body":    parseBody(string(p.Body)),
			"ID":      postID,
			"created": time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
			"parent":  p.ID,
			"author":  a.Name,
			"type":    "reply",
		}

		err := addPostToDB(post, a.Name, postID)
		if err != nil {
			log.Println(err)
			ajaxResponse(w, map[string]string{
				"success": "false",
				"error":   "Error setting database object",
			})
		}

		ajaxResponse(w, map[string]string{
			"success": "true",
			"error":   "nil",
			"postID":  postID,
		})
		return
	}
	ajaxResponse(w, map[string]string{
		"success": "false",
		"error":   "Not Logged In",
	})
}

func followOrUnfollow(w http.ResponseWriter, r *http.Request) {
	p, err := marshalCredentials(r)
	if err != nil {
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Bad JSON sent to server",
		})
		return
	}

	// Check if the user is logged in. You can't follow without being logged
	// in.
	c := r.Context().Value(ctxkey)
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		_, err := rdb.ZScore(rdbctx, a.Name+":following", p.Name).Result()
		if err != nil {
			rdb.ZAdd(rdbctx, a.Name+":following", makeZmem(p.Name))
			log.Println(a.Name + " followed " + p.Name)
			ajaxResponse(w, map[string]string{
				"success": "true",
				"message": "followed",
				"error":   "Not Logged In",
			})

		} else {
			rdb.ZRem(rdbctx, a.Name+":following", p.Name)
			log.Println(a.Name + " unfollowed " + p.Name)
			ajaxResponse(w, map[string]string{
				"success": "true",
				"message": "unfollowed",
				"error":   "Not Logged In",
			})

		}
	}
}
func anonSignin(w http.ResponseWriter, r *http.Request) {
	c := &credentials{
		// Name:     strings.Split(r.RemoteAddr, "]")[0],
		Name:     r.RemoteAddr,
		Password: genPostID(15),
	}
	// If username is unique and valid, we attempt to hash
	// the password
	hash, err := hashPassword(c.Password)
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Invalid Password",
		})
		return
	}

	// Add the user the the USERS set in redis. This
	// associates a score with the user that can be
	// incremented or decremented
	_, err = rdb.ZAdd(rdbctx, "USERS", makeZmem(c.Name)).Result()
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Error ",
		})
		return
	}

	// If the password is hashable, and we were able to add
	// the user to the redis ZSET, we store the hash in the
	// database with the username as the key and the hash
	// as the value thats returned by the key.
	_, err = rdb.Set(rdbctx, c.Name, hash, 0).Result()
	if err != nil {
		log.Println(err)
		ajaxResponse(w, map[string]string{
			"success": "false",
			"error":   "Error ",
		})
		return
	}

	// Set user token/credentials
	newClaims(w, r, c)

	return
}
