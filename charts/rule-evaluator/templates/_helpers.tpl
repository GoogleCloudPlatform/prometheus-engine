{{/*
Expand the name of the chart.
*/}}
{{- define "rule-evaluator.name" -}}
  {{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "rule-evaluator.fullname" -}}
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
{{- define "rule-evaluator.chart" -}}
  {{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "rule-evaluator.labels" -}}
app.kubernetes.io/name: {{ include "rule-evaluator.name" . }}
  {{- if .Values.commonLabels }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ eq .Release.Name "release-name" | ternary (printf "%s-%s" ( include "rule-evaluator.name" . ) .Chart.AppVersion) .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
helm.sh/chart: {{ include "rule-evaluator.chart" . }}
  {{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "rule-evaluator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rule-evaluator.name" . }}
{{- end }}

{{/*
Template labels
*/}}
{{- define "rule-evaluator.templateLabels" -}}
{{- include "rule-evaluator.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "rule-evaluator.serviceAccountName" -}}
  {{- if .Values.serviceAccount.create }}
    {{- default (include "rule-evaluator.fullname" .) .Values.serviceAccount.name }}
  {{- else }}
    {{- default "default" .Values.serviceAccount.name }}
  {{- end }}
{{- end }}
