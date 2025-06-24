// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secrets

import (
	"context"

	"github.com/go-kit/log"
)

// Secret represents a sensitive value.
type Secret interface {
	// Fetch fetches the secret content.
	Fetch(ctx context.Context) (string, error)
}

// SecretFn wraps a function to make it a Secret.
type SecretFn func(ctx context.Context) (string, error)

// Fetch implements Secret.Fetch.
func (fn *SecretFn) Fetch(ctx context.Context) (string, error) {
	return (*fn)(ctx)
}

// ProviderOptions provides options for a Provider.
type ProviderOptions struct {
	Logger log.Logger
}
