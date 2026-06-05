// Package schema generates JSON-shaped schemas and default-value
// skeletons from Go types via reflection. It serves the explorer UI
// (endpoint request/response previews, limit-filter schemas) and
// expects the same json struct-tag conventions used by encoding/json.
package schema

import (
	"reflect"
	"strings"
)

// Reflect generates a schema map from a Go type using reflection.
// Non-struct types return a single-entry map keyed by "type" carrying
// the kind name. Struct types return a map keyed by json-tag field
// name (falling back to the Go field name), with each value populated
// by DefaultValue. Unexported fields, fields tagged "-", and fields
// tagged with "omitempty" are skipped. Pointers are dereferenced; nil
// input returns nil.
func Reflect(t any) map[string]any {
	if t == nil {
		return nil
	}

	rv := reflect.ValueOf(t)
	rt := rv.Type()

	// Handle pointer types
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
		rv = reflect.New(rt).Elem()
	}

	if rt.Kind() != reflect.Struct {
		return map[string]any{"type": rt.Kind().String()}
	}

	schema := make(map[string]any)
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		parts := strings.Split(jsonTag, ",")
		fieldName := field.Name
		if parts[0] != "" {
			fieldName = parts[0]
		}

		// Skip omitempty fields in request schema - they're optional
		hasOmitempty := false
		for _, opt := range parts[1:] {
			if opt == "omitempty" {
				hasOmitempty = true
				break
			}
		}
		if hasOmitempty {
			continue
		}

		// Generate default value based on type
		schema[fieldName] = DefaultValue(field.Type)
	}

	return schema
}

// DefaultValue returns a zero-shaped default for a reflect.Type
// suitable for previewing a JSON payload of that shape. Slices return
// a one-element array of the element's default so the UI shows the
// nested shape. Structs recurse with the same json-tag rules as
// Reflect. Pointers are dereferenced.
func DefaultValue(t reflect.Type) any {
	switch t.Kind() {
	case reflect.String:
		return ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 0
	case reflect.Float32, reflect.Float64:
		return 0.0
	case reflect.Bool:
		return false
	case reflect.Slice:
		elemDefault := DefaultValue(t.Elem())
		return []any{elemDefault}
	case reflect.Map:
		return map[string]any{}
	case reflect.Struct:
		// Recursively handle nested structs
		nested := make(map[string]any)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			jsonTag := field.Tag.Get("json")
			if jsonTag == "-" {
				continue
			}
			parts := strings.Split(jsonTag, ",")
			fieldName := field.Name
			if parts[0] != "" {
				fieldName = parts[0]
			}
			// Skip omitempty fields
			hasOmitempty := false
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					hasOmitempty = true
					break
				}
			}
			if hasOmitempty {
				continue
			}
			nested[fieldName] = DefaultValue(field.Type)
		}
		return nested
	case reflect.Ptr:
		return DefaultValue(t.Elem())
	default:
		return nil
	}
}
