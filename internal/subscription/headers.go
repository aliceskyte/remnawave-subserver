package subscription

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"subserver/internal/jsonutil"
)

type HeaderOverride struct {
	Mode   string                         `json:"mode"`
	Value  string                         `json:"value,omitempty"`
	Params map[string]HeaderParamOverride `json:"params,omitempty"`
}

type HeaderParamOverride struct {
	Mode  string `json:"mode"`
	Value string `json:"value,omitempty"`
}

func BuildHeadersFromRaw(raw map[string]any) map[string]string {
	prepared := map[string]string{}
	if raw == nil {
		return prepared
	}

	headers, ok := raw["headers"].(map[string]any)
	if !ok {
		headers = map[string]any{}
	}

	subscriptionUserinfoValue := ""
	for key, value := range headers {
		cleanKey, ok := sanitizeHeaderName(key)
		if !ok || isUnsafeResponseHeader(cleanKey) {
			continue
		}
		if value == nil {
			continue
		}
		cleanValue, ok := sanitizeHeaderValue(fmt.Sprint(value))
		if !ok {
			continue
		}
		if strings.EqualFold(cleanKey, "subscription-userinfo") {
			subscriptionUserinfoValue = cleanValue
			continue
		}
		prepared[cleanKey] = cleanValue
	}

	if subscriptionUserinfoValue == "" {
		if value, ok := buildSubscriptionUserinfo(raw); ok {
			subscriptionUserinfoValue = value
		}
	}

	if cleanValue, ok := sanitizeHeaderValue(subscriptionUserinfoValue); ok && cleanValue != "" {
		prepared["subscription-userinfo"] = cleanValue
	}

	return prepared
}

func ApplyHeaderOverrides(base map[string]string, overrides map[string]HeaderOverride) map[string]string {
	if len(overrides) == 0 {
		return base
	}
	result := map[string]string{}
	for key, value := range base {
		result[key] = value
	}
	for key, override := range overrides {
		cleanKey, ok := sanitizeHeaderName(key)
		if !ok || isUnsafeResponseHeader(cleanKey) {
			continue
		}
		mode := normalizeOverrideMode(override.Mode)
		if mode == "remove" {
			deleteHeader(result, cleanKey)
			continue
		}

		if len(override.Params) > 0 && strings.EqualFold(cleanKey, "subscription-userinfo") {
			actualValue, _ := findHeaderValue(result, cleanKey)
			if actualValue == "" {
				actualValue = strings.TrimSpace(override.Value)
			}
			actualParams := parseUserinfoValue(actualValue)
			finalParams := applyUserinfoOverrides(actualParams, override.Params)
			value := buildUserinfoValue(finalParams)
			value, ok = sanitizeHeaderValue(value)
			if !ok || strings.TrimSpace(value) == "" {
				deleteHeader(result, cleanKey)
				continue
			}
			deleteHeader(result, cleanKey)
			result[cleanKey] = value
			continue
		}

		if mode == "actual" {
			continue
		}

		value, ok := sanitizeHeaderValue(override.Value)
		if !ok || value == "" {
			deleteHeader(result, cleanKey)
			continue
		}
		deleteHeader(result, cleanKey)
		result[cleanKey] = value
	}
	return result
}

func sanitizeHeaderName(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	for _, r := range value {
		if r > 127 || !isHeaderTokenRune(byte(r)) {
			return "", false
		}
	}
	return value, true
}

func isHeaderTokenRune(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	}
	switch b {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}

func sanitizeHeaderValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	for _, r := range value {
		switch {
		case r == '\r' || r == '\n' || r == 0:
			return "", false
		case r < 32 && r != '\t':
			return "", false
		case r == 127:
			return "", false
		}
	}
	return value, true
}

func isUnsafeResponseHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "access-control-allow-credentials",
		"access-control-allow-headers",
		"access-control-allow-methods",
		"access-control-allow-origin",
		"access-control-expose-headers",
		"access-control-max-age",
		"alt-svc",
		"authorization",
		"cache-control",
		"connection",
		"content-length",
		"content-security-policy",
		"content-type",
		"cookie",
		"date",
		"expires",
		"keep-alive",
		"location",
		"pragma",
		"proxy-authenticate",
		"proxy-authorization",
		"referrer-policy",
		"server",
		"set-cookie",
		"strict-transport-security",
		"te",
		"trailer",
		"transfer-encoding",
		"upgrade",
		"x-content-type-options",
		"x-frame-options":
		return true
	default:
		return false
	}
}

