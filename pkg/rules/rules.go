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
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	yaml "gopkg.in/yaml.v2"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

// FromAPIRules constructs rule groups from a list of rule groups in the
// resource API format. It ensures that the groups are valid according to the
// Prometheus upstream validation logic.
func FromAPIRules(groups []monitoringv1.RuleGroup) (result rulefmt.RuleGroups, err error) {
	for _, g := range groups {
		var rules []rulefmt.RuleNode

		for _, r := range g.Rules {
			rule := rulefmt.RuleNode{
				Labels:      r.Labels,
				Annotations: r.Annotations,
			}
			rule.Expr.SetString(r.Expr)
			if r.Record != "" {
				rule.Record.SetString(r.Record)
			}
			if r.Alert != "" {
				rule.Alert.SetString(r.Alert)
			}
			if r.For != "" {
				rule.For, err = model.ParseDuration(r.For)
				if err != nil {
					return result, errors.Wrap(err, "parse 'for' duration")
				}
			}
			rules = append(rules, rule)
		}
		group := rulefmt.RuleGroup{
			Name:  g.Name,
			Rules: rules,
		}
		if g.Interval != "" {
			group.Interval, err = model.ParseDuration(g.Interval)
			if err != nil {
				return result, errors.Wrap(err, "parse evaluation interval")
			}
		}
		result.Groups = append(result.Groups, group)
	}
	// Do a marshal/unmarshal cycle to run the upstream validation.
	b, err := yaml.Marshal(result)
	if err != nil {
		return result, err
	}
	var validate rulefmt.RuleGroups
	if err := yaml.Unmarshal(b, &validate); err != nil {
		return result, errors.Wrap(err, "loading rules failed")
	}
	return result, nil
}

// Scope all rules in the given groups to the given labels. All metric selectors
// check for equality on the labels and all rule results are annotated with them again.
// This ensures that the scope is preserved in output data, even if the given label keys
// are aggregated away.
// An error is returned if metric selectors have a conflicting selector set.
func Scope(groups *rulefmt.RuleGroups, lset map[string]string) error {
	for _, g := range groups.Groups {
		for i, r := range g.Rules {
			expr, err := parser.ParseExpr(r.Expr.Value)
			if err != nil {
				return errors.Wrap(err, "parse PromQL expression")
			}

			// Traverse the query and inject label matchers to all metric selectors
			err = walkExpr(expr, func(n parser.Node) error {
				vs, ok := n.(*parser.VectorSelector)
				if ok {
					for name, value := range lset {
						if err := setSelector(vs, name, value); err != nil {
							return errors.Wrapf(err, "set isolation selector %s=%q on %s", name, value, vs)
						}
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			r.Expr.SetString(expr.String())

			// Add labels to produced label sets (metrics or alerts) in case
			// they got aggregated away.
			for name, value := range lset {
				if err := setLabel(&r, name, value); err != nil {
					return errors.Wrapf(err, "set result isolation label %s=%q on %v", name, value, r)
				}
			}

			g.Rules[i] = r
		}
	}
	return nil
}

func setLabel(r *rulefmt.RuleNode, name, value string) error {
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

func setSelector(s *parser.VectorSelector, name, value string) error {
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

func walkExpr(node parser.Node, f func(parser.Node) error) error {
	for _, c := range parser.Children(node) {
		if err := walkExpr(c, f); err != nil {
			return err
		}
	}
	return f(node)
}
