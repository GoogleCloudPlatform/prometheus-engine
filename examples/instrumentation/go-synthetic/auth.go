package main

import (
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
)

const (
	defaultRSABits      = 4096
	keyAlgorithmRSA     = "rsa"
	keyAlgorithmEd25519 = "ed25519"
)

// isFlagSet returns true if the flag was explicitly set in the command line by the user.
func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

type tlsConfig struct {
	// Provide a custom certificate.
	certPath string
	keyPath  string

	// Create a new self-signed certificate.
	createSelfSigned bool
	keyAlgorithm     string
	serverIP         string
	serverName       string

	// General mTLS flags.
	insecureSkipVerify bool
	minVersion         uint
	maxVersion         uint
}

func newTLSConfigFromFlags() *tlsConfig {
	c := &tlsConfig{}
	flag.StringVar(&c.certPath, "tls-cert", "", "Path to the server TLS certificate")
	flag.StringVar(&c.keyPath, "tls-key", "", "Path to the server TLS key")

	flag.BoolVar(&c.createSelfSigned, "tls-create-self-signed", false, "If true, a self-signed certificate will be created and used as the TLS server certificate.")
	flag.StringVar(&c.keyAlgorithm, "tls-key-algorithm", keyAlgorithmRSA, fmt.Sprintf("Which algorithm to use when creating a self-signed certificate. Supports %q or %q", keyAlgorithmRSA, keyAlgorithmEd25519))
	flag.StringVar(&c.serverName, "tls-server-name", "Example", "Name of the server, used to verify the TLS certificate")
	flag.StringVar(&c.serverIP, "tls-server-ip", "", "IP of the server. If unset, this will look for the POD_IP environment variable")

	flag.BoolVar(&c.insecureSkipVerify, "tls-insecure-skip-verify", false, "Whether to skip verifying the certificate")
	flag.UintVar(&c.minVersion, "tls-min-version", tls.VersionTLS12, "Minimum TLS version")
	flag.UintVar(&c.maxVersion, "tls-max-version", tls.VersionTLS13, "Maximum TLS version")
	return c
}

func (c *tlsConfig) isUserProvidedCertificate() bool {
	return c.certPath != "" || c.keyPath != ""
}

func (c *tlsConfig) isSelfSignedCertificate() bool {
	return c.createSelfSigned || isFlagSet("tls-key-algorithm") || isFlagSet("tls-server-name") || isFlagSet("tls-server-ip")
}

func (c *tlsConfig) hasCertificate() bool {
	return c.isUserProvidedCertificate() || c.isSelfSignedCertificate()
}

func (c *tlsConfig) isEnabled() bool {
	return c.hasCertificate() || isFlagSet("tls-insecure-skip-verify") || isFlagSet("tls-min-version") || isFlagSet("tls-max-version")
}

func (c *tlsConfig) validate() error {
	errs := []error{}
	if c.createSelfSigned && c.isUserProvidedCertificate() {
		errs = append(errs, errors.New("--tls-create-self-signed and cannot be used together with use-provided certificate flags --tls-cert or --tls-key"))
	}
	if !c.createSelfSigned {
		for _, flagName := range []string{"tls-key-algorithm", "tls-server-name", "tls-server-ip"} {
			if isFlagSet(flagName) {
				errs = append(errs, fmt.Errorf("--%s can only be specified with --tls-create-self-signed", flagName))
			}
		}
	}
	if c.isUserProvidedCertificate() && (c.certPath == "" || c.keyPath == "") {
		errs = append(errs, errors.New("--tls-cert and --tls-key must both be set"))
	}
	if c.isEnabled() && !c.hasCertificate() {
		for _, flagName := range []string{"tls-insecure-skip-verify", "tls-min-version", "tls-max-version"} {
			if isFlagSet(flagName) {
				errs = append(errs, fmt.Errorf("--%s can only be specified with --tls-cert or --tls-create-self-signed", flagName))
			}
		}
	}

	if c.keyAlgorithm != keyAlgorithmRSA && c.keyAlgorithm != keyAlgorithmEd25519 {
		errs = append(errs, fmt.Errorf("key algorithm %q is invalid", c.keyAlgorithm))
	}
	if c.serverIP == "" {
		c.serverIP = os.Getenv("POD_IP")
	}

	return errors.Join(errs...)
}

func (c *tlsConfig) getTLSConfig() (*tls.Config, error) {
	if !c.isEnabled() {
		return nil, nil
	}
	config := &tls.Config{
		ServerName:         c.serverName,
		InsecureSkipVerify: c.insecureSkipVerify,
		MinVersion:         uint16(c.minVersion),
		MaxVersion:         uint16(c.maxVersion),
	}
	if c.createSelfSigned {
		var privateKey, publicKey any
		if c.keyAlgorithm == keyAlgorithmRSA {
			rsaPrivateKey, err := rsa.GenerateKey(cryptorand.Reader, defaultRSABits)
			if err != nil {
				return nil, fmt.Errorf("unable to generate RSA key: %w", err)
			}
			privateKey = rsaPrivateKey
			publicKey = &rsaPrivateKey.PublicKey
		} else {
			var err error
			publicKey, privateKey, err = ed25519.GenerateKey(cryptorand.Reader)
			if err != nil {
				return nil, fmt.Errorf("unable to generate ed25519 key: %w", err)
			}
		}

		template := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				Organization: []string{c.serverName},
			},
			NotBefore: time.Now(),
			NotAfter:  time.Now().Add(time.Hour * 24 * 30),

			KeyUsage:              x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
		}
		if c.serverIP != "" {
			template.IPAddresses = append(template.IPAddresses, net.ParseIP(c.serverIP))
		}

		certBytes, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, publicKey, privateKey)
		if err != nil {
			return nil, fmt.Errorf("unable to create self-signed certificate: %w", err)
		}
		certPem := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certBytes,
		})

		privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal private key: %w", err)
		}
		privateKeyPem := pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privateKeyBytes,
		})

		cert, err := tls.X509KeyPair(certPem, privateKeyPem)
		if err != nil {
			return nil, fmt.Errorf("unable to encode self-signed certificate: %w", err)
		}

		config.Certificates = []tls.Certificate{cert}
	} else if c.certPath != "" && c.keyPath != "" {
		cert, err := tls.LoadX509KeyPair(c.certPath, c.keyPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load server cert and key: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	return config, nil
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
		if strings.TrimSpace(auth) == strings.TrimSpace(expected) {
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
	tls       *tlsConfig
	basicAuth *basicAuthConfig
	auth      *authorizationConfig
	oauth2    *oauth2Config
}

func newHttpClientConfigFromFlags() *httpClientConfig {
	return &httpClientConfig{
		tls:       newTLSConfigFromFlags(),
		basicAuth: newBasicAuthConfigFromFlags(),
		auth:      newAuthorizationConfigFromFlags(),
		oauth2:    newOAuth2ConfigFromFlags(),
	}
}

func (c *httpClientConfig) validate() error {
	var errs []error
	if err := c.tls.validate(); err != nil {
		errs = append(errs, err)
	}
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

func (c *httpClientConfig) getTLSConfig() (*tls.Config, error) {
	return c.tls.getTLSConfig()
}
