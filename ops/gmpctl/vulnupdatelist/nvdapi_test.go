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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCVEDetails(t *testing.T) {
	t.Skip("depends on NVDE API")

	c := getCVEDetails("", OSV{
		ID:      "GO-2021-0065",
		Aliases: []string{"GHSA-jmrx-5g74-6v2f", "CVE-2019-11250"},
	})
	require.Equal(t, "CVE-2019-11250", c.ID)
	require.Equal(t, "MEDIUM", c.Severity)
}
