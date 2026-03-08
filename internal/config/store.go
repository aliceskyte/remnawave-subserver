package config

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const defaultTemplateKey = "default"

var ErrTemplateSetMissing = errors.New("template set missing")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Exists(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("config store is nil")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_templates WHERE key = ?)", defaultTemplateKey).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) LoadTemplateSet(ctx context.Context) (TemplateLibrary, error) {
	if s == nil || s.db == nil {
		return TemplateLibrary{}, errors.New("config store is nil")
	}
	var payload []byte
	err := s.db.QueryRowContext(ctx, "SELECT payload FROM config_templates WHERE key = ?", defaultTemplateKey).Scan(&payload)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TemplateLibrary{}, ErrTemplateSetMissing
		}
		return TemplateLibrary{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return TemplateLibrary{}, err
	}
	return ParseTemplateLibrary(raw)
}

func (s *Store) SaveTemplateSet(ctx context.Context, templates TemplateLibrary) error {
	if s == nil || s.db == nil {
		return errors.New("config store is nil")
	}
	payload, err := json.Marshal(templates.Raw())
	if err != nil {
		return fmt.Errorf("marshal template set: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO config_templates (key, payload, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (key)
DO UPDATE SET payload = excluded.payload, updated_at = CURRENT_TIMESTAMP
`, defaultTemplateKey, payload)
	return err
}
