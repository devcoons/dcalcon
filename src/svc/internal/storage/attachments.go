package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/google/uuid"
)

var (
	ErrAttachmentTooLarge = errors.New("file is too large")
	ErrTooManyAttachments = errors.New("too many attachments")
	ErrAttachmentEmpty    = errors.New("file is empty")
)

type Attachment struct {
	ID          int64  `json:"-"`
	PublicID    string `json:"id"`
	CalendarID  int64  `json:"calendar_id"`
	ObjectHref  string `json:"-"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"-"`
	Data        []byte `json:"-"`
	CreatedAt   string `json:"created_at,omitempty"`
}

func (a Attachment) Managed() icsutil.ManagedAttach {
	return icsutil.ManagedAttach{
		PublicID:    a.PublicID,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		Size:        a.Size,
	}
}

func (db *DB) ListAttachments(ctx context.Context, calendarID int64, href string) ([]Attachment, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, public_id, calendar_id, object_href, filename, content_type, size, sha256, created_at
		FROM calendar_attachments
		WHERE calendar_id = ? AND object_href = ?
		ORDER BY id`, calendarID, href)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttachmentRows(rows)
}

func (db *DB) ListAttachmentsByCalendar(ctx context.Context, calendarID int64) (map[string][]Attachment, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, public_id, calendar_id, object_href, filename, content_type, size, sha256, created_at
		FROM calendar_attachments
		WHERE calendar_id = ?
		ORDER BY id`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanAttachmentRows(rows)
	if err != nil {
		return nil, err
	}
	out := map[string][]Attachment{}
	for _, a := range list {
		out[a.ObjectHref] = append(out[a.ObjectHref], a)
	}
	return out, nil
}

