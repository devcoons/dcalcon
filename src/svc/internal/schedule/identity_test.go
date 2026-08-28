package schedule_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
)

func TestFindUserLocalMailbox(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	u, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, ident := range []string{
		"alice",
		"alice@example.com",
		"mailto:alice@example.com",
		"alice@dcalcon.private",
		"mailto:ALICE@dcalcon.private",
		"alice@dcalcon.invalid",
	} {
		got, err := schedule.FindUser(t.Context(), db, ident)
		if err != nil {
			t.Fatalf("%s: %v", ident, err)
		}
		if got.ID != u.ID {
			t.Fatalf("%s: id %d", ident, got.ID)
		}
	}
	if _, err := schedule.FindUser(t.Context(), db, "nobody@dcalcon.private"); err == nil {
		t.Fatal("expected missing local user")
	}
	if !schedule.IsLocalMailbox("bob@dcalcon.private") || schedule.IsLocalMailbox("bob@gmail.com") {
		t.Fatal("IsLocalMailbox")
	}
}

func TestDeliverFromPutLocalAddress(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	alice, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := db.CreateUser(t.Context(), "bob", "bob@example.com", "secret-pass", "Bob", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(t.Context(), alice.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	raw := icsutil.NewEventICS("meet-local", "Meet", "20260828T090000Z", "20260828T100000Z", "", "")
	raw, err = icsutil.SetOrganizerAttendees(raw, schedule.LocalMailbox("alice"), "Alice", []icsutil.AttendeeInfo{
		{Value: "bob@dcalcon.private", CN: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	etag := icsutil.ETag(raw)
	if err := db.UpsertCalendarObject(t.Context(), cal.ID, "meet-local.ics", "meet-local", etag, "VEVENT", raw, "20260828T090000Z", "20260828T100000Z", "Meet"); err != nil {
		t.Fatal(err)
	}
	obj, err := db.CalendarObjectByHref(t.Context(), cal.ID, "meet-local.ics")
	if err != nil {
		t.Fatal(err)
	}
	if err := schedule.DeliverFromPut(t.Context(), db, alice, cal, obj, ""); err != nil {
		t.Fatal(err)
	}
	inbox, err := db.ListInbox(t.Context(), bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) == 0 {
		t.Fatal("bob should have a local invite")
	}
}

func TestRefreshPeopleBook(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	alice, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(t.Context(), "bob", "bob@example.com", "secret-pass", "Bob", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := schedule.RefreshPeopleBook(t.Context(), db, alice.ID); err != nil {
		t.Fatal(err)
	}
	book, err := db.AddressBookBySlug(t.Context(), alice.ID, schedule.PeopleBookSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !book.ReadOnly {
		t.Fatal("people book should be read-only")
	}
	list, err := db.ListAddressObjects(t.Context(), book.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want bob only, got %d", len(list))
	}
	if !strings.Contains(list[0].VCard, "bob@dcalcon.private") {
		t.Fatalf("card %s", list[0].VCard)
	}
	if list[0].Href != "bob.vcf" {
		t.Fatalf("href %s", list[0].Href)
	}
}
