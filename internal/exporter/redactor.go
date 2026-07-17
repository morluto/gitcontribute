package exporter

import (
	"reflect"
	"regexp"
	"strings"
	"time"
)

var (
	keyValuePattern   = regexp.MustCompile(`(?i)["']?[a-z_]*(?:token|secret|password|api[-_]?key|auth[-_]?token)[a-z_]*["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:Bearer|Basic|token)\s+[^\s,;}\]]+|[^\s,;}\]]+)`)
	authHeaderPattern = regexp.MustCompile(`(?i)(Authorization\s*:\s*(?:Bearer|token|Token|Basic)\s+)(\S+)`)
	legacyGitHubPat   = regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36}`)
	fineGrainedPat    = regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`)
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
	if s == "" {
		return ""
	}
	s = keyValuePattern.ReplaceAllStringFunc(s, redactKeyValueMatch)
	s = authHeaderPattern.ReplaceAllString(s, "${1}[REDACTED]")
	s = fineGrainedPat.ReplaceAllString(s, "[REDACTED]")
	s = legacyGitHubPat.ReplaceAllString(s, "[REDACTED]")
	return s
}

func redactKeyValueMatch(m string) string {
	for i, r := range m {
		if r == ':' || r == '=' {
			return strings.TrimRight(m[:i+1], " \t") + " [REDACTED]"
		}
	}
	return "[REDACTED]"
}
