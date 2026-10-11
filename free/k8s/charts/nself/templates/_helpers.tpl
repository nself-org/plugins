{{/*
Generic helpers over .Values.services (written by `nself k8s values`).
Services are never typed here: image, tag, env names, ports, mounts and probes
all come from the generated values. Secret VALUES (env values and postgres
init scripts) come from secrets.yaml (.Values.secrets, .Values.initdb), never
from values.yaml. The licence key is never a chart value: .Values.license names
a Secret that already exists in the cluster.
*/}}

{{/*
nself.host: an ingress host after the install --domain is applied. Input: dict
root, host. When .Values.domain and .Values.baseDomain (the project's
BASE_DOMAIN, generated) are both set and differ, the host equal to baseDomain
becomes domain, and a host ending in ".<baseDomain>" keeps its prefix (lower
cased) with the new domain. Hosts are compared case-insensitively. Any other
host is unchanged. internal/k8s/hosts.go (RewriteHost)
does the same for the hosts the CLI prints.
*/}}
{{- define "nself.host" -}}
{{- $domain := default "" .root.Values.domain -}}
{{- $base := lower (default "" .root.Values.baseDomain) -}}
{{- $host := lower .host -}}
{{- if and $domain $base (ne (lower $domain) $base) (eq $host $base) -}}
{{ $domain }}
{{- else if and $domain $base (ne (lower $domain) $base) (hasSuffix (printf ".%s" $base) $host) -}}
{{ trimSuffix $base $host }}{{ $domain }}
{{- else -}}
{{ .host }}
{{- end -}}
{{- end -}}

{{/*
nself.licenseEnv: "true" when the pod of service `name` gets the licence key
from the Secret named by .Values.license.secretName: every plugin workload
(named plugin-*, or marked `plugin` in the generated values). `nself k8s values`
never maps NSELF_PLUGIN_LICENSE_KEY into a service env, so there is no clash.
Input: dict root, name, svc.
*/}}
{{- define "nself.licenseEnv" -}}
{{- $lic := default dict .root.Values.license -}}
{{- if and $lic.secretName (or (hasPrefix "plugin-" .name) (default "" .svc.plugin)) -}}
true
{{- end -}}
{{- end -}}

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
    {{- $licenseEnv := include "nself.licenseEnv" (dict "root" $root "name" $name "svc" $svc) }}
    {{- if or $svc.env $licenseEnv }}
    env:
      {{- range $svc.env }}
      - name: {{ . }}
        valueFrom:
          secretKeyRef:
            name: {{ $root.Release.Name }}-{{ $name }}
            key: {{ . }}
      {{- end }}
      {{- if $licenseEnv }}
      {{- $lic := $root.Values.license }}
      - name: NSELF_PLUGIN_LICENSE_KEY
        valueFrom:
          secretKeyRef:
            name: {{ $lic.secretName }}
            key: {{ default "NSELF_PLUGIN_LICENSE_KEY" $lic.secretKey }}
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
    {{- if or $svc.mounts $svc.initdb }}
    volumeMounts:
      {{- range $svc.mounts }}
      - name: {{ (index $root.Values.volumes .volume).claim }}
        mountPath: {{ .path }}
        {{- if .readOnly }}
        readOnly: true
        {{- end }}
      {{- end }}
      {{- if $svc.initdb }}
      - name: nself-initdb
        mountPath: /docker-entrypoint-initdb.d
        readOnly: true
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
{{- if or $svc.mounts $svc.initdb }}
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
  {{- if $svc.initdb }}
  - name: nself-initdb
    secret:
      secretName: {{ $root.Release.Name }}-{{ $name }}-initdb
  {{- end }}
{{- end }}
{{- end -}}
