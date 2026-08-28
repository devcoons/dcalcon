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

func (h *Handler) listAppPasswords(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	list, err := h.Store.ListAppPasswords(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) createAppPassword(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	pw, secret, err := h.Store.CreateAppPassword(r.Context(), p.ID, strings.TrimSpace(body.Name))
	if errors.Is(err, storage.ErrConflict) {
		httpx.Error(w, http.StatusConflict, "too many app passwords — revoke one first")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":         pw.ID,
		"name":       pw.Name,
		"prefix":     pw.Prefix,
		"password":   secret,
		"created_at": pw.CreatedAt,
	})
}

func (h *Handler) deleteAppPassword(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := requireID(w, r, "id")
	if !ok {
		return
	}
	if err := h.Store.DeleteAppPassword(r.Context(), p.ID, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
