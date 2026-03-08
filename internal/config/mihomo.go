package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
	"subserver/internal/panel"
)

const (
	defaultMihomoMixedPort = 7890
	defaultMihomoSocksPort = 10808
	defaultMihomoHTTPPort  = 10809
	defaultMihomoDNSPort   = 1053
)

type MihomoEntry struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func ParseMihomoEntries(template any) ([]MihomoEntry, error) {
	if template == nil {
		return []MihomoEntry{}, nil
	}

	items, ok := template.([]any)
	if !ok {
		return nil, errors.New("mihomo_template_invalid")
	}

	entries := make([]MihomoEntry, 0, len(items))
	for idx, item := range items {
		entryMap, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("mihomo_entry_invalid")
		}
		name := strings.TrimSpace(fmt.Sprint(entryMap["name"]))
		content := strings.TrimSpace(fmt.Sprint(entryMap["content"]))
		if name == "" {
			name = fmt.Sprintf("entry-%d", idx+1)
		}
		if content == "" {
			return nil, errors.New("mihomo_entry_content_required")
		}
		entries = append(entries, MihomoEntry{Name: name, Content: content})
	}
	return entries, nil
}

func mihomoEntriesToTemplate(entries []MihomoEntry) (any, error) {
	result := make([]any, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return nil, errors.New("config_name_required")
		}
		content := strings.TrimSpace(entry.Content)
		if content == "" {
			return nil, errors.New("config_body_required")
		}
		result = append(result, map[string]any{
			"name":    name,
			"content": content,
		})
	}
	return result, nil
}

func BuildMihomo(template any, user panel.UserInfo) (string, error) {
	entries, err := ParseMihomoEntries(template)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", errors.New("mihomo_template_empty")
	}

	docs := make([]string, 0, len(entries))
	for _, entry := range entries {
		content := strings.TrimSpace(applyMihomoTextPlaceholders(entry.Content, user))
		if content == "" {
			return "", fmt.Errorf("%s: empty yaml", entry.Name)
		}
		if _, err := parseYAMLDocuments(content); err != nil {
			return "", fmt.Errorf("%s: %w", entry.Name, err)
		}
		docs = append(docs, content)
	}

	merged, ok, err := mergeMihomoStandaloneProfiles(docs)
	if err != nil {
		return "", err
	}
	if ok {
		return merged, nil
	}

	return strings.Join(docs, "\n---\n") + "\n", nil
}

func parseYAMLDocuments(content string) ([]any, error) {
	decoder := yaml.NewDecoder(strings.NewReader(content))
	decoder.KnownFields(false)
	docs := []any{}
	for {
		var doc any
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		docs = append(docs, normalizeYAMLValue(doc))
	}
	if len(docs) == 0 {
		return nil, errors.New("empty yaml")
	}
	return docs, nil
}

