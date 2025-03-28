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

package v1

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Rules defines Prometheus alerting and recording rules that are scoped
// to the namespace of the resource. Only metric data from this namespace is processed
// and all rule results have their project_id, cluster, and namespace label preserved
// for query processing.
// If the location label is not preserved by the rule, it defaults to the cluster's location.
//
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type Rules struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of rules to record and alert on.
	Spec RulesSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status RulesStatus `json:"status"`
}

type RulesCustomValidator struct{}

func (r *RulesCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, err := obj.(*Rules).RuleGroupsConfig("", "", "")
	return nil, err
}

func (r *RulesCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	// Validity does not depend on state changes.
	return r.ValidateCreate(ctx, newObj)
}

func (*RulesCustomValidator) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	// Deletions are always valid.
	return nil, nil
}

func (r *Rules) GetMonitoringStatus() *MonitoringStatus {
	return &r.Status.MonitoringStatus
}

// RulesList is a list of Rules.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RulesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rules `json:"items"`
}

// ClusterRules defines Prometheus alerting and recording rules that are scoped
// to the current cluster. Only metric data from the current cluster is processed
// and all rule results have their project_id and cluster label preserved
// for query processing.
// If the location label is not preserved by the rule, it defaults to the cluster's location.
//
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type ClusterRules struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of rules to record and alert on.
	Spec RulesSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status RulesStatus `json:"status"`
}

type ClusterRulesCustomValidator struct{}

func (r *ClusterRulesCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, err := obj.(*ClusterRules).RuleGroupsConfig("", "", "")
	return nil, err
}

func (r *ClusterRulesCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	// Validity does not depend on state changes.
	return r.ValidateCreate(ctx, newObj)
}

func (*ClusterRulesCustomValidator) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	// Deletions are always valid.
	return nil, nil
}

func (r *ClusterRules) GetMonitoringStatus() *MonitoringStatus {
	return &r.Status.MonitoringStatus
}

// ClusterRulesList is a list of ClusterRules.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterRulesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRules `json:"items"`
}

// GlobalRules defines Prometheus alerting and recording rules that are scoped
// to all data in the queried project.
// If the project_id or location labels are not preserved by the rule, they default to
// the values of the cluster.
//
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type GlobalRules struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of rules to record and alert on.
	Spec RulesSpec `json:"spec"`
	// Most recently observed status of the resource.
	// +optional
	Status RulesStatus `json:"status"`
}

type GlobalRulesCustomValidator struct{}

func (r *GlobalRulesCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, err := obj.(*GlobalRules).RuleGroupsConfig()
	return nil, err
}

func (r *GlobalRulesCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	// Validity does not depend on state changes.
	return r.ValidateCreate(ctx, newObj)
}

func (*GlobalRulesCustomValidator) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	// Deletions are always valid.
	return nil, nil
}

func (r *GlobalRules) GetMonitoringStatus() *MonitoringStatus {
	return &r.Status.MonitoringStatus
}

// GlobalRulesList is a list of GlobalRules.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GlobalRulesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalRules `json:"items"`
}

// RulesSpec contains specification parameters for a Rules resource.
type RulesSpec struct {
	// A list of Prometheus rule groups.
	Groups []RuleGroup `json:"groups"`
}

// RuleGroup declares rules in the Prometheus format:
// https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/
type RuleGroup struct {
	// The name of the rule group.
	Name string `json:"name"`
	// The interval at which to evaluate the rules. Must be a valid Prometheus duration.
	// +kubebuilder:validation:Format=duration
	// +kubebuilder:default="1m"
	Interval string `json:"interval,omitempty"`
	// A list of rules that are executed sequentially as part of this group.
	// +kubebuilder:validation:MinItems=1
	Rules []Rule `json:"rules"`
}

// Rule is a single rule in the Prometheus format:
// https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/
// +kubebuilder:validation:XValidation:rule="(has(self.record) ? 1 : 0) + (has(self.alert) ? 1 : 0) == 1",message="Must set exactly one of Record or Alert"
// +kubebuilder:validation:XValidation:rule="!has(self.annotations) || has(self.alert)",message="Annotations are only allowed for alerting rules"
type Rule struct {
	// Record the result of the expression to this metric name.
	// Only one of `record` and `alert` must be set.
	// +kubebuilder:validation:Pattern="^[a-zA-Z_:][a-zA-Z0-9_:]*$"
	Record string `json:"record,omitempty"`
	// Name of the alert to evaluate the expression as.
	// Only one of `record` and `alert` must be set.
	Alert string `json:"alert,omitempty"`
	// The PromQL expression to evaluate.
	Expr string `json:"expr"`
	// The duration to wait before a firing alert produced by this rule is sent to Alertmanager.
	// Only valid if `alert` is set.
	// +kubebuilder:validation:Format=duration
	For string `json:"for,omitempty"`
	// A set of labels to attach to the result of the query expression.
	Labels map[string]string `json:"labels,omitempty"`
	// A set of annotations to attach to alerts produced by the query expression.
	// Only valid if `alert` is set.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// RulesStatus contains status information for a Rules resource.
type RulesStatus struct {
	MonitoringStatus `json:",inline"`
}
