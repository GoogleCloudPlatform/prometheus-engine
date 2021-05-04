// Copyright 2021 Google LLC
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

package rules

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/pkg/rulefmt"
)

func TestIsolate(t *testing.T) {
	input := `
groups:
- name: test
  rules:
  - record: foo
    expr: vector(1)
  - alert: Bar
    expr: foo > 0
`
	groups, errs := rulefmt.Parse([]byte(input))
	if len(errs) > 0 {
		t.Fatalf("Unexpected input errors: %s", errs)
	}
	err := Isolate(groups, IsolationLevel{
		ProjectID: "gpe-test-1",
		Location:  "us-central1-a",
	})
	fmt.Println(groups, err)
	t.Fail()
}
