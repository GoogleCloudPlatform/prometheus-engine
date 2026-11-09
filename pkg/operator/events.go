// Copyright 2026 Google LLC
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
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"unicode/utf8"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

const (
	reportingController   = "monitoring.googleapis.com/gmp-operator"
	maxEventNoteLength    = 1024
	maxEventObjNameLength = 140
)

// recordWarningEvents emits Kubernetes Warning Events when a GMP custom resource fails
// operator-side validation. An event is always emitted in opNamespace with regarding set
// to Deployment/gmp-operator (and related set to obj) so it surfaces in the Google Cloud
// Console workload events tab. When obj is namespaced, a second event is emitted in
// obj.GetNamespace() with regarding set directly to obj. Event creation failures are
// logged and otherwise ignored so they never block reconciliation.
func recordWarningEvents(ctx context.Context, c client.Client, opNamespace string, obj client.Object, kind, reason string, now metav1.Time, warnErr error) {
	logger, _ := logr.FromContext(ctx)
	if opNamespace == "" {
		opNamespace = DefaultOperatorNamespace
	}

	eventTime := metav1.NewMicroTime(now.Time)
	objRef := corev1.ObjectReference{
		APIVersion: monitoringv1.SchemeGroupVersion.String(),
		Kind:       kind,
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
		UID:        obj.GetUID(),
	}

	lowerKind := strings.ToLower(kind)
	objName := truncateEventObjName(obj.GetName())
	workloadEventName := fmt.Sprintf("%s.%s.%s.%x", NameOperator, lowerKind, objName, now.UnixNano())
	workloadNote := fmt.Sprintf("%s %s: %s", kind, obj.GetName(), warnErr.Error())
	if obj.GetNamespace() != "" {
		workloadEventName = fmt.Sprintf("%s.%s.%s.%s.%x", NameOperator, lowerKind, obj.GetNamespace(), objName, now.UnixNano())
		workloadNote = fmt.Sprintf("%s %s/%s: %s", kind, obj.GetNamespace(), obj.GetName(), warnErr.Error())
	}

	workloadEvent := &eventsv1.Event{
		Name:                workloadEventName,
		Namespace:           opNamespace,
		EventTime:           eventTime,
		ReportingController: reportingController,
		ReportingInstance:   NameOperator,
		Action:              "Ignored",
		Reason:              reason,
		Note:                truncateEventNote(workloadNote),
		Type:                corev1.EventTypeWarning,
		Regarding: corev1.ObjectReference{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
			Namespace:  opNamespace,
			Name:       NameOperator,
		},
		Related: &objRef,
	}
	if err := c.Create(ctx, workloadEvent); err != nil && !apierrors.IsAlreadyExists(err) {
		logger.Error(err, "create operator workload warning event", "kind", kind, "namespace", obj.GetNamespace(), "name", obj.GetName())
	}

	if obj.GetNamespace() != "" {
		resourceEvent := &eventsv1.Event{
			Name:                fmt.Sprintf("%s.%s.%x", lowerKind, objName, now.UnixNano()),
			Namespace:           obj.GetNamespace(),
			EventTime:           eventTime,
			ReportingController: reportingController,
			ReportingInstance:   NameOperator,
			Action:              "Ignored",
			Reason:              reason,
			Note:                truncateEventNote(warnErr.Error()),
			Type:                corev1.EventTypeWarning,
			Regarding:           objRef,
		}
		if err := c.Create(ctx, resourceEvent); err != nil && !apierrors.IsAlreadyExists(err) {
			logger.Error(err, "create resource warning event", "kind", kind, "namespace", obj.GetNamespace(), "name", obj.GetName())
		}
	}
}

// truncateEventNote caps an event note to maxEventNoteLength bytes without splitting
// a multi-byte UTF-8 rune at the cut boundary.
func truncateEventNote(note string) string {
	if len(note) <= maxEventNoteLength {
		return note
	}
	cut := maxEventNoteLength - 3
	for cut > 0 && !utf8.RuneStart(note[cut]) {
		cut--
	}
	return note[:cut] + "..."
}

// truncateEventObjName caps a resource name so generated Kubernetes Event names stay
// within the 253-character DNS subdomain limit and end on an alphanumeric character.
// When truncating, an 8-character FNV-1a hex digest suffix is appended so distinct long
// names that share a common prefix do not collide.
func truncateEventObjName(name string) string {
	if len(name) <= maxEventObjNameLength {
		return name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	prefix := strings.TrimRight(name[:maxEventObjNameLength-9], "-.")
	if prefix == "" {
		return fmt.Sprintf("%08x", h.Sum32())
	}
	return fmt.Sprintf("%s-%08x", prefix, h.Sum32())
}
