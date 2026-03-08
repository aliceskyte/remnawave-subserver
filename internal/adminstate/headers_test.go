package adminstate

import (
	"testing"

	"subserver/internal/subscription"
)

func TestNormalizeOverrides(t *testing.T) {
	input := map[string]subscription.HeaderOverride{
		"  key1  ": {Mode: " custom ", Value: " value1 "},
		"":         {Mode: "custom", Value: "should-be-skipped"},
		"key2":     {Mode: "actual", Value: "value2"},
	}
	result := normalizeOverrides(input)

	if _, ok := result["key1"]; !ok {
		t.Error("key1 should exist (trimmed)")
	}
	if result["key1"].Mode != "custom" {
		t.Errorf("key1 mode = %q, want 'custom'", result["key1"].Mode)
	}
	if result["key1"].Value != "value1" {
		t.Errorf("key1 value = %q, want 'value1'", result["key1"].Value)
	}
	if _, ok := result[""]; ok {
		t.Error("empty key should be skipped")
	}
	if _, ok := result["  key1  "]; ok {
		t.Error("untrimmed key should not exist")
	}
}

func TestNormalizeOverridesNil(t *testing.T) {
	result := normalizeOverrides(nil)
	if result == nil {
		t.Error("normalizeOverrides(nil) should return non-nil empty map")
	}
	if len(result) != 0 {
		t.Error("normalizeOverrides(nil) should return empty map")
	}
}

func TestSelectOverridesDefault(t *testing.T) {
	set := HeaderOverridesSet{
		Default: map[string]subscription.HeaderOverride{
			"key1": {Mode: "custom", Value: "default-value"},
		},
		Squads: map[string]map[string]subscription.HeaderOverride{},
	}
	result := selectOverrides(set, nil)
	if result["key1"].Value != "default-value" {
		t.Errorf("expected default-value, got %q", result["key1"].Value)
	}
}

func TestSelectOverridesSquadMerge(t *testing.T) {
	set := HeaderOverridesSet{
		Default: map[string]subscription.HeaderOverride{
			"key1": {Mode: "custom", Value: "default-value"},
			"key2": {Mode: "custom", Value: "default-only"},
		},
		Squads: map[string]map[string]subscription.HeaderOverride{
			"squad-1": {
				"key1": {Mode: "custom", Value: "squad-value"},
				"key3": {Mode: "custom", Value: "squad-only"},
			},
		},
	}
	result := selectOverrides(set, []string{"squad-1"})

	if result["key1"].Value != "squad-value" {
		t.Errorf("key1 should be overridden by squad, got %q", result["key1"].Value)
	}
	if result["key2"].Value != "default-only" {
		t.Errorf("key2 should remain from default, got %q", result["key2"].Value)
	}
	if result["key3"].Value != "squad-only" {
		t.Errorf("key3 should come from squad, got %q", result["key3"].Value)
	}
}

func TestSelectOverridesUnknownSquad(t *testing.T) {
	set := HeaderOverridesSet{
		Default: map[string]subscription.HeaderOverride{
			"key1": {Mode: "custom", Value: "default-value"},
		},
		Squads: map[string]map[string]subscription.HeaderOverride{
			"squad-1": {
				"key1": {Mode: "custom", Value: "squad-value"},
			},
		},
	}
	result := selectOverrides(set, []string{"unknown-squad"})
	if result["key1"].Value != "default-value" {
		t.Errorf("unknown squad should use default, got %q", result["key1"].Value)
	}
}

func TestNormalizeOverridesSetFunc(t *testing.T) {
	set := HeaderOverridesSet{
		Default: map[string]subscription.HeaderOverride{
			"key": {Mode: "custom", Value: "value"},
		},
		Squads: map[string]map[string]subscription.HeaderOverride{
			"  squad  ": {
				"key": {Mode: "custom", Value: "squad-value"},
			},
			"": {
				"key": {Mode: "custom", Value: "empty-squad"},
			},
			"default": {
				"key": {Mode: "custom", Value: "default-squad"},
			},
		},
	}
	result := normalizeOverridesSet(set)

	if _, ok := result.Squads["squad"]; !ok {
		t.Error("squad key should be trimmed and present")
	}
	if _, ok := result.Squads[""]; ok {
		t.Error("empty squad key should be removed")
	}
	if _, ok := result.Squads["default"]; ok {
		t.Error("'default' squad key should be removed")
	}
}

func TestNormalizeParams(t *testing.T) {
	params := map[string]subscription.HeaderParamOverride{
		"  upload  ": {Mode: " actual ", Value: "  100  "},
		"":           {Mode: "custom", Value: "skip"},
	}
	result := normalizeParams(params)

	if _, ok := result["upload"]; !ok {
		t.Error("upload should be present (trimmed)")
	}
	if result["upload"].Mode != "actual" {
		t.Errorf("upload mode = %q, want 'actual'", result["upload"].Mode)
	}
	if result["upload"].Value != "100" {
		t.Errorf("upload value = %q, want '100'", result["upload"].Value)
	}
	if _, ok := result[""]; ok {
		t.Error("empty key should be skipped")
	}
}

func TestCloneOverridesSetFunc(t *testing.T) {
	set := HeaderOverridesSet{
		Default: map[string]subscription.HeaderOverride{
			"key": {Mode: "custom", Value: "original"},
		},
		Squads: map[string]map[string]subscription.HeaderOverride{
			"squad-1": {
				"key": {Mode: "custom", Value: "squad-original"},
			},
		},
	}
	cloned := cloneOverridesSet(set)

	// Modify cloned
	cloned.Default["key"] = subscription.HeaderOverride{Mode: "custom", Value: "modified"}
	cloned.Squads["squad-1"]["key"] = subscription.HeaderOverride{Mode: "custom", Value: "modified"}

	// Original should be unchanged
	if set.Default["key"].Value != "original" {
		t.Error("original Default should be unchanged")
	}
	if set.Squads["squad-1"]["key"].Value != "squad-original" {
		t.Error("original Squads should be unchanged")
	}
}

func TestDecodeOverridesSetFlat(t *testing.T) {
	content := []byte(`{
		"profile-update-interval": {"mode": "custom", "value": "12"},
		"support-url": "https://example.com"
	}`)
	result, err := DecodeOverridesSet(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Xray.Default) != 2 {
		t.Errorf("expected 2 default overrides, got %d", len(result.Xray.Default))
	}
	if result.Xray.Default["profile-update-interval"].Mode != "custom" {
		t.Errorf("expected mode=custom, got %q", result.Xray.Default["profile-update-interval"].Mode)
	}
}

func TestDecodeOverridesSetWithSquads(t *testing.T) {
	content := []byte(`{
		"default": {"foo": {"mode": "custom", "value": "bar"}},
		"squads": {
			"squad-1": {"baz": {"mode": "actual"}}
		}
	}`)
	result, err := DecodeOverridesSet(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Xray.Default) != 1 {
		t.Errorf("expected 1 default override, got %d", len(result.Xray.Default))
	}
	if len(result.Xray.Squads) != 1 {
		t.Errorf("expected 1 squad, got %d", len(result.Xray.Squads))
	}
}

func TestDecodeOverridesSetInvalidJSON(t *testing.T) {
	_, err := DecodeOverridesSet([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
