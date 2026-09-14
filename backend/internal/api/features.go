package api

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"urara-vision/backend/internal/store/postgres"
)

type chatFeature struct {
	Available bool `json:"available"` // deployed: CHAT_ENABLED
	Enabled   bool `json:"enabled"`   // available and switched on
}

type featuresResponse struct {
	Chat chatFeature `json:"chat"`
}

type patchSettingsRequest struct {
	ChatEnabled *bool `json:"chatEnabled"`
}

// handleFeatures reports which optional features are on.
func (s *Server) handleFeatures(w http.ResponseWriter, r *http.Request) {
	chat := chatFeature{Available: s.cfg.ChatEnabled}
	// Not deployed means off, whatever the row says, so the store is not read.
	if chat.Available {
		on, err := s.pg.GetBoolSetting(r.Context(), postgres.SettingChatEnabled, true)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		chat.Enabled = on
	}
	writeJSON(w, http.StatusOK, featuresResponse{Chat: chat})
}

// handlePatchSettings changes the runtime chat switch.
func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	var req patchSettingsRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	if req.ChatEnabled == nil {
		s.badRequest(w, "\"chatEnabled\" is required")
		return
	}
	if *req.ChatEnabled && !s.cfg.ChatEnabled {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "chat is not deployed"})
		return
	}
	if err := s.pg.SetBoolSetting(r.Context(), postgres.SettingChatEnabled, *req.ChatEnabled); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("setting changed",
		"key", postgres.SettingChatEnabled,
		"value", *req.ChatEnabled,
		"request_id", middleware.GetReqID(r.Context()))
	s.handleFeatures(w, r)
}
