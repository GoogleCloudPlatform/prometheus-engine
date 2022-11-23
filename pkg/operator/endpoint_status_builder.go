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

package operator

import (
	"sort"
	"strconv"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/pkg/errors"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// How many targets to keep in each group.
	maxSampleTargetSize = 5
)

func buildEndpointStatuses(targets []*prometheusv1.TargetsResult) (map[string][]monitoringv1.ScrapeEndpointStatus, error) {
	endpointBuilder := &scrapeEndpointBuilder{
		mapByJobByEndpoint: make(map[string]map[string]*scrapeEndpointStatusBuilder),
		total:              0,
		failed:             0,
		time:               metav1.Now(),
	}

	for _, target := range targets {
		if err := endpointBuilder.add(target); err != nil {
			return nil, err
		}
	}

	return endpointBuilder.build(), nil
}

type scrapeEndpointBuilder struct {
	mapByJobByEndpoint map[string]map[string]*scrapeEndpointStatusBuilder
	total              uint32
	failed             uint32
	time               metav1.Time
}

func (b *scrapeEndpointBuilder) add(target *prometheusv1.TargetsResult) error {
	b.total += 1
	if target != nil {
		for _, activeTarget := range target.Active {
			if err := b.addActiveTarget(activeTarget, b.time); err != nil {
				return err
			}
		}
	} else {
		b.failed += 1
	}
	return nil
}

func (b *scrapeEndpointBuilder) addActiveTarget(activeTarget prometheusv1.ActiveTarget, time metav1.Time) error {
	portIndex := strings.LastIndex(activeTarget.ScrapePool, "/")
	if portIndex == -1 {
		return errors.New("Malformed scrape pool: " + activeTarget.ScrapePool)
	}
	job := activeTarget.ScrapePool[:portIndex]
	endpoint := activeTarget.ScrapePool[portIndex+1:]
	mapByEndpoint, ok := b.mapByJobByEndpoint[job]
	if !ok {
		tmp := make(map[string]*scrapeEndpointStatusBuilder)
		mapByEndpoint = tmp
		b.mapByJobByEndpoint[job] = mapByEndpoint
	}

	statusBuilder, exists := mapByEndpoint[endpoint]
	if !exists {
		statusBuilder = newScrapeEndpointStatusBuilder(&activeTarget, time)
		mapByEndpoint[endpoint] = statusBuilder
	}
	statusBuilder.addSampleTarget(&activeTarget)
	return nil
}

func (b *scrapeEndpointBuilder) build() map[string][]monitoringv1.ScrapeEndpointStatus {
	fraction := float64(b.total-b.failed) / float64(b.total)
	collectorsFraction := strconv.FormatFloat(fraction, 'f', -1, 64)
	resultMap := make(map[string][]monitoringv1.ScrapeEndpointStatus)

	for job, endpointMap := range b.mapByJobByEndpoint {
		endpointStatuses := make([]monitoringv1.ScrapeEndpointStatus, 0)
		for _, statusBuilder := range endpointMap {
			endpointStatus := statusBuilder.build()
			endpointStatus.CollectorsFraction = collectorsFraction
			endpointStatuses = append(endpointStatuses, endpointStatus)
		}

		// Make endpoint status deterministic.
		sort.SliceStable(endpointStatuses, func(i, j int) bool {
			lhsName := endpointStatuses[i].Name
			rhsName := endpointStatuses[j].Name
			return lhsName < rhsName
		})
		resultMap[job] = endpointStatuses
	}
	return resultMap
}

type scrapeEndpointStatusBuilder struct {
	status       monitoringv1.ScrapeEndpointStatus
	groupByError map[string]*monitoringv1.SampleGroup
}

func newScrapeEndpointStatusBuilder(target *prometheusv1.ActiveTarget, time metav1.Time) *scrapeEndpointStatusBuilder {
	return &scrapeEndpointStatusBuilder{
		status: monitoringv1.ScrapeEndpointStatus{
			Name:               target.ScrapePool,
			ActiveTargets:      0,
			UnhealthyTargets:   0,
			LastUpdateTime:     time,
			CollectorsFraction: "0",
		},
		groupByError: make(map[string]*monitoringv1.SampleGroup),
	}
}

// Adds a sample target, potentially merging with a pre-existing one.
func (b *scrapeEndpointStatusBuilder) addSampleTarget(target *prometheusv1.ActiveTarget) {
	b.status.ActiveTargets += 1
	errorType := target.LastError
	lastError := &errorType
	if target.Health == "up" {
		if len(target.LastError) == 0 {
			lastError = nil
		}
	} else {
		b.status.UnhealthyTargets += 1
	}

	sampleGroup, ok := b.groupByError[errorType]
	sampleTarget := monitoringv1.SampleTarget{
		Health:                    string(target.Health),
		LastError:                 lastError,
		Labels:                    target.Labels,
		LastScrapeDurationSeconds: strconv.FormatFloat(target.LastScrapeDuration, 'f', -1, 64),
	}
	if !ok {
		sampleGroup = &monitoringv1.SampleGroup{
			SampleTargets: []monitoringv1.SampleTarget{},
			Count:         new(int32),
		}
		b.groupByError[errorType] = sampleGroup
	}
	*sampleGroup.Count += 1
	sampleGroup.SampleTargets = append(sampleGroup.SampleTargets, sampleTarget)
}

// build a deterministic (regarding array ordering) status object.
func (b *scrapeEndpointStatusBuilder) build() monitoringv1.ScrapeEndpointStatus {
	// Deterministic sample group by error.
	for _, sampleGroup := range b.groupByError {
		sort.SliceStable(sampleGroup.SampleTargets, func(i, j int) bool {
			// Every sample target is guaranteed to have an instance label.
			lhsInstance := sampleGroup.SampleTargets[i].Labels["instance"]
			rhsInstance := sampleGroup.SampleTargets[j].Labels["instance"]
			return lhsInstance < rhsInstance
		})
		sampleTargetsSize := len(sampleGroup.SampleTargets)
		if sampleTargetsSize > maxSampleTargetSize {
			sampleTargetsSize = maxSampleTargetSize
		}
		sampleGroup.SampleTargets = sampleGroup.SampleTargets[0:sampleTargetsSize]
		b.status.SampleGroups = append(b.status.SampleGroups, *sampleGroup)
	}
	sort.SliceStable(b.status.SampleGroups, func(i, j int) bool {
		// Assumes that every sample target in a group has the same error.
		lhsError := b.status.SampleGroups[i].SampleTargets[0].LastError
		rhsError := b.status.SampleGroups[j].SampleTargets[0].LastError
		if lhsError == nil {
			return false
		} else if rhsError == nil {
			return true
		}
		return *lhsError < *rhsError
	})
	return b.status
}
