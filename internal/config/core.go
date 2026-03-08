package config

import "strings"

type Core string

const (
	CoreXray   Core = "xray"
	CoreMihomo Core = "mihomo"
)

var supportedCores = []Core{CoreXray, CoreMihomo}

func SupportedCores() []Core {
	out := make([]Core, len(supportedCores))
	copy(out, supportedCores)
	return out
}

func NormalizeCore(value string) (Core, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(CoreXray):
		return CoreXray, true
	case string(CoreMihomo):
		return CoreMihomo, true
	default:
		return "", false
	}
}

func CoreOrDefault(value string) Core {
	if core, ok := NormalizeCore(value); ok {
		return core
	}
	return CoreXray
}

type TemplateLibrary struct {
	Xray   TemplateSet
	Mihomo TemplateSet
}

func ParseTemplateLibrary(raw any) (TemplateLibrary, error) {
	empty := TemplateLibrary{
		Xray:   TemplateSet{Default: []any{}, Squads: map[string]any{}},
		Mihomo: TemplateSet{Default: []any{}, Squads: map[string]any{}},
	}
	if raw == nil {
		return empty, nil
	}

	root, ok := raw.(map[string]any)
	if !ok {
		xray, err := ParseTemplateSet(raw)
		if err != nil {
			return TemplateLibrary{}, err
		}
		empty.Xray = xray
		return empty, nil
	}

	if _, hasXray := root[string(CoreXray)]; hasXray {
		result := empty
		for _, core := range SupportedCores() {
			rawSet, exists := root[string(core)]
			if !exists {
				continue
			}
			parsed, err := ParseTemplateSet(rawSet)
			if err != nil {
				return TemplateLibrary{}, err
			}
			result = result.WithCore(core, parsed)
		}
		return result, nil
	}

	xray, err := ParseTemplateSet(raw)
	if err != nil {
		return TemplateLibrary{}, err
	}
	empty.Xray = xray
	return empty, nil
}

func (l TemplateLibrary) Clone() (TemplateLibrary, error) {
	xray, err := l.ForCore(CoreXray).Clone()
	if err != nil {
		return TemplateLibrary{}, err
	}
	mihomo, err := l.ForCore(CoreMihomo).Clone()
	if err != nil {
		return TemplateLibrary{}, err
	}
	return TemplateLibrary{Xray: xray, Mihomo: mihomo}, nil
}

func (l TemplateLibrary) Raw() map[string]any {
	return map[string]any{
		string(CoreXray):   l.ForCore(CoreXray).Raw(),
		string(CoreMihomo): l.ForCore(CoreMihomo).Raw(),
	}
}

func (l TemplateLibrary) ForCore(core Core) TemplateSet {
	switch core {
	case CoreMihomo:
		return normalizeTemplateSet(l.Mihomo)
	default:
		return normalizeTemplateSet(l.Xray)
	}
}

func (l TemplateLibrary) WithCore(core Core, set TemplateSet) TemplateLibrary {
	set = normalizeTemplateSet(set)
	switch core {
	case CoreMihomo:
		l.Mihomo = set
	default:
		l.Xray = set
	}
	return l
}

func (l TemplateLibrary) SelectTemplate(core Core, squads []string) any {
	return l.ForCore(core).SelectTemplate(squads)
}

func normalizeTemplateSet(set TemplateSet) TemplateSet {
	if set.Default == nil {
		set.Default = []any{}
	}
	if set.Squads == nil {
		set.Squads = map[string]any{}
	}
	return set
}
