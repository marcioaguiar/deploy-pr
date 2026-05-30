{{/*
Common labels stamped on every resource so `deploy-pr list` and any future
reaper can find the cluster's preview footprint by selector.
*/}}
{{- define "preview.labels" -}}
app.kubernetes.io/managed-by: deploy-pr
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/name: {{ .Release.Name }}
{{- if .Values.prNumber }}
deploy-pr/pr: {{ .Values.prNumber | quote }}
{{- end }}
{{- if .Values.sha }}
deploy-pr/sha: {{ .Values.sha | quote }}
{{- end }}
{{- end -}}

{{/*
Selector labels are a stable subset of full labels: changing managed-by or
SHA must not change the Pod selector or rollouts will fail.
*/}}
{{- define "preview.selectorLabels" -}}
app.kubernetes.io/name: {{ .Release.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Selector labels for the optional PostgreSQL workload.
*/}}
{{- define "preview.postgresSelectorLabels" -}}
app.kubernetes.io/name: {{ include "preview.postgresName" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: postgres
{{- end -}}

{{/*
Full labels for PostgreSQL resources. This mirrors preview.labels but uses the
PostgreSQL workload name so selectors stay unambiguous.
*/}}
{{- define "preview.postgresLabels" -}}
app.kubernetes.io/managed-by: deploy-pr
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/name: {{ include "preview.postgresName" . }}
app.kubernetes.io/component: postgres
{{- if .Values.prNumber }}
deploy-pr/pr: {{ .Values.prNumber | quote }}
{{- end }}
{{- if .Values.sha }}
deploy-pr/sha: {{ .Values.sha | quote }}
{{- end }}
{{- end -}}

{{/*
TLS secret name derived from host: dots are not legal in Secret names, so
they collapse to dashes (pr-1.preview.example.com -> pr-1-preview-example-com-tls).
*/}}
{{- define "preview.tlsSecretName" -}}
{{ printf "%s-tls" (replace "." "-" .Values.host) }}
{{- end -}}

{{/*
PostgreSQL names and connection material. The generated password is stable for
the release name and intended only for disposable preview databases.
*/}}
{{- define "preview.postgresName" -}}
{{ printf "%s-postgres" .Release.Name }}
{{- end -}}

{{- define "preview.postgresSecretName" -}}
{{ printf "%s-postgres" .Release.Name }}
{{- end -}}

{{- define "preview.postgresPassword" -}}
{{ printf "%s-postgres" .Release.Name | sha256sum | trunc 32 }}
{{- end -}}

{{- define "preview.databaseURL" -}}
{{ printf "postgres://%s:%s@%s:%v/%s?sslmode=disable" .Values.postgres.username (include "preview.postgresPassword" .) (include "preview.postgresName" .) .Values.postgres.port .Values.postgres.database }}
{{- end -}}
