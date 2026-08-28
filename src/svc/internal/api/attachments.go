package api

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) publicURL() string {
	return strings.TrimRight(h.Cfg.HTTP.PublicURL, "/")
}

func (h *Handler) attachmentsFor(ctx context.Context, calendarID int64, href string) []storage.Attachment {
	list, err := h.Store.ListAttachments(ctx, calendarID, href)
	if err != nil || list == nil {
		return []storage.Attachment{}
	}
	return list
}

func (h *Handler) attachmentMap(ctx context.Context, calendarID int64) map[string][]storage.Attachment {
	m, err := h.Store.ListAttachmentsByCalendar(ctx, calendarID)
	if err != nil || m == nil {
		return map[string][]storage.Attachment{}
	}
	return m
}

func (h *Handler) saveObjectICS(ctx context.Context, c *storage.Calendar, o *storage.CalendarObject, ics string) error {
	return h.Store.UpsertCalendarObject(ctx, c.ID, o.Href, o.UID, icsutil.ETag(ics), o.Component, ics, o.DTStart, o.DTEnd, o.Summary)
}

func (h *Handler) loadAttachItem(w http.ResponseWriter, r *http.Request, wantTodo bool) (*storage.Calendar, *storage.CalendarObject, bool) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return nil, nil, false
	}
	kind := "event"
	if wantTodo {
		kind = "task"
	}
	href, ok := objectHref(w, r, kind)
	if !ok {
		return nil, nil, false
	}
	o, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil || isTodo(*o) != wantTodo {
		if wantTodo {
			httpx.Error(w, http.StatusNotFound, "task")
		} else {
			httpx.Error(w, http.StatusNotFound, "event")
		}
		return nil, nil, false
	}
	return c, o, true
}

func (h *Handler) listEventAttachments(w http.ResponseWriter, r *http.Request) {
	h.listItemAttachments(w, r, false)
}

func (h *Handler) listTaskAttachments(w http.ResponseWriter, r *http.Request) {
	h.listItemAttachments(w, r, true)
}

func (h *Handler) listItemAttachments(w http.ResponseWriter, r *http.Request, wantTodo bool) {
	c, o, ok := h.loadAttachItem(w, r, wantTodo)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, h.attachmentsFor(r.Context(), c.ID, o.Href))
}

func (h *Handler) uploadEventAttachments(w http.ResponseWriter, r *http.Request) {
	h.uploadItemAttachments(w, r, false)
}

func (h *Handler) uploadTaskAttachments(w http.ResponseWriter, r *http.Request) {
	h.uploadItemAttachments(w, r, true)
}

func (h *Handler) uploadItemAttachments(w http.ResponseWriter, r *http.Request, wantTodo bool) {
	c, o, ok := h.loadAttachItem(w, r, wantTodo)
	if !ok {
		return
	}
	if writeDenied(w, c) {
		return
	}
	if err := r.ParseMultipartForm(limits.MaxAttachmentBytes + 1<<20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, storage.AttachLimitMessage(storage.ErrAttachmentTooLarge))
			return
		}
		httpx.Error(w, http.StatusBadRequest, "expected multipart form with file")
		return
	}
	var files []*multipart.FileHeader
	if r.MultipartForm != nil {
		files = append(files, r.MultipartForm.File["file"]...)
		files = append(files, r.MultipartForm.File["files"]...)
	}
	if len(files) == 0 {
		httpx.Error(w, http.StatusBadRequest, "choose a file")
		return
	}
	type pending struct {
		name, ctype string
		data        []byte
	}
	items := make([]pending, 0, len(files))
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "could not read file")
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, limits.MaxAttachmentBytes+1))
		_ = f.Close()
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "could not read file")
			return
		}
		if int64(len(data)) > limits.MaxAttachmentBytes {
			httpx.Error(w, http.StatusRequestEntityTooLarge, storage.AttachLimitMessage(storage.ErrAttachmentTooLarge))
			return
		}
		items = append(items, pending{name: fh.Filename, ctype: fh.Header.Get("Content-Type"), data: data})
	}
	added := make([]storage.Attachment, 0, len(items))
	err := h.Store.WithTx(r.Context(), func(ctx context.Context) error {
		for _, it := range items {
			a, err := h.Store.InsertAttachment(ctx, c.ID, o.Href, it.name, it.ctype, it.data)
			if err != nil {
				return err
			}
			added = append(added, *a)
		}
		fresh, err := h.Store.CalendarObjectByHref(ctx, c.ID, o.Href)
		if err != nil {
			return err
		}
		ics, err := h.Store.RewriteManagedAttachments(ctx, c.ID, o.Href, h.publicURL(), fresh.ICS)
		if err != nil {
			return err
		}
		return h.saveObjectICS(ctx, c, fresh, ics)
	})
	if err != nil {
		if st := storage.AttachLimitStatus(err); st != 0 {
			httpx.Error(w, st, storage.AttachLimitMessage(err))
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "could not save file")
		return
	}
	httpx.JSON(w, http.StatusCreated, added)
}

func (h *Handler) deleteEventAttachment(w http.ResponseWriter, r *http.Request) {
	h.deleteItemAttachment(w, r, false)
}

func (h *Handler) deleteTaskAttachment(w http.ResponseWriter, r *http.Request) {
	h.deleteItemAttachment(w, r, true)
}

func (h *Handler) deleteItemAttachment(w http.ResponseWriter, r *http.Request, wantTodo bool) {
	c, o, ok := h.loadAttachItem(w, r, wantTodo)
	if !ok {
		return
	}
	if writeDenied(w, c) {
		return
	}
	id := chi.URLParam(r, "attId")
	if !storage.ValidAttachmentPublicID(id) {
		httpx.Error(w, http.StatusNotFound, "attachment")
		return
	}
	if err := h.Store.WithTx(r.Context(), func(ctx context.Context) error {
		if err := h.Store.DeleteAttachment(ctx, c.ID, o.Href, id); err != nil {
			return err
		}
		fresh, err := h.Store.CalendarObjectByHref(ctx, c.ID, o.Href)
		if err != nil {
			return err
		}
		ics, err := h.Store.RewriteManagedAttachments(ctx, c.ID, o.Href, h.publicURL(), fresh.ICS)
		if err != nil {
			return err
		}
		return h.saveObjectICS(ctx, c, fresh, ics)
	}); err != nil {
		httpx.Error(w, http.StatusNotFound, "attachment")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return
	}
	attID := chi.URLParam(r, "attId")
	if !storage.ValidAttachmentPublicID(attID) {
		httpx.Error(w, http.StatusNotFound, "attachment")
		return
	}
	att, err := h.Store.AttachmentByPublicID(r.Context(), attID)
	if err != nil || att.CalendarID != c.ID {
		httpx.Error(w, http.StatusNotFound, "attachment")
		return
	}
	httpx.WriteDownload(w, att.Filename, att.ContentType, att.Data)
}