func deleteHeader(headers map[string]string, key string) {
	for existing := range headers {
		if strings.EqualFold(existing, key) {
			delete(headers, existing)
			return
		}
	}
}

func normalizeOverrideMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "actual", "custom", "remove":
		return mode
	default:
		return "custom"
	}
}

func findHeaderValue(headers map[string]string, key string) (string, bool) {
	for existing, value := range headers {
		if strings.EqualFold(existing, key) {
			return value, true
		}
	}
	return "", false
}

func parseUserinfoValue(value string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(value) == "" {
		return result
	}
	parts := strings.Split(value, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		if key == "" {
			continue
		}
		result[key] = strings.TrimSpace(pair[1])
	}
	return result
}

func buildUserinfoValue(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	orderedKeys := []string{"upload", "download", "total", "expire"}
	parts := []string{}
	used := map[string]struct{}{}
	for _, key := range orderedKeys {
		value, ok := values[key]
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		used[key] = struct{}{}
	}

	rest := make([]string, 0, len(values))
	for key := range values {
		if _, ok := used[key]; ok {
			continue
		}
		rest = append(rest, key)
	}
	sort.Strings(rest)
	for _, key := range rest {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}

	return strings.Join(parts, "; ")
}

func applyUserinfoOverrides(actual map[string]string, overrides map[string]HeaderParamOverride) map[string]string {
	result := map[string]string{}
	for key, value := range actual {
		result[strings.ToLower(key)] = value
	}
	for key, override := range overrides {
		cleanKey := strings.ToLower(strings.TrimSpace(key))
		if cleanKey == "" {
			continue
		}
		mode := normalizeOverrideMode(override.Mode)
		switch mode {
		case "actual":
			continue
		case "remove":
			delete(result, cleanKey)
			continue
		default:
			value := strings.TrimSpace(override.Value)
			if value == "" {
				delete(result, cleanKey)
				continue
			}
			result[cleanKey] = value
		}
	}
	return result
}

func buildSubscriptionUserinfo(raw map[string]any) (string, bool) {
	if headers, ok := raw["headers"].(map[string]any); ok {
		for _, key := range []string{"subscription-userinfo", "Subscription-Userinfo", "Subscription-UserInfo"} {
			if value, ok := headers[key]; ok {
				valueStr := strings.TrimSpace(fmt.Sprint(value))
				if valueStr != "" {
					return valueStr, true
				}
			}
		}
	}

	user, ok := raw["user"].(map[string]any)
	if !ok {
		return "", false
	}

	traffic := map[string]any{}
	if trafficValue, ok := user["userTraffic"].(map[string]any); ok {
		traffic = trafficValue
	}

	used, usedOk := jsonutil.SafeInt(user["trafficUsedBytes"])
	if !usedOk {
		if fallback, ok := jsonutil.SafeInt(traffic["usedTrafficBytes"]); ok {
			used = fallback
			usedOk = true
		}
	}
	total, totalOk := jsonutil.SafeInt(user["trafficLimitBytes"])
	if !usedOk || !totalOk {
		return "", false
	}

	parts := []string{
		"upload=0",
		fmt.Sprintf("download=%d", used),
		fmt.Sprintf("total=%d", total),
	}

	expireValue := user["expireAt"]
	if !jsonutil.Truthy(expireValue) {
		expireValue = user["expiresAt"]
	}

	if expireValue != nil {
		if valueStr, ok := expireValue.(string); ok {
			if ts, ok := parseExpire(valueStr); ok {
				parts = append(parts, fmt.Sprintf("expire=%d", ts))
			}
		} else if ts, ok := jsonutil.SafeInt(expireValue); ok {
			parts = append(parts, fmt.Sprintf("expire=%d", ts))
		}
	}

	return strings.Join(parts, "; "), true
}

func parseExpire(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if strings.HasSuffix(value, "Z") {
		value = strings.TrimSuffix(value, "Z") + "+00:00"
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Unix(), true
		}
	}

	return 0, false
}
