package main

import (
	"flag"
	"net/http"
)

type basicAuthConfig struct {
	username string
	password string
}

func newBasicAuthConfigFromFlags() *basicAuthConfig {
	c := &basicAuthConfig{}
	flag.StringVar(&c.username, "basic-auth-username", "", "BasicAuth username")
	flag.StringVar(&c.password, "basic-auth-password", "", "BasicAuth password")
	return c
}

func (c *basicAuthConfig) isEnabled() bool {
	return c.username != "" || c.password != ""
}

func (c *basicAuthConfig) handle(handler http.Handler) http.Handler {
	if !c.isEnabled() {
		return handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok && username == c.username && password == c.password {
			handler.ServeHTTP(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}
