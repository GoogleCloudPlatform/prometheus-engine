// Copyright 2022 Google LLC
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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/prometheus/model/rulefmt"
	yaml "gopkg.in/yaml.v3"
)

func TestScope(t *testing.T) {
	input := `groups:
- name: test
  rules:
  - record: rule:1
    expr: vector(1)
  - record: rule:2
    expr: sum by(foo, bar) (rate(my_metric[5m]))
    labels:
      a: b
  - alert: Bar
    expr: my_metric1 / my_metric2{a="b"} > 0
`
	groups, errs := rulefmt.Parse([]byte(input))
	if len(errs) > 0 {
		t.Fatalf("Unexpected input errors: %s", errs)
	}
	err := Scope(groups, map[string]string{
		"l1": "v1",
		"l2": "v2",
	})
	want := `groups:
    - name: test
      rules:
        - record: rule:1
          expr: vector(1)
          labels:
            l1: v1
            l2: v2
        - record: rule:2
          expr: sum by (foo, bar) (rate(my_metric{l1="v1",l2="v2"}[5m]))
          labels:
            a: b
            l1: v1
            l2: v2
        - alert: Bar
          expr: my_metric1{l1="v1",l2="v2"} / my_metric2{a="b",l1="v1",l2="v2"} > 0
          labels:
            l1: v1
            l2: v2
`
	got, err := yaml.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Fatalf("unexpected result (-want, +got):\n %s", diff)
	}
}
