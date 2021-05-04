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
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
)

// The isolation level represented by a set of populated labels. The last filled out
// field represents the final level. All previous field must be non-empty.
type IsolationLevel struct {
	ProjectID string
	Location  string
	Cluster   string
	Namespace string
	Job       string
}

const (
	labelProjectID = "project_id"
	labelLocation  = "location"
)

func Isolate(groups *rulefmt.RuleGroups, level IsolationLevel) error {
	// Put the isolation labels into a dynamic list so it's easier to use onwards.
	lvl := []labels.Label{
		{"project_id", level.ProjectID},
		{"location", level.Location},
		{"cluster", level.Cluster},
		{"namespace", level.Namespace},
		{"job", level.Job},
	}

	// Validate that all labels up to the final isolation level are set.
	terminal := ""
	for _, l := range lvl {
		if terminal != "" && l.Value != "" {
			return errors.Errorf("label %q set unexpectedly, label %q was already unset", l.Name, terminal)
		}
		if terminal == "" && l.Value == "" {
			terminal = l.Name
		}
	}

	for _, g := range groups.Groups {
		for i, r := range g.Rules {
			expr, err := parser.ParseExpr(r.Expr.Value)
			if err != nil {
				return errors.Wrap(err, "parse PromQL expression")
			}

			// Traverse the query and inject label matchers to all metric selectors
			err = inspect(expr, func(n parser.Node, _ []parser.Node) error {
				vs, ok := n.(*parser.VectorSelector)
				if !ok {
					return nil
				}
				for _, l := range lvl {
					if err := setIsolationSelector(vs, l.Name, l.Value); err != nil {
						return errors.Wrapf(err, "set isolation selector %s=%q on %s", l.Name, l.Value, vs)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}

			// Add labels to produced label sets (metrics or alerts) in case
			// they got aggregated away.
			for _, l := range lvl {
				if err := setIsolationLabel(&r, l.Name, l.Value); err != nil {
					return errors.Wrapf(err, "set result isolation label %s=%q on %v", l.Name, l.Value, r)
				}
			}

			g.Rules[i] = r
		}
	}
	return nil
}

func setIsolationLabel(r *rulefmt.RuleNode, name, value string) error {
	if v, ok := r.Labels[name]; ok {
		return errors.Errorf("label %q already set on rule with unexpected value %q", name, v)
	}
	if value == "" {
		return nil
	}
	if r.Labels == nil {
		r.Labels = map[string]string{}
	}
	r.Labels[name] = value
	return nil
}

func setIsolationSelector(s *parser.VectorSelector, name, value string) error {
	for _, m := range s.LabelMatchers {
		if m.Name != name {
			continue
		}
		if m.Type != labels.MatchEqual || m.Value != value {
			return errors.Errorf("conflicting label matcher %s found", m)
		}
	}
	if value != "" {
		s.LabelMatchers = append(s.LabelMatchers, &labels.Matcher{
			Type:  labels.MatchEqual,
			Name:  name,
			Value: value,
		})
	}
	return nil
}

// Inspect traverses an AST in depth-first order: It starts by calling
// f(node, path); node must not be nil. If f returns a nil error, Inspect invokes f
// for all the non-nil children of node, recursively.
//
// TODO(freinartz): this is adapted from the parser package's implementation where the
// type signature was broken so that it became impossible to use. Fix this upstream.
func inspect(node parser.Node, f func(parser.Node, []parser.Node) error) error {
	return parser.Walk(inspector(f), node, nil)
}

type inspector func(parser.Node, []parser.Node) error

func (f inspector) Visit(node parser.Node, path []parser.Node) (parser.Visitor, error) {
	if err := f(node, path); err != nil {
		return nil, err
	}

	return f, nil
}