func mergeMihomoStandaloneProfiles(contents []string) (string, bool, error) {
	if len(contents) <= 1 {
		return "", false, nil
	}

	profiles := make([]map[string]any, 0, len(contents))
	proxies := make([]any, 0, len(contents))
	groupMembers := make([]string, 0, len(contents))
	preservedRules := make([]string, 0, len(contents))
	seenRules := map[string]struct{}{}
	nameCounts := map[string]int{}

	for _, content := range contents {
		docs, err := parseYAMLDocuments(content)
		if err != nil {
			return "", false, err
		}
		if len(docs) != 1 {
			return "", false, nil
		}

		profile, ok := docs[0].(map[string]any)
		if !ok || profile == nil {
			return "", false, nil
		}

		rawProxies, ok := profile["proxies"].([]any)
		if !ok || len(rawProxies) == 0 {
			return "", false, nil
		}

		for _, item := range rawProxies {
			proxy, ok := item.(map[string]any)
			if !ok || proxy == nil {
				continue
			}

			clonedProxy, err := cloneMapJSON(proxy)
			if err != nil {
				return "", false, err
			}

			name := stringValue(clonedProxy["name"])
			if name == "" {
				return "", false, nil
			}
			if seen := nameCounts[name]; seen > 0 {
				clonedProxy["name"] = fmt.Sprintf("%s #%d", name, seen+1)
			}
			nameCounts[name]++

			proxies = append(proxies, clonedProxy)
			groupMembers = append(groupMembers, stringValue(clonedProxy["name"]))
		}

		rawRules, _ := profile["rules"].([]any)
		for _, item := range rawRules {
			rule := strings.TrimSpace(stringValue(item))
			if !keepMergedMihomoRule(rule) {
				continue
			}
			if _, exists := seenRules[rule]; exists {
				continue
			}
			seenRules[rule] = struct{}{}
			preservedRules = append(preservedRules, rule)
		}

		profiles = append(profiles, profile)
	}

	if len(proxies) == 0 {
		return "", false, nil
	}

	base, err := cloneMapJSON(profiles[0])
	if err != nil {
		return "", false, err
	}

	mergeMihomoDNS(base, profiles)

	base["proxies"] = proxies
	base["proxy-groups"] = []any{
		map[string]any{
			"name":    "PROXY",
			"type":    "select",
			"proxies": uniqueStringsToAny(append(groupMembers, "DIRECT")),
		},
	}

	rules := make([]any, 0, len(preservedRules)+1)
	for _, rule := range preservedRules {
		rules = append(rules, rule)
	}
	rules = append(rules, "MATCH,PROXY")
	base["rules"] = rules

	content, err := yaml.Marshal(normalizeJSONNumbers(base))
	if err != nil {
		return "", false, err
	}

	return string(content), true, nil
}

func keepMergedMihomoRule(rule string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return false
	}
	return strings.HasSuffix(rule, ",DIRECT") ||
		strings.Contains(rule, ",DIRECT,") ||
		strings.HasSuffix(rule, ",REJECT") ||
		strings.Contains(rule, ",REJECT,")
}

func mergeMihomoDNS(base map[string]any, profiles []map[string]any) {
	dns, _ := base["dns"].(map[string]any)
	if dns == nil {
		return
	}

	nameservers := []string{}
	defaultNameservers := []string{}
	for _, profile := range profiles {
		profileDNS, _ := profile["dns"].(map[string]any)
		if profileDNS == nil {
			continue
		}
		nameservers = append(nameservers, stringsFromAnySlice(profileDNS["nameserver"])...)
		defaultNameservers = append(defaultNameservers, stringsFromAnySlice(profileDNS["default-nameserver"])...)
	}

	if len(nameservers) > 0 {
		dns["nameserver"] = uniqueStringsToAny(nameservers)
	}
	if len(defaultNameservers) > 0 {
		dns["default-nameserver"] = uniqueStringsToAny(defaultNameservers)
	}
	base["dns"] = dns
}

func stringsFromAnySlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		str := stringValue(item)
		if str == "" {
			continue
		}
		result = append(result, str)
	}
	return result
}

func normalizeJSONNumbers(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[key] = normalizeJSONNumbers(item)
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizeJSONNumbers(item))
		}
		return normalized
	case json.Number:
		if intValue, err := typed.Int64(); err == nil {
			return int(intValue)
		}
		if floatValue, err := typed.Float64(); err == nil {
			return floatValue
		}
		return typed.String()
	default:
		return typed
	}
}

func normalizeYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, val := range typed {
			normalized[key] = normalizeYAMLValue(val)
		}
		return normalized
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, val := range typed {
			normalized[fmt.Sprint(key)] = normalizeYAMLValue(val)
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizeYAMLValue(item))
		}
		return normalized
	default:
		return typed
	}
}

