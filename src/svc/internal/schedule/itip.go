package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
)

type OutboxResult struct {
	Recipient    string
	Status       string
	CalendarData string
}

func HandleOutboxPOST(ctx context.Context, db *storage.DB, user *storage.User, raw string) ([]OutboxResult, error) {
	method := icsutil.MethodFromICS(raw)
	comp := icsutil.CalendarComponentFromICS(raw)
	if strings.EqualFold(comp, "VFREEBUSY") || strings.Contains(strings.ToUpper(raw), "BEGIN:VFREEBUSY") {
		return freeBusyOutbox(ctx, db, user, raw)
	}
	cal, err := icsutil.ParseCalendar(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar")
	}
	uid := icsutil.CalendarUID(cal)
	obj := &storage.CalendarObject{
		UID:       uid,
		ICS:       raw,
		Component: icsutil.CalendarComponent(cal),
		Summary:   icsutil.CalendarSummary(cal),
	}
	obj.DTStart, obj.DTEnd = icsutil.CalendarRange(cal)

	switch method {
	case "CANCEL":
		atts := localAttendees(ctx, db, user, raw)
		if err := deliverCancel(ctx, db, user, obj, atts); err != nil {
			return nil, err
		}
		return recipientOK(atts), nil
	case "REPLY":
		orgIdent := icsutil.OrganizerValue(raw)
		org, err := FindUser(ctx, db, orgIdent)
		if err != nil {
			return []OutboxResult{{Recipient: orgIdent, Status: "3.7;Invalid calendar user"}}, nil
		}
		if err := MergeReply(ctx, db, org, raw); err != nil {
			return nil, err
		}
		return []OutboxResult{{Recipient: icsutil.Mailto(org.Username), Status: "2.0;Success"}}, nil
	default:
		atts := localAttendees(ctx, db, user, raw)
		strip, err := icsutil.StripMethod(raw)
		if err == nil {
			obj.ICS = strip
		}
		if err := deliverCopies(ctx, db, user, obj, atts); err != nil {
			return nil, err
		}
		return recipientOK(atts), nil
	}
}

func recipientOK(users []*storage.User) []OutboxResult {
	out := make([]OutboxResult, 0, len(users))
	for _, u := range users {
		out = append(out, OutboxResult{Recipient: icsutil.Mailto(LocalMailbox(u.Username)), Status: "2.0;Success"})
	}
	return out
}

func CancelFromDelete(ctx context.Context, db *storage.DB, organizer *storage.User, cal *storage.Calendar, obj *storage.CalendarObject) error {
	if obj == nil || organizer == nil || cal == nil {
		return nil
	}
	if cal.Kind != "personal" && cal.Kind != "shared" {
		return nil
	}
	org := icsutil.AddrOf(icsutil.OrganizerValue(obj.ICS))
	if org != "" && !SameUser(organizer, org) {
		return nil
	}
	atts := localAttendees(ctx, db, organizer, obj.ICS)
	if len(atts) == 0 {
		return nil
	}
	return deliverCancel(ctx, db, organizer, obj, atts)
}

func deliverCancel(ctx context.Context, db *storage.DB, organizer *storage.User, obj *storage.CalendarObject, attendees []*storage.User) error {
	cancelICS, err := icsutil.WithMethod(obj.ICS, "CANCEL")
	if err != nil {
		cancelICS = obj.ICS
	}
	etag := icsutil.ETag(cancelICS)
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
			inbox, err := db.CalendarBySlug(ctx, a.ID, "inbox")
			if err != nil {
				return err
			}
			href := fmt.Sprintf("%s-%s.ics", obj.UID, a.Username)
			if err := db.UpsertCalendarObject(ctx, inbox.ID, href, obj.UID, etag, obj.Component, cancelICS, obj.DTStart, obj.DTEnd, obj.Summary); err != nil {
				return err
			}
			if err := db.InsertSchedule(ctx, a.ID, "inbox", "CANCEL", obj.UID, organizer.Username, a.Username, "cancelled", cancelICS); err != nil {
				return err
			}
			if err := db.ClosePendingInbox(ctx, a.ID, obj.UID, "cancelled"); err != nil {
				return err
			}
			cals, err := db.ListCalendars(ctx, a.ID)
			if err != nil {
				return err
			}
			for i := range cals {
				c := &cals[i]
				if c.Shared || c.Kind == "inbox" || c.Kind == "outbox" {
					continue
				}
				if err := db.DeleteObjectsByUID(ctx, c.ID, obj.UID); err != nil {
					return err
				}
			}
			if err := db.UpsertCalendarObject(ctx, outbox.ID, href, obj.UID, etag, obj.Component, cancelICS, obj.DTStart, obj.DTEnd, obj.Summary); err != nil {
				return err
			}
			if err := db.InsertSchedule(ctx, organizer.ID, "outbox", "CANCEL", obj.UID, organizer.Username, a.Username, "processed", cancelICS); err != nil {
				return err
			}
		}
		return nil
	})
}

func MergeReply(ctx context.Context, db *storage.DB, organizer *storage.User, replyICS string) error {
	if organizer == nil {
		return nil
	}
	cal, err := icsutil.ParseCalendar(replyICS)
	if err != nil {
		return err
	}
	uid := icsutil.CalendarUID(cal)
	if uid == "" {
		return nil
	}
	atts := icsutil.Attendees(replyICS)
	if len(atts) == 0 {
		return nil
	}
	target, obj, err := db.FindOwnedObjectByUID(ctx, organizer.ID, uid)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return err
	}
	updated := obj.ICS
	for _, a := range atts {
		part := a.Partstat
		if part == "" {
			continue
		}
		updated, err = icsutil.SetAttendeePartstat(updated, a.Value, part)
		if err != nil {
			return err
		}
	}
	if updated == obj.ICS {
		return nil
	}
	etag := icsutil.ETag(updated)
	ds, de := rangeOf(updated)
	return db.UpsertCalendarObject(ctx, target.ID, obj.Href, obj.UID, etag, obj.Component, updated, ds, de, obj.Summary)
}
