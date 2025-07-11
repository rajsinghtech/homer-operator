{{/*
Expand the name of the chart.
*/}}
{{- define "homer-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "homer-operator.fullname" -}}
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
{{- define "homer-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "homer-operator.labels" -}}
helm.sh/chart: {{ include "homer-operator.chart" . }}
{{ include "homer-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: homer-operator
{{- end }}

{{/*
Selector labels
*/}}
{{- define "homer-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "homer-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "homer-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "homer-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the namespace to use
*/}}
{{- define "homer-operator.namespace" -}}
{{- .Release.Namespace }}
{{- end }}

{{/*
Create the name of the manager deployment
*/}}
{{- define "homer-operator.managerName" -}}
{{- include "homer-operator.fullname" . }}
{{- end }}

{{/*
Create the name of the metrics service
*/}}
{{- define "homer-operator.metricsServiceName" -}}
{{- printf "%s-metrics" (include "homer-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the webhook service
*/}}
{{- define "homer-operator.webhookServiceName" -}}
{{- printf "%s-webhook" (include "homer-operator.fullname" .) }}
{{- end }}

{{/*
Create leader election role name
*/}}
{{- define "homer-operator.leaderElectionRoleName" -}}
{{- printf "%s-leader-election" (include "homer-operator.fullname" .) }}
{{- end }}

{{/*
Create manager role name
*/}}
{{- define "homer-operator.managerRoleName" -}}
{{- printf "%s-manager" (include "homer-operator.fullname" .) }}
{{- end }}

{{/*
Create metrics reader role name
*/}}
{{- define "homer-operator.metricsReaderRoleName" -}}
{{- printf "%s-metrics-reader" (include "homer-operator.fullname" .) }}
{{- end }}

{{/*
Create proxy role name
*/}}
{{- define "homer-operator.proxyRoleName" -}}
{{- printf "%s-proxy" (include "homer-operator.fullname" .) }}
{{- end }}

{{/*
Create proxy role binding name
*/}}
{{- define "homer-operator.proxyRoleBindingName" -}}
{{- printf "%s-proxy" (include "homer-operator.fullname" .) }}
{{- end }}

{{/*
Create the image name
*/}}
{{- define "homer-operator.image" -}}
{{- if .Values.image.tag }}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository .Chart.AppVersion }}
{{- end }}
{{- end }}

{{/*
Create environment variables for the operator
*/}}
{{- define "homer-operator.env" -}}
{{- if .Values.operator.enableGatewayAPI }}
- name: ENABLE_GATEWAY_API
  value: "true"
{{- end }}
{{- with .Values.env }}
{{- toYaml . }}
{{- end }}
{{- end }}

{{/*
Create environment variables from secrets/configmaps
*/}}
{{- define "homer-operator.envFrom" -}}
{{- with .Values.envFrom }}
{{- toYaml . }}
{{- end }}
{{- end }}

{{/*
Standardized annotations helper
*/}}
{{- define "homer-operator.annotations" -}}
{{- $annotations := . -}}
{{- if $annotations }}
annotations:
  {{- toYaml $annotations | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Component labels helper
*/}}
{{- define "homer-operator.componentLabels" -}}
{{- $component := . -}}
{{- if $component }}
app.kubernetes.io/component: {{ $component }}
{{- end }}
{{- end }}