// Copyright 2025 Google LLC
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

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompileUpdateList_IgnoredModules(t *testing.T) {
	mockJSON := `
{"osv":{"id":"GO-2024-0001","summary":"Test vuln 1"}}
{"finding":{"osv":"GO-2024-0001","fixed_version":"1.2.3","trace":[{"module":"github.com/prometheus/prometheus","version":"1.0.0"}]}}
{"osv":{"id":"GO-2024-0002","summary":"Test vuln 2"}}
{"finding":{"osv":"GO-2024-0002","fixed_version":"0.1.5","trace":[{"module":"golang.org/x/net","version":"0.1.0"}]}}
`

	t.Run("without ignored modules", func(t *testing.T) {
		updates, err := compileUpdateList(strings.NewReader(mockJSON), false, nil)
		require.NoError(t, err)
		require.Len(t, updates, 2)
	})

	t.Run("with ignored module", func(t *testing.T) {
		updates, err := compileUpdateList(strings.NewReader(mockJSON), false, []string{"github.com/prometheus/prometheus"})
		require.NoError(t, err)
		require.Len(t, updates, 1)
		require.Equal(t, "golang.org/x/net", updates[0].Module)
	})
}
