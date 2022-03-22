// Copyright 2015 The Kubernetes Authors.
// Modifications copyright (C) 2020 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package export

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
	"google.golang.org/api/googleapi"
)

const (
	// Max QPS to allow through to the token URL.
	tokenURLQPS = .05 // back off to once every 20 seconds when failing
	// Maximum burst of requests to token URL before limiting.
	tokenURLBurst = 3
)

// AltTokenSource is the structure holding the data for the functionality needed to generates tokens
type AltTokenSource struct {
	oauthClient *http.Client
	tokenURL    string
	tokenBody   string
	throttle    *rate.Limiter
}

// Token returns a token which may be used for authentication
func (a *AltTokenSource) Token() (*oauth2.Token, error) {
	r := a.throttle.Reserve()
	if !r.OK() {
		return nil, errors.Errorf("Rate limiter (rate: %f, burst: %d) cannot provide the requested token.",
			a.throttle.Limit(), a.throttle.Burst())
	}
	time.Sleep(r.Delay())
	return a.token()
}
func (a *AltTokenSource) token() (*oauth2.Token, error) {
	req, err := http.NewRequest("POST", a.tokenURL, strings.NewReader(a.tokenBody))
	if err != nil {
		return nil, err
	}
	res, err := a.oauthClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	var tok struct {
		AccessToken string    `json:"accessToken"`
		ExpireTime  time.Time `json:"expireTime"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tok); err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: tok.AccessToken,
		Expiry:      tok.ExpireTime,
	}, nil
}

// NewAltTokenSource constructs a new alternate token source for generating tokens.
func NewAltTokenSource(tokenURL, tokenBody string) oauth2.TokenSource {
	client := oauth2.NewClient(context.Background(), google.ComputeTokenSource(""))
	a := &AltTokenSource{
		oauthClient: client,
		tokenURL:    tokenURL,
		tokenBody:   tokenBody,
		throttle:    rate.NewLimiter(tokenURLQPS, tokenURLBurst),
	}
	return oauth2.ReuseTokenSource( /* token */ nil, a)
}
