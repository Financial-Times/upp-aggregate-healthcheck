{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 24 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "cluster_subdomain" -}}
{{- required "The __ext.target_cluster.sub_domain value is required for this app. Use helm upgrade ... --set __ext.target_cluster.sub_domain=... when installing. Example value: upp-prod-publish-us" .Values.__ext.target_cluster.sub_domain -}}
{{- end -}}
