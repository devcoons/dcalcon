package schedule

import (
	"context"
	"time"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
)

func BusyForUser(ctx context.Context, db *storage.DB, userID int64, start, end time.Time) ([]icsutil.BusyPeriod, error) {
	cals, err := db.ListCalendars(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out []icsutil.BusyPeriod
	for i := range cals {
		c := &cals[i]
		if c.Shared || c.Kind != "personal" {
			continue
		}
		periods, err := BusyForCalendar(ctx, db, c.ID, start, end)
		if err != nil {
			return nil, err
		}
		out = append(out, periods...)
	}
	return out, nil
}

func BusyForCalendar(ctx context.Context, db *storage.DB, calendarID int64, start, end time.Time) ([]icsutil.BusyPeriod, error) {
	list, err := db.ListCalendarObjectsByComponent(ctx, calendarID, "VEVENT")
	if err != nil {
		return nil, err
	}
	var out []icsutil.BusyPeriod
	for _, o := range list {
		out = append(out, icsutil.BusyPeriods(o.ICS, start, end)...)
	}
	return out, nil
}

func freeBusyOutbox(ctx context.Context, db *storage.DB, user *storage.User, raw string) ([]OutboxResult, error) {
	cal, err := icsutil.ParseCalendar(raw)
	if err != nil {
		return nil, err
	}
	ds, de := icsutil.CalendarRange(cal)
	start, _ := icsutil.ParseICSTime(ds)
	end, _ := icsutil.ParseICSTime(de)
	if start.IsZero() {
		start = time.Now().UTC()
	}
	if end.IsZero() || !end.After(start) {
		end = start.Add(7 * 24 * time.Hour)
	}
	atts := icsutil.Attendees(raw)
	query := func(u *storage.User) (OutboxResult, error) {
		periods, err := BusyForUser(ctx, db, u.ID, start, end)
		if err != nil {
			return OutboxResult{}, err
		}
		uid := icsutil.CalendarUID(cal)
		if uid == "" {
			uid = "fb-" + u.Username
		}
		return OutboxResult{
			Recipient:    icsutil.Mailto(LocalMailbox(u.Username)),
			Status:       "2.0;Success",
			CalendarData: icsutil.EncodeFreeBusy(uid, LocalMailbox(u.Username), start, end, periods),
		}, nil
	}
	if len(atts) == 0 {
		r, err := query(user)
		if err != nil {
			return nil, err
		}
		return []OutboxResult{r}, nil
	}
	out := make([]OutboxResult, 0, len(atts))
	for _, a := range atts {
		u, err := FindUser(ctx, db, a.Value)
		if err != nil {
			out = append(out, OutboxResult{Recipient: a.Value, Status: "3.7;Invalid calendar user"})
			continue
		}
		r, err := query(u)
		if err != nil {
			return nil, err
		}
		r.Recipient = icsutil.Mailto(a.Value)
		out = append(out, r)
	}
	return out, nil
}
