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

	"github.com/go-redis/redis"
	"github.com/golang-jwt/jwt"
)

type credentials struct {
	Name       string   `json:"username"`
	Password   string   `json:"password"`
	IsLoggedIn bool     `json:"isLoggedIn"`
	Posts      []string `json:"posts"`
	jwt.StandardClaims
	// IP         string `json:"IP"`
	// HasGoogle  bool   `json:"hasGoogle"`
	// GoogleCredentials googleCredentials `json:"gcred"`
	// GoogleToken string `json:"googleToken"`
}

type pageData struct {
	UserData *credentials `json:"userData"`
	Posts    []*postData  `json:"posts"`
	Tags     []string     `json:"tags"`
	Thread   *threadData
}

type threadData struct {
	Thread   *postData
	Children []*postData
	Parent   string
	// Tags     []byte `json:"tags"`
}

type postData struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	ID       string `json:"ID"`
	Author   string `json:"author"`
	Parent   string `json:"parent"`
	Children []*postData
	TS       string   `json:"timestamp"`
	Tags     []string `json:"tags"`
}

var (
	redisIP = os.Getenv("redisIP")
	client  = redis.NewClient(&redis.Options{
		Addr:     redisIP + ":6379",
		Password: "",
		DB:       0,
	})

	templates        = template.Must(template.New("main").ParseGlob("internal/*/*.tmpl"))
	hmacSampleSecret = []byte("0r9ck0r9cr09kcr09kcreiwn fwn f0ewf0ewncremcrecm")
	Posts            = make(map[string][]*postData)
	tags             = []string{"politics", "stem", "arts", "sports", "other"}
)

func main() {
	fmt.Println("Sending ping() to redis")
	rs := client.Ping()
	str, err := rs.Result()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(str)

	rand.Seed(time.Now().UTC().UnixNano())

	mux := http.NewServeMux()
	mux.Handle("/", checkAuth(http.HandlerFunc(home)))
	mux.Handle("/user/", checkAuth(http.HandlerFunc(userPosts)))
	mux.Handle("/tag/", checkAuth(http.HandlerFunc(getTags)))
	mux.Handle("/api/newthread", checkAuth(http.HandlerFunc(newThread)))
	mux.Handle("/api/reply", checkAuth(http.HandlerFunc(newReply)))
	mux.HandleFunc("/view/", view)
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
