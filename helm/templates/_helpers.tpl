{{- define "shopware-operator.goMemLimit" -}}
{{- $quantity := toString . -}}
{{- $units := dict "Ki" 1024 "Mi" 1048576 "Gi" 1073741824 "Ti" 1099511627776 "k" 1000 "K" 1000 "M" 1000000 "G" 1000000000 "T" 1000000000000 -}}
{{- $number := regexFind "^[0-9]+(\\.[0-9]+)?" $quantity -}}
{{- $suffix := trimPrefix $number $quantity -}}
{{- if not $number -}}
{{- fail (printf "unable to parse resources.limits.memory %q for GOMEMLIMIT" $quantity) -}}
{{- end -}}
{{- $multiplier := 1 -}}
{{- if $suffix -}}
{{- if not (hasKey $units $suffix) -}}
{{- fail (printf "unsupported unit %q in resources.limits.memory for GOMEMLIMIT" $suffix) -}}
{{- end -}}
{{- $multiplier = get $units $suffix -}}
{{- end -}}
{{- int64 (floor (mulf (float64 $number) $multiplier 0.8)) -}}
{{- end -}}
