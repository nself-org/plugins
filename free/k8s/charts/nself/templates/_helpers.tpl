{{/*
Generic helpers over .Values.services (written by `nself k8s values`).
Services are never typed here: image, tag, env names, ports, mounts and probes
all come from the generated values. Secret VALUES come from secrets.yaml
(.Values.secrets), never from values.yaml.
*/}}

{{/* nself.image: repository[:tag][@digest] of one service. */}}
{{- define "nself.image" -}}
{{ .image }}{{ if .tag }}:{{ .tag }}{{ end }}{{ if .digest }}@{{ .digest }}{{ end }}
{{- end -}}

{{/* nself.selector: labels that pick the pods of one service. */}}
{{- define "nself.selector" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
{{- end -}}

{{/* nself.labels: selector labels plus the managed-by label. */}}
{{- define "nself.labels" -}}
{{ include "nself.selector" . }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
{{- end -}}

{{/* nself.claim: PVC name of a named compose volume. */}}
{{- define "nself.claim" -}}
{{ .root.Release.Name }}-{{ (index .root.Values.volumes .volume).claim }}
{{- end -}}

{{/*
nself.pod: containers and volumes of one service. Input: dict root, name,
svc, restart (Always or Never).
*/}}
{{- define "nself.pod" -}}
{{- $root := .root -}}
{{- $name := .name -}}
{{- $svc := .svc -}}
restartPolicy: {{ .restart }}
containers:
  - name: {{ $name }}
    image: {{ include "nself.image" $svc | quote }}
    {{- with $svc.entrypoint }}
    command: {{ toJson . }}
    {{- end }}
    {{- with $svc.command }}
    args: {{ toJson . }}
    {{- end }}
    {{- if $svc.env }}
    env:
      {{- range $svc.env }}
      - name: {{ . }}
        valueFrom:
          secretKeyRef:
            name: {{ $root.Release.Name }}-{{ $name }}
            key: {{ . }}
      {{- end }}
    {{- end }}
    {{- if $svc.ports }}
    ports:
      {{- range $svc.ports }}
      - containerPort: {{ .port }}
        protocol: {{ .protocol }}
      {{- end }}
    {{- end }}
    {{- with $svc.resources }}
    resources:
      {{- toYaml . | nindent 6 }}
    {{- end }}
    {{- if $svc.mounts }}
    volumeMounts:
      {{- range $svc.mounts }}
      - name: {{ (index $root.Values.volumes .volume).claim }}
        mountPath: {{ .path }}
        {{- if .readOnly }}
        readOnly: true
        {{- end }}
      {{- end }}
    {{- end }}
    {{- with $svc.probe }}
    readinessProbe:
      exec:
        command: {{ toJson .exec }}
      periodSeconds: {{ .periodSeconds }}
      timeoutSeconds: {{ .timeoutSeconds }}
      failureThreshold: {{ .failureThreshold }}
      initialDelaySeconds: {{ .initialDelaySeconds }}
    {{- end }}
{{- if $svc.mounts }}
{{- $seen := dict }}
volumes:
  {{- range $svc.mounts }}
  {{- $claim := (index $root.Values.volumes .volume).claim }}
  {{- if not (hasKey $seen $claim) }}
  {{- $_ := set $seen $claim true }}
  - name: {{ $claim }}
    persistentVolumeClaim:
      claimName: {{ include "nself.claim" (dict "root" $root "volume" .volume) }}
  {{- end }}
  {{- end }}
{{- end }}
{{- end -}}
