package util

import (
	"reflect"
	"slices"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

var secretRefType = reflect.TypeFor[v1.SecretRef]()

func GetSecretRefs(store v1.Store) []v1.SecretRef {
	var refs []v1.SecretRef
	collectSecretRefs(reflect.ValueOf(store.Spec), &refs)
	return refs
}

func GetReferencedSecretNames(store v1.Store) []string {
	var names []string
	for _, ref := range GetSecretRefs(store) {
		if ref.Name != "" && !slices.Contains(names, ref.Name) {
			names = append(names, ref.Name)
		}
	}
	return names
}

func collectSecretRefs(value reflect.Value, refs *[]v1.SecretRef) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			collectSecretRefs(value.Elem(), refs)
		}
	case reflect.Struct:
		if value.Type() == secretRefType {
			*refs = append(*refs, value.Interface().(v1.SecretRef))
			return
		}
		for i := range value.NumField() {
			if value.Type().Field(i).IsExported() {
				collectSecretRefs(value.Field(i), refs)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			collectSecretRefs(value.Index(i), refs)
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			collectSecretRefs(iter.Value(), refs)
		}
	}
}
