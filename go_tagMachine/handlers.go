package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

func signin(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	var ctx context.Context
	ctx = context.WithValue(r.Context(), "credentials", &credentials{})
	hash, err := client.Get(ctx, r.Form["username"][0]).Result()
	if err != nil {
		fmt.Println(err)
	} else {
		doesMatch := CheckPasswordHash(r.Form["password"][0], hash)
		if doesMatch {
			ctx = setTokenCookie(w, r)
		}
	}
	http.Redirect(w, r.WithContext(ctx), "/", http.StatusSeeOther)
}

func signup(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	hash, err := HashPassword(r.Form["password"][0])
	if err != nil {
		fmt.Println(err)
	}

	match, err := regexp.MatchString("^[A-Za-z0-9]+(?:[ _-][A-Za-z0-9]+)*$", r.Form["username"][0])
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("match: ", match)
	if match && (len(r.Form["username"]) < 25) {
		client.Set(ctx, r.Form["username"][0], hash, 0)
		ctx := setTokenCookie(w, r)
		http.Redirect(w, r.WithContext(ctx), "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func logout(w http.ResponseWriter, r *http.Request) {
	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: "loggedout", Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func home(w http.ResponseWriter, r *http.Request) {
	c := r.Context().Value("credentials")
	if a, ok := c.(*credentials); ok && a.IsLoggedIn {
		err := templates.ExecuteTemplate(w, "home.tmpl", a)
		if err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	err := templates.ExecuteTemplate(w, "home.tmpl", nil)
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