func applyMihomoTextPlaceholders(content string, user panel.UserInfo) string {
	replaced := strings.ReplaceAll(content, "{user}", user.Username)
	replaced = strings.ReplaceAll(replaced, "{username}", user.Username)
	replaced = strings.ReplaceAll(replaced, "{vless_uuid}", user.VlessUUID)
	return replaced
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func ConvertXrayTemplateSetToMihomo(source TemplateSet) (TemplateSet, error) {
	converted := normalizeTemplateSet(TemplateSet{})

	defaultEntries, err := convertXrayTemplateToMihomoEntries(source.Default)
	if err != nil {
		return TemplateSet{}, err
	}
	defaultTemplate, err := mihomoEntriesToTemplate(defaultEntries)
	if err != nil {
		return TemplateSet{}, err
	}
	converted.Default = defaultTemplate

	for key, tmpl := range source.Squads {
		entries, err := convertXrayTemplateToMihomoEntries(tmpl)
		if err != nil {
			return TemplateSet{}, err
		}
		template, err := mihomoEntriesToTemplate(entries)
		if err != nil {
			return TemplateSet{}, err
		}
		converted.Squads[key] = template
	}

	return converted, nil
}

func convertXrayTemplateToMihomoEntries(template any) ([]MihomoEntry, error) {
	entries, err := xrayTemplateToConfigEntries(template)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return []MihomoEntry{}, nil
	}

	result := make([]MihomoEntry, 0, len(entries))
	for _, entry := range entries {
		converted, err := convertXrayConfigToMihomoEntry(entry)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

type xrayConfigEntry struct {
	Name   string
	Config map[string]any
}

func xrayTemplateToConfigEntries(template any) ([]xrayConfigEntry, error) {
	switch value := template.(type) {
	case nil:
		return []xrayConfigEntry{}, nil
	case []any:
		entries := make([]xrayConfigEntry, 0, len(value))
		for idx, item := range value {
			cfg, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("config_entry_invalid")
			}
			cloned, err := cloneMapJSON(cfg)
			if err != nil {
				return nil, err
			}
			name := stringValue(cloned["remarks"])
			if name == "" {
				name = fmt.Sprintf("config-%d", idx+1)
			}
			entries = append(entries, xrayConfigEntry{Name: name, Config: cloned})
		}
		return entries, nil
	case map[string]any:
		cloned, err := cloneMapJSON(value)
		if err != nil {
			return nil, err
		}
		name := stringValue(cloned["remarks"])
		if name == "" {
			name = "config-1"
		}
		return []xrayConfigEntry{{Name: name, Config: cloned}}, nil
	default:
		return nil, errors.New("config_template_invalid")
	}
}

func cloneMapJSON(value map[string]any) (map[string]any, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func convertXrayConfigToMihomoEntry(entry xrayConfigEntry) (MihomoEntry, error) {
	profile, omittedBittorrent, err := buildMihomoProfile(entry.Name, entry.Config)
	if err != nil {
		return MihomoEntry{}, err
	}

	content, err := yaml.Marshal(profile)
	if err != nil {
		return MihomoEntry{}, err
	}

	body := string(content)
	if omittedBittorrent {
		body = "# Xray rule with protocol=bittorrent was omitted because Mihomo has no equivalent protocol matcher.\n" + body
	}

	return MihomoEntry{
		Name:    entry.Name,
		Content: strings.TrimSpace(body),
	}, nil
}

func buildMihomoProfile(profileName string, cfg map[string]any) (map[string]any, bool, error) {
	proxies, targets, err := buildMihomoProxies(profileName, cfg)
	if err != nil {
		return nil, false, err
	}
	if len(proxies) == 0 {
		return nil, false, errors.New("mihomo_proxies_required")
	}

	profile := map[string]any{
		"allow-lan": false,
		"mode":      "rule",
		"log-level": "info",
		"dns":       buildMihomoDNS(cfg),
		"proxies":   proxies,
	}

	applyMihomoPorts(profile, cfg)

	primaryProxy := targets["proxy"]
	if primaryProxy == "" {
		if firstProxy, ok := proxies[0].(map[string]any); ok {
			primaryProxy = stringValue(firstProxy["name"])
		}
	}
	if primaryProxy == "" {
		primaryProxy = "DIRECT"
	}

	rawRules, _ := cfg["routing"].(map[string]any)["rules"].([]any)
	rules, omittedBittorrent := buildMihomoRules(rawRules, targets, primaryProxy)
	profile["rules"] = rules

	return profile, omittedBittorrent, nil
}

func buildMihomoProxies(profileName string, cfg map[string]any) ([]any, map[string]string, error) {
	rawOutbounds, _ := cfg["outbounds"].([]any)
	targets := map[string]string{
		"direct": "DIRECT",
		"block":  "REJECT",
	}
	vlessOutbounds := make(map[string]map[string]any, len(rawOutbounds))
	orderedTags := make([]string, 0, len(rawOutbounds))

	for _, item := range rawOutbounds {
		outbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag := stringValue(outbound["tag"])
		if stringValue(outbound["protocol"]) != "vless" {
			continue
		}
		vlessOutbounds[tag] = outbound
		orderedTags = append(orderedTags, tag)
	}

	selectedTag := selectPreferredMihomoOutboundTag(orderedTags)
	selectedOutbound, ok := vlessOutbounds[selectedTag]
	if !ok {
		return nil, nil, errors.New("mihomo_proxies_required")
	}

	proxyName := strings.TrimSpace(profileName)
	if proxyName == "" {
		proxyName = deriveMihomoProxyName(profileName, selectedTag, nil)
	}
	proxy, err := convertXrayOutboundToMihomoProxy(proxyName, selectedOutbound)
	if err != nil {
		return nil, nil, err
	}
	if proxy == nil {
		return nil, nil, errors.New("mihomo_proxies_required")
	}

	targets[selectedTag] = stringValue(proxy["name"])
	return []any{proxy}, targets, nil
}

func selectPreferredMihomoOutboundTag(tags []string) string {
	for _, preferred := range []string{"proxy_ru", "proxy"} {
		if containsString(tags, preferred) {
			return preferred
		}
	}
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

func deriveMihomoProxyName(profileName, tag string, hints []string) string {
	profileName = strings.TrimSpace(profileName)
	tag = strings.TrimSpace(tag)
	switch tag {
	case "", "proxy":
		return profileName
	case "proxy_ru":
		if hintsContainAny(hints, "geosite:youtube", "geosite:discord", "geosite:soundcloud") {
			return profileName + " · Media"
		}
	case "proxy_de":
		if hintsContainAny(hints, "geosite:tiktok") {
			return profileName + " · TikTok"
		}
	}

	label := prettifyMihomoTag(tag)
	if profileName == "" {
		return label
	}
	return profileName + " · " + label
}

func deriveMihomoGroupName(tag string, hints []string) string {
	tag = strings.TrimSpace(tag)
	switch tag {
	case "", "proxy":
		return "PROXY"
	case "proxy_ru":
		if hintsContainAny(hints, "geosite:youtube", "geosite:discord", "geosite:soundcloud") {
			return "MEDIA"
		}
	case "proxy_de":
		if hintsContainAny(hints, "geosite:tiktok") {
			return "TIKTOK"
		}
	}
	return strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))
}

func prettifyMihomoTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "Proxy"
	}
	tag = strings.ReplaceAll(tag, "-", " ")
	tag = strings.ReplaceAll(tag, "_", " ")
	parts := strings.Fields(tag)
	for idx, part := range parts {
		if len(part) <= 3 {
			parts[idx] = strings.ToUpper(part)
			continue
		}
		parts[idx] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	if len(parts) == 0 {
		return "Proxy"
	}
	return strings.Join(parts, " ")
}

