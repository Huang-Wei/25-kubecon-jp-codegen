resources:{{ if not .ResourceTypes }} []{{ end }}
{{- range .ResourceTypes }}
- {{ . }}.yaml
{{- end }}

namePrefix: "{{ .CloudProvider }}-{{ .AccountID }}-{{ .RegionCode }}"
