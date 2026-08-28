package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/storage"
)

type createdUserDTO struct {
	User  *storage.User `json:"user"`
	Setup setupDTO      `json:"setup"`
}

func (h *Handler) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Timezone    string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	body.Email = strings.TrimSpace(body.Email)
	body.DisplayName = strings.TrimSpace(body.DisplayName)
	if body.Role == "" {
		body.Role = "user"
	}
	if !validUsername(body.Username) {
		httpx.Error(w, http.StatusBadRequest, "username must be 2–32 characters (letters, numbers, . _ -)")
		return
	}
	if !validEmail(body.Email) {
		httpx.Error(w, http.StatusBadRequest, "invalid email")
		return
	}
	if !validPassword(body.Password) {
		httpx.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if !validRole(body.Role) {
		httpx.Error(w, http.StatusBadRequest, "role must be admin or user")
		return
	}
	u, err := h.Store.CreateUser(r.Context(), body.Username, body.Email, body.Password, body.DisplayName, body.Role, normalizeTimezone(body.Timezone))
	if errors.Is(err, storage.ErrConflict) {
		httpx.Error(w, http.StatusConflict, "username or email already exists")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "user.create", u.Username)
	httpx.JSON(w, http.StatusCreated, createdUserDTO{User: u, Setup: h.setupFor(u.Username)})
}

func (h *Handler) adminPatchUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.MustPrincipal(r.Context())
	id, ok := requireID(w, r, "id")
	if !ok {
		return
	}
	existing, err := h.Store.UserByID(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user")
		return
	}
	var body struct {
		Email       *string `json:"email"`
		DisplayName *string `json:"display_name"`
		Role        *string `json:"role"`
		Status      *string `json:"status"`
		Timezone    *string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	email, display, role, status, tz := existing.Email, existing.DisplayName, existing.Role, existing.Status, existing.Timezone
	if body.Email != nil {
		email = strings.TrimSpace(*body.Email)
		if !validEmail(email) {
			httpx.Error(w, http.StatusBadRequest, "invalid email")
			return
		}
	}
	if body.DisplayName != nil {
		display = strings.TrimSpace(*body.DisplayName)
		if display == "" {
			display = existing.Username
		}
	}
	if body.Role != nil {
		role = *body.Role
		if !validRole(role) {
			httpx.Error(w, http.StatusBadRequest, "role must be admin or user")
			return
		}
	}
	if body.Status != nil {
		status = *body.Status
		if !validStatus(status) {
			httpx.Error(w, http.StatusBadRequest, "status must be active or disabled")
			return
		}
	}
	if body.Timezone != nil {
		tz = normalizeTimezone(*body.Timezone)
	}

	if actor.ID == id && status == "disabled" {
		httpx.Error(w, http.StatusBadRequest, "you cannot disable your own account")
		return
	}
	if existing.Role == "admin" && existing.Status == "active" && (role != "admin" || status != "active") {
		n, err := h.Store.ActiveAdminCount(r.Context())
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n <= 1 {
			httpx.Error(w, http.StatusBadRequest, "cannot remove the last active administrator")
			return
		}
	}

	if err := h.Store.AdminUpdateUser(r.Context(), id, email, display, role, status, tz); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			httpx.Error(w, http.StatusConflict, "email already in use")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status == "disabled" {
		_ = h.Store.DeleteSessionsForUser(r.Context(), id)
	}
	u, err := h.Store.UserByID(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) adminResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := requireID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.Store.UserByID(r.Context(), id); err != nil {
		httpx.Error(w, http.StatusNotFound, "user")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !validPassword(body.Password) {
		httpx.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if err := h.Store.SetPassword(r.Context(), id, body.Password); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.Store.DeleteSessionsForUser(r.Context(), id)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
