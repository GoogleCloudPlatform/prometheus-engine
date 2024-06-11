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
	"errors"
	"fmt"
	"strings"
	"sync"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/maps"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/proto"
)

type MetricDatabase struct {
	mtx                 sync.RWMutex
	timeSeriesByProject map[string][]*monitoringpb.TimeSeries
}

func NewMetricDatabase() *MetricDatabase {
	return &MetricDatabase{
		timeSeriesByProject: make(map[string][]*monitoringpb.TimeSeries),
	}
}

func (db *MetricDatabase) All() []*monitoringpb.TimeSeries {
	db.mtx.RLock()
	defer db.mtx.RUnlock()
	var timeSeries []*monitoringpb.TimeSeries
	for _, timeSeriesFromProject := range db.timeSeriesByProject {
		timeSeries = append(timeSeries, timeSeriesFromProject...)
	}
	return timeSeries
}

func (db *MetricDatabase) Get(project string) []*monitoringpb.TimeSeries {
	db.mtx.RLock()
	defer db.mtx.RUnlock()
	return db.timeSeriesByProject[project]
}

func (db *MetricDatabase) Insert(project string, timeSeries []*monitoringpb.TimeSeries) error {
	db.mtx.RLock()
	defer db.mtx.RUnlock()
	var errs []error
	for _, timeSeriesAdd := range timeSeries {
		if len(timeSeriesAdd.Points) == 0 {
			errs = append(errs, fmt.Errorf("empty time series %q", timeSeriesAdd.GetMetric().GetType()))
		}

		timeSeriesList, ok := db.timeSeriesByProject[project]
		if !ok {
			db.timeSeriesByProject[project] = []*monitoringpb.TimeSeries{timeSeriesAdd}
			continue
		}

		found := false
		for _, timeSeries := range timeSeriesList {
			if isTimeSeriesSame(timeSeries, timeSeriesAdd) {
				found = true
				for i, newPoint := range timeSeriesAdd.Points {
					newPointInterval := newPoint.GetInterval()
					if timeSeries.GetMetricKind() == metricpb.MetricDescriptor_GAUGE {
						if newPointInterval.GetStartTime() == nil {
							newPointInterval.StartTime = newPointInterval.EndTime
						} else if !proto.Equal(newPointInterval.GetStartTime(), newPointInterval.GetEndTime()) {
							errs = append(errs, fmt.Errorf("time series %s point %d gauge start time and end time must be same", timeSeriesAdd.GetMetric().GetType(), i))
							continue
						}
					}
					if newPointInterval.GetStartTime() == nil {
						errs = append(errs, fmt.Errorf("time series %s point %d missing start time", timeSeriesAdd.GetMetric().GetType(), i))
						continue
					}
					if newPointInterval.GetEndTime() == nil {
						errs = append(errs, fmt.Errorf("time series %s point %d missing end time", timeSeriesAdd.GetMetric().GetType(), i))
						continue
					}

					newPointStart := newPointInterval.GetStartTime().AsTime()
					newPointEnd := newPointInterval.GetEndTime().AsTime()
					if newPointStart.After(newPointEnd) {
						errs = append(errs, fmt.Errorf("time series %s point %d start time after end time", timeSeriesAdd.GetMetric().GetType(), i))
						continue
					}

					lastPoint := LatestPoint(timeSeries)

					if newPointStart.Before(lastPoint.GetInterval().GetStartTime().AsTime()) {
						errs = append(errs, fmt.Errorf("time series %s point %d new start time before last start time", timeSeriesAdd.GetMetric().GetType(), i))
						continue
					}
					if newPointEnd.Before(lastPoint.GetInterval().GetEndTime().AsTime()) {
						errs = append(errs, fmt.Errorf("time series %s point %d new end time before last end time", timeSeriesAdd.GetMetric().GetType(), i))
						continue
					}

					timeSeries.Points = append([]*monitoringpb.Point{newPoint}, timeSeries.Points...)
				}
				break
			}
		}

		if !found {
			db.timeSeriesByProject[project] = append(timeSeriesList, timeSeriesAdd)
		}
	}

	return errors.Join(errs...)
}

func LatestPoint(timeSeries *monitoringpb.TimeSeries) *monitoringpb.Point {
	points := timeSeries.GetPoints()
	if len(points) == 0 {
		return nil
	}
	return points[len(points)-1]
}

// func (db *MetricDatabase) Querier(mint, maxt int64) (storage.Querier, error) {
// 	intervalFilter := newIntervalFilter(&monitoringpb.TimeInterval{
// 		StartTime: &timestamppb.Timestamp{
// 			Seconds: mint,
// 		},
// 		EndTime: &timestamppb.Timestamp{
// 			Seconds: maxt,
// 		},
// 	})
// 	return &storage.MockQuerier{
// 		SelectMockFunction: func(sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
// 			filter := &andExpression{
// 				left: intervalFilter,
// 				right: &matchersFilter{
// 					matchers: matchers,
// 				},
// 			}
// 			return ToSeriesSet(runFilter(db.All(), filter))
// 		},
// 	}, nil
// }

