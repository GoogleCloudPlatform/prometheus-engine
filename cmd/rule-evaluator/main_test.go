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

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// convert storage.SeriesSet to promql.Matrix.
func expandSeriesSet(s storage.SeriesSet) promql.Matrix {
	m := promql.Matrix{}
	for s.Next() {
		storageSeries := s.At()
		it := storageSeries.Iterator(nil)
		pts := []promql.Point{}
		for it.Next() != chunkenc.ValNone {
			t, v := it.At()
			pts = append(pts, promql.Point{
				T: t,
				V: v,
			})
		}
		m = append(m, promql.Series{
			Metric: storageSeries.Labels(),
			Points: pts,
		})
	}
	return m
}

// compareWarningsEquality compares two warnings.
func compareWarningsEquality(w1, w2 storage.Warnings) bool {
	if len(w1) != len(w2) {
		return false
	}
	for i := 0; i < len(w1); i++ {
		if w1[i] != w2[i] && w1[i].Error() != w2[i].Error() {
			return false
		}
	}
	return true
}

// cmpErrsEquality compares two errors.
func cmpErrsEquality(err1, err2 error) bool {
	if err1 == nil || err2 == nil {
		return err1 == err2
	}
	return err1.Error() == err2.Error()
}

func TestSelect(t *testing.T) {
	cases := []struct {
		description string
		db          *queryAccess
		want        *listSeriesSet
	}{
		// Success case
		{
			description: "success case",
			db: &queryAccess{
				mint: 1000,
				maxt: 2000,
				query: func(ctx context.Context, q string, timeValue time.Time, v1api v1.API) (parser.Value, v1.Warnings, error) {
					maxt := time.Unix(2000, 0)
					expectedQuery := "{__name__=\"testLabel\"}[1000s]"
					if q != expectedQuery {
						return nil, nil, fmt.Errorf("Expected query to be: %s, Actual query: %s ", expectedQuery, q)
					}
					if timeValue != maxt {
						return nil, nil, fmt.Errorf("Expected t to be: %s, Actual t: %s ", maxt.String(), timeValue.String())
					}
					return promql.Matrix{{
						Metric: labels.FromStrings(model.MetricNameLabel, "testLabel"),
						Points: []promql.Point{{T: 600613, V: 1.0}}}}, nil, nil
				},
			},
			want: &listSeriesSet{
				m: promql.Matrix{{
					Metric: labels.FromStrings(model.MetricNameLabel, "testLabel"),
					Points: []promql.Point{{T: 600613, V: 1.0}}}},
			},
		},
		// Error cases
		{
			description: "queryfunc returns an error",
			db: &queryAccess{
				mint: 1000,
				maxt: 2000,
				query: func(ctx context.Context, q string, timeValue time.Time, v1api v1.API) (parser.Value, v1.Warnings, error) {
					return nil, nil, errors.New("Query Error")
				},
			},
			want: &listSeriesSet{
				m:   promql.Matrix{},
				err: errors.New("Query Error"),
			},
		},
		{
			description: "mint can't equal maxt",
			db:          &queryAccess{},
			want: &listSeriesSet{
				m: promql.Matrix{},
			},
		},
		{
			description: "queryfunc returns a vector instead of a matrix",
			db: &queryAccess{
				maxt: 1000,
				query: func(ctx context.Context, q string, timeValue time.Time, v1api v1.API) (parser.Value, v1.Warnings, error) {
					return promql.Vector{}, nil, nil
				},
			},
			want: &listSeriesSet{
				m:   promql.Matrix{},
				err: errors.New("Error querying Prometheus, Expected type matrix response. Actual type vector"),
			},
		},
		{
			description: "queryfunc returns a warning",
			db: &queryAccess{
				mint: 0,
				maxt: 1000,
				query: func(ctx context.Context, q string, timeValue time.Time, v1api v1.API) (parser.Value, v1.Warnings, error) {
					return promql.Matrix{}, v1.Warnings{"warning test"}, nil
				},
			},
			want: &listSeriesSet{
				m:        promql.Matrix{},
				warnings: storage.Warnings{errors.New("warning test")},
			},
		},
	}
	for i, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			matchers, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "testLabel")
			if err != nil {
				t.Errorf("Case %d: NewMatcher returned unexpected error: %s", i, err)
			}

			got := c.db.Select(false, nil, matchers)
			if !cmp.Equal(got.Err(), c.want.Err(), cmp.Comparer(cmpErrsEquality)) {
				t.Errorf("Case %d: Expected error: %s, Actual error: %s", i, c.want.Err(), got.Err())
			}
			if !cmp.Equal(got.Warnings(), c.want.Warnings(), cmp.Comparer(compareWarningsEquality)) {
				t.Errorf("Case %d: Expected warnings %s, Actual warnings: %s", i, c.want.Warnings(), got.Warnings())
			}
			if diff := cmp.Diff(expandSeriesSet(got), c.want.m); diff != "" {
				t.Errorf("Case %d: unexpected result: %s", i, diff)
			}
		})
	}
}
