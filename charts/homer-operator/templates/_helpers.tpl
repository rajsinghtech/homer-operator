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
{{- default (include "homer-operator.fullname" .) .Values.serviceAccount.name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- default "default" .Values.serviceAccount.name | trunc 63 | trimSuffix "-" }}
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
{{- include "homer-operator.suffixedName" (list (include "homer-operator.fullname" .) "metrics") }}
{{- end }}

{{/*
Create leader election role name
*/}}
{{- define "homer-operator.leaderElectionRoleName" -}}
{{- include "homer-operator.suffixedName" (list (include "homer-operator.fullname" .) "leader-election") }}
{{- end }}

{{/*
Create manager role name
*/}}
{{- define "homer-operator.managerRoleName" -}}
{{- include "homer-operator.suffixedName" (list (include "homer-operator.fullname" .) "manager") }}
{{- end }}

{{/*
Create metrics reader role name
*/}}
{{- define "homer-operator.metricsReaderRoleName" -}}
{{- include "homer-operator.suffixedName" (list (include "homer-operator.fullname" .) "metrics-reader") }}
{{- end }}

{{/*
Append a suffix without exceeding Kubernetes' 63-character name limit.
*/}}
{{- define "homer-operator.suffixedName" -}}
{{- $base := index . 0 -}}
{{- $suffix := index . 1 -}}
{{- $maxBaseLength := int (sub 63 (add 1 (len $suffix))) -}}
{{- printf "%s-%s" ($base | trunc $maxBaseLength | trimSuffix "-") $suffix | trimSuffix "-" }}
{{- end }}

{{/*
Create the ServiceAccount used by the secure ServiceMonitor.
*/}}
{{- define "homer-operator.metricsScraperServiceAccountName" -}}
{{- $default := include "homer-operator.suffixedName" (list (include "homer-operator.fullname" .) "metrics") -}}
{{- default $default .Values.serviceMonitor.auth.serviceAccountName | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the token Secret used by the secure ServiceMonitor.
*/}}
{{- define "homer-operator.metricsScraperTokenSecretName" -}}
{{- $default := include "homer-operator.suffixedName" (list (include "homer-operator.fullname" .) "metrics-token") -}}
{{- default $default .Values.serviceMonitor.auth.tokenSecretName | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the optional Grafana dashboard ConfigMap.
*/}}
{{- define "homer-operator.grafanaDashboardName" -}}
{{- include "homer-operator.suffixedName" (list (include "homer-operator.fullname" .) "grafana-dashboard") }}
{{- end }}

{{/*
Create the image name
*/}}
{{- define "homer-operator.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else if .Values.image.tag }}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository .Chart.AppVersion }}
{{- end }}
{{- end }}

{{/*
Create the homer dashboard image name
*/}}
{{- define "homer-operator.homerImage" -}}
{{- printf "%s:%s" .Values.homer.image.repository .Values.homer.image.tag }}
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
