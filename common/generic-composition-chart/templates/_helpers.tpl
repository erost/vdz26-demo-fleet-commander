{{/*
Renders the Crossplane Function resource for a composition.

Required values:
  .Values.registry  - OCI registry prefix (e.g. ghcr.io/myorg)
  .Values.tag       - image tag

Optional values:
  .Values.pullSecret - name of an imagePullSecret for private registries
*/}}
{{- define "generic-composition-chart.function" -}}
apiVersion: pkg.crossplane.io/v1beta1
kind: Function
metadata:
  name: {{ .Chart.Name }}
spec:
  package: {{ .Values.registry }}/{{ .Chart.Name }}:{{ .Values.tag }}
  packagePullPolicy: IfNotPresent
  {{- if .Values.pullSecret }}
  packagePullSecrets:
    - name: {{ .Values.pullSecret }}
  {{- end }}
{{- end }}
