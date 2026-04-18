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
	"fmt"
	"sort"
	"strconv"
	"strings"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// How many targets to keep in each group.
	maxSampleTargetSize = 5
)

func buildEndpointStatuses(targets []*prometheusv1.TargetsResult) (map[string][]monitoringv1.ScrapeEndpointStatus, error) {
	endpointBuilder := &scrapeEndpointBuilder{
		mapByKeyByEndpoint: make(map[string]map[string]*scrapeEndpointStatusBuilder),
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
	mapByKeyByEndpoint map[string]map[string]*scrapeEndpointStatusBuilder
	total              uint32
	failed             uint32
	time               metav1.Time
}

func (b *scrapeEndpointBuilder) add(target *prometheusv1.TargetsResult) error {
	b.total++
	if target != nil {
		for _, activeTarget := range target.Active {
			if err := b.addActiveTarget(activeTarget, b.time); err != nil {
				return err
			}
		}
	} else {
		b.failed++
	}
	return nil
}

func setNamespacedObjectByScrapeJobKey(o monitoringv1.PodMonitoringCRD, split []string, full string) (monitoringv1.PodMonitoringCRD, error) {
	if len(split) != 3 {
		return nil, fmt.Errorf("invalid %s scrape key format %q", split[0], full)
	}

	o.SetNamespace(split[1])
	o.SetName(split[2])
	return o, nil
}

func setClusterScopedObjectByScrapeJobKey(o monitoringv1.PodMonitoringCRD, split []string, full string) (monitoringv1.PodMonitoringCRD, error) {
	if len(split) != 2 {
		return nil, fmt.Errorf("invalid %s scrape key format %q", split[0], full)
	}

	o.SetName(split[1])
	return o, nil
}

// getObjectByScrapeJobKey converts the key to a CRD. See monitoringv1.PodMonitoringCRD.GetKey().
func getObjectByScrapeJobKey(key string) (monitoringv1.PodMonitoringCRD, error) {
	split := strings.Split(key, "/")
	// Generally:
	// - "kind" for scrape pools without a respective CRD.
	// - "kind/name" for cluster-scoped resources.
	// - "kind/namespace/name" for namespaced resources.
	switch split[0] {
	case "kubelet":
		if len(split) != 1 {
			return nil, fmt.Errorf("invalid kubelet scrape key format %q", key)
		}
		return nil, nil
	case "PodMonitoring":
		return setNamespacedObjectByScrapeJobKey(&monitoringv1.PodMonitoring{}, split, key)
	case "ClusterPodMonitoring":
		return setClusterScopedObjectByScrapeJobKey(&monitoringv1.ClusterPodMonitoring{}, split, key)
	case "ClusterNodeMonitoring":
		if _, err := setClusterScopedObjectByScrapeJobKey(&monitoringv1.ClusterPodMonitoring{}, split, key); err != nil {
			return nil, err
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown scrape kind %q", split[0])
	}
}

// scrapePool is the parsed Prometheus scrape pool, which is assigned to the job name from our
// configurations. For example, for PodMonitoring this is `PodMonitoring/namespace/name/port`. The
// key is what identifies the resource (`PodMonitoring/namespace/name`) and the group indicates a
// small subset of that resource (`port`).
type scrapePool struct {
	key   string
	group string
}

func getNamespacedScrapePool(full string, split []string) scrapePool {
	// Same as: len(strings.Join(split, "/")) for "kind/namespace/name"
	index := len(split[0]) + 1 + len(split[1]) + 1 + len(split[2])
	return scrapePool{
		key:   full[:index],
		group: full[index:],
	}
}

func getClusterScopedScrapePool(full string, split []string) scrapePool {
	// Same as: len(strings.Join(split, "/")) for "kind/namespace"
	index := len(split[0]) + 1 + len(split[1])
	return scrapePool{
		key:   full[:index],
		group: full[index:],
	}
}

func parseScrapePool(pool string) (scrapePool, error) {
	split := strings.Split(pool, "/")
	switch split[0] {
	case "kubelet":
		if len(split) != 2 {
			return scrapePool{}, fmt.Errorf("invalid kubelet scrape pool format %q", pool)
		}
		return scrapePool{
			key:   split[0],
			group: split[1],
		}, nil
	case "PodMonitoring":
		if len(split) != 4 {
			return scrapePool{}, fmt.Errorf("invalid PodMonitoring scrape pool format %q", pool)
		}
		return getNamespacedScrapePool(pool, split), nil
	case "ClusterPodMonitoring":
		if len(split) != 3 {
			return scrapePool{}, fmt.Errorf("invalid ClusterPodMonitoring scrape pool format %q", pool)
		}
		return getClusterScopedScrapePool(pool, split), nil
	case "ClusterNodeMonitoring":
		if len(split) != 3 && len(split) != 4 {
			return scrapePool{}, fmt.Errorf("invalid ClusterNodeMonitoring scrape pool format %q", pool)
		}
		return getClusterScopedScrapePool(pool, split), nil
	default:
		return scrapePool{}, fmt.Errorf("unknown scrape kind %q", split[0])
	}
}

func (b *scrapeEndpointBuilder) addActiveTarget(activeTarget prometheusv1.ActiveTarget, time metav1.Time) error {
	scrapePool, err := parseScrapePool(activeTarget.ScrapePool)
	if err != nil {
		return err
	}
	mapByEndpoint, ok := b.mapByKeyByEndpoint[scrapePool.key]
	if !ok {
		tmp := make(map[string]*scrapeEndpointStatusBuilder)
		mapByEndpoint = tmp
		b.mapByKeyByEndpoint[scrapePool.key] = mapByEndpoint
	}

	statusBuilder, exists := mapByEndpoint[scrapePool.group]
	if !exists {
		statusBuilder = newScrapeEndpointStatusBuilder(&activeTarget, time)
		mapByEndpoint[scrapePool.group] = statusBuilder
	}
	statusBuilder.addSampleTarget(&activeTarget)
	return nil
}

func (b *scrapeEndpointBuilder) build() map[string][]monitoringv1.ScrapeEndpointStatus {
	fraction := float64(b.total-b.failed) / float64(b.total)
	collectorsFraction := strconv.FormatFloat(fraction, 'f', -1, 64)
	resultMap := make(map[string][]monitoringv1.ScrapeEndpointStatus)

	for key, endpointMap := range b.mapByKeyByEndpoint {
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
		resultMap[key] = endpointStatuses
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
	b.status.ActiveTargets++
	errorType := target.LastError
	lastError := &errorType
	if target.Health == "up" {
		if len(target.LastError) == 0 {
			lastError = nil
		}
	} else {
		b.status.UnhealthyTargets++
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
	*sampleGroup.Count++
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
		sampleTargetsSize := min(len(sampleGroup.SampleTargets), maxSampleTargetSize)
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
