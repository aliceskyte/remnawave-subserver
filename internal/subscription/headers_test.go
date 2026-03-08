package subscription

import (
	"encoding/json"
	"testing"
)

func TestBuildHeadersFromRaw(t *testing.T) {
	raw := map[string]any{
		"headers": map[string]any{
			"profile-title":         "MyProfile",
			"subscription-userinfo": "upload=0; download=100; total=1000",
			"Content-Type":          "text/plain", // should be skipped
		},
	}
	headers := BuildHeadersFromRaw(raw)
	if headers["profile-title"] != "MyProfile" {
		t.Errorf("profile-title = %q, want MyProfile", headers["profile-title"])
	}
	if headers["subscription-userinfo"] != "upload=0; download=100; total=1000" {
		t.Errorf("subscription-userinfo = %q", headers["subscription-userinfo"])
	}
	if _, ok := headers["Content-Type"]; ok {
		t.Error("Content-Type should be skipped")
	}
}

func TestBuildHeadersFromRawNil(t *testing.T) {
	headers := BuildHeadersFromRaw(nil)
	if len(headers) != 0 {
		t.Errorf("expected empty map, got %v", headers)
	}
}

func TestBuildHeadersFromRawUserTraffic(t *testing.T) {
	raw := map[string]any{
		"user": map[string]any{
			"trafficUsedBytes":  json.Number("500"),
			"trafficLimitBytes": json.Number("1000"),
			"expireAt":          "2025-12-31T23:59:59Z",
		},
	}
	headers := BuildHeadersFromRaw(raw)
	userinfo := headers["subscription-userinfo"]
	if userinfo == "" {
		t.Fatal("expected subscription-userinfo header")
	}
	// Should contain upload, download, total
	if !containsSubstring(userinfo, "download=500") {
		t.Errorf("userinfo missing download: %s", userinfo)
	}
	if !containsSubstring(userinfo, "total=1000") {
		t.Errorf("userinfo missing total: %s", userinfo)
	}
	if !containsSubstring(userinfo, "expire=") {
		t.Errorf("userinfo missing expire: %s", userinfo)
	}
}

func TestApplyHeaderOverrides(t *testing.T) {
	base := map[string]string{
		"profile-title":         "Original",
		"subscription-userinfo": "upload=0; download=100; total=1000",
	}
	overrides := map[string]HeaderOverride{
		"profile-title": {Mode: "custom", Value: "Custom Title"},
	}
	result := ApplyHeaderOverrides(base, overrides)
	if result["profile-title"] != "Custom Title" {
		t.Errorf("profile-title = %q, want Custom Title", result["profile-title"])
	}
}

func TestApplyHeaderOverridesRemove(t *testing.T) {
	base := map[string]string{
		"profile-title": "Test",
		"other":         "value",
	}
	overrides := map[string]HeaderOverride{
		"profile-title": {Mode: "remove"},
	}
	result := ApplyHeaderOverrides(base, overrides)
	if _, ok := result["profile-title"]; ok {
		t.Error("profile-title should be removed")
	}
	if result["other"] != "value" {
		t.Error("other header should remain")
	}
}

func TestApplyHeaderOverridesActual(t *testing.T) {
	base := map[string]string{
		"profile-title": "Original",
	}
	overrides := map[string]HeaderOverride{
		"profile-title": {Mode: "actual"},
	}
	result := ApplyHeaderOverrides(base, overrides)
	if result["profile-title"] != "Original" {
		t.Errorf("actual mode should keep original, got %q", result["profile-title"])
	}
}

func TestApplyHeaderOverridesEmpty(t *testing.T) {
	base := map[string]string{"key": "value"}
	result := ApplyHeaderOverrides(base, nil)
	if result["key"] != "value" {
		t.Error("nil overrides should return base unchanged")
	}
}

func TestApplyHeaderOverridesUserinfo(t *testing.T) {
	base := map[string]string{
		"subscription-userinfo": "upload=0; download=100; total=1000; expire=9999",
	}
	overrides := map[string]HeaderOverride{
		"subscription-userinfo": {
			Mode: "custom",
			Params: map[string]HeaderParamOverride{
				"download": {Mode: "custom", Value: "999"},
				"upload":   {Mode: "actual"},
				"total":    {Mode: "remove"},
			},
		},
	}
	result := ApplyHeaderOverrides(base, overrides)
	userinfo := result["subscription-userinfo"]
	if !containsSubstring(userinfo, "download=999") {
		t.Errorf("download should be overridden to 999: %s", userinfo)
	}
	if !containsSubstring(userinfo, "upload=0") {
		t.Errorf("upload should remain actual (0): %s", userinfo)
	}
	if containsSubstring(userinfo, "total=") {
		t.Errorf("total should be removed: %s", userinfo)
	}
}

func TestBuildHeadersFromRawSkipsUnsafeHeaders(t *testing.T) {
	raw := map[string]any{
		"headers": map[string]any{
			"Set-Cookie":    "session=abc",
			"profile-title": "Safe Title",
			"bad header":    "nope",
		},
	}
	headers := BuildHeadersFromRaw(raw)
	if _, ok := headers["Set-Cookie"]; ok {
		t.Fatal("unsafe header should be skipped")
	}
	if _, ok := headers["bad header"]; ok {
		t.Fatal("invalid header name should be skipped")
	}
	if headers["profile-title"] != "Safe Title" {
		t.Fatalf("profile-title = %q, want Safe Title", headers["profile-title"])
	}
}

func TestApplyHeaderOverridesRejectsInvalidValues(t *testing.T) {
	base := map[string]string{
		"profile-title": "Original",
	}
	overrides := map[string]HeaderOverride{
		"profile-title": {Mode: "custom", Value: "bad\r\nvalue"},
		"Set-Cookie":    {Mode: "custom", Value: "session=abc"},
	}
	result := ApplyHeaderOverrides(base, overrides)
	if _, ok := result["profile-title"]; ok {
		t.Fatal("invalid override should remove profile-title")
	}
	if _, ok := result["Set-Cookie"]; ok {
		t.Fatal("unsafe override header should be skipped")
	}
}

func TestParseExpire(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"2025-12-31T23:59:59Z", true},
		{"2025-12-31T23:59:59+00:00", true},
		{"2025-12-31", true},
		{"", false},
		{"not-a-date", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, ok := parseExpire(tt.input)
			if ok != tt.valid {
				t.Errorf("parseExpire(%q) ok = %v, want %v", tt.input, ok, tt.valid)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
