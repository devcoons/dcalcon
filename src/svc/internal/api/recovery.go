package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/storage"
)

const recoveryTTL = 2 * time.Hour
const recoverMinDelay = 200 * time.Millisecond

func newResetToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plain = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return plain, hash, nil
}

func (h *Handler) recoveryURL(token string) string {
	return h.publicURL() + "/recover/" + token
}

func (h *Handler) issueRecovery(w http.ResponseWriter, r *http.Request, u *storage.User, reveal bool) {
	plain, hash, err := newResetToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token")
		return
	}
	if err := h.Store.ReplacePasswordReset(r.Context(), u.ID, hash, recoveryTTL); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	url := h.recoveryURL(plain)
	body := "A password reset was requested for your dCalCon account (" + u.Username + ").\n\n" +
		"Open this link within two hours:\n" + url + "\n\n" +
		"If you did not request this, you can ignore this message.\n"
	delivered := "logged"
	lastErr := ""
	if h.Mailer != nil && h.Mailer.Configured() {
		mctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		if err := h.Mailer.Send(mctx, u.Email, "Reset your dCalCon password", body); err != nil {
			delivered = "error"
			lastErr = err.Error()
		} else {
			delivered = "sent"
		}
		cancel()
	}
	_ = h.Store.InsertRecoveryOutbox(r.Context(), u.ID, u.Email, delivered, lastErr)
	action := "password.recover"
	if reveal {
		action = "password.recover.admin"
	}
	h.audit(r, action, u.Username)
	if !reveal {
		httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	resp := map[string]any{"ok": true, "emailed": delivered == "sent", "recovery_url": url, "delivered": delivered}
	if lastErr != "" {
		resp["mail_error"] = lastErr
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (h *Handler) recover(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	defer padRecover(started)
	key := "recover:" + httpx.ClientIP(r)
	if ok, retry := h.Limit.Hit(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		httpx.Error(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !validEmail(body.Email) {
		httpx.Error(w, http.StatusBadRequest, "invalid email")
		return
	}
	u, err := h.Store.UserByEmail(r.Context(), strings.TrimSpace(body.Email))
	if err == nil && u.Status == "active" {
		h.issueRecovery(w, r, u, false)
		return
	}
	_, _, _ = newResetToken()
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func padRecover(started time.Time) {
	if d := recoverMinDelay - time.Since(started); d > 0 {
		time.Sleep(d)
	}
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	key := "reset:" + httpx.ClientIP(r)
	if ok, retry := h.Limit.Allow(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		httpx.Error(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Token = strings.TrimSpace(body.Token)
	if body.Token == "" || !validPassword(body.Password) {
		httpx.Error(w, http.StatusBadRequest, "token and a password of at least 8 characters are required")
		return
	}
	sum := sha256.Sum256([]byte(body.Token))
	hash := hex.EncodeToString(sum[:])
	u, err := h.Store.ConsumePasswordReset(r.Context(), hash, body.Password)
	if err != nil {
		locked, retry := h.Limit.Fail(key)
		if locked {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			httpx.Error(w, http.StatusTooManyRequests, "too many attempts, try again later")
			return
		}
		httpx.Error(w, http.StatusBadRequest, "this reset link is invalid or has expired")
		return
	}
	h.Limit.Success(key)
	h.audit(r, "password.reset", u.Username)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) adminSendRecovery(w http.ResponseWriter, r *http.Request) {
	id, ok := requireID(w, r, "id")
	if !ok {
		return
	}
	u, err := h.Store.UserByID(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user")
		return
	}
	if u.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "user is disabled")
		return
	}
	h.issueRecovery(w, r, u, true)
}

func (h *Handler) adminRecoveryOutbox(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListRecoveryOutbox(r.Context(), 50)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []storage.RecoveryMessage{}
	}
	httpx.JSON(w, http.StatusOK, list)
}
