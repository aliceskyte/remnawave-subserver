package jsonutil_test

import (
	"encoding/json"
	"testing"

	"subserver/internal/jsonutil"
)

func TestTruthy(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		// nil
		{"nil", nil, false},

		// booleans
		{"false", false, false},
		{"true", true, true},

		// strings
		{"empty string", "", false},
		{"non-empty string", "hello", true},

		// integer types
		{"int 0", int(0), false},
		{"int 1", int(1), true},
		{"int -1", int(-1), true},
		{"int8 0", int8(0), false},
		{"int8 1", int8(1), true},
		{"int16 0", int16(0), false},
		{"int16 1", int16(1), true},
		{"int32 0", int32(0), false},
		{"int32 1", int32(1), true},
		{"int64 0", int64(0), false},
		{"int64 1", int64(1), true},

		// unsigned integer types
		{"uint 0", uint(0), false},
		{"uint 1", uint(1), true},
		{"uint8 0", uint8(0), false},
		{"uint8 1", uint8(1), true},
		{"uint16 0", uint16(0), false},
		{"uint16 1", uint16(1), true},
		{"uint32 0", uint32(0), false},
		{"uint32 1", uint32(1), true},
		{"uint64 0", uint64(0), false},
		{"uint64 1", uint64(1), true},

		// float types
		{"float64 0", float64(0), false},
		{"float64 3.14", float64(3.14), true},
		{"float64 -1.5", float64(-1.5), true},
		{"float32 0", float32(0), false},
		{"float32 1.5", float32(1.5), true},

		// json.Number
		{"json.Number 0", json.Number("0"), false},
		{"json.Number 42", json.Number("42"), true},
		{"json.Number -1", json.Number("-1"), true},
		{"json.Number 0.0", json.Number("0.0"), false},
		{"json.Number 3.14", json.Number("3.14"), true},

		// default case – anything else is truthy
		{"struct", struct{}{}, true},
		{"slice", []int{1, 2, 3}, true},
		{"empty slice", []int{}, true},
		{"map", map[string]int{"a": 1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonutil.Truthy(tt.value)
			if got != tt.want {
				t.Errorf("Truthy(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestSafeInt(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		wantN  int64
		wantOK bool
	}{
		// nil
		{"nil", nil, 0, false},

		// json.Number
		{"json.Number 42", json.Number("42"), 42, true},
		{"json.Number -10", json.Number("-10"), -10, true},
		{"json.Number 3.7 (float truncated)", json.Number("3.7"), 3, true},
		{"json.Number invalid", json.Number("abc"), 0, false},

		// float types
		{"float64 3.7", float64(3.7), 3, true},
		{"float64 0", float64(0), 0, true},
		{"float64 -2.9", float64(-2.9), -2, true},
		{"float32 1.5", float32(1.5), 1, true},

		// integer types
		{"int 5", int(5), 5, true},
		{"int -3", int(-3), -3, true},
		{"int8 127", int8(127), 127, true},
		{"int16 1000", int16(1000), 1000, true},
		{"int32 100000", int32(100000), 100000, true},
		{"int64 999", int64(999), 999, true},

		// unsigned integer types
		{"uint 10", uint(10), 10, true},
		{"uint8 255", uint8(255), 255, true},
		{"uint16 65535", uint16(65535), 65535, true},
		{"uint32 100", uint32(100), 100, true},
		{"uint64 200", uint64(200), 200, true},

		// string
		{"string 123", "123", 123, true},
		{"string -42", "-42", -42, true},
		{"string with spaces", "  56  ", 56, true},
		{"string abc", "abc", 0, false},
		{"string empty", "", 0, false},
		{"string float", "3.14", 0, false},

		// bool
		{"bool true", true, 1, true},
		{"bool false", false, 0, true},

		// unsupported types
		{"struct", struct{}{}, 0, false},
		{"slice", []int{1}, 0, false},
		{"map", map[string]int{"a": 1}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotN, gotOK := jsonutil.SafeInt(tt.value)
			if gotN != tt.wantN || gotOK != tt.wantOK {
				t.Errorf("SafeInt(%v) = (%d, %v), want (%d, %v)",
					tt.value, gotN, gotOK, tt.wantN, tt.wantOK)
			}
		})
	}
}

func TestCloneJSON_Map(t *testing.T) {
	original := map[string]any{
		"name":  "test",
		"count": json.Number("42"),
		"nested": map[string]any{
			"inner": "value",
		},
		"list": []any{"a", "b"},
	}

	cloned, err := jsonutil.CloneJSON(original)
	if err != nil {
		t.Fatalf("CloneJSON returned error: %v", err)
	}

	clonedMap, ok := cloned.(map[string]any)
	if !ok {
		t.Fatalf("CloneJSON result is %T, want map[string]any", cloned)
	}

	// Verify values match
	if clonedMap["name"] != "test" {
		t.Errorf("cloned name = %v, want %q", clonedMap["name"], "test")
	}
	if clonedMap["count"] != json.Number("42") {
		t.Errorf("cloned count = %v (%T), want json.Number(42)", clonedMap["count"], clonedMap["count"])
	}

	// Verify deep copy: mutating original doesn't affect clone
	original["name"] = "mutated"
	if clonedMap["name"] == "mutated" {
		t.Error("mutating original affected clone — not a deep copy")
	}

	nestedOrig := original["nested"].(map[string]any)
	nestedOrig["inner"] = "changed"
	nestedClone := clonedMap["nested"].(map[string]any)
	if nestedClone["inner"] == "changed" {
		t.Error("mutating nested original affected clone — not a deep copy")
	}
}

func TestCloneJSON_Slice(t *testing.T) {
	original := []any{"hello", json.Number("1"), true, nil}

	cloned, err := jsonutil.CloneJSON(original)
	if err != nil {
		t.Fatalf("CloneJSON returned error: %v", err)
	}

	clonedSlice, ok := cloned.([]any)
	if !ok {
		t.Fatalf("CloneJSON result is %T, want []any", cloned)
	}

	if len(clonedSlice) != 4 {
		t.Fatalf("cloned slice length = %d, want 4", len(clonedSlice))
	}
	if clonedSlice[0] != "hello" {
		t.Errorf("cloned[0] = %v, want %q", clonedSlice[0], "hello")
	}
	if clonedSlice[1] != json.Number("1") {
		t.Errorf("cloned[1] = %v (%T), want json.Number(1)", clonedSlice[1], clonedSlice[1])
	}
	if clonedSlice[2] != true {
		t.Errorf("cloned[2] = %v, want true", clonedSlice[2])
	}
	if clonedSlice[3] != nil {
		t.Errorf("cloned[3] = %v, want nil", clonedSlice[3])
	}

	// Verify independence
	original[0] = "modified"
	if clonedSlice[0] == "modified" {
		t.Error("mutating original slice affected clone")
	}
}

func TestCloneJSON_Primitives(t *testing.T) {
	// string
	cloned, err := jsonutil.CloneJSON("hello")
	if err != nil {
		t.Fatalf("CloneJSON(string) error: %v", err)
	}
	if cloned != "hello" {
		t.Errorf("CloneJSON(string) = %v, want %q", cloned, "hello")
	}

	// number preserves json.Number via UseNumber
	cloned, err = jsonutil.CloneJSON(42)
	if err != nil {
		t.Fatalf("CloneJSON(int) error: %v", err)
	}
	if cloned != json.Number("42") {
		t.Errorf("CloneJSON(42) = %v (%T), want json.Number(42)", cloned, cloned)
	}

	// boolean
	cloned, err = jsonutil.CloneJSON(true)
	if err != nil {
		t.Fatalf("CloneJSON(bool) error: %v", err)
	}
	if cloned != true {
		t.Errorf("CloneJSON(true) = %v, want true", cloned)
	}

	// nil
	cloned, err = jsonutil.CloneJSON(nil)
	if err != nil {
		t.Fatalf("CloneJSON(nil) error: %v", err)
	}
	if cloned != nil {
		t.Errorf("CloneJSON(nil) = %v, want nil", cloned)
	}
}

func TestCloneJSON_Error(t *testing.T) {
	// Channels are not JSON-serializable
	ch := make(chan int)
	_, err := jsonutil.CloneJSON(ch)
	if err == nil {
		t.Error("CloneJSON(chan) should return error, got nil")
	}
}

func TestCloneMap(t *testing.T) {
	original := map[string]any{
		"key1": "value1",
		"key2": json.Number("99"),
		"nested": map[string]any{
			"deep": "data",
		},
	}

	cloned, err := jsonutil.CloneMap(original)
	if err != nil {
		t.Fatalf("CloneMap returned error: %v", err)
	}

	// Verify values
	if cloned["key1"] != "value1" {
		t.Errorf("cloned key1 = %v, want %q", cloned["key1"], "value1")
	}
	if cloned["key2"] != json.Number("99") {
		t.Errorf("cloned key2 = %v (%T), want json.Number(99)", cloned["key2"], cloned["key2"])
	}

	// Verify independence: mutate original, check clone is unaffected
	original["key1"] = "mutated"
	if cloned["key1"] == "mutated" {
		t.Error("mutating original affected clone")
	}

	nestedOrig := original["nested"].(map[string]any)
	nestedOrig["deep"] = "changed"
	nestedClone := cloned["nested"].(map[string]any)
	if nestedClone["deep"] == "changed" {
		t.Error("mutating nested original affected clone")
	}
}

func TestCloneMap_Nil(t *testing.T) {
	cloned, err := jsonutil.CloneMap(nil)
	if err != nil {
		t.Fatalf("CloneMap(nil) returned error: %v", err)
	}
	// json.Marshal(nil) → "null", decoding "null" into map[string]any → nil
	if cloned != nil {
		t.Errorf("CloneMap(nil) = %v, want nil", cloned)
	}
}

func TestCloneMap_Empty(t *testing.T) {
	original := map[string]any{}

	cloned, err := jsonutil.CloneMap(original)
	if err != nil {
		t.Fatalf("CloneMap(empty) returned error: %v", err)
	}

	if cloned == nil {
		t.Fatal("CloneMap(empty) returned nil, want empty map")
	}
	if len(cloned) != 0 {
		t.Errorf("CloneMap(empty) has %d entries, want 0", len(cloned))
	}
}

func TestLooksLikeConfigObject(t *testing.T) {
	tests := []struct {
		name  string
		value map[string]any
		want  bool
	}{
		{"nil map", nil, false},
		{"empty map", map[string]any{}, false},
		{"random keys", map[string]any{"foo": 1, "bar": 2}, false},

		// Each known key should individually trigger true
		{"outbounds", map[string]any{"outbounds": []any{}}, true},
		{"inbounds", map[string]any{"inbounds": []any{}}, true},
		{"routing", map[string]any{"routing": map[string]any{}}, true},
		{"dns", map[string]any{"dns": map[string]any{}}, true},
		{"log", map[string]any{"log": map[string]any{}}, true},
		{"policy", map[string]any{"policy": map[string]any{}}, true},
		{"stats", map[string]any{"stats": map[string]any{}}, true},
		{"api", map[string]any{"api": map[string]any{}}, true},
		{"transport", map[string]any{"transport": map[string]any{}}, true},
		{"remarks", map[string]any{"remarks": "test"}, true},
		{"meta", map[string]any{"meta": map[string]any{}}, true},

		// Mixed: known key among unknown keys
		{"mixed with outbounds", map[string]any{"foo": 1, "outbounds": []any{}, "bar": 2}, true},

		// Multiple known keys
		{"multiple known keys", map[string]any{"outbounds": []any{}, "dns": map[string]any{}, "log": map[string]any{}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonutil.LooksLikeConfigObject(tt.value)
			if got != tt.want {
				t.Errorf("LooksLikeConfigObject(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
