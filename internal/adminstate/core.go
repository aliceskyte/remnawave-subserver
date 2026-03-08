package adminstate

import "subserver/internal/config"

type CoreHeaderOverridesSet struct {
	Xray   HeaderOverridesSet
	Mihomo HeaderOverridesSet
}

func (s CoreHeaderOverridesSet) ForCore(core config.Core) HeaderOverridesSet {
	switch core {
	case config.CoreMihomo:
		return normalizeOverridesSet(s.Mihomo)
	default:
		return normalizeOverridesSet(s.Xray)
	}
}

func (s CoreHeaderOverridesSet) WithCore(core config.Core, value HeaderOverridesSet) CoreHeaderOverridesSet {
	value = normalizeOverridesSet(value)
	switch core {
	case config.CoreMihomo:
		s.Mihomo = value
	default:
		s.Xray = value
	}
	return s
}

func (s CoreHeaderOverridesSet) normalize() CoreHeaderOverridesSet {
	s.Xray = normalizeOverridesSet(s.Xray)
	s.Mihomo = normalizeOverridesSet(s.Mihomo)
	return s
}

func EnsureMihomoOverrides(set CoreHeaderOverridesSet) (CoreHeaderOverridesSet, bool) {
	set = set.normalize()
	if !headerOverridesSetIsEmpty(set.Mihomo) {
		return set, false
	}
	return set.WithCore(config.CoreMihomo, set.Xray), true
}

func headerOverridesSetIsEmpty(set HeaderOverridesSet) bool {
	if len(set.Default) > 0 {
		return false
	}
	for _, overrides := range set.Squads {
		if len(overrides) > 0 {
			return false
		}
	}
	return true
}
