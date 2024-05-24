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

package v1

import (
	"context"
	"fmt"

	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	"gomodules.xyz/jsonpatch/v2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func OperatorConfigDefaulter(kubeClient client.Client, publicNamespace string) admission.Handler {
	return admission.HandlerFunc(func(ctx context.Context, req admission.Request) admission.Response {
		oc := req.AdmissionRequest.Object.Object.(*OperatorConfig)
		if oc.Collection.KubeletScraping == nil {
			return admission.Allowed("")
		}

		if err := createKubeletScraping(ctx, kubeClient, publicNamespace, oc.Collection.KubeletScraping.Interval); err != nil {
			return admission.Denied(err.Error())
		}
		oc.Collection.KubeletScraping = nil

		return admission.Patched("", jsonpatch.NewOperation("remove", "spec/collection/kubeletScraping", nil)).
			WithWarnings("Field `spec.collection.kubeletScraping` is deprecated. Please edit %q.")
	})
}

func createKubeletScraping(ctx context.Context, kubeClient client.Client, publicNamespace string, interval string) error {
	if _, err := prommodel.ParseDuration(interval); err != nil {
		return fmt.Errorf("invalid scrape interval: %w", err)
	}

	dropByName := func(pattern string) RelabelingRule {
		return RelabelingRule{
			Action:       string(relabel.Drop),
			SourceLabels: []string{"__name__"},
			Regex:        pattern,
		}
	}

	cnm := ClusterNodeMonitoring{
		ObjectMeta: v1.ObjectMeta{
			Name:      "kubelet",
			Namespace: publicNamespace,
		},
		Spec: ClusterNodeMonitoringSpec{
			Endpoints: []ScrapeNodeEndpoint{
				{
					Path:     "/metrics",
					Interval: interval,
					Scheme:   "https",
					MetricRelabeling: []RelabelingRule{
						dropByName(`kubelet_(pod_worker_latency_microseconds|pod_start_latency_microseconds|cgroup_manager_latency_microseconds|pod_worker_start_latency_microseconds|pleg_relist_latency_microseconds|pleg_relist_interval_microseconds|runtime_operations|runtime_operations_latency_microseconds|runtime_operations_errors|eviction_stats_age_microseconds|device_plugin_registration_count|device_plugin_alloc_latency_microseconds|network_plugin_operations_latency_microseconds)`),
						dropByName(`scheduler_(e2e_scheduling_latency_microseconds|scheduling_algorithm_predicate_evaluation|scheduling_algorithm_priority_evaluation|scheduling_algorithm_preemption_evaluation|scheduling_algorithm_latency_microseconds|binding_latency_microseconds|scheduling_latency_seconds)`),
						dropByName(`apiserver_(request_count|request_latencies|request_latencies_summary|dropped_requests|storage_data_key_generation_latencies_microseconds|storage_transformation_failures_total|storage_transformation_latencies_microseconds|proxy_tunnel_sync_latency_secs|longrunning_gauge|registered_watchers)`),
						dropByName(`kubelet_docker_(operations|operations_latency_microseconds|operations_errors|operations_timeout)`),
						dropByName(`reflector_(items_per_list|items_per_watch|list_duration_seconds|lists_total|short_watches_total|watch_duration_seconds|watches_total)`),
						dropByName(`etcd_(helper_cache_hit_count|helper_cache_miss_count|helper_cache_entry_count|object_counts|request_cache_get_latencies_summary|request_cache_add_latencies_summary|request_latencies_summary)`),
						dropByName(`transformation_(transformation_latencies_microseconds|failures_total)`),
						dropByName(`(admission_quota_controller_adds|admission_quota_controller_depth|admission_quota_controller_longest_running_processor_microseconds|admission_quota_controller_queue_latency|admission_quota_controller_unfinished_work_seconds|admission_quota_controller_work_duration|APIServiceOpenAPIAggregationControllerQueue1_adds|APIServiceOpenAPIAggregationControllerQueue1_depth|APIServiceOpenAPIAggregationControllerQueue1_longest_running_processor_microseconds|APIServiceOpenAPIAggregationControllerQueue1_queue_latency|APIServiceOpenAPIAggregationControllerQueue1_retries|APIServiceOpenAPIAggregationControllerQueue1_unfinished_work_seconds|APIServiceOpenAPIAggregationControllerQueue1_work_duration|APIServiceRegistrationController_adds|APIServiceRegistrationController_depth|APIServiceRegistrationController_longest_running_processor_microseconds|APIServiceRegistrationController_queue_latency|APIServiceRegistrationController_retries|APIServiceRegistrationController_unfinished_work_seconds|APIServiceRegistrationController_work_duration|autoregister_adds|autoregister_depth|autoregister_longest_running_processor_microseconds|autoregister_queue_latency|autoregister_retries|autoregister_unfinished_work_seconds|autoregister_work_duration|AvailableConditionController_adds|AvailableConditionController_depth|AvailableConditionController_longest_running_processor_microseconds|AvailableConditionController_queue_latency|AvailableConditionController_retries|AvailableConditionController_unfinished_work_seconds|AvailableConditionController_work_duration|crd_autoregistration_controller_adds|crd_autoregistration_controller_depth|crd_autoregistration_controller_longest_running_processor_microseconds|crd_autoregistration_controller_queue_latency|crd_autoregistration_controller_retries|crd_autoregistration_controller_unfinished_work_seconds|crd_autoregistration_controller_work_duration|crdEstablishing_adds|crdEstablishing_depth|crdEstablishing_longest_running_processor_microseconds|crdEstablishing_queue_latency|crdEstablishing_retries|crdEstablishing_unfinished_work_seconds|crdEstablishing_work_duration|crd_finalizer_adds|crd_finalizer_depth|crd_finalizer_longest_running_processor_microseconds|crd_finalizer_queue_latency|crd_finalizer_retries|crd_finalizer_unfinished_work_seconds|crd_finalizer_work_duration|crd_naming_condition_controller_adds|crd_naming_condition_controller_depth|crd_naming_condition_controller_longest_running_processor_microseconds|crd_naming_condition_controller_queue_latency|crd_naming_condition_controller_retries|crd_naming_condition_controller_unfinished_work_seconds|crd_naming_condition_controller_work_duration|crd_openapi_controller_adds|crd_openapi_controller_depth|crd_openapi_controller_longest_running_processor_microseconds|crd_openapi_controller_queue_latency|crd_openapi_controller_retries|crd_openapi_controller_unfinished_work_seconds|crd_openapi_controller_work_duration|DiscoveryController_adds|DiscoveryController_depth|DiscoveryController_longest_running_processor_microseconds|DiscoveryController_queue_latency|DiscoveryController_retries|DiscoveryController_unfinished_work_seconds|DiscoveryController_work_duration|kubeproxy_sync_proxy_rules_latency_microseconds|non_structural_schema_condition_controller_adds|non_structural_schema_condition_controller_depth|non_structural_schema_condition_controller_longest_running_processor_microseconds|non_structural_schema_condition_controller_queue_latency|non_structural_schema_condition_controller_retries|non_structural_schema_condition_controller_unfinished_work_seconds|non_structural_schema_condition_controller_work_duration|rest_client_request_latency_seconds|storage_operation_errors_total|storage_operation_status_count)`),
					},
				},
				{
					Path:     "/metrics/cadvisor",
					Interval: interval,
					Scheme:   "https",
					MetricRelabeling: []RelabelingRule{
						dropByName(`container_(network_tcp_usage_total|network_udp_usage_total|tasks_state|cpu_load_average_10s|blkio_device_usage_total|memory_failures_total)`),
					},
				},
			},
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, kubeClient, cnm.DeepCopy(), func() error {
		return nil
	})
	return err
}

type OperatorConfigValidator struct {
	Namespace    string
	Name         string
	VPAAvailable bool
}

func (v *OperatorConfigValidator) ValidateCreate(_ context.Context, o runtime.Object) (admission.Warnings, error) {
	oc := o.(*OperatorConfig)

	if oc.Namespace != v.Namespace || oc.Name != v.Name {
		return nil, fmt.Errorf("OperatorConfig must be in namespace %q with name %q", v.Namespace, v.Name)
	}
	if oc.Scaling.VPA.Enabled && !v.VPAAvailable {
		return nil, fmt.Errorf("vertical pod autoscaling is not available - install vpa support and restart the operator")
	}
	return nil, oc.Validate()
}

func (v *OperatorConfigValidator) ValidateUpdate(ctx context.Context, _, o runtime.Object) (admission.Warnings, error) {
	return v.ValidateCreate(ctx, o)
}

func (v *OperatorConfigValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
