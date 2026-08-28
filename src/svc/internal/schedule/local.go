package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
)

func ApplyLocalInvite(ctx context.Context, db *storage.DB, organizer *storage.User, cal *storage.Calendar, obj *storage.CalendarObject, attendees []*storage.User) error {
	infos := make([]icsutil.AttendeeInfo, 0, len(attendees))
	for _, a := range attendees {
		cn := a.DisplayName
		if cn == "" {
			cn = a.Username
		}
		infos = append(infos, icsutil.AttendeeInfo{Value: LocalMailbox(a.Username), CN: cn})
	}
	cn := organizer.DisplayName
	if cn == "" {
		cn = organizer.Username
	}
	updated, err := icsutil.MergeOrganizerAttendees(obj.ICS, organizer.Email, cn, infos)
	if err != nil {
		return err
	}
	ds, de := rangeOf(updated)
	etag := icsutil.ETag(updated)
	return db.WithTx(ctx, func(ctx context.Context) error {
		if err := db.UpsertCalendarObject(ctx, cal.ID, obj.Href, obj.UID, etag, obj.Component, updated, ds, de, obj.Summary); err != nil {
			return err
		}
		obj.ICS, obj.ETag, obj.DTStart, obj.DTEnd = updated, etag, ds, de
		return deliverCopies(ctx, db, organizer, obj, attendees)
	})
}

func MergeExternalEmails(ctx context.Context, db *storage.DB, organizer *storage.User, cal *storage.Calendar, obj *storage.CalendarObject, emails []string) error {
	infos := make([]icsutil.AttendeeInfo, 0, len(emails))
	for _, e := range emails {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		infos = append(infos, icsutil.AttendeeInfo{Value: e, CN: e})
	}
	if len(infos) == 0 {
		return nil
	}
	cn := organizer.DisplayName
	if cn == "" {
		cn = organizer.Username
	}
	updated, err := icsutil.MergeOrganizerAttendees(obj.ICS, organizer.Email, cn, infos)
	if err != nil {
		return err
	}
	ds, de := rangeOf(updated)
	etag := icsutil.ETag(updated)
	if err := db.UpsertCalendarObject(ctx, cal.ID, obj.Href, obj.UID, etag, obj.Component, updated, ds, de, obj.Summary); err != nil {
		return err
	}
	obj.ICS, obj.ETag, obj.DTStart, obj.DTEnd = updated, etag, ds, de
	return nil
}

func DeliverFromPut(ctx context.Context, db *storage.DB, organizer *storage.User, cal *storage.Calendar, obj *storage.CalendarObject, prevICS string) error {
	if cal.Kind != "personal" && cal.Kind != "shared" {
		return nil
	}
	org := icsutil.AddrOf(icsutil.OrganizerValue(obj.ICS))
	if org != "" && !SameUser(organizer, org) {
		return nil
	}
	cur := localAttendees(ctx, db, organizer, obj.ICS)
	if prevICS != "" {
		prev := localAttendees(ctx, db, organizer, prevICS)
		var removed []*storage.User
		curIDs := map[int64]bool{}
		for _, u := range cur {
			curIDs[u.ID] = true
		}
		for _, u := range prev {
			if !curIDs[u.ID] {
				removed = append(removed, u)
			}
		}
		if len(removed) > 0 {
			if err := deliverCancel(ctx, db, organizer, obj, removed); err != nil {
				return err
			}
		}
	}
	if len(cur) == 0 {
		return nil
	}
	return deliverCopies(ctx, db, organizer, obj, cur)
}

func localAttendees(ctx context.Context, db *storage.DB, organizer *storage.User, raw string) []*storage.User {
	var users []*storage.User
	seen := map[int64]bool{}
	for _, a := range icsutil.Attendees(raw) {
		u, err := FindUser(ctx, db, a.Value)
		if err != nil || u.Status != "active" || u.ID == organizer.ID || seen[u.ID] {
			continue
		}
		seen[u.ID] = true
		users = append(users, u)
	}
	return users
}

