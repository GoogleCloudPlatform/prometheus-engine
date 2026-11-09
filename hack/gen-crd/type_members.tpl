{{- define "type_members" -}}
{{- $field := . -}}
{{- if eq $field.Type.Name "ObjectMeta" -}}
Refer to Kubernetes API documentation for fields of `metadata`.
{{- else -}}
{{ markdownRenderFieldDoc $field.Doc }}
{{- end -}}
{{- end -}}
