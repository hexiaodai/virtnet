{{/*
generate the CA cert
*/}}
{{- define "generate-ca-certs" }}
    {{- $ca := genCA "virtnest.io" (.Values.tls.caExpiration | int) -}}
    {{- $_ := set . "ca" $ca -}}
{{- end }}
