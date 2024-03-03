# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

{{/*
Expand the name of the chart.
*/}}
{{- define "prometheus-engine.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "prometheus-engine.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "prometheus-engine.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "prometheus-engine.labels" -}}
  {{- if not .Values.noCommonLabels -}}
app.kubernetes.io/name: {{ include "prometheus-engine.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ eq .Release.Name "release-name" | ternary (printf "%s-%s" ( include "prometheus-engine.name" . ) .Chart.AppVersion) .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/part-of: {{ eq .Release.Name "release-name" | ternary "gmp" .Release.Name }}
helm.sh/chart: {{ include "prometheus-engine.chart" . }}
  {{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "prometheus-engine.selectorLabels" -}}
app.kubernetes.io/name: {{ include "prometheus-engine.name" . }}
{{- end }}

{{/*
Operator labels
*/}}
{{- define "prometheus-engine.operator.labels" -}}
app: managed-prometheus-operator
app.kubernetes.io/component: operator
app.kubernetes.io/name: gmp-operator
  {{- if not .Values.noCommonLabels }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ eq .Release.Name "release-name" | ternary (printf "%s-%s" ( include "prometheus-engine.name" . ) .Chart.AppVersion) .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
  {{- end }}
app.kubernetes.io/part-of: {{ eq .Release.Name "release-name" | ternary "gmp" .Release.Name }}
  {{- if not .Values.noCommonLabels }}
helm.sh/chart: {{ include "prometheus-engine.chart" . }}
  {{- end }}
{{- end }}

{{/*
Operator selector labels
*/}}
{{- define "prometheus-engine.operator.selectorLabels" -}}
app.kubernetes.io/component: operator
app.kubernetes.io/name: gmp-operator
app.kubernetes.io/part-of: {{ eq .Release.Name "release-name" | ternary "gmp" .Release.Name }}
{{- end }}

{{/*
Operator template labels
*/}}
{{- define "prometheus-engine.operator.templateLabels" -}}
app: managed-prometheus-operator
{{ include "prometheus-engine.operator.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
{{- end }}

{{/*
Collector labels
*/}}
{{- define "prometheus-engine.collector.labels" -}}
{{ include "prometheus-engine.labels" . }}
{{- end }}

{{/*
Collector selector labels
*/}}
{{- define "prometheus-engine.collector.selectorLabels" -}}
app.kubernetes.io/name: collector
{{- end }}

{{/*
Collector template labels
*/}}
{{- define "prometheus-engine.collector.templateLabels" -}}
app: managed-prometheus-collector
{{ include "prometheus-engine.collector.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
{{- end }}

{{/*
Rule-evaluator labels
*/}}
{{- define "prometheus-engine.rule-evaluator.labels" -}}
{{ include "prometheus-engine.labels" . }}
{{- end }}

{{/*
Rule-evaluator selector labels
*/}}
{{- define "prometheus-engine.rule-evaluator.selectorLabels" -}}
app.kubernetes.io/name: rule-evaluator
{{- end }}

{{/*
Rule-evaluator template labels
*/}}
{{- define "prometheus-engine.rule-evaluator.templateLabels" -}}
{{ include "prometheus-engine.rule-evaluator.selectorLabels" . }}
app: managed-prometheus-rule-evaluator
app.kubernetes.io/version: {{ .Chart.AppVersion }}
{{- end }}

{{/*
Alertmanager labels
*/}}
{{- define "prometheus-engine.alertmanager.labels" -}}
{{ include "prometheus-engine.labels" . }}
{{- end }}

{{/*
Alertmanager selector labels
*/}}
{{- define "prometheus-engine.alertmanager.selectorLabels" -}}
app: managed-prometheus-alertmanager
app.kubernetes.io/name: alertmanager
{{- end }}

{{/*
Alertmanager template labels
*/}}
{{- define "prometheus-engine.alertmanager.templateLabels" -}}
{{ include "prometheus-engine.alertmanager.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
{{- end }}

{{/*
Create the name of the collector service account to use
*/}}
{{- define "prometheus-engine.collector.serviceAccountName" -}}
  {{- if .Values.collector.serviceAccount.create }}
    {{- default "collector" .Values.collector.serviceAccount.name }}
  {{- else }}
    {{- default "default" .Values.collector.serviceAccount.name }}
  {{- end }}
{{- end }}

{{/*
Create the name of the operator service account to use
*/}}
{{- define "prometheus-engine.operator.serviceAccountName" -}}
  {{- if .Values.operator.serviceAccount.create }}
    {{- default "operator" .Values.operator.serviceAccount.name }}
  {{- else }}
    {{- default "default" .Values.operator.serviceAccount.name }}
  {{- end }}
{{- end }}
