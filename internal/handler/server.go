package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"subserver/internal/config"
	"subserver/internal/httpx"
	"subserver/internal/panel"
	"subserver/internal/subscription"
)

type Options struct {
	SubPathPrefix   string
	Panel           *panel.Client
	Builder         *config.Builder
	Logger          *slog.Logger
	RateLimiter     *RateLimiter
	HeaderOverrides HeaderOverridesProvider
}

type HeaderOverridesProvider interface {
	HeaderOverridesForCoreAndSquads(core config.Core, squads []string) map[string]subscription.HeaderOverride
}

const legacySubPathSegment = "RSqMYdaMGwej"
const legacySubPathPrefix = "/" + legacySubPathSegment + "/"

type Server struct {
	subPathPrefix   string
	panel           *panel.Client
	builder         *config.Builder
	logger          *slog.Logger
	rateLimiter     *RateLimiter
	headerOverrides HeaderOverridesProvider
}

func NewServer(opts Options) *Server {
	subPathPrefix := opts.SubPathPrefix
	if !strings.HasPrefix(subPathPrefix, "/") {
		subPathPrefix = "/" + subPathPrefix
	}
	if !strings.HasSuffix(subPathPrefix, "/") {
		subPathPrefix = subPathPrefix + "/"
	}

	return &Server{
		subPathPrefix:   subPathPrefix,
		panel:           opts.Panel,
		builder:         opts.Builder,
		logger:          opts.Logger,
		rateLimiter:     opts.RateLimiter,
		headerOverrides: opts.HeaderOverrides,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	recorder := httpx.NewResponseRecorder(w, http.StatusOK)
	defer func() {
		if s.logger == nil {
			return
		}
		s.logger.Info("http request",
			"method", r.Method,
			"path", s.requestLogPath(r.URL.Path),
			"status", recorder.Status,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", httpx.ClientIP(r),
			"ua", r.UserAgent(),
		)
	}()
	w = recorder
	setCommonSecurityHeaders(w)

	path := r.URL.Path
	if r.URL.RawQuery == "" {
		if normalizedPath, legacyQuery, ok := splitLegacyPathQuery(path); ok {
			path = normalizedPath
			r.URL.Path = normalizedPath
			r.URL.RawPath = normalizedPath
			r.URL.RawQuery = legacyQuery
		}
	}

	if path == "/health" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			s.sendJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"}, nil)
			return
		}
		s.sendHTML(w, http.StatusOK, "ok")
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		s.sendJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"}, nil)
		return
	}

	if s.rateLimiter != nil {
		ip := httpx.ClientIP(r)
		if !s.rateLimiter.Allow(ip) {
			w.Header().Set("Retry-After", "1")
			s.sendJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"}, nil)
			return
		}
	}

	if shortUUID, ok := s.extractShortUUID(path); ok {
		s.handleSubscription(w, r, shortUUID)
		return
	}

	s.sendHTML(w, http.StatusNotFound, "<h1>Not found</h1>")
}

func (s *Server) requestLogPath(path string) string {
	normalized := path
	if candidate, _, ok := splitLegacyPathQuery(path); ok {
		normalized = candidate
	}
	if _, ok := s.extractShortUUID(normalized); !ok {
		return normalized
	}
	switch {
	case strings.HasPrefix(normalized, s.subPathPrefix):
		return s.subPathPrefix + ":short_uuid"
	case strings.HasPrefix(normalized, legacySubPathPrefix):
		return legacySubPathPrefix + ":short_uuid"
	default:
		return "/:short_uuid"
	}
}

func (s *Server) extractShortUUID(path string) (string, bool) {
	if strings.HasPrefix(path, s.subPathPrefix) {
		shortUUID := strings.Trim(strings.TrimPrefix(path, s.subPathPrefix), "/")
		return normalizeShortUUID(shortUUID), true
	}
	if strings.HasPrefix(path, legacySubPathPrefix) {
		shortUUID := strings.Trim(strings.TrimPrefix(path, legacySubPathPrefix), "/")
		return normalizeShortUUID(shortUUID), true
	}
	return "", false
}

func normalizeShortUUID(candidate string) string {
	candidate = strings.Trim(candidate, "/")
	if candidate == "" {
		return ""
	}
	parts := strings.Split(candidate, "/")
	if parts[0] != legacySubPathSegment {
		return candidate
	}
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts[1:], "/")
}

func splitLegacyPathQuery(path string) (string, string, bool) {
	if path == "" || !strings.Contains(path, "&") {
		return path, "", false
	}

	lastSlash := strings.LastIndex(path, "/")
	firstAmp := strings.Index(path, "&")
	if firstAmp == -1 || firstAmp <= lastSlash {
		return path, "", false
	}

	rawQuery := path[firstAmp+1:]
	if rawQuery == "" || !strings.Contains(rawQuery, "=") || strings.Contains(rawQuery, "/") {
		return path, "", false
	}
	if _, err := url.ParseQuery(rawQuery); err != nil {
		return path, "", false
	}

	return path[:firstAmp], rawQuery, true
}

func (s *Server) handlePanelError(w http.ResponseWriter, err error) {
	code, ok := panel.Code(err)
	if !ok {
		if s.logger != nil {
			s.logger.Error("unknown panel error", "error", err)
		}
		s.sendJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_error"}, nil)
		return
	}

	switch {
	case httpx.IsStatusCode(code, http.StatusNotFound):
		s.sendJSON(w, http.StatusNotFound, map[string]string{"error": "user_not_found"}, nil)
	case httpx.IsStatusCode(code, http.StatusUnauthorized):
		s.sendJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unauthorized"}, nil)
	case code == "panel_unreachable":
		s.sendJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unreachable"}, nil)
	default:
		if s.logger != nil {
			s.logger.Warn("panel error", "code", code)
		}
		s.sendJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_error"}, nil)
	}
}

func (s *Server) sendJSON(w http.ResponseWriter, status int, payload any, headers map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		if s.logger != nil {
			s.logger.Error("failed to encode JSON response", "error", err)
		}
	}
}

func (s *Server) sendHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (s *Server) sendText(w http.ResponseWriter, status int, contentType string, payload any, headers map[string]string) {
	if strings.TrimSpace(contentType) == "" {
		contentType = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(strings.TrimSpace(fmt.Sprint(payload)) + "\n"))
}

func setCommonSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