func deliverCopies(ctx context.Context, db *storage.DB, organizer *storage.User, obj *storage.CalendarObject, attendees []*storage.User) error {
	reqICS, err := icsutil.WithMethod(obj.ICS, "REQUEST")
	if err != nil {
		reqICS = obj.ICS
	}
	reqETag := icsutil.ETag(reqICS)
	ds, de := obj.DTStart, obj.DTEnd
	comp := obj.Component
	if comp == "" {
		comp = icsutil.CalendarComponentFromICS(obj.ICS)
	}
	if comp == "" {
		comp = "VEVENT"
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		if err := db.EnsureSchedulingCollections(ctx, organizer.ID); err != nil {
			return err
		}
		outbox, err := db.CalendarBySlug(ctx, organizer.ID, "outbox")
		if err != nil {
			return err
		}
		for _, a := range attendees {
			if a.ID == organizer.ID || a.Status != "active" {
				continue
			}
			if err := db.EnsureSchedulingCollections(ctx, a.ID); err != nil {
				return err
			}
			prev, err := db.ScheduleByUID(ctx, a.ID, "inbox", "REQUEST", obj.UID, a.Username)
			if err != nil && !errors.Is(err, storage.ErrNotFound) {
				return err
			}
			settled := prev != nil && (prev.Status == "accepted" || prev.Status == "declined")
			if !settled {
				inbox, err := db.CalendarBySlug(ctx, a.ID, "inbox")
				if err != nil {
					return err
				}
				href := fmt.Sprintf("%s-%s.ics", obj.UID, a.Username)
				if err := db.UpsertCalendarObject(ctx, inbox.ID, href, obj.UID, reqETag, comp, reqICS, ds, de, obj.Summary); err != nil {
					return err
				}
			}
			if err := db.InsertSchedule(ctx, a.ID, "inbox", "REQUEST", obj.UID, organizer.Username, a.Username, "pending", reqICS); err != nil {
				return err
			}
			if settled && prev.Status == "accepted" {
				if err := refreshAcceptedCopy(ctx, db, a, obj); err != nil {
					return err
				}
			}
			if err := db.UpsertCalendarObject(ctx, outbox.ID, hrefFor(obj.UID, a.Username), obj.UID, reqETag, comp, reqICS, ds, de, obj.Summary); err != nil {
				return err
			}
			if err := db.InsertSchedule(ctx, organizer.ID, "outbox", "REQUEST", obj.UID, organizer.Username, a.Username, "processed", reqICS); err != nil {
				return err
			}
		}
		return nil
	})
}

func hrefFor(uid, username string) string {
	return fmt.Sprintf("%s-%s.ics", uid, username)
}

func refreshAcceptedCopy(ctx context.Context, db *storage.DB, attendee *storage.User, obj *storage.CalendarObject) error {
	cals, err := db.ListCalendars(ctx, attendee.ID)
	if err != nil {
		return err
	}
	clean, err := icsutil.StripMethod(obj.ICS)
	if err != nil {
		clean = obj.ICS
	}
	clean, err = icsutil.SetAttendeePartstatAny(clean, Identities(attendee), "ACCEPTED")
	if err != nil {
		return err
	}
	etag := icsutil.ETag(clean)
	ds, de := rangeOf(clean)
	sum := icsutil.SummaryFromICS(clean)
	comp := obj.Component
	if comp == "" {
		comp = icsutil.CalendarComponentFromICS(clean)
	}
	for i := range cals {
		c := &cals[i]
		if c.Kind == "inbox" || c.Kind == "outbox" || !c.CanWrite() {
			continue
		}
		existing, err := db.CalendarObjectByUID(ctx, c.ID, obj.UID)
		if err != nil {
			continue
		}
		return db.UpsertCalendarObject(ctx, c.ID, existing.Href, obj.UID, etag, comp, clean, ds, de, sum)
	}
	return nil
}

func rangeOf(raw string) (string, string) {
	parsed, err := icsutil.ParseCalendar(raw)
	if err != nil {
		return "", ""
	}
	return icsutil.CalendarRange(parsed)
}

