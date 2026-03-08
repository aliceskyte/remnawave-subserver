package handler

import (
	"net/http"
	"strings"

	"subserver/internal/config"
	"subserver/internal/panel"
	"subserver/internal/subscription"
)

func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request, shortUUID string) {
	if shortUUID == "" {
		s.sendJSON(w, http.StatusBadRequest, map[string]string{"error": "short_uuid_required"}, nil)
		return
	}
	if !panel.ValidShortUUID(shortUUID) {
		s.sendJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_short_uuid"}, nil)
		return
	}

	user, err := s.panel.UserByShortUUID(r.Context(), shortUUID)
	if err != nil {
		s.handlePanelError(w, err)
		return
	}

	rawSubscription, err := s.panel.SubscriptionRawByShortUUID(r.Context(), shortUUID)
	if err != nil {
		s.handlePanelError(w, err)
		return
	}

	core, ok := config.NormalizeCore(r.URL.Query().Get("core"))
	if !ok {
		rawCore := strings.TrimSpace(r.URL.Query().Get("core"))
		if rawCore != "" {
			s.sendJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_core"}, nil)
			return
		}
		core = config.CoreXray
	}

	result, err := s.builder.Build(r.Context(), user, core)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to build config", "error", err)
		}
		s.sendJSON(w, http.StatusInternalServerError, map[string]string{"error": "panel_error"}, nil)
		return
	}
	baseHeaders := subscription.BuildHeadersFromRaw(rawSubscription)
	headers := baseHeaders
	var overrides map[string]subscription.HeaderOverride
	if s.headerOverrides != nil {
		overrides = s.headerOverrides.HeaderOverridesForCoreAndSquads(core, user.SquadUUIDs)
		headers = subscription.ApplyHeaderOverrides(headers, overrides)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Cache-Control"] = "no-store"
	headers["Pragma"] = "no-cache"
	headers["Expires"] = "0"
	if strings.HasPrefix(result.ContentType, "text/yaml") || strings.HasPrefix(result.ContentType, "application/yaml") {
		s.sendText(w, http.StatusOK, result.ContentType, result.Content, headers)
		return
	}
	s.sendJSON(w, http.StatusOK, result.Content, headers)
}
