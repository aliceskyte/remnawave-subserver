package config

import (
	"strings"

	"subserver/internal/jsonutil"
)

type TemplateSet struct {
	Default any
	Squads  map[string]any
}

func ParseTemplateSet(raw any) (TemplateSet, error) {
	if raw == nil {
		return TemplateSet{Default: []any{}, Squads: map[string]any{}}, nil
	}

	switch value := raw.(type) {
	case map[string]any:
		if isTemplateSetMap(value) {
			defaultTemplate, ok := value["default"]
			if !ok || defaultTemplate == nil {
				defaultTemplate = []any{}
			}
			squads := map[string]any{}
			if rawSquads, ok := value["squads"].(map[string]any); ok {
				for key, tmpl := range rawSquads {
					cleanKey := strings.TrimSpace(key)
					if cleanKey == "" {
						continue
					}
					squads[cleanKey] = tmpl
				}
			}
			return TemplateSet{Default: defaultTemplate, Squads: squads}, nil
		}
		return TemplateSet{Default: value, Squads: map[string]any{}}, nil
	case []any:
		return TemplateSet{Default: value, Squads: map[string]any{}}, nil
	default:
		return TemplateSet{Default: value, Squads: map[string]any{}}, nil
	}
}

func (t TemplateSet) Clone() (TemplateSet, error) {
	defaultClone, err := jsonutil.CloneJSON(t.Default)
	if err != nil {
		return TemplateSet{}, err
	}
	squadsClone := map[string]any{}
	for key, tmpl := range t.Squads {
		cloned, err := jsonutil.CloneJSON(tmpl)
		if err != nil {
			return TemplateSet{}, err
		}
		squadsClone[key] = cloned
	}
	return TemplateSet{Default: defaultClone, Squads: squadsClone}, nil
}

func (t TemplateSet) Raw() map[string]any {
	squads := t.Squads
	if squads == nil {
		squads = map[string]any{}
	}
	return map[string]any{
		"default": t.Default,
		"squads":  squads,
	}
}

// SelectTemplate returns the config template for the given squad UUIDs.
// It uses the first squad UUID that has a matching template.
// If no squad template matches, the default template is returned.
func (t TemplateSet) SelectTemplate(squads []string) any {
	if len(squads) > 0 {
		if tmpl, ok := t.Squads[squads[0]]; ok {
			return tmpl
		}
	}
	return t.Default
}

func isTemplateSetMap(value map[string]any) bool {
	if _, ok := value["squads"]; ok {
		return true
	}
	if _, ok := value["default"]; ok && !jsonutil.LooksLikeConfigObject(value) {
		return true
	}
	return false
}
