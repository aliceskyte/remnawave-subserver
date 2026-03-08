package adminstate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"subserver/internal/config"
	"subserver/internal/subscription"
)

const defaultOverridesKey = "default"

// HeaderStore keeps header overrides in memory and persists them to the database.
type HeaderStore struct {
	mu        sync.RWMutex
	db        *sql.DB
	overrides CoreHeaderOverridesSet
}

type HeaderOverridesSet struct {
	Default map[string]subscription.HeaderOverride
	Squads  map[string]map[string]subscription.HeaderOverride
}

func LoadHeaderStore(db *sql.DB) (*HeaderStore, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	store := &HeaderStore{
		db: db,
		overrides: CoreHeaderOverridesSet{
			Xray: HeaderOverridesSet{
				Default: map[string]subscription.HeaderOverride{},
				Squads:  map[string]map[string]subscription.HeaderOverride{},
			},
			Mihomo: HeaderOverridesSet{
				Default: map[string]subscription.HeaderOverride{},
				Squads:  map[string]map[string]subscription.HeaderOverride{},
			},
		},
	}

	var payload []byte
	err := db.QueryRow("SELECT payload FROM header_overrides WHERE key = ?", defaultOverridesKey).Scan(&payload)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store, nil
		}
		return nil, err
	}

	decoded, err := DecodeOverridesSet(payload)
	if err != nil {
		return nil, err
	}
	store.overrides = decoded.normalize()
	return store, nil
}

func (s *HeaderStore) Exists(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("header store is nil")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM header_overrides WHERE key = ?)", defaultOverridesKey).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *HeaderStore) HeaderOverrides() map[string]subscription.HeaderOverride {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	copyMap := cloneOverrides(s.overrides.Xray.Default)
	s.mu.RUnlock()
	return copyMap
}

func (s *HeaderStore) HeaderOverridesSet() CoreHeaderOverridesSet {
	if s == nil {
		return CoreHeaderOverridesSet{
			Xray:   HeaderOverridesSet{Default: map[string]subscription.HeaderOverride{}, Squads: map[string]map[string]subscription.HeaderOverride{}},
			Mihomo: HeaderOverridesSet{Default: map[string]subscription.HeaderOverride{}, Squads: map[string]map[string]subscription.HeaderOverride{}},
		}
	}
	s.mu.RLock()
	current := s.overrides
	s.mu.RUnlock()
	return cloneCoreOverridesSet(current)
}

func (s *HeaderStore) HeaderOverridesForCoreAndSquads(core config.Core, squads []string) map[string]subscription.HeaderOverride {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	current := s.overrides
	s.mu.RUnlock()
	return selectOverrides(current.ForCore(core), squads)
}

func (s *HeaderStore) Save(overrides map[string]subscription.HeaderOverride) error {
	return s.SaveSet(CoreHeaderOverridesSet{
		Xray: HeaderOverridesSet{Default: overrides},
	})
}

func (s *HeaderStore) SaveSet(overrides CoreHeaderOverridesSet) error {
	if s == nil || s.db == nil {
		return errors.New("header store is nil")
	}
	normalized := overrides.normalize()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.save(ctx, normalized); err != nil {
		return err
	}
	s.mu.Lock()
	s.overrides = normalized
	s.mu.Unlock()
	return nil
}

