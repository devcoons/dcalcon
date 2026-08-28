package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/google/uuid"
)

func (h *Handler) bookFor(w http.ResponseWriter, r *http.Request) (*storage.AddressBook, bool) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusNotFound, "addressbook")
		return nil, false
	}
	book, err := h.Store.AddressBookByID(r.Context(), p.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "addressbook")
		return nil, false
	}
	if book.Slug == schedule.PeopleBookSlug {
		if err := schedule.RefreshPeopleBook(r.Context(), h.Store, p.ID); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return nil, false
		}
		if refreshed, err := h.Store.AddressBookByID(r.Context(), p.ID, id); err == nil {
			book = refreshed
		}
	}
	return book, true
}

func bookWritable(w http.ResponseWriter, book *storage.AddressBook) bool {
	if book.ReadOnly || book.Slug == schedule.PeopleBookSlug {
		httpx.Error(w, http.StatusForbidden, "this address book is read-only")
		return false
	}
	return true
}

func (h *Handler) listAddressBooks(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := h.Store.EnsurePeopleBook(r.Context(), p.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := h.Store.ListAddressBooks(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []storage.AddressBook{}
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	list, err := h.Store.ListAddressObjects(r.Context(), book.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]contactDTO, 0, len(list))
	for _, o := range list {
		out = append(out, toContactDTO(o))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) createContact(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	if !bookWritable(w, book) {
		return
	}
	body, ok := decodeContact(w, r)
	if !ok {
		return
	}
	uid := uuid.NewString()
	raw, err := icsutil.EncodeContact(uid, body, "")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "could not encode contact")
		return
	}
	if err := rejectVCard(w, raw); err != nil {
		return
	}
	href := icsutil.VCardHref(uid)
	if err := h.putCard(r.Context(), book.ID, href, uid, raw, body.FN, body.BDAY, body.Anniversary); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"href": href, "uid": uid})
}

func (h *Handler) getContact(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "contact")
	if !ok {
		return
	}
	o, err := h.Store.AddressObjectByHref(r.Context(), book.ID, href)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "contact")
		return
	}
	httpx.JSON(w, http.StatusOK, toContactDTO(*o))
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "contact")
	if !ok {
		return
	}
	existing, err := h.Store.AddressObjectByHref(r.Context(), book.ID, href)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "contact")
		return
	}
	if !bookWritable(w, book) {
		return
	}
	body, ok := decodeContact(w, r)
	if !ok {
		return
	}
	raw, err := icsutil.EncodeContact(existing.UID, body, existing.VCard)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "could not update contact")
		return
	}
	if err := rejectVCard(w, raw); err != nil {
		return
	}
	if err := h.putCard(r.Context(), book.ID, href, existing.UID, raw, body.FN, body.BDAY, body.Anniversary); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	o, err := h.Store.AddressObjectByHref(r.Context(), book.ID, href)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, toContactDTO(*o))
}

func (h *Handler) deleteContact(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	if !bookWritable(w, book) {
		return
	}
	href, ok := objectHref(w, r, "contact")
	if !ok {
		return
	}
	if err := h.Store.DeleteAddressObject(r.Context(), book.ID, href); err != nil {
		httpx.Error(w, http.StatusNotFound, "contact")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) exportContacts(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	list, err := h.Store.ListAddressObjects(r.Context(), book.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	raws := make([]string, 0, len(list))
	for _, o := range list {
		raws = append(raws, o.VCard)
	}
	name := "contacts.vcf"
	if book.Slug != "" {
		name = icsutil.VCardHref(book.Slug)
	}
	writeVCardFile(w, name, icsutil.JoinCards(raws))
}

func (h *Handler) exportContact(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "contact")
	if !ok {
		return
	}
	o, err := h.Store.AddressObjectByHref(r.Context(), book.ID, href)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "contact")
		return
	}
	writeVCardFile(w, icsutil.VCardFileName(o.FN, o.UID), o.VCard)
}

func (h *Handler) importContacts(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bookFor(w, r)
	if !ok {
		return
	}
	if !bookWritable(w, book) {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "body too large")
			return
		}
		httpx.Error(w, http.StatusBadRequest, "could not read body")
		return
	}
	blocks, err := icsutil.ImportCardBlocks(raw)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(blocks) == 0 {
		httpx.Error(w, http.StatusBadRequest, "no vCard found")
		return
	}
	if len(blocks) > limits.MaxImportCards {
		httpx.Error(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("at most %d contacts per import", limits.MaxImportCards))
		return
	}

	created, updated, skipped := 0, 0, 0
	errs := make([]string, 0)
	note := func(i int, err error) {
		skipped++
		if len(errs) < 20 {
			errs = append(errs, fmt.Sprintf("card %d: %s", i+1, err.Error()))
		}
	}
	err = h.Store.WithTx(r.Context(), func(ctx context.Context) error {
		for i, block := range blocks {
			card, err := icsutil.PrepareImportedCard(block, uuid.NewString())
			if err != nil {
				note(i, err)
				continue
			}
			href, isNew, err := h.hrefForImport(ctx, book.ID, card.UID)
			if err != nil {
				note(i, err)
				continue
			}
			if err := h.putCard(ctx, book.ID, href, card.UID, card.Raw, card.FN, card.BDAY, card.Anniversary); err != nil {
				note(i, err)
				continue
			}
			if isNew {
				created++
			} else {
				updated++
			}
		}
		return nil
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not import contacts")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"errors":  errs,
	})
}

func (h *Handler) hrefForImport(ctx context.Context, bookID int64, uid string) (string, bool, error) {
	existing, err := h.Store.AddressObjectByUID(ctx, bookID, uid)
	if err == nil {
		return existing.Href, false, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return "", false, err
	}
	href := icsutil.VCardHref(uid)
	conflict, err := h.Store.AddressObjectByHref(ctx, bookID, href)
	if err == nil && conflict.UID != uid {
		href = icsutil.VCardHref(uuid.NewString())
	} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return "", false, err
	}
	return href, true, nil
}

func (h *Handler) putCard(ctx context.Context, bookID int64, href, uid, raw, fn, bday, ann string) error {
	return h.Store.UpsertAddressObject(ctx, bookID, href, uid, icsutil.ETag(raw), raw, fn, bday, ann)
}

func decodeContact(w http.ResponseWriter, r *http.Request) (icsutil.ContactInput, bool) {
	var body icsutil.ContactInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return body, false
	}
	body.Normalize()
	if !body.NameOK() {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return body, false
	}
	return body, true
}

func rejectVCard(w http.ResponseWriter, raw string) error {
	card, err := icsutil.ParseCard(raw)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid vcard")
		return err
	}
	if err := icsutil.CheckVCardSize(raw, card); err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, err.Error())
		return err
	}
	return nil
}

func writeVCardFile(w http.ResponseWriter, filename, body string) {
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	httpx.DispositionAttachment(w, filename)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}