func buildMihomoRules(rawRules []any, aliases map[string]string, defaultTarget string) ([]any, bool) {
	rules := make([]any, 0, len(rawRules)+1)
	seen := map[string]struct{}{}
	omittedBittorrent := false

	addRule := func(rule string) {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			return
		}
		if _, exists := seen[rule]; exists {
			return
		}
		seen[rule] = struct{}{}
		rules = append(rules, rule)
	}

	for _, item := range rawRules {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}

		target := resolveMihomoRuleTarget(stringValue(rule["outboundTag"]), aliases)
		if target == "" {
			continue
		}

		if rawDomains, ok := rule["domain"].([]any); ok {
			for _, domainValue := range rawDomains {
				domain := stringValue(domainValue)
				if domain == "" {
					continue
				}
				switch {
				case strings.HasPrefix(domain, "geosite:"):
					addRule(fmt.Sprintf("GEOSITE,%s,%s", strings.TrimPrefix(domain, "geosite:"), target))
				case strings.HasPrefix(domain, "domain:"):
					addRule(fmt.Sprintf("DOMAIN,%s,%s", strings.TrimPrefix(domain, "domain:"), target))
				default:
					addRule(fmt.Sprintf("DOMAIN,%s,%s", domain, target))
				}
			}
		}

		if rawIPs, ok := rule["ip"].([]any); ok {
			for _, ipValue := range rawIPs {
				ip := stringValue(ipValue)
				if ip == "" {
					continue
				}
				addRule(fmt.Sprintf("IP-CIDR,%s,%s,no-resolve", ip, target))
			}
		}

		if rawProtocols, ok := rule["protocol"].([]any); ok {
			for _, protocolValue := range rawProtocols {
				if strings.EqualFold(stringValue(protocolValue), "bittorrent") {
					omittedBittorrent = true
				}
			}
		}
	}

	if strings.TrimSpace(defaultTarget) == "" {
		defaultTarget = "DIRECT"
	}
	addRule(fmt.Sprintf("MATCH,%s", defaultTarget))
	return rules, omittedBittorrent
}

