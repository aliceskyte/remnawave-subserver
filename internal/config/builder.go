package config

import (
	"context"
	"errors"
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

	updateConfigItem := func(cfg map[string]any) {
		directives := parseBuildDirectives(cfg)
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
					return
				}
				cfg["remarks"] = trimmed
				return
			}
		}
		cfg["remarks"] = user.Username
	}

	switch value := cloned.(type) {
	case []any:
		for _, entry := range value {
			cfg, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			updateConfigItem(cfg)
		}
		return BuildResult{
			Content:     value,
			ContentType: "application/json; charset=utf-8",
		}, nil
	case map[string]any:
		updateConfigItem(value)
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
	items, ok := raw["skipOutboundTags"].([]any)
	if !ok {
		return buildDirectives{}
	}
	skipTags := make(map[string]struct{}, len(items))
	for _, item := range items {
		tag, ok := item.(string)
		if !ok {
			continue
		}
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		skipTags[tag] = struct{}{}
	}
	return buildDirectives{skipOutboundTags: skipTags}
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
