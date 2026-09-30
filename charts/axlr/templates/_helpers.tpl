{{- define "axlr.name" -}}
{{- printf "%s-axlr" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- define "axlr.labels" -}}
app.kubernetes.io/name: axlr
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}
