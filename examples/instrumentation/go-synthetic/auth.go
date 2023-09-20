package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
)

func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

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

func authorizationHandler(handler http.Handler, scheme, parameters string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := scheme + " " + parameters
		if auth == expected {
			handler.ServeHTTP(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", scheme+` realm="restricted", charset="UTF-8"`)
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
	return authorizationHandler(handler, c.scheme, c.parameters)
}

type oauth2Config struct {
	clientID     string
	clientSecret string
	scopes       string
	accessToken  string
}

func newOAuth2ConfigFromFlags() *oauth2Config {
	c := &oauth2Config{}
	flag.StringVar(&c.clientID, "oauth2-client-id", "", "OAuth2 client ID")
	flag.StringVar(&c.clientSecret, "oauth2-client-secret", "", "OAuth2 client secret")
	flag.StringVar(&c.scopes, "oauth2-scopes", "", "Required OAuth2 comma-separated scopes")
	flag.StringVar(&c.accessToken, "oauth2-access-token", "", "OAuth2 access token or empty to generate one. /token will provision this token")
	return c
}

func (c *oauth2Config) isEnabled() bool {
	return c.clientID != "" || c.clientSecret != "" || c.scopes != "" || isFlagSet("oauth2-access-token")
}

const oauth2TokenCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~+/"
const defaultOAuth2TokenLength = 8

func (c *oauth2Config) validate() error {
	if c.accessToken == "" {
		builder := strings.Builder{}
		builder.Grow(defaultOAuth2TokenLength)
		for i := 0; i < builder.Cap(); i++ {
			builder.WriteByte(oauth2TokenCharset[rand.Intn(len(oauth2TokenCharset))])
		}
		c.accessToken = builder.String()
	}
	return nil
}

func oauthTokenErrorResponse(errorCode string) []byte {
	return []byte(fmt.Sprintf("{\n\terror: %q,\n}\n", errorCode))
}

func (c *oauth2Config) tokenHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		grantType := r.URL.Query().Get("grant_type")
		clientID := r.URL.Query().Get("client_id")
		clientSecret := r.URL.Query().Get("client_secret")
		scopes := r.URL.Query().Get("scope")
		if grantType != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write(oauthTokenErrorResponse("unsupported_grant_type"))
			return
		}

		if clientID != c.clientID || clientSecret != c.clientSecret {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(oauthTokenErrorResponse("invalid_client"))
			return
		}

		if len(c.scopes) > 0 {
			requiredScopes := strings.Split(c.scopes, ",")
			sort.Strings(requiredScopes)
			requestedScopes := strings.Split(scopes, " ")
			sort.Strings(requestedScopes)
			if !cmp.Equal(requestedScopes, requiredScopes) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write(oauthTokenErrorResponse("invalid_scope"))
				return
			}
		}

		response := fmt.Sprintf("{\n\taccess_token: %q,\n\ttoken_type: %q\n}\n", c.accessToken, "bearer")
		w.Write([]byte(response))
	})
}

func (c *oauth2Config) handle(handler http.Handler) http.Handler {
	return authorizationHandler(handler, "Bearer", c.accessToken)
}

type httpClientConfig struct {
	basicAuth *basicAuthConfig
	auth      *authorizationConfig
	oauth2    *oauth2Config
}

func newHttpClientConfigFromFlags() *httpClientConfig {
	return &httpClientConfig{
		basicAuth: newBasicAuthConfigFromFlags(),
		auth:      newAuthorizationConfigFromFlags(),
		oauth2:    newOAuth2ConfigFromFlags(),
	}
}

func (c *httpClientConfig) validate() error {
	var errs []error
	if c.basicAuth.isEnabled() {
		if c.auth.isEnabled() {
			errs = append(errs, errors.New("cannot specify both --basic-auth and --auth flags"))
		}
		if c.oauth2.isEnabled() {
			errs = append(errs, errors.New("cannot specify both --basic-auth and --oauth2 flags"))
		}
	}
	if c.auth.isEnabled() && c.oauth2.isEnabled() {
		errs = append(errs, errors.New("cannot specify both --auth and --oa2uth flags"))
	}
	if err := c.auth.validate(); err != nil {
		errs = append(errs, err)
	}
	if err := c.oauth2.validate(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c *httpClientConfig) register(mux *http.ServeMux) {
	if c.oauth2.isEnabled() {
		mux.Handle("/token", c.oauth2.tokenHandler())
	}
}

func (c *httpClientConfig) handle(handler http.Handler) http.Handler {
	if c.oauth2.isEnabled() {
		return c.oauth2.handle(handler)
	}
	if c.auth.isEnabled() {
		return c.auth.handle(handler)
	}
	if c.basicAuth.isEnabled() {
		return c.basicAuth.handle(handler)
	}
	return handler
}