func (s *HeaderStore) save(ctx context.Context, overrides CoreHeaderOverridesSet) error {
	payload := map[string]any{
		string(config.CoreXray): map[string]any{
			"default": overrides.Xray.Default,
			"squads":  overrides.Xray.Squads,
		},
		string(config.CoreMihomo): map[string]any{
			"default": overrides.Mihomo.Default,
			"squads":  overrides.Mihomo.Squads,
		},
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO header_overrides (key, payload, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (key)
DO UPDATE SET payload = excluded.payload, updated_at = CURRENT_TIMESTAMP
`, defaultOverridesKey, content)
	return err
}

func DecodeOverridesSet(content []byte) (CoreHeaderOverridesSet, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return CoreHeaderOverridesSet{}, err
	}

	entries, ok := payload.(map[string]any)
	if !ok {
		return CoreHeaderOverridesSet{}, fmt.Errorf("invalid overrides payload")
	}

	if rawXray, ok := entries[string(config.CoreXray)]; ok {
		result := CoreHeaderOverridesSet{
			Xray: HeaderOverridesSet{
				Default: map[string]subscription.HeaderOverride{},
				Squads:  map[string]map[string]subscription.HeaderOverride{},
			},
			Mihomo: HeaderOverridesSet{
				Default: map[string]subscription.HeaderOverride{},
				Squads:  map[string]map[string]subscription.HeaderOverride{},
			},
		}
		for _, core := range config.SupportedCores() {
			raw, exists := entries[string(core)]
			if !exists {
				continue
			}
			coreEntries, ok := raw.(map[string]any)
			if !ok {
				return CoreHeaderOverridesSet{}, fmt.Errorf("invalid overrides payload")
			}
			defaultOverrides, err := decodeOverridesMap(coreEntries["default"])
			if err != nil {
				return CoreHeaderOverridesSet{}, err
			}
			squadOverrides, err := decodeSquadOverrides(coreEntries["squads"])
			if err != nil {
				return CoreHeaderOverridesSet{}, err
			}
			result = result.WithCore(core, HeaderOverridesSet{Default: defaultOverrides, Squads: squadOverrides})
		}
		_ = rawXray
		return result.normalize(), nil
	}

	if rawSquads, ok := entries["squads"]; ok {
		defaultOverrides, err := decodeOverridesMap(entries["default"])
		if err != nil {
			return CoreHeaderOverridesSet{}, err
		}
		squadOverrides, err := decodeSquadOverrides(rawSquads)
		if err != nil {
			return CoreHeaderOverridesSet{}, err
		}
		return CoreHeaderOverridesSet{
			Xray: HeaderOverridesSet{Default: defaultOverrides, Squads: squadOverrides},
			Mihomo: HeaderOverridesSet{
				Default: map[string]subscription.HeaderOverride{},
				Squads:  map[string]map[string]subscription.HeaderOverride{},
			},
		}.normalize(), nil
	}

	defaultOverrides, err := decodeOverridesMap(entries)
	if err != nil {
		return CoreHeaderOverridesSet{}, err
	}
	return CoreHeaderOverridesSet{
		Xray: HeaderOverridesSet{Default: defaultOverrides, Squads: map[string]map[string]subscription.HeaderOverride{}},
		Mihomo: HeaderOverridesSet{
			Default: map[string]subscription.HeaderOverride{},
			Squads:  map[string]map[string]subscription.HeaderOverride{},
		},
	}.normalize(), nil
}

func decodeOverridesMap(raw any) (map[string]subscription.HeaderOverride, error) {
	if raw == nil {
		return map[string]subscription.HeaderOverride{}, nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid overrides payload")
	}

	overrides := make(map[string]subscription.HeaderOverride, len(items))
	for key, value := range items {
		override := subscription.HeaderOverride{Mode: "custom"}
		switch v := value.(type) {
		case string:
			override.Value = v
		case map[string]any:
			if mode, ok := v["mode"].(string); ok {
				override.Mode = mode
			}
			if val, ok := v["value"]; ok {
				override.Value = strings.TrimSpace(fmt.Sprint(val))
			}
			if paramsRaw, ok := v["params"]; ok {
				override.Params = parseParamOverrides(paramsRaw)
			}
		default:
			continue
		}
		overrides[key] = override
	}

	return overrides, nil
}

func decodeSquadOverrides(raw any) (map[string]map[string]subscription.HeaderOverride, error) {
	if raw == nil {
		return map[string]map[string]subscription.HeaderOverride{}, nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid overrides payload")
	}
	result := make(map[string]map[string]subscription.HeaderOverride, len(items))
	for key, value := range items {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" || cleanKey == "default" {
			continue
		}
		overrides, err := decodeOverridesMap(value)
		if err != nil {
			return nil, err
		}
		result[cleanKey] = overrides
	}
	return result, nil
}

func normalizeOverridesSet(overrides HeaderOverridesSet) HeaderOverridesSet {
	result := HeaderOverridesSet{
		Default: normalizeOverrides(overrides.Default),
		Squads:  map[string]map[string]subscription.HeaderOverride{},
	}
	for key, value := range overrides.Squads {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" || cleanKey == "default" {
			continue
		}
		result.Squads[cleanKey] = normalizeOverrides(value)
	}
	return result
}

func cloneCoreOverridesSet(current CoreHeaderOverridesSet) CoreHeaderOverridesSet {
	return CoreHeaderOverridesSet{
		Xray:   cloneOverridesSet(current.Xray),
		Mihomo: cloneOverridesSet(current.Mihomo),
	}
}

func normalizeOverrides(overrides map[string]subscription.HeaderOverride) map[string]subscription.HeaderOverride {
	if overrides == nil {
		return map[string]subscription.HeaderOverride{}
	}
	result := make(map[string]subscription.HeaderOverride, len(overrides))
	for key, value := range overrides {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			continue
		}
		result[cleanKey] = subscription.HeaderOverride{
			Mode:   strings.TrimSpace(value.Mode),
			Value:  strings.TrimSpace(value.Value),
			Params: normalizeParams(value.Params),
		}
	}
	return result
}

func cloneOverridesSet(overrides HeaderOverridesSet) HeaderOverridesSet {
	return HeaderOverridesSet{
		Default: cloneOverrides(overrides.Default),
		Squads:  cloneSquadOverrides(overrides.Squads),
	}
}

func cloneOverrides(overrides map[string]subscription.HeaderOverride) map[string]subscription.HeaderOverride {
	if overrides == nil {
		return map[string]subscription.HeaderOverride{}
	}
	copyMap := make(map[string]subscription.HeaderOverride, len(overrides))
	for key, value := range overrides {
		copyMap[key] = value
	}
	return copyMap
}

func cloneSquadOverrides(overrides map[string]map[string]subscription.HeaderOverride) map[string]map[string]subscription.HeaderOverride {
	if overrides == nil {
		return map[string]map[string]subscription.HeaderOverride{}
	}
	copyMap := make(map[string]map[string]subscription.HeaderOverride, len(overrides))
	for key, value := range overrides {
		copyMap[key] = cloneOverrides(value)
	}
	return copyMap
}

func selectOverrides(overrides HeaderOverridesSet, squads []string) map[string]subscription.HeaderOverride {
	result := normalizeOverrides(overrides.Default)
	if len(squads) == 0 {
		return result
	}
	for _, squad := range squads {
		clean := strings.TrimSpace(squad)
		if clean == "" {
			continue
		}
		if squadOverrides, ok := overrides.Squads[clean]; ok {
			for key, value := range normalizeOverrides(squadOverrides) {
				result[key] = value
			}
		}
	}
	return result
}

func parseParamOverrides(raw any) map[string]subscription.HeaderParamOverride {
	items, ok := raw.(map[string]any)
	if !ok {
		return map[string]subscription.HeaderParamOverride{}
	}
	result := make(map[string]subscription.HeaderParamOverride, len(items))
	for key, value := range items {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			continue
		}
		override := subscription.HeaderParamOverride{Mode: "custom"}
		switch v := value.(type) {
		case string:
			override.Value = v
		case map[string]any:
			if mode, ok := v["mode"].(string); ok {
				override.Mode = mode
			}
			if val, ok := v["value"]; ok {
				override.Value = strings.TrimSpace(fmt.Sprint(val))
			}
		default:
			continue
		}
		result[cleanKey] = override
	}
	return result
}

func normalizeParams(params map[string]subscription.HeaderParamOverride) map[string]subscription.HeaderParamOverride {
	if params == nil {
		return map[string]subscription.HeaderParamOverride{}
	}
	result := make(map[string]subscription.HeaderParamOverride, len(params))
	for key, value := range params {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			continue
		}
		result[cleanKey] = subscription.HeaderParamOverride{
			Mode:  strings.TrimSpace(value.Mode),
			Value: strings.TrimSpace(value.Value),
		}
	}
	return result
}