type MetricCollector struct {
	logger           logr.Logger
	metricTypePrefix string
	metricDatabase   *MetricDatabase
}

func NewMetricCollector(logger logr.Logger, metricTypePrefix string, metricDatabase *MetricDatabase) prometheus.Collector {
	return &MetricCollector{
		logger:           logger,
		metricTypePrefix: metricTypePrefix,
		metricDatabase:   metricDatabase,
	}
}

func (collector *MetricCollector) Describe(_ chan<- *prometheus.Desc) {
	// Tell the Prometheus registry that the metrics are dynamically generated.
}

func (collector *MetricCollector) Collect(ch chan<- prometheus.Metric) {
	for _, timeSeries := range collector.metricDatabase.All() {
		timeSeriesName := timeSeries.GetMetric().GetType()
		point := LatestPoint(timeSeries)
		switch timeSeries.GetValueType() {
		case metricpb.MetricDescriptor_DOUBLE:
			metricName, metricType := extractMetricMetadata(collector.metricTypePrefix, timeSeriesName)
			labels := prometheus.Labels(Labels(timeSeries))
			switch metricType {
			case "gauge":
				gauge := prometheus.NewGauge(prometheus.GaugeOpts{
					Name:        metricName,
					ConstLabels: labels,
				})
				gauge.Set(point.Value.GetDoubleValue())
				ch <- gauge
			case "counter":
				counter := prometheus.NewCounter(prometheus.CounterOpts{
					Name:        metricName,
					ConstLabels: labels,
				})
				counter.Add(point.Value.GetDoubleValue())
				ch <- counter
			default:
				collector.logger.Info("unsupported metric type", "time_series", timeSeriesName, "metric_name", metricName, "metric_type", metricType)
			}
		case metricpb.MetricDescriptor_DISTRIBUTION:
			collector.logger.Info("unsupported metric distribution", "time_series", timeSeriesName)
		}
	}
}

func extractMetricMetadata(prefix string, name string) (metricType string, metricName string) {
	if !strings.HasPrefix(name, prefix) {
		return "", ""
	}
	meta := name[len(prefix)+1:]
	split := strings.Split(meta, "/")
	if len(split) != 2 {
		return "", ""
	}
	return split[0], split[1]
}

// func ToSeries(timeSeries *monitoringpb.TimeSeries) storage.Series {
// 	var timestamps []int64
// 	var values []float64
// 	for _, point := range timeSeries.GetPoints() {
// 		timestamps = append(timestamps, point.GetInterval().GetStartTime().GetSeconds())
// 		timestamps = append(timestamps, point.GetInterval().GetEndTime().GetSeconds())
// 		doubleValue := point.Value.GetDoubleValue()
// 		// Add value twice, once for the start time and once for the end time.
// 		values = append(values, doubleValue, doubleValue)
// 	}
// 	return storage.MockSeries(timestamps, values, maps.Values(Labels(timeSeries)))
// }

func Labels(timeSeries *monitoringpb.TimeSeries) map[string]string {
	labels := make(map[string]string)
	if resource := timeSeries.GetResource(); resource != nil {
		maps.Copy(labels, resource.GetLabels())
	}
	if metric := timeSeries.GetMetric(); metric != nil {
		maps.Copy(labels, metric.GetLabels())
	}
	return labels
}

// func ToSeriesSet(timeSeries []*monitoringpb.TimeSeries) storage.SeriesSet {
// 	var series []storage.Series
// 	for _, ts := range timeSeries {
// 		series = append(series, ToSeries(ts))
// 	}
// 	return &mockSeriesSet{series: series}
// }

// type mockSeriesSet struct {
// 	series []storage.Series
// 	index  uint
// }

// func (s *mockSeriesSet) Next() bool {
// 	if len(s.series) >= int(s.index) {
// 		return false
// 	}
// 	s.index++
// 	return true
// }

// func (s *mockSeriesSet) At() storage.Series {
// 	return s.series[s.index]
// }

// func (s *mockSeriesSet) Err() error {
// 	return nil
// }

// func (s *mockSeriesSet) Warnings() storage.Warnings {
// 	return nil
// }

// type matchersFilter struct {
// 	matchers []*labels.Matcher
// }

// func (f *matchersFilter) filter(timeSeries *monitoringpb.TimeSeries, _ *monitoringpb.Point) bool {
// 	for _, matcher := range f.matchers {
// 		if matcher.Matches(getLabelValue(timeSeries, matcher.Name)) {
// 			return true
// 		}
// 	}
// 	return false
// }

// func getLabelValue(timeSeries *monitoringpb.TimeSeries, labelName string) string {
// 	if metric := timeSeries.GetMetric(); metric != nil {
// 		if val, ok := metric.GetLabels()[labelName]; ok {
// 			return val
// 		}
// 	}
// 	if resource := timeSeries.GetResource(); resource != nil {
// 		if val, ok := resource.GetLabels()[labelName]; ok {
// 			return val
// 		}
// 	}
// 	return ""
// }