func (db *DB) AttachmentByPublicID(ctx context.Context, publicID string) (*Attachment, error) {
	if !ValidAttachmentPublicID(publicID) {
		return nil, ErrNotFound
	}
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, public_id, calendar_id, object_href, filename, content_type, size, sha256, data, created_at
		FROM calendar_attachments WHERE public_id = ?`, publicID)
	a, err := scanAttachment(row.Scan, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (db *DB) InsertAttachment(ctx context.Context, calendarID int64, href, filename, contentType string, data []byte) (*Attachment, error) {
	return db.insertAttachment(ctx, calendarID, href, "", filename, contentType, data)
}

func (db *DB) RestoreAttachment(ctx context.Context, calendarID int64, href, publicID, filename, contentType string, data []byte) (*Attachment, error) {
	return db.insertAttachment(ctx, calendarID, href, publicID, filename, contentType, data)
}

func (db *DB) insertAttachment(ctx context.Context, calendarID int64, href, publicID, filename, contentType string, data []byte) (*Attachment, error) {
	if len(data) == 0 {
		return nil, ErrAttachmentEmpty
	}
	if int64(len(data)) > limits.MaxAttachmentBytes {
		return nil, ErrAttachmentTooLarge
	}
	n, total, err := db.attachmentUsage(ctx, calendarID, href)
	if err != nil {
		return nil, err
	}
	if n >= limits.MaxAttachmentsPerObject {
		return nil, ErrTooManyAttachments
	}
	if total+int64(len(data)) > limits.MaxAttachmentsBytesPerObject {
		return nil, ErrAttachmentTooLarge
	}
	filename = SanitizeFilename(filename)
	contentType = SanitizeContentType(contentType, filename)
	if LooksActive(data) {
		contentType = "application/octet-stream"
	}
	id := strings.TrimSpace(publicID)
	if !ValidAttachmentPublicID(id) || db.attachmentIDTaken(ctx, id) {
		id = uuid.NewString()
	}
	sum := sha256.Sum256(data)
	a := &Attachment{
		PublicID:    id,
		CalendarID:  calendarID,
		ObjectHref:  href,
		Filename:    filename,
		ContentType: contentType,
		Size:        int64(len(data)),
		SHA256:      hex.EncodeToString(sum[:]),
		Data:        data,
		CreatedAt:   nowUTC(),
	}
	res, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO calendar_attachments
			(public_id, calendar_id, object_href, filename, content_type, size, sha256, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.PublicID, a.CalendarID, a.ObjectHref, a.Filename, a.ContentType, a.Size, a.SHA256, a.Data, a.CreatedAt)
	if err != nil {
		return nil, err
	}
	a.ID, _ = res.LastInsertId()
	a.Data = nil
	return a, nil
}

func (db *DB) DeleteAttachment(ctx context.Context, calendarID int64, href, publicID string) error {
	res, err := db.conn(ctx).ExecContext(ctx, `
		DELETE FROM calendar_attachments
		WHERE calendar_id = ? AND object_href = ? AND public_id = ?`, calendarID, href, publicID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) PutICSWithAttachments(ctx context.Context, calendarID int64, href, uid, component, ics, dtstart, dtend, summary, publicURL string) error {
	if err := davpath.CheckObjectHref(href); err != nil {
		return err
	}
	split, err := icsutil.SplitAttachments(ics, calendarID)
	if err != nil {
		return err
	}
	if len(split.Inlines) == 0 && len(split.ManagedIDs) == 0 && len(split.External) == 0 {
		n, _, err := db.attachmentUsage(ctx, calendarID, href)
		if err != nil {
			return err
		}
		if n == 0 {
			return db.UpsertCalendarObject(ctx, calendarID, href, uid, icsutil.ETag(ics), component, ics, dtstart, dtend, summary)
		}
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		if err := db.UpsertCalendarObject(ctx, calendarID, href, uid, icsutil.ETag(split.ICS), component, split.ICS, dtstart, dtend, summary); err != nil {
			return err
		}
		final, err := db.ApplyICSAttachments(ctx, calendarID, href, publicURL, split)
		if err != nil {
			return err
		}
		if final == split.ICS {
			return nil
		}
		return db.UpsertCalendarObject(ctx, calendarID, href, uid, icsutil.ETag(final), component, final, dtstart, dtend, summary)
	})
}

func (db *DB) RewriteManagedAttachments(ctx context.Context, calendarID int64, href, publicURL, ics string) (string, error) {
	split, err := icsutil.SplitAttachments(ics, calendarID)
	if err != nil {
		return "", err
	}
	return db.writeManaged(ctx, calendarID, href, publicURL, split)
}

func (db *DB) ApplyICSAttachments(ctx context.Context, calendarID int64, href, publicURL string, split icsutil.AttachSplit) (string, error) {
	existing, err := db.ListAttachments(ctx, calendarID, href)
	if err != nil {
		return "", err
	}
	keep := map[string]bool{}
	for _, id := range split.ManagedIDs {
		keep[id] = true
	}
	for _, a := range existing {
		if keep[a.PublicID] {
			continue
		}
		if err := db.DeleteAttachment(ctx, calendarID, href, a.PublicID); err != nil && !errors.Is(err, ErrNotFound) {
			return "", err
		}
	}
	n, total, err := db.attachmentUsage(ctx, calendarID, href)
	if err != nil {
		return "", err
	}
	if err := CheckInlineAttachmentLimits(n, total, split.Inlines); err != nil {
		return "", err
	}
	for _, in := range split.Inlines {
		if _, err := db.InsertAttachment(ctx, calendarID, href, in.Filename, in.ContentType, in.Data); err != nil {
			return "", err
		}
	}
	return db.writeManaged(ctx, calendarID, href, publicURL, split)
}

func (db *DB) writeManaged(ctx context.Context, calendarID int64, href, publicURL string, split icsutil.AttachSplit) (string, error) {
	atts, err := db.ListAttachments(ctx, calendarID, href)
	if err != nil {
		return "", err
	}
	managed := make([]icsutil.ManagedAttach, 0, len(atts))
	for _, a := range atts {
		managed = append(managed, a.Managed())
	}
	return icsutil.WriteAttachments(split, managed, publicURL, calendarID)
}

func (db *DB) attachmentUsage(ctx context.Context, calendarID int64, href string) (count int, total int64, err error) {
	err = db.conn(ctx).QueryRowContext(ctx, `
		SELECT COUNT(1), COALESCE(SUM(size), 0)
		FROM calendar_attachments WHERE calendar_id = ? AND object_href = ?`, calendarID, href).Scan(&count, &total)
	return
}

func scanAttachmentRows(rows *sql.Rows) ([]Attachment, error) {
	var out []Attachment
	for rows.Next() {
		a, err := scanAttachment(rows.Scan, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanAttachment(scan func(dest ...any) error, withData bool) (*Attachment, error) {
	a := &Attachment{}
	var err error
	if withData {
		err = scan(&a.ID, &a.PublicID, &a.CalendarID, &a.ObjectHref, &a.Filename, &a.ContentType, &a.Size, &a.SHA256, &a.Data, &a.CreatedAt)
	} else {
		err = scan(&a.ID, &a.PublicID, &a.CalendarID, &a.ObjectHref, &a.Filename, &a.ContentType, &a.Size, &a.SHA256, &a.CreatedAt)
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func SanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == 127 || r == '/' || unicode.Is(unicode.C, r) {
			continue
		}
		b.WriteRune(r)
	}
	name = strings.Trim(b.String(), " .")
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	if utf8.RuneCountInString(name) > 180 {
		runes := []rune(name)
		ext := filepath.Ext(name)
		keep := 180 - utf8.RuneCountInString(ext)
		if keep < 8 {
			keep = 8
		}
		name = string(runes[:keep]) + ext
	}
	return name
}

func SanitizeContentType(ct, filename string) string {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	if ct == "" {
		ct = mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
		ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	}
	switch ct {
	case "text/html", "image/svg+xml", "application/xhtml+xml", "text/xml", "application/xml", "text/javascript", "application/javascript", "application/x-javascript", "text/ecmascript":
		return "application/octet-stream"
	}
	if ct == "" || strings.ContainsAny(ct, " \t\r\n") {
		return "application/octet-stream"
	}
	return ct
}

func ValidAttachmentPublicID(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

func LooksActive(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	s := data
	if len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF {
		s = s[3:]
	}
	if len(s) >= 2 && ((s[0] == 0xFF && s[1] == 0xFE) || (s[0] == 0xFE && s[1] == 0xFF)) {
		return true
	}
	n := 512
	if len(s) < n {
		n = len(s)
	}
	low := bytes.ToLower(bytes.TrimSpace(s[:n]))
	for _, p := range [][]byte{
		[]byte("<html"), []byte("<!doctype"), []byte("<script"),
		[]byte("<?xml"), []byte("<svg"), []byte("<iframe"),
		[]byte("<?php"), []byte("<img"), []byte("<body"),
	} {
		if bytes.HasPrefix(low, p) {
			return true
		}
	}
	if bytes.Contains(low, []byte("<script")) {
		return true
	}
	return false
}

func magicContentType(data []byte) string {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "image/png"
	}
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return "image/gif"
	}
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return "application/pdf"
	}
	return ""
}

func DownloadContentType(_ string, data []byte) string {
	if LooksActive(data) {
		return "application/octet-stream"
	}
	if mag := magicContentType(data); mag != "" {
		return mag
	}
	return "application/octet-stream"
}

func CheckInlineAttachmentLimits(existingCount int, existingBytes int64, inlines []icsutil.InlineAttach) error {
	if existingCount+len(inlines) > limits.MaxAttachmentsPerObject {
		return ErrTooManyAttachments
	}
	total := existingBytes
	for _, in := range inlines {
		if len(in.Data) == 0 {
			return ErrAttachmentEmpty
		}
		if int64(len(in.Data)) > limits.MaxAttachmentBytes {
			return ErrAttachmentTooLarge
		}
		total += int64(len(in.Data))
		if total > limits.MaxAttachmentsBytesPerObject {
			return ErrAttachmentTooLarge
		}
	}
	return nil
}

func AttachLimitStatus(err error) int {
	switch {
	case errors.Is(err, ErrAttachmentTooLarge), errors.Is(err, ErrTooManyAttachments):
		return 413
	case errors.Is(err, ErrAttachmentEmpty):
		return 400
	default:
		return 0
	}
}

func AttachLimitMessage(err error) string {
	switch {
	case errors.Is(err, ErrAttachmentTooLarge):
		return fmt.Sprintf("each file must be at most %d MiB, and files on one item at most %d MiB", limits.MaxAttachmentBytes>>20, limits.MaxAttachmentsBytesPerObject>>20)
	case errors.Is(err, ErrTooManyAttachments):
		return fmt.Sprintf("at most %d files per event or task", limits.MaxAttachmentsPerObject)
	case errors.Is(err, ErrAttachmentEmpty):
		return "file is empty"
	default:
		return err.Error()
	}
}
