package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/devcoons/dcalcon/internal/userbackup"
)

func (h *Handler) exportTakeout(w http.ResponseWriter, r *http.Request) {
	h.writeUserBackup(w, r, userbackup.KindData)
}

func (h *Handler) exportBackupGET(w http.ResponseWriter, r *http.Request) {
	kind, err := userbackup.NormalizeKind(r.URL.Query().Get("kind"))
	if err != nil || kind == userbackup.KindFull {
		httpx.Error(w, http.StatusBadRequest, "use POST /api/v1/me/backup/export with your password for a full backup")
		return
	}
	h.writeUserBackup(w, r, kind)
}

func (h *Handler) exportBackupPOST(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Kind     string `json:"kind"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	kind, err := userbackup.NormalizeKind(body.Kind)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "kind must be data or full")
		return
	}
	if kind == userbackup.KindFull {
		if strings.TrimSpace(body.Password) == "" {
			httpx.Error(w, http.StatusUnauthorized, "current password required")
			return
		}
		if _, err := h.Store.Authenticate(r.Context(), p.Username, body.Password); err != nil {
			httpx.Error(w, http.StatusForbidden, "current password is incorrect")
			return
		}
	}
	h.writeUserBackup(w, r, kind)
}

func (h *Handler) writeUserBackup(w http.ResponseWriter, r *http.Request, kind string) {
	p := auth.MustPrincipal(r.Context())
	var buf bytes.Buffer
	if err := userbackup.Build(r.Context(), h.Store, p.ID, kind, &buf); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	name := fmt.Sprintf("dcalcon-%s-%s.zip", davpath.ZipSegment(p.Username), kind)
	w.Header().Set("Content-Type", "application/zip")
	httpx.DispositionAttachment(w, name)
	w.Header().Set("Cache-Control", "private, no-store")
	h.audit(r, "backup.export", "kind="+kind)
	_, _ = w.Write(buf.Bytes())
}

func (h *Handler) importBackup(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := r.ParseMultipartForm(limits.MaxBackupZip); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "backup too large")
			return
		}
		httpx.Error(w, http.StatusBadRequest, "expected a zip file")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limits.MaxBackupZip+1))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "could not read backup")
		return
	}
	if int64(len(data)) > limits.MaxBackupZip {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "backup too large")
		return
	}
	bundle, err := userbackup.Open(data)
	if err != nil {
		httpx.Error(w, backupStatus(err), backupMessage(err))
		return
	}
	if bundle.Manifest.Kind == userbackup.KindFull {
		password := strings.TrimSpace(r.FormValue("password"))
		if password == "" {
			httpx.Error(w, http.StatusUnauthorized, "current password required")
			return
		}
		if _, err := h.Store.Authenticate(r.Context(), p.Username, password); err != nil {
			httpx.Error(w, http.StatusForbidden, "current password is incorrect")
			return
		}
	}
	res, err := userbackup.Restore(r.Context(), h.Store, p.ID, h.publicURL(), bundle)
	if err != nil {
		httpx.Error(w, backupStatus(err), backupMessage(err))
		return
	}
	if bundle.Manifest.Kind == userbackup.KindFull {
		keep := ""
		if c, err := r.Cookie(auth.SessionCookie); err == nil {
			keep = c.Value
		}
		_ = h.Store.DeleteSessionsForUserExcept(r.Context(), p.ID, keep)
	}
	h.audit(r, "backup.import", "kind="+bundle.Manifest.Kind)
	httpx.JSON(w, http.StatusOK, res)
}

func backupStatus(err error) int {
	switch {
	case errors.Is(err, userbackup.ErrUsername):
		return http.StatusForbidden
	case errors.Is(err, userbackup.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, userbackup.ErrNotBackup), errors.Is(err, userbackup.ErrUnsafeZip),
		errors.Is(err, userbackup.ErrUnsupported), errors.Is(err, userbackup.ErrKind):
		return http.StatusBadRequest
	case errors.Is(err, storage.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func backupMessage(err error) string {
	switch {
	case errors.Is(err, userbackup.ErrUsername):
		return "this backup belongs to a different user"
	case errors.Is(err, userbackup.ErrUnsafeZip):
		return "backup contains an unsafe path"
	case errors.Is(err, userbackup.ErrTooLarge):
		return "backup is too large"
	case errors.Is(err, userbackup.ErrUnsupported):
		return "unsupported backup version"
	case errors.Is(err, userbackup.ErrKind):
		return "kind must be data or full"
	case errors.Is(err, userbackup.ErrNotBackup):
		return "not a dCalCon user backup"
	default:
		return err.Error()
	}
}
