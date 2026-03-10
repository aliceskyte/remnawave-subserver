package config

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"math/big"
	"strings"
	"sync"

	"subserver/internal/jsonutil"
	"subserver/internal/panel"
)

type Builder struct {
	mu        sync.RWMutex
	templates TemplateLibrary
}

type BuildResult struct {
	Content     any
	ContentType string
}

func NewBuilder(templates TemplateLibrary) *Builder {
	return &Builder{
		templates: templates,
	}
}

type buildDirectives struct {
	skipOutboundTags map[string]struct{}
	randomizeGroups  map[string][]string
}

func (b *Builder) Build(_ context.Context, user panel.UserInfo, core Core) (BuildResult, error) {
	templateLibrary := b.getTemplateSet()
	template := templateLibrary.SelectTemplate(core, user.SquadUUIDs)
	if template == nil {
		return BuildResult{}, errors.New("no config template available")
	}

	if core == CoreMihomo {
		content, err := BuildMihomo(template, user)
		if err != nil {
			return BuildResult{}, err
		}
		return BuildResult{
			Content:     content,
			ContentType: "text/yaml; charset=utf-8",
		}, nil
	}

	cloned, err := jsonutil.CloneJSON(template)
	if err != nil {
		return BuildResult{}, err
	}

	updateConfigItem := func(cfg map[string]any) error {
		directives := parseBuildDirectives(cfg)
		if err := applyRandomizeGroups(cfg, directives.randomizeGroups); err != nil {
			return err
		}
		delete(cfg, "subserver")

		outbounds, ok := cfg["outbounds"].([]any)
		if ok {
			for _, outboundValue := range outbounds {
				outbound, ok := outboundValue.(map[string]any)
				if !ok {
					continue
				}
				if outbound["protocol"] != "vless" {
					continue
				}
				if directives.skipOutbound(outbound) {
					continue
				}
				settings, _ := outbound["settings"].(map[string]any)
				vnext, _ := settings["vnext"].([]any)
				for _, serverValue := range vnext {
					serverEntry, ok := serverValue.(map[string]any)
					if !ok {
						continue
					}
					users, _ := serverEntry["users"].([]any)
					for _, userValue := range users {
						userEntry, ok := userValue.(map[string]any)
						if !ok {
							continue
						}
						userEntry["id"] = user.VlessUUID
					}
				}
			}
		}

		if remarkValue, ok := cfg["remarks"].(string); ok {
			trimmed := strings.TrimSpace(remarkValue)
			if trimmed != "" {
				if strings.Contains(trimmed, "{user}") || strings.Contains(trimmed, "{username}") {
					replaced := strings.ReplaceAll(trimmed, "{user}", user.Username)
					replaced = strings.ReplaceAll(replaced, "{username}", user.Username)
					cfg["remarks"] = replaced
					return nil
				}
				cfg["remarks"] = trimmed
				return nil
			}
		}
		cfg["remarks"] = user.Username
		return nil
	}

	switch value := cloned.(type) {
	case []any:
		for _, entry := range value {
			cfg, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if err := updateConfigItem(cfg); err != nil {
				return BuildResult{}, err
			}
		}
		return BuildResult{
			Content:     value,
			ContentType: "application/json; charset=utf-8",
		}, nil
	case map[string]any:
		if err := updateConfigItem(value); err != nil {
			return BuildResult{}, err
		}
		return BuildResult{
			Content:     value,
			ContentType: "application/json; charset=utf-8",
		}, nil
	default:
		return BuildResult{
			Content:     cloned,
			ContentType: "application/json; charset=utf-8",
		}, nil
	}
}

func parseBuildDirectives(cfg map[string]any) buildDirectives {
	raw, ok := cfg["subserver"].(map[string]any)
	if !ok {
		return buildDirectives{}
	}
	return buildDirectives{
		skipOutboundTags: parseStringSet(raw["skipOutboundTags"]),
		randomizeGroups:  parseRandomizeGroups(raw["randomize"], cfg["outbounds"]),
	}
}

func parseStringSet(raw any) map[string]struct{} {
	items := parseStringList(raw)
	if len(items) == 0 {
		return nil
	}
	values := make(map[string]struct{}, len(items))
	for _, item := range items {
		values[item] = struct{}{}
	}
	return values
}

