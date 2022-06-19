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

type credentials struct {
	Name string `json:"username"`
	// IP         string `json:"IP"`
	IsLoggedIn bool `json:"isLoggedIn"`
	// HasGoogle  bool   `json:"hasGoogle"`
	// GoogleCredentials googleCredentials `json:"gcred"`
	// GoogleToken string `json:"googleToken"`
	jwt.StandardClaims
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
)

func main() {
	rs := client.Ping(ctx)
	str, err := rs.Result()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(str)

	rand.Seed(time.Now().UTC().UnixNano())

	mux := http.NewServeMux()
	mux.Handle("/", checkAuth(http.HandlerFunc(home)))
	mux.HandleFunc("/api/signup", signup)
	mux.HandleFunc("/api/signin", signin)
	mux.HandleFunc("/api/logout", logout)
	mux.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("public"))))

	// Server configuration
	srv := &http.Server{
		// in production only ust SSL
		Addr:              ":8081",
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