func Accept(ctx context.Context, db *storage.DB, attendee *storage.User, item *storage.ScheduleItem, calendarID int64) error {
	clean, err := icsutil.StripMethod(item.ICS)
	if err != nil {
		clean = item.ICS
	}
	clean, err = icsutil.SetAttendeePartstatAny(clean, Identities(attendee), "ACCEPTED")
	if err != nil {
		return err
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		target, err := acceptCalendar(ctx, db, attendee.ID, calendarID)
		if err != nil {
			return err
		}
		comp := icsutil.CalendarComponentFromICS(clean)
		href := item.UID + ".ics"
		etag := icsutil.ETag(clean)
		ds, de := rangeOf(clean)
		if err := db.UpsertCalendarObject(ctx, target.ID, href, item.UID, etag, comp, clean, ds, de, icsutil.SummaryFromICS(clean)); err != nil {
			return err
		}
		if err := writeReply(ctx, db, attendee, item, "ACCEPTED", clean); err != nil {
			return err
		}
		if err := deleteInboxCopies(ctx, db, attendee.ID, item.UID); err != nil {
			return err
		}
		return db.SetScheduleStatus(ctx, attendee.ID, item.ID, "accepted")
	})
}

func Decline(ctx context.Context, db *storage.DB, attendee *storage.User, item *storage.ScheduleItem) error {
	clean, err := icsutil.StripMethod(item.ICS)
	if err != nil {
		clean = item.ICS
	}
	clean, err = icsutil.SetAttendeePartstatAny(clean, Identities(attendee), "DECLINED")
	if err != nil {
		return err
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		if err := writeReply(ctx, db, attendee, item, "DECLINED", clean); err != nil {
			return err
		}
		if err := deleteInboxCopies(ctx, db, attendee.ID, item.UID); err != nil {
			return err
		}
		return db.SetScheduleStatus(ctx, attendee.ID, item.ID, "declined")
	})
}

func acceptCalendar(ctx context.Context, db *storage.DB, userID, calendarID int64) (*storage.Calendar, error) {
	if calendarID != 0 {
		c, err := db.CalendarByID(ctx, userID, calendarID)
		if err != nil {
			return nil, err
		}
		if !c.CanWrite() {
			return nil, fmt.Errorf("calendar is read-only")
		}
		return c, nil
	}
	cals, err := db.ListCalendars(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range cals {
		c := &cals[i]
		if c.Kind == "personal" && !c.Shared && c.CanWrite() {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no writable calendar")
}

func deleteInboxCopies(ctx context.Context, db *storage.DB, userID int64, uid string) error {
	inbox, err := db.CalendarBySlug(ctx, userID, "inbox")
	if err != nil {
		return err
	}
	return db.DeleteObjectsByUID(ctx, inbox.ID, uid)
}

func writeReply(ctx context.Context, db *storage.DB, attendee *storage.User, item *storage.ScheduleItem, partstat, eventICS string) error {
	org, err := FindUser(ctx, db, item.Organizer)
	if err != nil {
		return err
	}
	reply, err := icsutil.WithMethod(eventICS, "REPLY")
	if err != nil {
		return err
	}
	if err := db.EnsureSchedulingCollections(ctx, org.ID); err != nil {
		return err
	}
	inbox, err := db.CalendarBySlug(ctx, org.ID, "inbox")
	if err != nil {
		return err
	}
	href := fmt.Sprintf("%s-reply-%s.ics", item.UID, attendee.Username)
	etag := icsutil.ETag(reply)
	comp := icsutil.CalendarComponentFromICS(eventICS)
	if err := db.UpsertCalendarObject(ctx, inbox.ID, href, item.UID, etag, comp, reply, "", "", icsutil.SummaryFromICS(eventICS)); err != nil {
		return err
	}
	if err := db.InsertSchedule(ctx, org.ID, "inbox", "REPLY", item.UID, org.Username, attendee.Username, strings.ToLower(partstat), reply); err != nil {
		return err
	}
	return MergeReply(ctx, db, org, reply)
}
