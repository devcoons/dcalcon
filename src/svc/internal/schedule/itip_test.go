package schedule_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
)

func twoUsers(t *testing.T) (*storage.DB, *storage.User, *storage.User) {
	t.Helper()
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
	return db, alice, bob
}

func putMeet(t *testing.T, db *storage.DB, alice *storage.User) (*storage.Calendar, *storage.CalendarObject) {
	t.Helper()
	cal, err := db.CalendarBySlug(t.Context(), alice.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	raw := icsutil.NewEventICS("meet-1", "Meet", "20260828T090000Z", "20260828T100000Z", "", "")
	raw, err = icsutil.SetOrganizerAttendees(raw, schedule.LocalMailbox("alice"), "Alice", []icsutil.AttendeeInfo{
		{Value: "bob@dcalcon.private", CN: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	etag := icsutil.ETag(raw)
	if err := db.UpsertCalendarObject(t.Context(), cal.ID, "meet-1.ics", "meet-1", etag, "VEVENT", raw, "20260828T090000Z", "20260828T100000Z", "Meet"); err != nil {
		t.Fatal(err)
	}
	obj, err := db.CalendarObjectByHref(t.Context(), cal.ID, "meet-1.ics")
	if err != nil {
		t.Fatal(err)
	}
	return cal, obj
}

func TestCancelAndReplyMerge(t *testing.T) {
	db, alice, bob := twoUsers(t)
	cal, obj := putMeet(t, db, alice)
	if err := schedule.DeliverFromPut(t.Context(), db, alice, cal, obj, ""); err != nil {
		t.Fatal(err)
	}
	inbox, err := db.ListInbox(t.Context(), bob.ID)
	if err != nil || len(inbox) == 0 {
		t.Fatalf("inbox %v %d", err, len(inbox))
	}
	other, err := db.CreateCalendar(t.Context(), bob.ID, "work", "Work", "", "#111111", "personal", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := schedule.Accept(t.Context(), db, bob, &inbox[0], other.ID); err != nil {
		t.Fatal(err)
	}
	copied, err := db.CalendarObjectByUID(t.Context(), other.ID, "meet-1")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Summary != "Meet" {
		t.Fatalf("accepted into work: %s", copied.Summary)
	}
	bobInbox, err := db.CalendarBySlug(t.Context(), bob.ID, "inbox")
	if err != nil {
		t.Fatal(err)
	}
	left, err := db.ListCalendarObjects(t.Context(), bobInbox.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("stale inbox copies: %d", len(left))
	}
	aliceObj, err := db.CalendarObjectByHref(t.Context(), cal.ID, "meet-1.ics")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range icsutil.Attendees(aliceObj.ICS) {
		if strings.Contains(strings.ToLower(a.Value), "bob") && strings.EqualFold(a.Partstat, "ACCEPTED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("organizer PARTSTAT not merged: %s", aliceObj.ICS)
	}

	if err := schedule.CancelFromDelete(t.Context(), db, alice, cal, aliceObj); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CalendarObjectByUID(t.Context(), other.ID, "meet-1"); err == nil {
		t.Fatal("accepted copy should be removed on CANCEL")
	}
	after, err := db.ListInbox(t.Context(), bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range after {
		if it.UID == "meet-1" && it.Status == "pending" {
			t.Fatalf("cancel should close pending invite: %+v", it)
		}
	}
}

func TestOrganizerUpdateKeepsAcceptedCopy(t *testing.T) {
	db, alice, bob := twoUsers(t)
	cal, obj := putMeet(t, db, alice)
	if err := schedule.DeliverFromPut(t.Context(), db, alice, cal, obj, ""); err != nil {
		t.Fatal(err)
	}
	inbox, err := db.ListInbox(t.Context(), bob.ID)
	if err != nil || len(inbox) == 0 {
		t.Fatal(err)
	}
	other, err := db.CreateCalendar(t.Context(), bob.ID, "work", "Work", "", "#111111", "personal", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := schedule.Accept(t.Context(), db, bob, &inbox[0], other.ID); err != nil {
		t.Fatal(err)
	}
	updated := strings.ReplaceAll(obj.ICS, "Meet", "Meet later")
	etag := icsutil.ETag(updated)
	if err := db.UpsertCalendarObject(t.Context(), cal.ID, obj.Href, obj.UID, etag, "VEVENT", updated, obj.DTStart, obj.DTEnd, "Meet later"); err != nil {
		t.Fatal(err)
	}
	obj, err = db.CalendarObjectByHref(t.Context(), cal.ID, obj.Href)
	if err != nil {
		t.Fatal(err)
	}
	if err := schedule.DeliverFromPut(t.Context(), db, alice, cal, obj, ""); err != nil {
		t.Fatal(err)
	}
	for _, it := range mustInbox(t, db, bob.ID) {
		if it.UID == "meet-1" && it.Status == "pending" {
			t.Fatalf("accepted invite re-opened as pending: %+v", it)
		}
	}
	copied, err := db.CalendarObjectByUID(t.Context(), other.ID, "meet-1")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Summary != "Meet later" && !strings.Contains(copied.ICS, "Meet later") {
		t.Fatalf("accepted copy not refreshed: %s", copied.ICS)
	}
}

func mustInbox(t *testing.T, db *storage.DB, userID int64) []storage.ScheduleItem {
	t.Helper()
	list, err := db.ListInbox(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestOutboxFreeBusyAndBusyWindows(t *testing.T) {
	db, alice, bob := twoUsers(t)
	cal, obj := putMeet(t, db, alice)
	_ = obj
	start, _ := icsutil.ParseICSTime("20260828T000000Z")
	end, _ := icsutil.ParseICSTime("20260829T000000Z")
	periods, err := schedule.BusyForUser(t.Context(), db, alice.ID, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) == 0 {
		t.Fatal("alice should be busy")
	}
	fb := "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nBEGIN:VFREEBUSY\r\nUID:fb-1\r\nDTSTART:20260828T000000Z\r\nDTEND:20260829T000000Z\r\nATTENDEE:mailto:alice@dcalcon.private\r\nEND:VFREEBUSY\r\nEND:VCALENDAR\r\n"
	res, err := schedule.HandleOutboxPOST(t.Context(), db, bob, fb)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !strings.Contains(res[0].CalendarData, "FREEBUSY") {
		t.Fatalf("freebusy %+v", res)
	}
	_ = cal
}

func TestBusySkipsTransparent(t *testing.T) {
	raw := icsutil.NewEventICS("t1", "Focus", "20260828T090000Z", "20260828T100000Z", "", "")
	raw = strings.Replace(raw, "END:VEVENT", "TRANSP:TRANSPARENT\r\nEND:VEVENT", 1)
	start, _ := icsutil.ParseICSTime("20260828T000000Z")
	end, _ := icsutil.ParseICSTime("20260829T000000Z")
	if n := icsutil.BusyPeriods(raw, start, end); len(n) != 0 {
		t.Fatalf("transparent still busy: %v", n)
	}
}

func TestSplitAndJoinICS(t *testing.T) {
	a := icsutil.NewEventICS("a", "A", "20260828T090000Z", "20260828T100000Z", "", "")
	b := icsutil.NewEventICS("b", "B", "20260828T110000Z", "20260828T120000Z", "", "")
	joined := icsutil.JoinCalendars([]string{a, b})
	parts, err := icsutil.SplitCalendarObjects(joined)
	if err != nil || len(parts) != 2 {
		t.Fatalf("split %v %d", err, len(parts))
	}
}

func TestRRuleAndAlarmRoundTrip(t *testing.T) {
	raw := icsutil.NewEventICS("r1", "Standup", "20260828T090000Z", "20260828T093000Z", "", "")
	raw, err := icsutil.SetRRule(raw, "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO")
	if err != nil {
		t.Fatal(err)
	}
	raw, err = icsutil.SetDisplayAlarm(raw, 15)
	if err != nil {
		t.Fatal(err)
	}
	if icsutil.RRuleFromICS(raw) != "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO" {
		t.Fatalf("rrule %s", icsutil.RRuleFromICS(raw))
	}
	if icsutil.AlarmMinutesFromICS(raw) != 15 {
		t.Fatalf("alarm %d", icsutil.AlarmMinutesFromICS(raw))
	}
}

func TestTodoICS(t *testing.T) {
	raw := icsutil.NewTodoICS("td1", "Buy milk", "20260829", "notes", "NEEDS-ACTION")
	if icsutil.CalendarComponentFromICS(raw) != "VTODO" {
		t.Fatal(icsutil.CalendarComponentFromICS(raw))
	}
	raw, err := icsutil.UpdateTodoICS(raw, "Buy milk", "20260829", "notes", "COMPLETED")
	if err != nil {
		t.Fatal(err)
	}
	if icsutil.TodoStatusFromICS(raw) != "COMPLETED" {
		t.Fatal(icsutil.TodoStatusFromICS(raw))
	}
}

func TestBusyExpandRecurrence(t *testing.T) {
	raw := icsutil.NewEventICS("w1", "Standup", "20260803T090000Z", "20260803T100000Z", "", "")
	raw, err := icsutil.SetRRule(raw, "FREQ=WEEKLY")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	periods := icsutil.BusyPeriods(raw, start, end)
	if len(periods) == 0 {
		t.Fatal("weekly event should expand into range")
	}
}
