package userbackup

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"testing"

	"github.com/devcoons/dcalcon/internal/storage"
)

func TestOpenRejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("calendars/../account.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("{}"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(buf.Bytes()); err != ErrUnsafeZip {
		t.Fatalf("want ErrUnsafeZip got %v", err)
	}
}

func TestBuildRestoreDataRoundTrip(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(ctx, u.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:round-1\r\nDTSTAMP:20260828T090000Z\r\nDTSTART:20260828T090000Z\r\nDTEND:20260828T100000Z\r\nSUMMARY:Round\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if err := db.UpsertCalendarObject(ctx, cal.ID, "round-1.ics", "round-1", "etag", "VEVENT", ics, "20260828T090000Z", "20260828T100000Z", "Round"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Build(ctx, db, u.ID, KindData, &buf); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteCalendarObject(ctx, cal.ID, "round-1.ics"); err != nil {
		t.Fatal(err)
	}
	bundle, err := Open(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Manifest.Kind != KindData || bundle.Account != nil {
		t.Fatalf("manifest %+v account %v", bundle.Manifest, bundle.Account)
	}
	res, err := Restore(ctx, db, u.ID, "http://cal.example.test", bundle)
	if err != nil {
		t.Fatal(err)
	}
	if res.Objects < 1 {
		t.Fatalf("result %+v", res)
	}
	if _, err := db.CalendarObjectByHref(ctx, cal.ID, "round-1.ics"); err != nil {
		t.Fatalf("object missing: %v", err)
	}
}

func TestFullRestoreRejectsOtherUser(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	alice, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := db.CreateUser(ctx, "bob", "bob@example.com", "secret-pass", "Bob", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Build(ctx, db, alice.ID, KindFull, &buf); err != nil {
		t.Fatal(err)
	}
	bundle, err := Open(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, db, bob.ID, "", bundle); err != ErrUsername {
		t.Fatalf("want ErrUsername got %v", err)
	}
}
