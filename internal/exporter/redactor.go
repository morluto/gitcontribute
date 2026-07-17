package exporter

import (
	"reflect"
	"time"

	"github.com/morluto/gitcontribute/internal/redaction"
)

var timeType = reflect.TypeOf(time.Time{})

// redact returns a deep copy of v with common secret patterns replaced by
// "[REDACTED]". It preserves exported struct fields and collection order.
func redact(v any) any {
	rv := reflect.ValueOf(v)
	out := redactValue(rv)
	if !out.IsValid() {
		return nil
	}
	return out.Interface()
}

func redactValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}

	switch v.Kind() {
	case reflect.String:
		return reflect.ValueOf(redactString(v.String())).Convert(v.Type())
	case reflect.Ptr:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		elem := redactValue(v.Elem())
		if !elem.IsValid() {
			return reflect.Zero(v.Type())
		}
		ptr := reflect.New(elem.Type())
		ptr.Elem().Set(elem)
		return ptr
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		return redactValue(v.Elem())
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		n := v.Len()
		out := reflect.MakeSlice(v.Type(), n, n)
		for i := 0; i < n; i++ {
			out.Index(i).Set(redactValue(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(redactValue(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		for _, key := range v.MapKeys() {
			rk := redactValue(key)
			rv := redactValue(v.MapIndex(key))
			out.SetMapIndex(rk, rv)
		}
		return out
	case reflect.Struct:
		if v.Type() == timeType {
			return v
		}
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if !out.Field(i).CanSet() {
				continue
			}
			out.Field(i).Set(redactValue(v.Field(i)))
		}
		return out
	default:
		if v.CanInterface() {
			return reflect.ValueOf(v.Interface())
		}
		return v
	}
}

func redactString(s string) string {
	return redaction.String(s)
}
