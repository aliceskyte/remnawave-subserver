package admin

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"subserver/internal/adminstate"
	"subserver/internal/config"
	"subserver/internal/handler"
	"subserver/internal/httpx"
	"subserver/internal/jsonutil"
	"subserver/internal/panel"
	"subserver/internal/subscription"
)

type Options struct {
	RootDir     string
	ConfigStore *config.Store
	Panel       *panel.Client
	Builder     *config.Builder
	Headers     *adminstate.HeaderStore
	Logger      *slog.Logger
	Token       string
	RateLimiter *handler.RateLimiter
}

type Server struct {
	rootDir     string
	configStore *config.Store
	panel       *panel.Client
	builder     *config.Builder
	headers     *adminstate.HeaderStore
	logger      *slog.Logger
	token       string
	rateLimiter *handler.RateLimiter
}

type ConfigEntry struct {
	Name    string         `json:"name"`
	Config  map[string]any `json:"config,omitempty"`
	Content string         `json:"content,omitempty"`
}

type ConfigSet struct {
	Default []ConfigEntry            `json:"default"`
	Squads  map[string][]ConfigEntry `json:"squads"`
}

type CoreConfigSet struct {
	Xray   ConfigSet `json:"xray"`
	Mihomo ConfigSet `json:"mihomo"`
}

const maxAdminBodyBytes = 1 << 20

