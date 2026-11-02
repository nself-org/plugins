package attest

// Purpose: scrub every string of a model value structurally, so a string field added later to
// the runner or attempt records is published scrubbed without anyone listing it.
// Inputs:  a value of a model type (structs, slices, pointers, maps of strings) and a function
// from (field name, text) to the published text.
// Outputs: a deep copy of the value with every string replaced by the function's result; the
// input is never modified. Unexported fields are not copied (the model types have none).

import "reflect"

// scrubCopy returns a deep copy of in with every string passed through f.
func scrubCopy[T any](in T, f func(field, s string) string) T {
	out, _ := copyScrub(reflect.ValueOf(&in).Elem(), "", f).Interface().(T)
	return out
}

func copyScrub(v reflect.Value, name string, f func(field, s string) string) reflect.Value {
	t := v.Type()
	nv := reflect.New(t).Elem()
	switch v.Kind() {
	case reflect.String:
		nv.SetString(f(name, v.String()))
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).IsExported() {
				nv.Field(i).Set(copyScrub(v.Field(i), t.Field(i).Name, f))
			}
		}
	case reflect.Slice:
		if v.IsNil() {
			return nv
		}
		nv.Set(reflect.MakeSlice(t, v.Len(), v.Len()))
		for i := 0; i < v.Len(); i++ {
			nv.Index(i).Set(copyScrub(v.Index(i), name, f))
		}
	case reflect.Ptr:
		if v.IsNil() {
			return nv
		}
		nv.Set(reflect.New(t.Elem()))
		nv.Elem().Set(copyScrub(v.Elem(), name, f))
	case reflect.Map:
		if v.IsNil() {
			return nv
		}
		nv.Set(reflect.MakeMapWithSize(t, v.Len()))
		for _, k := range v.MapKeys() {
			nv.SetMapIndex(copyScrub(k, name, f), copyScrub(v.MapIndex(k), name, f))
		}
	default:
		nv.Set(v)
	}
	return nv
}
