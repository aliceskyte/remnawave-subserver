package config

import (
	"encoding/json"
	"testing"
)

func TestParseTemplateSetNil(t *testing.T) {
	ts, err := ParseTemplateSet(nil)
	if err != nil {
		t.Fatalf("ParseTemplateSet(nil) error = %v", err)
	}
	if ts.Default == nil {
		t.Error("Default should not be nil")
	}
}

func TestParseTemplateSetArray(t *testing.T) {
	raw := []any{
		map[string]any{"remarks": "test1", "outbounds": []any{}},
	}
	ts, err := ParseTemplateSet(raw)
	if err != nil {
		t.Fatalf("ParseTemplateSet() error = %v", err)
	}
	items, ok := ts.Default.([]any)
	if !ok {
		t.Fatal("Default should be []any")
	}
	if len(items) != 1 {
		t.Errorf("len(Default) = %d, want 1", len(items))
	}
}

func TestParseTemplateSetWithSquads(t *testing.T) {
	raw := map[string]any{
		"default": []any{map[string]any{"remarks": "default"}},
		"squads": map[string]any{
			"squad-1": []any{map[string]any{"remarks": "squad1"}},
		},
	}
	ts, err := ParseTemplateSet(raw)
	if err != nil {
		t.Fatalf("ParseTemplateSet() error = %v", err)
	}
	if ts.Squads["squad-1"] == nil {
		t.Error("squad-1 template should exist")
	}
}

func TestParseTemplateSetConfigObject(t *testing.T) {
	raw := map[string]any{
		"outbounds": []any{},
		"remarks":   "single-config",
	}
	ts, err := ParseTemplateSet(raw)
	if err != nil {
		t.Fatalf("ParseTemplateSet() error = %v", err)
	}
	cfg, ok := ts.Default.(map[string]any)
	if !ok {
		t.Fatal("Default should be map for single config object")
	}
	if cfg["remarks"] != "single-config" {
		t.Errorf("remarks = %v, want single-config", cfg["remarks"])
	}
}

func TestSelectTemplateDefault(t *testing.T) {
	ts := TemplateSet{
		Default: "default-template",
		Squads:  map[string]any{"squad-1": "squad-template"},
	}
	result := ts.SelectTemplate(nil)
	if result != "default-template" {
		t.Errorf("SelectTemplate(nil) = %v, want default-template", result)
	}
}

func TestSelectTemplateWithSquad(t *testing.T) {
	ts := TemplateSet{
		Default: "default-template",
		Squads:  map[string]any{"squad-1": "squad-template"},
	}
	result := ts.SelectTemplate([]string{"squad-1"})
	if result != "squad-template" {
		t.Errorf("SelectTemplate([squad-1]) = %v, want squad-template", result)
	}
}

func TestSelectTemplateUnknownSquad(t *testing.T) {
	ts := TemplateSet{
		Default: "default-template",
		Squads:  map[string]any{"squad-1": "squad-template"},
	}
	result := ts.SelectTemplate([]string{"unknown-squad"})
	if result != "default-template" {
		t.Errorf("SelectTemplate([unknown-squad]) = %v, want default-template", result)
	}
}

func TestClone(t *testing.T) {
	ts := TemplateSet{
		Default: map[string]any{"key": "value"},
		Squads: map[string]any{
			"s1": map[string]any{"k": "v"},
		},
	}
	cloned, err := ts.Clone()
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	// Modify cloned
	clonedDefault := cloned.Default.(map[string]any)
	clonedDefault["key"] = "modified"

	// Original should be unchanged
	originalDefault := ts.Default.(map[string]any)
	if originalDefault["key"] != "value" {
		t.Error("Clone() did not deep clone Default")
	}
}

func TestRaw(t *testing.T) {
	ts := TemplateSet{
		Default: []any{"a"},
		Squads:  map[string]any{"s1": "v1"},
	}
	raw := ts.Raw()
	if _, ok := raw["default"]; !ok {
		t.Error("Raw() missing 'default' key")
	}
	if _, ok := raw["squads"]; !ok {
		t.Error("Raw() missing 'squads' key")
	}
}

func TestRawNilSquads(t *testing.T) {
	ts := TemplateSet{Default: nil, Squads: nil}
	raw := ts.Raw()
	squads, ok := raw["squads"].(map[string]any)
	if !ok {
		t.Error("Raw() squads should be a map when nil")
	}
	if len(squads) != 0 {
		t.Error("Raw() squads should be empty when nil")
	}
}

func TestParseTemplateSetPreservesNumbers(t *testing.T) {
	raw := map[string]any{
		"default": []any{
			map[string]any{
				"remarks": "test",
				"port":    json.Number("443"),
			},
		},
	}
	ts, err := ParseTemplateSet(raw)
	if err != nil {
		t.Fatalf("ParseTemplateSet() error = %v", err)
	}
	items := ts.Default.([]any)
	cfg := items[0].(map[string]any)
	if _, ok := cfg["port"].(json.Number); !ok {
		t.Error("json.Number should be preserved")
	}
}