func parseStringList(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	values := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func parseRandomizeGroups(raw any, rawOutbounds any) map[string][]string {
	groupsRaw, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	availableTags := collectOutboundTags(rawOutbounds)
	if len(availableTags) == 0 {
		return nil
	}

	sanitized := make(map[string][]string, len(groupsRaw))
	tagUsage := map[string]int{}
	for rawLogicalTag, rawCandidates := range groupsRaw {
		logicalTag := strings.TrimSpace(rawLogicalTag)
		if logicalTag == "" {
			continue
		}

		candidates := parseStringList(rawCandidates)
		if len(candidates) == 0 {
			continue
		}

		filtered := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			if _, exists := availableTags[candidate]; !exists {
				continue
			}
			filtered = append(filtered, candidate)
		}
		if len(filtered) < 2 {
			continue
		}
		if _, aliasExists := availableTags[logicalTag]; aliasExists && !containsString(filtered, logicalTag) {
			continue
		}

		sanitized[logicalTag] = filtered
		for _, candidate := range filtered {
			tagUsage[candidate]++
		}
	}

	if len(sanitized) == 0 {
		return nil
	}

	result := make(map[string][]string, len(sanitized))
	for logicalTag, candidates := range sanitized {
		hasOverlap := false
		for _, candidate := range candidates {
			if tagUsage[candidate] > 1 {
				hasOverlap = true
				break
			}
		}
		if hasOverlap {
			continue
		}
		result[logicalTag] = candidates
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func collectOutboundTags(rawOutbounds any) map[string]struct{} {
	outbounds, ok := rawOutbounds.([]any)
	if !ok {
		return nil
	}

	tags := make(map[string]struct{}, len(outbounds))
	for _, item := range outbounds {
		outbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		tags[tag] = struct{}{}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func applyRandomizeGroups(cfg map[string]any, groups map[string][]string) error {
	if len(groups) == 0 {
		return nil
	}

	selected, err := selectRandomizeTargets(groups)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}

	trimRandomizedOutbounds(cfg, groups, selected)
	renameSelectedRandomizedOutbounds(cfg, selected)
	return nil
}

func selectRandomizeTargets(groups map[string][]string) (map[string]string, error) {
	selected := make(map[string]string, len(groups))
	for logicalTag, candidates := range groups {
		if len(candidates) == 0 {
			continue
		}
		index, err := randomIndex(len(candidates))
		if err != nil {
			return nil, err
		}
		selected[logicalTag] = candidates[index]
	}
	return selected, nil
}

func randomIndex(limit int) (int, error) {
	if limit <= 1 {
		return 0, nil
	}
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func renameSelectedRandomizedOutbounds(cfg map[string]any, selected map[string]string) {
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return
	}

	replacements := make(map[string]string, len(selected))
	for alias, candidate := range selected {
		candidate = strings.TrimSpace(candidate)
		alias = strings.TrimSpace(alias)
		if candidate == "" || alias == "" || candidate == alias {
			continue
		}
		replacements[candidate] = alias
	}
	if len(replacements) == 0 {
		return
	}

	for _, item := range outbounds {
		outbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		replacement, exists := replacements[tag]
		if !exists {
			continue
		}
		outbound["tag"] = replacement
	}
}

func trimRandomizedOutbounds(cfg map[string]any, groups map[string][]string, selected map[string]string) {
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return
	}

	candidateTags := make(map[string]struct{})
	for _, candidates := range groups {
		for _, candidate := range candidates {
			candidateTags[candidate] = struct{}{}
		}
	}
	selectedTags := make(map[string]struct{}, len(selected))
	for _, candidate := range selected {
		selectedTags[candidate] = struct{}{}
	}

	filtered := make([]any, 0, len(outbounds))
	for _, item := range outbounds {
		outbound, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if _, candidate := candidateTags[tag]; candidate {
			if _, keep := selectedTags[tag]; keep {
				filtered = append(filtered, item)
			}
			continue
		}
		filtered = append(filtered, item)
	}
	cfg["outbounds"] = filtered
}

func (d buildDirectives) skipOutbound(outbound map[string]any) bool {
	if len(d.skipOutboundTags) == 0 {
		return false
	}
	tag, _ := outbound["tag"].(string)
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	_, ok := d.skipOutboundTags[tag]
	return ok
}

func (b *Builder) SetTemplateSet(templates TemplateLibrary) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.templates = templates
	b.mu.Unlock()
}

func (b *Builder) TemplateSet() TemplateLibrary {
	if b == nil {
		return TemplateLibrary{
			Xray:   TemplateSet{Default: []any{}, Squads: map[string]any{}},
			Mihomo: TemplateSet{Default: []any{}, Squads: map[string]any{}},
		}
	}
	b.mu.RLock()
	current := b.templates
	b.mu.RUnlock()
	cloned, err := current.Clone()
	if err != nil {
		return current
	}
	return cloned
}

func (b *Builder) getTemplateSet() TemplateLibrary {
	if b == nil {
		return TemplateLibrary{}
	}
	b.mu.RLock()
	current := b.templates
	b.mu.RUnlock()
	return current
}
