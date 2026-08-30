// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secutil

import (
	"crypto/sha256"
	"crypto/subtle"
)

// ConstTimeEqual compares strings with an extra security layer against timing attacks.
func ConstTimeEqual(a, b string) bool {
	// We need to ensure inputs are equal lengths, otherwise the check would leak unqual length information.
	// While SHA256 is not ideal for storing passwords as per https://codeql.github.com/codeql-query-help/go/go-weak-sensitive-data-hashing/#id1
	// it's good enough for ensuring fixed length for this check.
	ax, bx := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ax[:], bx[:]) == 1
}
