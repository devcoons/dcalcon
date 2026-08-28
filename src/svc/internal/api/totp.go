package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/metrics"
	"github.com/devcoons/dcalcon/internal/otp"
	"github.com/devcoons/dcalcon/internal/secret"
	"github.com/devcoons/dcalcon/internal/storage"
)

var errTOTPRequired = errors.New("authenticator code required")

func (h *Handler) totpIssuer() string {
	return "dCalCon"
}

func (h *Handler) totpSetup(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	_, _, enabled, err := h.totpFields(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if enabled {
		httpx.Error(w, http.StatusConflict, "authenticator is already enabled")
		return
	}
	pending, otpauth, err := otp.Generate(h.totpIssuer(), p.Username)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not generate authenticator secret")
		return
	}
	if err := h.Store.SetTOTPPending(r.Context(), p.ID, h.sealTOTP(pending)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{
		"secret":  pending,
		"otpauth": otpauth,
	})
}

func (h *Handler) totpEnable(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	_, pending, enabled, err := h.totpFields(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if enabled {
		httpx.Error(w, http.StatusConflict, "authenticator is already enabled")
		return
	}
	if pending == "" {
		httpx.Error(w, http.StatusBadRequest, "scan the QR code first")
		return
	}
	if !otp.Valid(body.Code, pending) {
		httpx.Error(w, http.StatusBadRequest, "that code is not valid yet — wait for a new code on your phone and try again")
		return
	}
	if err := h.Store.EnableTOTP(r.Context(), p.ID, h.sealTOTP(pending)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "totp.enable", "")
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "enabled", "totp_enabled": true})
}

func (h *Handler) totpDisable(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	secret, _, enabled, err := h.totpFields(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok := false
	if strings.TrimSpace(body.Password) != "" {
		if _, err := h.Store.Authenticate(r.Context(), p.Username, body.Password); err == nil {
			ok = true
		}
	}
	if !ok && enabled && otp.Valid(body.Code, secret) {
		ok = true
	}
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "password or authenticator code is required")
		return
	}
	if err := h.Store.DisableTOTP(r.Context(), p.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "totp.disable", "")
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "disabled", "totp_enabled": false})
}

func (h *Handler) totpCancel(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	_, _, enabled, err := h.totpFields(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if enabled {
		httpx.Error(w, http.StatusConflict, "authenticator is already enabled")
		return
	}
	if err := h.Store.SetTOTPPending(r.Context(), p.ID, ""); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *Handler) resetWithTOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Code     string `json:"code"`
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
	key := "reset-totp:" + httpx.ClientIP(r) + ":" + strings.ToLower(strings.TrimSpace(body.Username))
	if ok, retry := h.Limit.Allow(key); !ok {
		metrics.IncAuthLockout()
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		httpx.Error(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	fail := func() {
		locked, retry := h.Limit.Fail(key)
		metrics.IncAuthFail()
		if locked {
			metrics.IncAuthLockout()
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			httpx.Error(w, http.StatusTooManyRequests, "too many attempts, try again later")
			return
		}
		httpx.Error(w, http.StatusUnauthorized, "invalid username or authenticator code")
	}
	u, err := h.Store.UserByUsername(r.Context(), strings.TrimSpace(body.Username))
	if err != nil || u.Status != "active" {
		fail()
		return
	}
	secret, _, enabled, err := h.totpFields(r.Context(), u.ID)
	if err != nil || !enabled || !otp.Valid(body.Code, secret) {
		fail()
		return
	}
	if err := h.Store.SetPassword(r.Context(), u.ID, body.Password); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.Store.DeleteSessionsForUser(r.Context(), u.ID)
	h.Limit.Success(key)
	h.audit(r, "password.reset-totp", u.Username)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) authenticateLogin(r *http.Request, username, password, code string) (*storage.User, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	code = strings.TrimSpace(code)
	if username == "" || password == "" {
		return nil, storage.ErrUnauthorized
	}
	if storage.IsAppPasswordSecret(password) {
		return nil, storage.ErrUnauthorized
	}
	u, err := h.Store.Authenticate(r.Context(), username, password)
	if err != nil {
		return nil, storage.ErrUnauthorized
	}
	secret, _, enabled, err := h.totpFields(r.Context(), u.ID)
	if err != nil {
		return nil, storage.ErrUnauthorized
	}
	if enabled {
		if code == "" {
			return nil, errTOTPRequired
		}
		if !otp.Valid(code, secret) {
			return nil, storage.ErrUnauthorized
		}
	}
	return u, nil
}

func (h *Handler) adminDisableTOTP(w http.ResponseWriter, r *http.Request) {
	id, ok := requireID(w, r, "id")
	if !ok {
		return
	}
	u, err := h.Store.UserByID(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user")
		return
	}
	if err := h.Store.DisableTOTP(r.Context(), id); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "totp.disable.admin", u.Username)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

const totpSealPrefix = "s1:"

func (h *Handler) totpFields(ctx context.Context, userID int64) (secret, pending string, enabled bool, err error) {
	secret, pending, enabled, err = h.Store.TOTPState(ctx, userID)
	if err != nil {
		return "", "", false, err
	}
	return h.openTOTP(secret), h.openTOTP(pending), enabled, nil
}

func (h *Handler) sealTOTP(plain string) string {
	if plain == "" {
		return ""
	}
	key, err := secret.Key(h.Cfg.Auth.TokenKey)
	if err != nil {
		return plain
	}
	ct, nonce, err := secret.Seal(key, []byte(plain))
	if err != nil {
		return plain
	}
	return totpSealPrefix + hex.EncodeToString(nonce) + ":" + hex.EncodeToString(ct)
}

func (h *Handler) openTOTP(stored string) string {
	if stored == "" || !strings.HasPrefix(stored, totpSealPrefix) {
		return stored
	}
	nonceHex, ctHex, ok := strings.Cut(strings.TrimPrefix(stored, totpSealPrefix), ":")
	if !ok {
		return ""
	}
	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return ""
	}
	ct, err := hex.DecodeString(ctHex)
	if err != nil {
		return ""
	}
	key, err := secret.Key(h.Cfg.Auth.TokenKey)
	if err != nil {
		return ""
	}
	plain, err := secret.Open(key, ct, nonce)
	if err != nil {
		return ""
	}
	return string(plain)
}