func NewServer(opts Options) *Server {
	return &Server{
		rootDir:     opts.RootDir,
		configStore: opts.ConfigStore,
		panel:       opts.Panel,
		builder:     opts.Builder,
		headers:     opts.Headers,
		logger:      opts.Logger,
		token:       opts.Token,
		rateLimiter: opts.RateLimiter,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	recorder := httpx.NewResponseRecorder(w, http.StatusOK)
	defer func() {
		if s.logger == nil {
			return
		}
		s.logger.Info("admin http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.Status,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", httpx.ClientIP(r),
			"ua", r.UserAgent(),
		)
	}()
	w = recorder
	setAdminCommonHeaders(w)

	switch {
	case r.URL.Path == "/health":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	case strings.HasPrefix(r.URL.Path, "/api/"):
		s.handleAPI(w, r)
		return
	default:
		s.serveStatic(w, r)
	}
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	if s.rateLimiter != nil {
		ip := httpx.ClientIP(r)
		if !s.rateLimiter.Allow(ip) {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
			return
		}
	}

	if err := s.authorize(r); err != nil {
		if s.logger != nil {
			s.logger.Warn("admin auth failed", "path", r.URL.Path, "error", err.Error())
		}
		s.writeAuthError(w, err)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/check":
		if s.logger != nil {
			s.logger.Info("admin auth ok")
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.Method == http.MethodGet && r.URL.Path == "/api/configs":
		s.handleGetConfigs(w)
	case r.Method == http.MethodPost && r.URL.Path == "/api/configs":
		s.handleSetConfigs(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/headers":
		s.handleGetHeaders(w)
	case r.Method == http.MethodPost && r.URL.Path == "/api/headers":
		s.handleSetHeaders(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/remnawave/headers":
		s.handleFetchHeaders(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/remnawave/internal-squads":
		s.handleGetInternalSquads(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/"):
		s.writeMethodOrNotFound(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

func (s *Server) writeMethodOrNotFound(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/auth/check":
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	case "/api/configs", "/api/headers":
		w.Header().Set("Allow", "GET, HEAD, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	case "/api/remnawave/headers", "/api/remnawave/internal-squads":
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

func (s *Server) handleGetConfigs(w http.ResponseWriter) {
	if s.builder == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_unavailable"})
		return
	}
	templateSet := s.builder.TemplateSet()
	configSet, err := templateLibraryToConfigSet(templateSet)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to parse config template", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, configSet)
}

func (s *Server) handleSetConfigs(w http.ResponseWriter, r *http.Request) {
	if s.builder == nil || s.configStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_unavailable"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)
	payload, err := decodeJSON(r.Body)
	if err != nil {
		if isBodyTooLarge(err) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "payload_too_large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	configSet, err := parseCoreConfigSet(payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	templateSet, err := configSetToTemplateLibrary(configSet)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := s.configStore.SaveTemplateSet(r.Context(), templateSet); err != nil {
		if s.logger != nil {
			s.logger.Error("failed to save config template", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config_save_failed"})
		return
	}

	s.builder.SetTemplateSet(templateSet)
	if s.logger != nil {
		s.logger.Info("configs updated",
			"xray_default_count", len(configSet.Xray.Default),
			"xray_squad_count", len(configSet.Xray.Squads),
			"mihomo_default_count", len(configSet.Mihomo.Default),
			"mihomo_squad_count", len(configSet.Mihomo.Squads),
		)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleGetHeaders(w http.ResponseWriter) {
	if s.headers == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			string(config.CoreXray): map[string]any{
				"default":   map[string]subscription.HeaderOverride{},
				"squads":    map[string]map[string]subscription.HeaderOverride{},
				"overrides": map[string]subscription.HeaderOverride{},
			},
			string(config.CoreMihomo): map[string]any{
				"default":   map[string]subscription.HeaderOverride{},
				"squads":    map[string]map[string]subscription.HeaderOverride{},
				"overrides": map[string]subscription.HeaderOverride{},
			},
		})
		return
	}
	overrides := s.headers.HeaderOverridesSet()
	writeJSON(w, http.StatusOK, map[string]any{
		string(config.CoreXray): map[string]any{
			"default":   overrides.Xray.Default,
			"squads":    overrides.Xray.Squads,
			"overrides": overrides.Xray.Default,
		},
		string(config.CoreMihomo): map[string]any{
			"default":   overrides.Mihomo.Default,
			"squads":    overrides.Mihomo.Squads,
			"overrides": overrides.Mihomo.Default,
		},
	})
}

func (s *Server) handleSetHeaders(w http.ResponseWriter, r *http.Request) {
	if s.headers == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "header_store_unavailable"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)
	payload, err := decodeJSON(r.Body)
	if err != nil {
		if isBodyTooLarge(err) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "payload_too_large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	overrides, err := parseCoreOverridesSet(payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := s.headers.SaveSet(overrides); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "header_save_failed"})
		return
	}

	if s.logger != nil {
		s.logger.Info("header overrides updated",
			"xray_default_count", len(overrides.Xray.Default),
			"xray_squad_count", len(overrides.Xray.Squads),
			"mihomo_default_count", len(overrides.Mihomo.Default),
			"mihomo_squad_count", len(overrides.Mihomo.Squads),
		)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleFetchHeaders(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimSpace(r.URL.Query().Get("uuid"))
	if uuid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "uuid_required"})
		return
	}
	if !panel.ValidShortUUID(uuid) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_uuid"})
		return
	}
	if s.panel == nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unavailable"})
		return
	}

	raw, err := s.panel.SubscriptionRawByShortUUIDFresh(r.Context(), uuid)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_error"})
		return
	}

	headers := subscription.BuildHeadersFromRaw(raw)
	writeJSON(w, http.StatusOK, map[string]any{"headers": headers})
}

func (s *Server) handleGetInternalSquads(w http.ResponseWriter, r *http.Request) {
	if s.panel == nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unavailable"})
		return
	}
	squads, err := s.panel.InternalSquads(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"internalSquads": squads})
}

func (s *Server) authorize(r *http.Request) error {
	token := extractBearerToken(r)
	if token == "" || strings.TrimSpace(s.token) == "" {
		return errors.New("unauthorized")
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
		return errors.New("unauthorized")
	}
	return nil
}

func (s *Server) writeAuthError(w http.ResponseWriter, err error) {
	if err == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err.Error() == "unauthorized" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_error"})
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	clean := filepath.Clean("/" + strings.TrimPrefix(path, "/"))
	baseDir := filepath.Join(s.rootDir, "admin")
	filePath := filepath.Join(baseDir, clean)

	// Use filepath.Rel for more robust path traversal protection.
	rel, err := filepath.Rel(baseDir, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	info, statErr := os.Stat(filePath)
	if statErr != nil || info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	// Security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; object-src 'none'")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, filePath)
}

func extractBearerToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func setAdminCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
}

func decodeJSON(body io.Reader) (any, error) {
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, errors.New("empty body")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func parseCoreConfigSet(payload any) (CoreConfigSet, error) {
	root, ok := payload.(map[string]any)
	if !ok {
		legacy, err := parseConfigSetForCore(config.CoreXray, payload)
		if err != nil {
			return CoreConfigSet{}, err
		}
		return CoreConfigSet{Xray: legacy, Mihomo: emptyConfigSet()}, nil
	}

	if _, hasXray := root[string(config.CoreXray)]; hasXray {
		result := CoreConfigSet{
			Xray:   emptyConfigSet(),
			Mihomo: emptyConfigSet(),
		}
		for _, core := range config.SupportedCores() {
			rawSet, exists := root[string(core)]
			if !exists {
				continue
			}
			parsed, err := parseConfigSetForCore(core, rawSet)
			if err != nil {
				return CoreConfigSet{}, err
			}
			switch core {
			case config.CoreMihomo:
				result.Mihomo = parsed
			default:
				result.Xray = parsed
			}
		}
		return result, nil
	}

	legacy, err := parseConfigSetForCore(config.CoreXray, payload)
	if err != nil {
		return CoreConfigSet{}, err
	}
	return CoreConfigSet{Xray: legacy, Mihomo: emptyConfigSet()}, nil
}

func parseConfigSetForCore(core config.Core, payload any) (ConfigSet, error) {
	switch value := payload.(type) {
	case map[string]any:
		rawDefault, hasDefault := value["default"]
		_, hasSquads := value["squads"]
		if hasDefault || hasSquads {
			defaultEntries, err := parseConfigEntriesFromAny(core, rawDefault)
			if err != nil {
				return ConfigSet{}, err
			}
			squadEntries, err := parseSquadEntries(core, value["squads"])
			if err != nil {
				return ConfigSet{}, err
			}
			return ConfigSet{Default: defaultEntries, Squads: squadEntries}, nil
		}
		if rawConfigs, ok := value["configs"]; ok && core == config.CoreXray {
			entries, err := parseConfigEntriesFromAny(core, rawConfigs)
			if err != nil {
				return ConfigSet{}, err
			}
			return ConfigSet{Default: entries, Squads: map[string][]ConfigEntry{}}, nil
		}
		if core == config.CoreXray && jsonutil.LooksLikeConfigObject(value) {
			entries, err := parseConfigEntriesFromAny(core, []any{value})
			if err != nil {
				return ConfigSet{}, err
			}
			return ConfigSet{Default: entries, Squads: map[string][]ConfigEntry{}}, nil
		}
		if core == config.CoreMihomo {
			if _, hasContent := value["content"]; hasContent {
				entries, err := parseConfigEntriesFromAny(core, []any{value})
				if err != nil {
					return ConfigSet{}, err
				}
				return ConfigSet{Default: entries, Squads: map[string][]ConfigEntry{}}, nil
			}
		}
		return ConfigSet{}, errors.New("config_payload_invalid")
	case []any:
		entries, err := parseConfigEntriesFromAny(core, value)
		if err != nil {
			return ConfigSet{}, err
		}
		return ConfigSet{Default: entries, Squads: map[string][]ConfigEntry{}}, nil
	default:
		return ConfigSet{}, errors.New("config_payload_invalid")
	}
}

func parseConfigEntriesFromAny(core config.Core, raw any) ([]ConfigEntry, error) {
	if raw == nil {
		return []ConfigEntry{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New("configs_invalid")
	}
	return parseConfigEntryList(core, items)
}

func parseConfigEntryList(core config.Core, items []any) ([]ConfigEntry, error) {
	entries := make([]ConfigEntry, 0, len(items))
	for idx, item := range items {
		entryMap, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("config_entry_invalid")
		}

		if core == config.CoreMihomo {
			name, _ := entryMap["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				name = fmt.Sprintf("config-%d", idx+1)
			}
			content := strings.TrimSpace(fmt.Sprint(entryMap["content"]))
			if content == "" {
				return nil, errors.New("config_body_required")
			}
			entries = append(entries, ConfigEntry{Name: name, Content: content})
			continue
		}

		if rawConfig, ok := entryMap["config"]; ok {
			name, _ := entryMap["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				name = deriveConfigName(rawConfig, idx)
			}
			if name == "" {
				return nil, errors.New("config_name_required")
			}
			configValue, ok := rawConfig.(map[string]any)
			if !ok {
				return nil, errors.New("config_body_required")
			}
			cloned, err := jsonutil.CloneMap(configValue)
			if err != nil {
				return nil, errors.New("config_body_invalid")
			}
			entries = append(entries, ConfigEntry{Name: name, Config: cloned})
			continue
		}

		cloned, err := jsonutil.CloneMap(entryMap)
		if err != nil {
			return nil, errors.New("config_body_invalid")
		}
		name := deriveConfigName(entryMap, idx)
		if name == "" {
			name = fmt.Sprintf("config-%d", idx+1)
		}
		entries = append(entries, ConfigEntry{Name: name, Config: cloned})
	}
	return entries, nil
}

func deriveConfigName(raw any, index int) string {
	cfg, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	if remark, ok := cfg["remarks"]; ok {
		name := strings.TrimSpace(fmt.Sprint(remark))
		if name != "" {
			return name
		}
	}
	if meta, ok := cfg["meta"].(map[string]any); ok {
		if desc, ok := meta["serverDescription"]; ok {
			name := strings.TrimSpace(fmt.Sprint(desc))
			if name != "" {
				return name
			}
		}
	}
	if index >= 0 {
		return fmt.Sprintf("config-%d", index+1)
	}
	return ""
}

func parseSquadEntries(core config.Core, raw any) (map[string][]ConfigEntry, error) {
	if raw == nil {
		return map[string][]ConfigEntry{}, nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("configs_invalid")
	}
	result := make(map[string][]ConfigEntry, len(items))
	for key, value := range items {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" || cleanKey == "default" {
			continue
		}
		entries, err := parseConfigEntriesFromAny(core, value)
		if err != nil {
			return nil, err
		}
		result[cleanKey] = entries
	}
	return result, nil
}

func templateLibraryToConfigSet(templateSet config.TemplateLibrary) (CoreConfigSet, error) {
	xray, err := templateSetToConfigSet(config.CoreXray, templateSet.ForCore(config.CoreXray))
	if err != nil {
		return CoreConfigSet{}, err
	}
	mihomo, err := templateSetToConfigSet(config.CoreMihomo, templateSet.ForCore(config.CoreMihomo))
	if err != nil {
		return CoreConfigSet{}, err
	}
	return CoreConfigSet{Xray: xray, Mihomo: mihomo}, nil
}

func templateSetToConfigSet(core config.Core, templateSet config.TemplateSet) (ConfigSet, error) {
	defaultEntries, err := templateToEntries(core, templateSet.Default)
	if err != nil {
		return ConfigSet{}, err
	}
	squadEntries := make(map[string][]ConfigEntry, len(templateSet.Squads))
	for key, template := range templateSet.Squads {
		entries, err := templateToEntries(core, template)
		if err != nil {
			return ConfigSet{}, err
		}
		squadEntries[key] = entries
	}
	return ConfigSet{Default: defaultEntries, Squads: squadEntries}, nil
}

func configSetToTemplateLibrary(configSet CoreConfigSet) (config.TemplateLibrary, error) {
	xray, err := configSetToTemplateSet(config.CoreXray, configSet.Xray)
	if err != nil {
		return config.TemplateLibrary{}, err
	}
	mihomo, err := configSetToTemplateSet(config.CoreMihomo, configSet.Mihomo)
	if err != nil {
		return config.TemplateLibrary{}, err
	}
	result := config.TemplateLibrary{}
	result = result.WithCore(config.CoreXray, xray)
	result = result.WithCore(config.CoreMihomo, mihomo)
	return result, nil
}

func configSetToTemplateSet(core config.Core, configSet ConfigSet) (config.TemplateSet, error) {
	defaultTemplate, err := entriesToTemplate(core, configSet.Default)
	if err != nil {
		return config.TemplateSet{}, err
	}
	squadTemplates := make(map[string]any, len(configSet.Squads))
	for key, entries := range configSet.Squads {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" || cleanKey == "default" {
			continue
		}
		template, err := entriesToTemplate(core, entries)
		if err != nil {
			return config.TemplateSet{}, err
		}
		squadTemplates[cleanKey] = template
	}
	return config.TemplateSet{Default: defaultTemplate, Squads: squadTemplates}, nil
}

func entriesToTemplate(core config.Core, entries []ConfigEntry) (any, error) {
	if len(entries) == 0 {
		return []any{}, nil
	}

	if core == config.CoreMihomo {
		result := make([]any, 0, len(entries))
		for _, entry := range entries {
			name := strings.TrimSpace(entry.Name)
			content := strings.TrimSpace(entry.Content)
			if name == "" {
				return nil, errors.New("config_name_required")
			}
			if content == "" {
				return nil, errors.New("config_body_required")
			}
			result = append(result, map[string]any{"name": name, "content": content})
		}
		return result, nil
	}

	result := make([]any, 0, len(entries))
	for _, entry := range entries {
		if entry.Name == "" {
			return nil, errors.New("config_name_required")
		}
		if entry.Config == nil {
			return nil, errors.New("config_body_required")
		}
		cfg, err := jsonutil.CloneMap(entry.Config)
		if err != nil {
			return nil, errors.New("config_body_invalid")
		}
		cfg["remarks"] = entry.Name
		result = append(result, cfg)
	}
	return result, nil
}

func templateToEntries(core config.Core, template any) ([]ConfigEntry, error) {
	if template == nil {
		return []ConfigEntry{}, nil
	}

	if core == config.CoreMihomo {
		entries, err := config.ParseMihomoEntries(template)
		if err != nil {
			return nil, err
		}
		result := make([]ConfigEntry, 0, len(entries))
		for _, entry := range entries {
			result = append(result, ConfigEntry{Name: entry.Name, Content: entry.Content})
		}
		return result, nil
	}

	switch value := template.(type) {
	case []any:
		entries := make([]ConfigEntry, 0, len(value))
		for _, item := range value {
			cfg, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("config_entry_invalid")
			}
			cloned, err := jsonutil.CloneMap(cfg)
			if err != nil {
				return nil, err
			}
			name := ""
			if remark, ok := cloned["remarks"]; ok {
				name = strings.TrimSpace(fmt.Sprint(remark))
				delete(cloned, "remarks")
			}
			entries = append(entries, ConfigEntry{Name: name, Config: cloned})
		}
		return entries, nil
	case map[string]any:
		cloned, err := jsonutil.CloneMap(value)
		if err != nil {
			return nil, err
		}
		name := ""
		if remark, ok := cloned["remarks"]; ok {
			name = strings.TrimSpace(fmt.Sprint(remark))
			delete(cloned, "remarks")
		}
		return []ConfigEntry{{Name: name, Config: cloned}}, nil
	default:
		return nil, errors.New("config_template_invalid")
	}
}

func parseCoreOverridesSet(payload any) (adminstate.CoreHeaderOverridesSet, error) {
	root, ok := payload.(map[string]any)
	if !ok {
		return adminstate.CoreHeaderOverridesSet{}, errors.New("overrides_invalid")
	}

	if _, hasXray := root[string(config.CoreXray)]; hasXray {
		result := adminstate.CoreHeaderOverridesSet{
			Xray:   adminstate.HeaderOverridesSet{Default: map[string]subscription.HeaderOverride{}, Squads: map[string]map[string]subscription.HeaderOverride{}},
			Mihomo: adminstate.HeaderOverridesSet{Default: map[string]subscription.HeaderOverride{}, Squads: map[string]map[string]subscription.HeaderOverride{}},
		}
		for _, core := range config.SupportedCores() {
			rawSet, exists := root[string(core)]
			if !exists {
				continue
			}
			set, err := parseOverridesSet(rawSet)
			if err != nil {
				return adminstate.CoreHeaderOverridesSet{}, err
			}
			result = result.WithCore(core, set)
		}
		return result, nil
	}

	legacy, err := parseOverridesSet(payload)
	if err != nil {
		return adminstate.CoreHeaderOverridesSet{}, err
	}
	return adminstate.CoreHeaderOverridesSet{
		Xray:   legacy,
		Mihomo: adminstate.HeaderOverridesSet{Default: map[string]subscription.HeaderOverride{}, Squads: map[string]map[string]subscription.HeaderOverride{}},
	}, nil
}

func parseOverridesSet(payload any) (adminstate.HeaderOverridesSet, error) {
	root, ok := payload.(map[string]any)
	if !ok {
		return adminstate.HeaderOverridesSet{}, errors.New("overrides_invalid")
	}

	rawDefault, hasDefault := root["default"]
	_, hasSquads := root["squads"]
	if hasDefault || hasSquads {
		if !hasDefault {
			rawDefault = root["overrides"]
		}
		defaultOverrides, err := parseOverridesMap(rawDefault)
		if err != nil {
			return adminstate.HeaderOverridesSet{}, err
		}
		squadOverrides, err := parseSquadOverrides(root["squads"])
		if err != nil {
			return adminstate.HeaderOverridesSet{}, err
		}
		return adminstate.HeaderOverridesSet{Default: defaultOverrides, Squads: squadOverrides}, nil
	}
	if raw, ok := root["overrides"]; ok {
		defaultOverrides, err := parseOverridesMap(raw)
		if err != nil {
			return adminstate.HeaderOverridesSet{}, err
		}
		return adminstate.HeaderOverridesSet{Default: defaultOverrides, Squads: map[string]map[string]subscription.HeaderOverride{}}, nil
	}
	defaultOverrides, err := parseOverridesMap(root)
	if err != nil {
		return adminstate.HeaderOverridesSet{}, err
	}
	return adminstate.HeaderOverridesSet{Default: defaultOverrides, Squads: map[string]map[string]subscription.HeaderOverride{}}, nil
}

func emptyConfigSet() ConfigSet {
	return ConfigSet{Default: []ConfigEntry{}, Squads: map[string][]ConfigEntry{}}
}

func parseOverridesMap(raw any) (map[string]subscription.HeaderOverride, error) {
	if raw == nil {
		return map[string]subscription.HeaderOverride{}, nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("overrides_invalid")
	}

	result := make(map[string]subscription.HeaderOverride, len(items))
	for key, value := range items {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			continue
		}
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
		result[cleanKey] = override
	}

	return result, nil
}

func parseSquadOverrides(raw any) (map[string]map[string]subscription.HeaderOverride, error) {
	if raw == nil {
		return map[string]map[string]subscription.HeaderOverride{}, nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("overrides_invalid")
	}
	result := make(map[string]map[string]subscription.HeaderOverride, len(items))
	for key, value := range items {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" || cleanKey == "default" {
			continue
		}
		overrides, err := parseOverridesMap(value)
		if err != nil {
			return nil, err
		}
		result[cleanKey] = overrides
	}
	return result, nil
}

func parseParamOverrides(raw any) map[string]subscription.HeaderParamOverride {
	items, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]subscription.HeaderParamOverride, len(items))
	for key, value := range items {
		cleanKey := strings.TrimSpace(strings.ToLower(key))
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
