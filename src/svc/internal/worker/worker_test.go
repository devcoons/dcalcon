package worker

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/storage"
)

func TestSyncImportantDates(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	u, err := db.CreateUser(ctx, "ada", "ada@example.com", "password1", "Ada", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	books, _ := db.ListAddressBooks(ctx, u.ID)
	_ = db.UpsertAddressObject(ctx, books[0].ID, "ada.vcf", "ada", "etag",
		"BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ada\r\nFN:Ada Lovelace\r\nBDAY:1815-12-10\r\nEND:VCARD\r\n",
		"Ada Lovelace", "1815-12-10", "")
	_ = db.SaveImportantDates(ctx, u.ID, storage.ImportantDatesSettings{
		Enabled: true, IncludeBirthdays: true, IncludeAnniversaries: false, AlarmOffsets: []string{"-P1D"},
	})
	w := &Worker{Store: db, Cfg: config.Default()}
	w.Cfg.Worker.Interval = time.Minute
	if err := w.syncImportantDates(ctx); err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(ctx, u.ID, "important-dates")
	if err != nil {
		t.Fatal(err)
	}
	if !cal.ReadOnly {
		t.Fatal("important dates should be read-only")
	}
	objs, err := db.ListCalendarObjects(ctx, cal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) == 0 {
		t.Fatal("expected birthday event")
	}
	etag := objs[0].ETag
	ctag := cal.CTag
	if err := w.syncImportantDates(ctx); err != nil {
		t.Fatal(err)
	}
	cal, err = db.CalendarBySlug(ctx, u.ID, "important-dates")
	if err != nil {
		t.Fatal(err)
	}
	if cal.CTag != ctag {
		t.Fatalf("second sync bumped ctag %d -> %d", ctag, cal.CTag)
	}
	objs, err = db.ListCalendarObjects(ctx, cal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].ETag != etag {
		t.Fatalf("second sync rewrote objects: %+v", objs)
	}

	_ = db.SaveImportantDates(ctx, u.ID, storage.ImportantDatesSettings{Enabled: false})
	if err := w.syncImportantDates(ctx); err != nil {
		t.Fatal(err)
	}
	objs, err = db.ListCalendarObjects(ctx, cal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 0 {
		t.Fatalf("disabled important dates should clear managed events, got %d", len(objs))
	}
	cal, err = db.CalendarBySlug(ctx, u.ID, "important-dates")
	if err != nil {
		t.Fatal(err)
	}
	clearedCTag := cal.CTag
	if err := w.syncImportantDates(ctx); err != nil {
		t.Fatal(err)
	}
	cal, err = db.CalendarBySlug(ctx, u.ID, "important-dates")
	if err != nil {
		t.Fatal(err)
	}
	if cal.CTag != clearedCTag {
		t.Fatalf("empty disable sync bumped ctag %d -> %d", clearedCTag, cal.CTag)
	}
}
