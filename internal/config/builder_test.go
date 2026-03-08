package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"subserver/internal/panel"
)

func mustParseTemplate(t *testing.T, template any) any {
	t.Helper()
	raw, err := json.Marshal(template)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var parsed any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return parsed
}

func TestBuildVLESSUUIDInjection(t *testing.T) {
	template := []any{
		map[string]any{
			"remarks": "test-{user}",
			"outbounds": []any{
				map[string]any{
					"protocol": "vless",
					"settings": map[string]any{
						"vnext": []any{
							map[string]any{
								"users": []any{
									map[string]any{
										"id": "placeholder",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	library := TemplateLibrary{}.WithCore(CoreXray, TemplateSet{Default: mustParseTemplate(t, template), Squads: map[string]any{}})
	builder := NewBuilder(library)

	result, err := builder.Build(nil, panel.UserInfo{VlessUUID: "test-uuid-123", Username: "alice"}, CoreXray)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	configs, ok := result.Content.([]any)
	if !ok {
		t.Fatal("result is not a slice")
	}
	cfg := configs[0].(map[string]any)

	// Check remarks templating
	if cfg["remarks"] != "test-alice" {
		t.Errorf("remarks = %v, want test-alice", cfg["remarks"])
	}

	// Check VLESS UUID injection
	outbounds := cfg["outbounds"].([]any)
	outbound := outbounds[0].(map[string]any)
	settings := outbound["settings"].(map[string]any)
	vnext := settings["vnext"].([]any)
	server := vnext[0].(map[string]any)
	users := server["users"].([]any)
	user := users[0].(map[string]any)
	if user["id"] != "test-uuid-123" {
		t.Errorf("user id = %v, want test-uuid-123", user["id"])
	}
}

func TestBuildRemarksUsername(t *testing.T) {
	template := map[string]any{
		"remarks": "",
	}

	library := TemplateLibrary{}.WithCore(CoreXray, TemplateSet{Default: mustParseTemplate(t, template), Squads: map[string]any{}})
	builder := NewBuilder(library)

	result, err := builder.Build(nil, panel.UserInfo{Username: "bob"}, CoreXray)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cfg := result.Content.(map[string]any)
	if cfg["remarks"] != "bob" {
		t.Errorf("remarks = %v, want bob", cfg["remarks"])
	}
}

func TestBuildRemarksUsernameTemplate(t *testing.T) {
	template := map[string]any{
		"remarks": "Server-{username}",
	}

	library := TemplateLibrary{}.WithCore(CoreXray, TemplateSet{Default: mustParseTemplate(t, template), Squads: map[string]any{}})
	builder := NewBuilder(library)

	result, err := builder.Build(nil, panel.UserInfo{Username: "charlie"}, CoreXray)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cfg := result.Content.(map[string]any)
	if cfg["remarks"] != "Server-charlie" {
		t.Errorf("remarks = %v, want Server-charlie", cfg["remarks"])
	}
}

func TestBuildSkipOutboundTags(t *testing.T) {
	template := map[string]any{
		"remarks": "test",
		"subserver": map[string]any{
			"skipOutboundTags": []any{"keep-original"},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "replace-me",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"users": []any{
								map[string]any{"id": "old-1"},
							},
						},
					},
				},
			},
			map[string]any{
				"tag":      "keep-original",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"users": []any{
								map[string]any{"id": "old-2"},
							},
						},
					},
				},
			},
		},
	}

	library := TemplateLibrary{}.WithCore(CoreXray, TemplateSet{Default: mustParseTemplate(t, template), Squads: map[string]any{}})
	builder := NewBuilder(library)

	result, err := builder.Build(nil, panel.UserInfo{VlessUUID: "new-uuid", Username: "alice"}, CoreXray)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cfg := result.Content.(map[string]any)
	if _, exists := cfg["subserver"]; exists {
		t.Fatal("subserver should be removed from built config")
	}

	outbounds := cfg["outbounds"].([]any)
	firstUsers := outbounds[0].(map[string]any)["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)["users"].([]any)
	firstUser := firstUsers[0].(map[string]any)
	if firstUser["id"] != "new-uuid" {
		t.Errorf("first user id = %v, want new-uuid", firstUser["id"])
	}

	secondUsers := outbounds[1].(map[string]any)["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)["users"].([]any)
	secondUser := secondUsers[0].(map[string]any)
	if secondUser["id"] != "old-2" {
		t.Errorf("second user id = %v, want old-2", secondUser["id"])
	}
}

func TestBuildInvalidSubserverIgnored(t *testing.T) {
	template := map[string]any{
		"remarks":   "test",
		"subserver": "invalid",
		"outbounds": []any{
			map[string]any{
				"tag":      "replace-me",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"users": []any{
								map[string]any{"id": "old-1"},
							},
						},
					},
				},
			},
		},
	}

	library := TemplateLibrary{}.WithCore(CoreXray, TemplateSet{Default: mustParseTemplate(t, template), Squads: map[string]any{}})
	builder := NewBuilder(library)

	result, err := builder.Build(nil, panel.UserInfo{VlessUUID: "new-uuid", Username: "alice"}, CoreXray)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cfg := result.Content.(map[string]any)
	if _, exists := cfg["subserver"]; exists {
		t.Fatal("invalid subserver should still be removed from built config")
	}

	outbounds := cfg["outbounds"].([]any)
	users := outbounds[0].(map[string]any)["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)["users"].([]any)
	user := users[0].(map[string]any)
	if user["id"] != "new-uuid" {
		t.Errorf("user id = %v, want new-uuid", user["id"])
	}
}

func TestBuildNilBuilder(t *testing.T) {
	var builder *Builder
	ts := builder.TemplateSet()
	if ts.Xray.Default == nil {
		t.Error("nil builder TemplateSet().Xray.Default should not be nil")
	}
}

func TestBuildMihomoPlaceholders(t *testing.T) {
	template := []any{
		map[string]any{
			"name":    "Base",
			"content": "proxy-groups:\n  - name: Proxy\n    type: select\n    proxies: [DIRECT]\nrules:\n  - MATCH,Proxy\n",
		},
		map[string]any{
			"name":    "Node 1",
			"content": "type: vless\nserver: server.example\nport: 443\nuuid: {vless_uuid}\nname: \"{username}\"\nnetwork: tcp\ntls: true\nservername: example.com\n",
		},
	}

	library := TemplateLibrary{}.WithCore(CoreMihomo, TemplateSet{Default: mustParseTemplate(t, template), Squads: map[string]any{}})
	builder := NewBuilder(library)

	result, err := builder.Build(nil, panel.UserInfo{VlessUUID: "mihomo-uuid", Username: "alice"}, CoreMihomo)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	content, ok := result.Content.(string)
	if !ok {
		t.Fatal("expected mihomo build result to be a string")
	}
	if !strings.Contains(content, "uuid: mihomo-uuid") {
		t.Fatalf("expected uuid placeholder to be replaced, got:\n%s", content)
	}
	if !strings.Contains(content, "name: \"alice\"") {
		t.Fatalf("expected username placeholder to be replaced, got:\n%s", content)
	}
	if !strings.Contains(content, "\n---\n") {
		t.Fatalf("expected multi-document separator in mihomo output, got:\n%s", content)
	}
}

func TestBuildMihomoMergesStandaloneProfilesForImport(t *testing.T) {
	template := []any{
		map[string]any{
			"name": "Sweden",
			"content": "allow-lan: false\n" +
				"dns:\n" +
				"  nameserver:\n" +
				"    - 1.1.1.1\n" +
				"proxies:\n" +
				"  - name: Sweden\n" +
				"    type: vless\n" +
				"    server: se.example\n" +
				"    port: 443\n" +
				"    uuid: {vless_uuid}\n" +
				"    network: tcp\n" +
				"rules:\n" +
				"  - IP-CIDR,192.168.0.0/16,DIRECT,no-resolve\n" +
				"  - MATCH,Sweden\n",
		},
		map[string]any{
			"name": "Finland",
			"content": "allow-lan: false\n" +
				"dns:\n" +
				"  nameserver:\n" +
				"    - 8.8.8.8\n" +
				"proxies:\n" +
				"  - name: Finland\n" +
				"    type: vless\n" +
				"    server: fi.example\n" +
				"    port: 443\n" +
				"    uuid: {vless_uuid}\n" +
				"    network: ws\n" +
				"rules:\n" +
				"  - MATCH,Finland\n",
		},
	}

	library := TemplateLibrary{}.WithCore(CoreMihomo, TemplateSet{Default: mustParseTemplate(t, template), Squads: map[string]any{}})
	builder := NewBuilder(library)

	result, err := builder.Build(nil, panel.UserInfo{VlessUUID: "mihomo-uuid", Username: "alice"}, CoreMihomo)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	content, ok := result.Content.(string)
	if !ok {
		t.Fatal("expected mihomo build result to be a string")
	}
	if strings.Contains(content, "\n---\n") {
		t.Fatalf("expected merged single-document mihomo config, got:\n%s", content)
	}
	if !strings.Contains(content, "server: se.example") || !strings.Contains(content, "server: fi.example") {
		t.Fatalf("expected all proxies to be merged into one config, got:\n%s", content)
	}
	if !strings.Contains(content, "proxy-groups:") || !strings.Contains(content, "MATCH,PROXY") {
		t.Fatalf("expected merged config to include a selectable proxy group, got:\n%s", content)
	}
	if !strings.Contains(content, "1.1.1.1") || !strings.Contains(content, "8.8.8.8") {
		t.Fatalf("expected merged config to preserve DNS servers from profiles, got:\n%s", content)
	}
	if !strings.Contains(content, "uuid: mihomo-uuid") {
		t.Fatalf("expected uuid placeholder to be replaced across merged proxies, got:\n%s", content)
	}
}

func TestConvertXrayTemplateSetToMihomoCreatesSingleProxyProfiles(t *testing.T) {
	template := []any{
		map[string]any{
			"remarks": "Finland",
			"outbounds": []any{
				map[string]any{
					"tag":      "proxy",
					"protocol": "vless",
					"settings": map[string]any{
						"vnext": []any{
							map[string]any{
								"address": "fi.example",
								"port":    443,
								"users": []any{
									map[string]any{"id": "placeholder", "encryption": "none"},
								},
							},
						},
					},
					"streamSettings": map[string]any{
						"network":  "ws",
						"security": "tls",
						"tlsSettings": map[string]any{
							"serverName":    "fi.example",
							"fingerprint":   "firefox",
							"allowInsecure": false,
						},
						"wsSettings": map[string]any{
							"path": "/ws",
						},
					},
				},
				map[string]any{
					"tag":      "proxy_ru",
					"protocol": "vless",
					"settings": map[string]any{
						"vnext": []any{
							map[string]any{
								"address": "ru.example",
								"port":    8443,
								"users": []any{
									map[string]any{"id": "placeholder", "flow": "xtls-rprx-vision"},
								},
							},
						},
					},
					"streamSettings": map[string]any{
						"network":  "tcp",
						"security": "reality",
						"realitySettings": map[string]any{
							"serverName": "ads.example",
							"publicKey":  "public-key",
							"shortId":    "short-id",
						},
					},
				},
			},
			"routing": map[string]any{
				"rules": []any{
					map[string]any{
						"domain":      []any{"geosite:youtube", "geosite:discord"},
						"outboundTag": "proxy_ru",
					},
				},
			},
		},
	}

	converted, err := ConvertXrayTemplateSetToMihomo(TemplateSet{
		Default: mustParseTemplate(t, template),
		Squads:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("ConvertXrayTemplateSetToMihomo() error = %v", err)
	}

	entries, err := ParseMihomoEntries(converted.Default)
	if err != nil {
		t.Fatalf("ParseMihomoEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 mihomo profile, got %d", len(entries))
	}
	if entries[0].Name != "Finland" {
		t.Fatalf("profile name = %q, want Finland", entries[0].Name)
	}
	if strings.Contains(entries[0].Content, "\n---\n") {
		t.Fatalf("single mihomo profile should not be split into fragments:\n%s", entries[0].Content)
	}
	if !strings.Contains(entries[0].Content, "proxies:") {
		t.Fatalf("expected full mihomo config with proxies section, got:\n%s", entries[0].Content)
	}
	if strings.Contains(entries[0].Content, "proxy-groups:") {
		t.Fatalf("expected stripped mihomo config without proxy-groups section, got:\n%s", entries[0].Content)
	}
	if strings.Contains(entries[0].Content, "server: fi.example") {
		t.Fatalf("expected primary proxy to prefer proxy_ru over proxy, got:\n%s", entries[0].Content)
	}
	if !strings.Contains(entries[0].Content, "server: ru.example") {
		t.Fatalf("expected selected proxy to point to proxy_ru / Veesp-style outbound, got:\n%s", entries[0].Content)
	}
	if !strings.Contains(entries[0].Content, "MATCH,Finland") {
		t.Fatalf("expected final rule to point directly to the single proxy, got:\n%s", entries[0].Content)
	}
}
