// Package jsonutil provides shared JSON helper functions used across multiple packages.
package jsonutil

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// Truthy returns whether a value is considered truthy in a JSON context.
// nil, false, empty string, and zero numbers are falsy; everything else is truthy.
func Truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed != 0
		}
		if parsed, err := v.Float64(); err == nil {
			return parsed != 0
		}
		return true
	case float64:
		return v != 0
	case float32:
		return v != 0
	case int:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	case uint8:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	default:
		return true
	}
}

// SafeInt attempts to convert a value to int64. Returns the value and true on success.
func SafeInt(value any) (int64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed, true
		}
		if parsed, err := v.Float64(); err == nil {
			return int64(parsed), true
		}
		return 0, false
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// CloneJSON deep-clones an arbitrary JSON value via marshal/unmarshal,
// preserving json.Number types.
func CloneJSON(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// CloneMap deep-clones a map[string]any via JSON marshal/unmarshal,
// preserving json.Number types.
func CloneMap(value map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// LooksLikeConfigObject checks if a map looks like a raw xray/v2ray config object
// by checking for well-known top-level keys (superset from all call sites).
func LooksLikeConfigObject(value map[string]any) bool {
	if value == nil {
		return false
	}
	knownKeys := []string{
		"outbounds",
		"inbounds",
		"routing",
		"dns",
		"log",
		"policy",
		"stats",
		"api",
		"transport",
		"remarks",
		"meta",
	}
	for _, key := range knownKeys {
		if _, ok := value[key]; ok {
			return true
		}
	}
	return false
}
