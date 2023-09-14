package main

import (
	"errors"
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

type authorizationConfig struct {
	scheme     string
	parameters string
}

func newAuthorizationConfigFromFlags() *authorizationConfig {
	c := &authorizationConfig{}
	flag.StringVar(&c.scheme, "auth-scheme", "", "Authorization header scheme")
	flag.StringVar(&c.parameters, "auth-parameters", "", "Data to require in the Authorization header")
	return c
}

func (c *authorizationConfig) isEnabled() bool {
	return c.scheme != "" || c.parameters != ""
}

func (c *authorizationConfig) validate() error {
	var errs []error
	if c.scheme == "" && c.parameters != "" {
		errs = append(errs, errors.New("must specify --auth-scheme when using --auth-parameters"))
	}
	if c.scheme == "Basic" {
		errs = append(errs, errors.New("use --basic-auth flags to specify BasicAuth"))
	}
	return errors.Join(errs...)
}

func (c *authorizationConfig) handle(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := c.scheme + " " + c.parameters
		if auth == expected {
			handler.ServeHTTP(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", c.scheme+` realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

type httpClientConfig struct {
	basicAuth *basicAuthConfig
	auth      *authorizationConfig
}

func newHttpClientConfigFromFlags() *httpClientConfig {
	return &httpClientConfig{
		basicAuth: newBasicAuthConfigFromFlags(),
		auth:      newAuthorizationConfigFromFlags(),
	}
}

func (c *httpClientConfig) validate() error {
	var errs []error
	if c.basicAuth.isEnabled() && c.auth.isEnabled() {
		errs = append(errs, errors.New("cannot specify both --basic-auth and --auth flags"))
	}
	if err := c.auth.validate(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c *httpClientConfig) handle(handler http.Handler) http.Handler {
	if c.auth.isEnabled() {
		return c.auth.handle(handler)
	}
	if c.basicAuth.isEnabled() {
		return c.basicAuth.handle(handler)
	}
	return handler
}