func resolveMihomoRuleTarget(tag string, aliases map[string]string) string {
	switch tag {
	case "":
		return ""
	case "direct":
		return "DIRECT"
	case "block":
		return "REJECT"
	default:
		if alias, ok := aliases[tag]; ok {
			return alias
		}
		return tag
	}
}

func buildMihomoDNS(cfg map[string]any) map[string]any {
	servers := []string{}
	rawDNS, _ := cfg["dns"].(map[string]any)
	rawServers, _ := rawDNS["servers"].([]any)
	for _, item := range rawServers {
		server := stringValue(item)
		if server == "" || containsString(servers, server) {
			continue
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		servers = []string{"1.1.1.1", "1.0.0.1"}
	}

	nameservers := make([]any, 0, len(servers))
	for _, server := range servers {
		nameservers = append(nameservers, server)
	}

	return map[string]any{
		"enable":             true,
		"listen":             fmt.Sprintf("127.0.0.1:%d", defaultMihomoDNSPort),
		"ipv6":               false,
		"enhanced-mode":      "redir-host",
		"default-nameserver": nameservers,
		"nameserver":         nameservers,
	}
}

func applyMihomoPorts(profile map[string]any, cfg map[string]any) {
	socksPort := 0
	httpPort := 0

	rawInbounds, _ := cfg["inbounds"].([]any)
	for _, item := range rawInbounds {
		inbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		port := toInt(inbound["port"])
		if port <= 0 {
			continue
		}
		switch stringValue(inbound["protocol"]) {
		case "socks":
			socksPort = port
		case "http":
			httpPort = port
		}
	}

	if socksPort > 0 {
		profile["socks-port"] = socksPort
	}
	if httpPort > 0 {
		profile["port"] = httpPort
	}
	if socksPort == 0 && httpPort == 0 {
		profile["mixed-port"] = defaultMihomoMixedPort
		return
	}
	if socksPort == 0 {
		profile["socks-port"] = defaultMihomoSocksPort
	}
	if httpPort == 0 {
		profile["port"] = defaultMihomoHTTPPort
	}
}

func convertXrayOutboundToMihomoProxy(name string, outbound map[string]any) (map[string]any, error) {
	settings, _ := outbound["settings"].(map[string]any)
	vnext, _ := settings["vnext"].([]any)
	if len(vnext) == 0 {
		return nil, nil
	}
	serverEntry, ok := vnext[0].(map[string]any)
	if !ok {
		return nil, nil
	}
	users, _ := serverEntry["users"].([]any)
	if len(users) == 0 {
		return nil, nil
	}
	userEntry, ok := users[0].(map[string]any)
	if !ok {
		return nil, nil
	}

	proxy := map[string]any{
		"name":   name,
		"type":   "vless",
		"server": stringValue(serverEntry["address"]),
		"port":   toInt(serverEntry["port"]),
		"udp":    true,
		"uuid":   "{vless_uuid}",
	}

	if flow := stringValue(userEntry["flow"]); flow != "" {
		proxy["flow"] = flow
	}
	if encryption := stringValue(userEntry["encryption"]); encryption != "" && encryption != "none" {
		proxy["encryption"] = encryption
	}

	streamSettings, _ := outbound["streamSettings"].(map[string]any)
	network := stringValue(streamSettings["network"])
	if network == "" {
		network = "tcp"
	}
	proxy["network"] = network

	security := stringValue(streamSettings["security"])
	switch security {
	case "tls":
		proxy["tls"] = true
		tlsSettings, _ := streamSettings["tlsSettings"].(map[string]any)
		if serverName := stringValue(tlsSettings["serverName"]); serverName != "" {
			proxy["servername"] = serverName
		}
		if fingerprint := stringValue(tlsSettings["fingerprint"]); fingerprint != "" {
			proxy["client-fingerprint"] = fingerprint
		}
		if allowInsecure, ok := tlsSettings["allowInsecure"].(bool); ok {
			proxy["skip-cert-verify"] = allowInsecure
		}
	case "reality":
		proxy["tls"] = true
		realitySettings, _ := streamSettings["realitySettings"].(map[string]any)
		if serverName := stringValue(realitySettings["serverName"]); serverName != "" {
			proxy["servername"] = serverName
		}
		fingerprint := stringValue(realitySettings["fingerprint"])
		if fingerprint == "" {
			fingerprint = "chrome"
		}
		proxy["client-fingerprint"] = fingerprint
		proxy["reality-opts"] = map[string]any{
			"public-key": stringValue(realitySettings["publicKey"]),
			"short-id":   stringValue(realitySettings["shortId"]),
		}
	}

	switch network {
	case "ws":
		wsSettings, _ := streamSettings["wsSettings"].(map[string]any)
		wsOpts := map[string]any{}
		if path, exists := wsSettings["path"]; exists {
			wsOpts["path"] = fmt.Sprint(path)
		}
		if headers, ok := wsSettings["headers"].(map[string]any); ok {
			cleanHeaders := map[string]any{}
			for key, value := range headers {
				headerValue := stringValue(value)
				if strings.TrimSpace(key) == "" || headerValue == "" {
					continue
				}
				cleanHeaders[key] = headerValue
			}
			if len(cleanHeaders) > 0 {
				wsOpts["headers"] = cleanHeaders
			}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	}

	return proxy, nil
}

func uniqueStringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func proxyNames(proxies []any) []string {
	names := make([]string, 0, len(proxies))
	for _, item := range proxies {
		proxy, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := stringValue(proxy["name"])
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func hintsContainAny(hints []string, candidates ...string) bool {
	for _, candidate := range candidates {
		if containsString(hints, candidate) {
			return true
		}
	}
	return false
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func toInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
		if parsed, err := typed.Float64(); err == nil {
			return int(parsed)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(typed, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return 0
}
