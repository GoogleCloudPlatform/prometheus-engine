{{/*
Expand the name of the chart.
*/}}
{{- define "gmp.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "gmp.fullname" -}}
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
{{- define "gmp.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Operator labels
*/}}
{{- define "gmp.operatorLabels" -}}
helm.sh/chart: {{ include "gmp.chart" . }}
{{ include "gmp.operatorSelectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Operator selector labels
*/}}
{{- define "gmp.operatorSelectorLabels" -}}
app.kubernetes.io/name: {{ include "gmp.name" . }}-{{ .Values.operator.name }}
app.kubernetes.io/component: {{ .Values.operator.name }}
app.kubernetes.io/part-of: gmp
{{- end }}

{{/*
Frontend labels
*/}}
{{- define "gmp.frontendLabels" -}}
helm.sh/chart: {{ include "gmp.chart" . }}
{{ include "gmp.frontendSelectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Frontend selector labels
*/}}
{{- define "gmp.frontendSelectorLabels" -}}
app.kubernetes.io/name: {{ include "gmp.name" . }}-{{ .Values.frontend.name }}
app.kubernetes.io/component: {{ .Values.frontend.name }}
{{- end }}
