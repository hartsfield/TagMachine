package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt"
)

// How TagMachine should work:
// There are two sets of tags, the default tags (provided by TagMachine) and
//   auxillary tags created by users
// Each user thread is required to have a default tag, and an auxillary tag.
// TagMachine adds the aux tags to a sorted set in redis.
// TagMachine will only display and allow sorting using the 20 highest scoring
//   aux tags
// The tags score is incremented when a user posts or replies to a thread with
//   that tag, and decremented after a certain amount of time with no
//   threads/replies.
// Threads behave similar to tags, but are displayed in a different location
//   (under the tags)
// A thread is stored in a sorted set with a score and that score is
//   incremented when a user replies to that thread. The score is decremented
//   using a time-to-score ratio that eventually becomes zero, and the thread
//   is removed.
// Most threads should not last more than 72 hours. Threads with no activity
//   should be removed before threads with lots of activity.
// Threads can be sorted by rank and post date.
// Users have scores and have privileges based on those scores
//   - users with under 10 points can only post 1 thread an hour and 5 replies
//   - etc
// Each auxillary tag can only have 100 threads, when the 101st thread is
//   posted, the "weakest" thread is determined by an algorithm, and then
//   removed, making space for the new thread
// Each thread can only have 350 replies, then it will be auto-reposted with
//   a special badge indicating its popularity. This can continue for ???
//
// Goal: To quickly relay and funnel real-time world events into an easily
//       digested stream of information
//
// Goal: To eventually archive popular discussions/tags and create an event
//       time-line
//
// Rules:
//   - No spam/advertising
//   - No bots or organized information supression
//   - No sexualization of minors
//   - No racism/sexism
//   - No links to illegal content
//   - No posting personal information of anyone without consent
//   - No harrassment of other users
//

// credentials are user credentials and are used in the HTML templates and also
// by handlers that do authorized requests
type credentials struct {
	Name       string   `json:"username"`
	Password   string   `json:"password"`
	IsLoggedIn bool     `json:"isLoggedIn"`
	Posts      []string `json:"posts"`
	Score      uint     `json:"score"`
	jwt.StandardClaims
	// IP         string `json:"IP"`
	// HasGoogle  bool   `json:"hasGoogle"`
	// GoogleCredentials googleCredentials `json:"gcred"`
	// GoogleToken string `json:"googleToken"`
}

// pageData is used in the HTML templates as the main page model
type pageData struct {
	UserData    *credentials `json:"userData"`
	Posts       []*postData  `json:"posts"`
	DefaultTags []string     `json:"defaultTags"`
	Tags        []string     `json:"tags"`
	Thread      *threadData
}

// threadData is part of pageData and is used to display a single thread or
// thread information
type threadData struct {
	Thread   *postData
	Children []*postData
	Parent   string
	// Tags     []byte `json:"tags"`
}

// postData is used in pageData and threadData and contains all the information
// for a single post
type postData struct {
	Title    string        `json:"title"`
	Body     template.HTML `json:"body"`
	ID       string        `json:"ID"`
	Author   string        `json:"author"`
	Parent   string        `json:"parent"`
	Children []*postData
	TS       string   `json:"timestamp"`
	Tags     []string `json:"tags"`
}

// ckey/ctxkey is used as the key for the HTML context and is how we retrieve
// token information and pass it around to handlers
type ckey int

const (
	ctxkey ckey = iota
)

var (
	redisIP = os.Getenv("redisIP")
	rdb     = redis.NewClient(&redis.Options{
		Addr:     redisIP + ":6379",
		Password: "",
		DB:       0,
	})

	templates        = template.Must(template.New("main").ParseGlob("internal/*/*.tmpl"))
	hmacSampleSecret = []byte("0r9ck0r9cr09kcr09kcreiwn fwn f0ewf0ewncremcrecm")
	posts            = make(map[string][]*postData)
	frontpage        = make(map[string][]*postData)
	tags             = []string{}
	defaultTags      = []string{"politics", "stem", "arts", "other", "sports"}
	ctx              = context.Background()
)

func main() {
	fmt.Println("Sending ping() to redis")
	rs, err := rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(rs)

	rand.Seed(time.Now().UTC().UnixNano())

	mux := http.NewServeMux()
	mux.Handle("/", checkAuth(http.HandlerFunc(home)))
	mux.Handle("/user/", checkAuth(http.HandlerFunc(userPosts)))
	mux.Handle("/tag/", checkAuth(http.HandlerFunc(getTags)))
	mux.Handle("/api/newthread", checkAuth(http.HandlerFunc(newThread)))
	mux.Handle("/api/reply", checkAuth(http.HandlerFunc(newReply)))
	mux.Handle("/view/", checkAuth(http.HandlerFunc(view)))
	mux.HandleFunc("/api/signup", signup)
	mux.HandleFunc("/api/signin", signin)
	mux.HandleFunc("/api/logout", logout)
	mux.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))

	// Server configuration
	srv := &http.Server{
		// in production only ust SSL
		Addr:              ":8082",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       5 * time.Second,
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server two closed\n")
		} else if err != nil {
			fmt.Printf("error listening for server two: %s\n", err)
		}
		cancelCtx()
	}()

	fmt.Println("Server started @ " + srv.Addr)
	<-ctx.Done()
}

func init() {
	beginCache()
}
