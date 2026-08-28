package storage_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devcoons/dcalcon/internal/storage"
)

func TestOpenAndProvision(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Authenticate(ctx, "alice", "secret-pass"); err != nil {
		t.Fatal(err)
	}
	cals, err := db.ListCalendars(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cals) < 3 {
		t.Fatal("expected personal, inbox, and outbox calendars")
	}
	var kinds []string
	for _, c := range cals {
		kinds = append(kinds, c.Kind)
	}
	joined := strings.Join(kinds, ",")
	if !strings.Contains(joined, "personal") || !strings.Contains(joined, "inbox") || !strings.Contains(joined, "outbox") {
		t.Fatalf("kinds %v", kinds)
	}
	books, err := db.ListAddressBooks(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) == 0 {
		t.Fatal("expected default address book")
	}
	for _, c := range cals {
		switch c.Kind {
		case "personal":
			if !c.CanWrite() {
				t.Fatalf("personal should be writable: %+v", c)
			}
		case "inbox", "outbox":
			if c.CanWrite() {
				t.Fatalf("%s should not be writable via REST", c.Kind)
			}
		}
	}
}

func TestUpsertReplacesDuplicateUID(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(ctx, u.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCalendarObject(ctx, cal.ID, "a.ics", "same-uid", "etag1", "VEVENT", "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n", "20260828", "20260829", "A"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCalendarObject(ctx, cal.ID, "b.ics", "same-uid", "etag2", "VEVENT", "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n", "20260828", "20260829", "B"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CalendarObjectByHref(ctx, cal.ID, "a.ics"); err == nil {
		t.Fatal("old href should be replaced")
	}
	got, err := db.CalendarObjectByUID(ctx, cal.ID, "same-uid")
	if err != nil || got.Href != "b.ics" || got.Summary != "B" {
		t.Fatalf("uid row %+v %v", got, err)
	}
}

func TestAttachmentsStoreAndCascade(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(ctx, u.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:u1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828\r\nSUMMARY:A\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if err := db.UpsertCalendarObject(ctx, cal.ID, "u1.ics", "u1", "etag", "VEVENT", ics, "20260828", "", "A"); err != nil {
		t.Fatal(err)
	}
	a, err := db.InsertAttachment(ctx, cal.ID, "u1.ics", "notes.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Filename != "notes.txt" || a.Size != 5 {
		t.Fatalf("%+v", a)
	}
	list, err := db.ListAttachments(ctx, cal.ID, "u1.ics")
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v %v", list, err)
	}
	got, err := db.AttachmentByPublicID(ctx, a.PublicID)
	if err != nil || string(got.Data) != "hello" {
		t.Fatalf("get %+v %v", got, err)
	}
	if _, err := db.InsertAttachment(ctx, cal.ID, "u1.ics", "../evil.txt", "text/html", []byte("<b>x</b>")); err != nil {
		t.Fatal(err)
	}
	list, _ = db.ListAttachments(ctx, cal.ID, "u1.ics")
	if list[1].Filename != "evil.txt" || list[1].ContentType != "application/octet-stream" {
		t.Fatalf("sanitize %+v", list[1])
	}
	if err := db.DeleteCalendarObject(ctx, cal.ID, "u1.ics"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AttachmentByPublicID(ctx, a.PublicID); err == nil {
		t.Fatal("attachment should cascade-delete with the event")
	}
}

func TestListObjectsByComponentAndRefs(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(ctx, u.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCalendarObject(ctx, cal.ID, "e.ics", "e1", "etag-e", "VEVENT", "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n", "20260828", "", "E"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCalendarObject(ctx, cal.ID, "t.ics", "t1", "etag-t", "VTODO", "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n", "", "20260829", "T"); err != nil {
		t.Fatal(err)
	}
	events, err := db.ListCalendarObjectsByComponent(ctx, cal.ID, "vevent")
	if err != nil || len(events) != 1 || events[0].Href != "e.ics" {
		t.Fatalf("events %+v %v", events, err)
	}
	todos, err := db.ListCalendarObjectsByComponent(ctx, cal.ID, "VTODO")
	if err != nil || len(todos) != 1 || todos[0].Href != "t.ics" {
		t.Fatalf("todos %+v %v", todos, err)
	}
	refs, err := db.ListCalendarObjectRefs(ctx, cal.ID, "t.")
	if err != nil || len(refs) != 1 || refs[0].Href != "t.ics" || refs[0].ETag != "etag-t" {
		t.Fatalf("refs %+v %v", refs, err)
	}
	before, err := db.CalendarBySlug(ctx, u.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteCalendarObjectsByPrefix(ctx, cal.ID, "missing-"); err != nil {
		t.Fatal(err)
	}
	after, err := db.CalendarBySlug(ctx, u.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if after.CTag != before.CTag {
		t.Fatalf("empty prefix delete bumped ctag %d -> %d", before.CTag, after.CTag)
	}
}

func TestSessionStoredHashed(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	plain := "session-plain-token-32bytes-minimum!"
	if err := db.CreateSession(ctx, plain, u.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.SQL.QueryRowContext(ctx, `SELECT id FROM sessions WHERE user_id = ?`, u.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == plain {
		t.Fatal("session id stored in plaintext")
	}
	if len(stored) != 64 {
		t.Fatalf("hashed session length %d", len(stored))
	}
	got, err := db.UserBySession(ctx, plain)
	if err != nil || got.ID != u.ID {
		t.Fatalf("lookup by cookie %v %+v", err, got)
	}

	legacy := "legacy-plaintext-session-id"
	exp := time.Now().UTC().Add(time.Hour).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.SQL.ExecContext(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`, legacy, u.ID, exp); err != nil {
		t.Fatal(err)
	}
	got, err = db.UserBySession(ctx, legacy)
	if err != nil || got.ID != u.ID {
		t.Fatalf("legacy lookup %v", err)
	}
	var n int
	if err := db.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM sessions WHERE id = ?`, legacy).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("legacy plaintext session was not upgraded")
	}
}

func TestWebcalStoredHashed(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(ctx, u.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := db.RotateWebcal(ctx, u.ID, cal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok.Token) != 36 {
		t.Fatalf("rotate token length %d", len(tok.Token))
	}
	var stored string
	if err := db.SQL.QueryRowContext(ctx, `SELECT token FROM webcal_tokens WHERE calendar_id = ?`, cal.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == tok.Token {
		t.Fatal("webcal token stored in plaintext")
	}
	if len(stored) != 64 {
		t.Fatalf("hashed webcal length %d", len(stored))
	}
	got, err := db.WebcalByToken(ctx, tok.Token)
	if err != nil || got.CalendarID != cal.ID {
		t.Fatalf("lookup by secret %v %+v", err, got)
	}

	legacy := "0123456789abcdef0123456789abcdef0123"
	if _, err := db.SQL.ExecContext(ctx, `UPDATE webcal_tokens SET token = ? WHERE calendar_id = ?`, legacy, cal.ID); err != nil {
		t.Fatal(err)
	}
	got, err = db.WebcalByToken(ctx, legacy)
	if err != nil || got.CalendarID != cal.ID {
		t.Fatalf("legacy lookup %v", err)
	}
	var n int
	if err := db.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM webcal_tokens WHERE token = ?`, legacy).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("legacy plaintext webcal was not upgraded")
	}
	if err := db.PutWebcalToken(ctx, u.ID, cal.ID, tok.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := db.WebcalByToken(ctx, tok.Token); err != nil {
		t.Fatal(err)
	}
}
