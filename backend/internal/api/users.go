// User administration.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

const maxDisplayName = 100

var errLastAdmin = map[string]string{"error": "at least one admin must remain"}

func checkDisplayName(name string) error {
	if utf8.RuneCountInString(name) > maxDisplayName {
		return fmt.Errorf("displayName must be at most %d characters", maxDisplayName)
	}
	return nil
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.pg.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]model.User{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	username, err := auth.NormaliseUsername(req.Username)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	role, err := auth.ParseRole(req.Role)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	if err := checkDisplayName(req.DisplayName); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if errors.Is(err, auth.ErrPasswordPolicy) {
		s.badRequest(w, err.Error())
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.pg.CreatePasswordUser(r.Context(), username, req.DisplayName, string(role), hash)
	if errors.Is(err, postgres.ErrConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role        *string `json:"role"`
		DisplayName *string `json:"displayName"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	if req.Role == nil && req.DisplayName == nil {
		s.badRequest(w, "nothing to change: send role and/or displayName")
		return
	}
	if req.Role != nil {
		if _, err := auth.ParseRole(*req.Role); err != nil {
			s.badRequest(w, err.Error())
			return
		}
	}
	if req.DisplayName != nil {
		if err := checkDisplayName(*req.DisplayName); err != nil {
			s.badRequest(w, err.Error())
			return
		}
	}
	u, err := s.pg.UpdateUser(r.Context(), chi.URLParam(r, "id"), req.Role, req.DisplayName)
	if errors.Is(err, postgres.ErrLastAdmin) {
		writeJSON(w, http.StatusConflict, errLastAdmin)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if errors.Is(err, auth.ErrPasswordPolicy) {
		s.badRequest(w, err.Error())
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.pg.SetPasswordHash(r.Context(), id, hash); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.pg.DeleteUserSessions(r.Context(), id, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if p, _ := auth.PrincipalFrom(r.Context()); p.UserID == id {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "you cannot delete your own account"})
		return
	}
	err := s.pg.DeleteUser(r.Context(), id)
	if errors.Is(err, postgres.ErrLastAdmin) {
		writeJSON(w, http.StatusConflict, errLastAdmin)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
