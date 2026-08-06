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

		require.Equal(t, "github.com/prometheus/prometheus", updates[0].Module)
		require.Equal(t, "GO-2024-0001", updates[0].CVEID)
		require.Equal(t, "1.0.0", updates[0].Version)
		require.NotNil(t, updates[0].FixedVersion)
		require.Equal(t, "1.2.3", updates[0].FixedVersion.String())

		require.Equal(t, "golang.org/x/net", updates[1].Module)
		require.Equal(t, "GO-2024-0002", updates[1].CVEID)
		require.Equal(t, "0.1.0", updates[1].Version)
		require.NotNil(t, updates[1].FixedVersion)
		require.Equal(t, "0.1.5", updates[1].FixedVersion.String())
	})

	t.Run("with ignored module", func(t *testing.T) {
		ignored := map[string]struct{}{
			"github.com/prometheus/prometheus": {},
		}
		updates, err := compileUpdateList(strings.NewReader(mockJSON), false, ignored)
		require.NoError(t, err)
		require.Len(t, updates, 1)
		require.Equal(t, "golang.org/x/net", updates[0].Module)
		require.Equal(t, "GO-2024-0002", updates[0].CVEID)
		require.Equal(t, "0.1.0", updates[0].Version)
		require.NotNil(t, updates[0].FixedVersion)
		require.Equal(t, "0.1.5", updates[0].FixedVersion.String())
	})
}
