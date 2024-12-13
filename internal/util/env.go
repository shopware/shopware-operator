package util

import (
	corev1 "k8s.io/api/core/v1"
)

// Copy returns a new slice where src is overwriting dst.
// When a env in src is already present in dst,
// the value in dst will be overwritten by the value associated
// with the value in src.
func MergeEnv(dst, src []corev1.EnvVar) []corev1.EnvVar {
	toAppend := []corev1.EnvVar{}
	for si, srcEnv := range src {
		for di, dstEnv := range dst {
			// Overwrite if name exists in both slices
			if srcEnv.Name == dstEnv.Name {
				dst[di] = src[si]
				break
			}
		}
	}
	return append(dst, toAppend...)
}
