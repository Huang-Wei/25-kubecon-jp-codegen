apiVersion: s3.aws.upbound.io/v1beta1
kind: Bucket
metadata:
  name: {{ .Name }}
spec:
  forProvider:
    region: {{ .Region }}
  providerConfigRef:
    name: default
