// Copyright 2024 Google LLC
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

package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"k8s.io/utils/ptr"
)

var (
	metricLabelRegex        = regexp.MustCompile(`[a-z][a-z0-9_.\/]*`)
	quotedStringRegex       = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	boolRegex               = regexp.MustCompile(`(true)|(false)`)
	numberRegex             = regexp.MustCompile(`\d+`)
	expressionObjectRegex   = regexp.MustCompile(fmt.Sprintf(`(project)|(group\.id)|(metric\.type)|(metric\.labels\.%[1]s)|(resource\.type)|(resource\.labels\.%[1]s)|(metadata\.system_labels\.%[1]s)`, metricLabelRegex.String()))
	expressionOperatorRegex = regexp.MustCompile(`=|>|<|(>=)|(<=)|(!=)|:`)
	expressionValueRegex    = regexp.MustCompile(fmt.Sprintf(`(%s)|(%s)|(%s)`, quotedStringRegex.String(), boolRegex.String(), numberRegex.String()))
	expressionRegex         = regexp.MustCompile(fmt.Sprintf(`^(?P<object>%s)\s*(?P<operator>%s)\s*(?P<value>%s)(?P<rest>\s+.*)?$`, expressionObjectRegex.String(), expressionOperatorRegex.String(), expressionValueRegex.String()))
)

func runFilter(timeSeriesList []*monitoringpb.TimeSeries, filter pointFilter) []*monitoringpb.TimeSeries {
	var timeSeriesFiltered []*monitoringpb.TimeSeries
	for _, timeSeries := range timeSeriesList {
		var pointsFiltered []*monitoringpb.Point
	pointLabel:
		for _, point := range timeSeries.Points {
			if !filter.filter(timeSeries, point) {
				continue pointLabel
			}
			pointsFiltered = append(pointsFiltered, point)
		}
		if len(pointsFiltered) == 0 {
			continue
		}
		timeSeriesOut := &monitoringpb.TimeSeries{
			Metric:     timeSeries.Metric,
			Resource:   timeSeries.Resource,
			Metadata:   timeSeries.Metadata,
			MetricKind: timeSeries.MetricKind,
			ValueType:  timeSeries.ValueType,
			Points:     pointsFiltered,
			Unit:       timeSeries.Unit,
		}
		timeSeriesFiltered = append(timeSeriesFiltered, timeSeriesOut)
	}
	return timeSeriesFiltered
}

// parseFilter parses the Google Cloud Monitoring API filter.
//
// See: https://cloud.google.com/monitoring/api/v3/filters#filter_syntax
//
// Currently limited, e.g. does not support NOTs, parenthesis or order of operations.
func parseFilter(filter string) (pointFilter, error) {
	filter = strings.TrimSpace(filter)
	filter = strings.ReplaceAll(filter, "\n", " ")
	submatches := expressionRegex.FindStringSubmatch(filter)
	if submatches == nil {
		return nil, fmt.Errorf("invalid expression %q", filter)
	}
	object := submatches[expressionRegex.SubexpIndex("object")]
	operator := submatches[expressionRegex.SubexpIndex("operator")]
	value := submatches[expressionRegex.SubexpIndex("value")]
	rest := submatches[expressionRegex.SubexpIndex("rest")]

	if operator != "=" {
		return nil, fmt.Errorf("unsupported operator %q in expression %q", operator, filter)
	}
	eq := &equalExpression{
		object: object,
		// Extract from quotes, if quoted.
		value: strings.TrimSuffix(strings.TrimPrefix(value, "\""), "\""),
	}
	if rest == "" {
		return eq, nil
	}

	expressionLogicalRegex := regexp.MustCompile(`\s+(?P<operator>AND)\s+(?P<rest>.*)`)
	submatches = expressionLogicalRegex.FindStringSubmatch(rest)
	if submatches == nil {
		return nil, fmt.Errorf("invalid sub-expression %q", strings.TrimSpace(rest))
	}

	logicalOperator := submatches[expressionLogicalRegex.SubexpIndex("operator")]
	rest = submatches[expressionLogicalRegex.SubexpIndex("rest")]

	inner, err := parseFilter(rest)
	if err != nil {
		return nil, err
	}

	switch logicalOperator {
	case "AND":
		return &andExpression{
			left:  eq,
			right: inner,
		}, nil
	case "OR":
		return &orExpression{
			left:  eq,
			right: inner,
		}, nil
	default:
		return nil, fmt.Errorf("invalid logical operator %q in expression %q", logicalOperator, filter)
	}
}

type pointFilter interface {
	// filter returns true to keep the given point.
	filter(timeSeries *monitoringpb.TimeSeries, point *monitoringpb.Point) bool
}

type dateFilter struct {
	startTime time.Time
	endTime   *time.Time
}

func newIntervalFilter(interval *monitoringpb.TimeInterval) pointFilter {
	filter := &dateFilter{
		startTime: interval.StartTime.AsTime(),
	}
	if interval.EndTime != nil {
		filter.endTime = ptr.To(interval.EndTime.AsTime())
	}
	return filter
}

func (f *dateFilter) filter(_ *monitoringpb.TimeSeries, point *monitoringpb.Point) bool {
	if f.endTime == nil {
		return true
	}

	pointStartTime := point.Interval.StartTime.AsTime()
	pointEndTime := point.Interval.EndTime.AsTime()
	if f.endTime.Before(pointStartTime) {
		return false
	}

	// Include equal end times as true.
	return !pointEndTime.After(*f.endTime)
}

type equalExpression struct {
	object string
	value  string
}

func (e *equalExpression) filter(timeSeries *monitoringpb.TimeSeries, point *monitoringpb.Point) bool {
	return e.value == extractValue(timeSeries, point, e.object)
}

func extractValue(timeSeries *monitoringpb.TimeSeries, _ *monitoringpb.Point, object string) string {
	switch object {
	case "project":
		return timeSeries.GetResource().GetLabels()["project_id"]
	case "metric.type":
		return timeSeries.GetMetric().GetType()
	case "resource.type":
		return timeSeries.GetResource().GetType()
	}
	userLabelsPrefix := "metric.labels."
	if strings.HasPrefix(object, userLabelsPrefix) {
		labelName := object[len(userLabelsPrefix):]
		if metric := timeSeries.GetMetric(); metric != nil {
			if val, ok := metric.GetLabels()[labelName]; ok {
				return val
			}
		}
		return ""
	}
	resourceLabelsPrefix := "resource.labels."
	if strings.HasPrefix(object, resourceLabelsPrefix) {
		labelName := object[len(resourceLabelsPrefix):]
		if resource := timeSeries.GetResource(); resource != nil {
			if val, ok := resource.GetLabels()[labelName]; ok {
				return val
			}
		}
		return ""
	}
	return ""
}

type andExpression struct {
	left, right pointFilter
}

func (e *andExpression) filter(timeSeries *monitoringpb.TimeSeries, point *monitoringpb.Point) bool {
	return e.left.filter(timeSeries, point) && e.right.filter(timeSeries, point)
}

type orExpression struct {
	left, right pointFilter
}

func (e *orExpression) filter(timeSeries *monitoringpb.TimeSeries, point *monitoringpb.Point) bool {
	return e.left.filter(timeSeries, point) || e.right.filter(timeSeries, point)
}
