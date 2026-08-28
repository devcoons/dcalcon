package userbackup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/google/uuid"
)

func Restore(ctx context.Context, db *storage.DB, userID int64, publicURL string, b *Bundle) (*Result, error) {
	if b == nil {
		return nil, ErrNotBackup
	}
	u, err := db.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if b.Manifest.Kind == KindFull {
		want := strings.TrimSpace(b.Manifest.Username)
		if want != "" && !strings.EqualFold(want, u.Username) {
			return nil, ErrUsername
		}
	}
	res := &Result{Kind: b.Manifest.Kind, Skipped: []string{}}
	err = db.WithTx(ctx, func(ctx context.Context) error {
		if err := db.SaveImportantDates(ctx, userID, b.Dates); err != nil {
			return err
		}
		if err := restoreCalendars(ctx, db, userID, publicURL, b, res); err != nil {
			return err
		}
		if err := restoreBooks(ctx, db, userID, b, res); err != nil {
			return err
		}
		if b.Manifest.Kind == KindFull && b.Account != nil {
			return restoreAccount(ctx, db, u, b.Account, res)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(res.Skipped) == 0 {
		res.Skipped = nil
	}
	return res, nil
}

func restoreCalendars(ctx context.Context, db *storage.DB, userID int64, publicURL string, b *Bundle, res *Result) error {
	if err := db.EnsureSchedulingCollections(ctx, userID); err != nil {
		return err
	}
	for _, cb := range b.Cals {
		kind := restorableCalendarKind(cb.Meta.Kind)
		if kind == "" {
			res.skip("calendar " + cb.Meta.Slug)
			continue
		}
		slug := strings.TrimSpace(cb.Meta.Slug)
		if !davpath.ValidSlug(slug) {
			res.skip("calendar slug")
			continue
		}
		cal, err := resolveCalendar(ctx, db, userID, slug, kind, cb.Meta)
		if err != nil {
			res.skip("calendar " + slug + ": " + err.Error())
			continue
		}
		if err := db.PatchCalendarMeta(ctx, cal.ID, cb.Meta.Name, cb.Meta.Description, cb.Meta.Color); err != nil {
			return err
		}
		_ = db.SetCalendarTimezone(ctx, cal.ID, cb.Meta.Timezone)
		existing, err := db.ListCalendarObjects(ctx, cal.ID)
		if err != nil {
			return err
		}
		for _, o := range existing {
			if err := db.DeleteCalendarObject(ctx, cal.ID, o.Href); err != nil && !errors.Is(err, storage.ErrNotFound) {
				return err
			}
		}
		for _, item := range cb.Meta.Items {
			ics, ok := cb.ICS[item.Href]
			if !ok {
				res.skip("missing object " + item.Href)
				continue
			}
			if err := restoreObject(ctx, db, cal.ID, publicURL, item, ics, cb.Files[item.Href], res); err != nil {
				res.skip(item.Href + ": " + err.Error())
			}
		}
		if b.Manifest.Kind == KindFull && cb.Meta.WebcalToken != "" {
			if err := db.PutWebcalToken(ctx, userID, cal.ID, cb.Meta.WebcalToken); err != nil {
				res.skip("webcal " + slug)
			}
		}
		res.Calendars++
	}
	return nil
}

func resolveCalendar(ctx context.Context, db *storage.DB, userID int64, slug, kind string, meta CalendarMeta) (*storage.Calendar, error) {
	if kind == "inbox" || kind == "outbox" {
		c, err := db.CalendarBySlug(ctx, userID, slug)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	c, err := db.CalendarBySlug(ctx, userID, slug)
	if err == nil {
		if restorableCalendarKind(c.Kind) != "personal" {
			return nil, fmt.Errorf("not a personal calendar")
		}
		return c, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = slug
	}
	return db.CreateCalendar(ctx, userID, slug, name, meta.Description, meta.Color, "personal", false)
}

func restoreObject(ctx context.Context, db *storage.DB, calendarID int64, publicURL string, item ObjectRef, ics string, files []fileBlob, res *Result) error {
	if err := davpath.CheckObjectHref(item.Href); err != nil {
		return err
	}
	if err := icsutil.CheckICSSize(ics); err != nil {
		return err
	}
	cal, err := icsutil.ParseCalendar(ics)
	if err != nil {
		return err
	}
	uid := strings.TrimSpace(item.UID)
	if uid == "" {
		uid = icsutil.CalendarUID(cal)
	}
	if uid == "" {
		uid = uuid.NewString()
	}
	comp := strings.TrimSpace(item.Component)
	if comp == "" {
		comp = icsutil.CalendarComponent(cal)
	}
	ds, de := icsutil.CalendarRange(cal)
	sum := icsutil.CalendarSummary(cal)
	split, err := icsutil.SplitAttachments(ics, calendarID)
	if err != nil {
		return err
	}
	if err := db.UpsertCalendarObject(ctx, calendarID, item.Href, uid, icsutil.ETag(split.ICS), comp, split.ICS, ds, de, sum); err != nil {
		return err
	}
	for _, f := range files {
		if _, err := db.RestoreAttachment(ctx, calendarID, item.Href, f.Ref.PublicID, f.Ref.Filename, f.Ref.ContentType, f.Data); err != nil {
			res.skip("file " + f.Ref.Filename)
			continue
		}
		res.Files++
	}
	for _, in := range split.Inlines {
		if _, err := db.InsertAttachment(ctx, calendarID, item.Href, in.Filename, in.ContentType, in.Data); err != nil {
			res.skip("inline file")
			continue
		}
		res.Files++
	}
	rewritten, err := db.RewriteManagedAttachments(ctx, calendarID, item.Href, publicURL, split.ICS)
	if err != nil {
		return err
	}
	if err := db.UpsertCalendarObject(ctx, calendarID, item.Href, uid, icsutil.ETag(rewritten), comp, rewritten, ds, de, sum); err != nil {
		return err
	}
	res.Objects++
	return nil
}

func restoreBooks(ctx context.Context, db *storage.DB, userID int64, b *Bundle, res *Result) error {
	for _, bb := range b.Books {
		slug := strings.TrimSpace(bb.Meta.Slug)
		if slug == "people" || !davpath.ValidSlug(slug) {
			res.skip("address book " + slug)
			continue
		}
		book, err := db.EnsureAddressBook(ctx, userID, slug, bb.Meta.Name, bb.Meta.Description)
		if err != nil {
			res.skip("address book " + slug)
			continue
		}
		if book.ReadOnly {
			res.skip("address book " + slug)
			continue
		}
		existing, err := db.ListAddressObjects(ctx, book.ID)
		if err != nil {
			return err
		}
		for _, o := range existing {
			if err := db.DeleteAddressObject(ctx, book.ID, o.Href); err != nil && !errors.Is(err, storage.ErrNotFound) {
				return err
			}
		}
		for _, item := range bb.Meta.Items {
			raw, ok := bb.Cards[item.Href]
			if !ok {
				continue
			}
			if err := restoreCard(ctx, db, book.ID, item, raw); err != nil {
				res.skip("contact " + item.Href)
				continue
			}
			res.Contacts++
		}
	}
	return nil
}

func restoreCard(ctx context.Context, db *storage.DB, bookID int64, item ObjectRef, raw string) error {
	if err := davpath.CheckObjectHref(item.Href); err != nil {
		return err
	}
	card, err := icsutil.ParseCard(raw)
	if err != nil {
		return err
	}
	if err := icsutil.CheckVCardSize(raw, card); err != nil {
		return err
	}
	uid := strings.TrimSpace(item.UID)
	if uid == "" {
		uid = icsutil.CardUID(card)
	}
	if uid == "" {
		uid = uuid.NewString()
	}
	return db.UpsertAddressObject(ctx, bookID, item.Href, uid, icsutil.ETag(raw), raw, icsutil.CardFN(card), icsutil.CardBDAY(card), icsutil.CardAnniversary(card))
}

func restoreAccount(ctx context.Context, db *storage.DB, u *storage.User, acc *Account, res *Result) error {
	email := strings.TrimSpace(acc.Email)
	if email == "" {
		email = u.Email
	} else if other, err := db.UserByEmail(ctx, email); err == nil && other.ID != u.ID {
		email = u.Email
		res.skip("email already in use")
	}
	display := strings.TrimSpace(acc.DisplayName)
	if display == "" {
		display = u.DisplayName
	}
	tz := strings.TrimSpace(acc.Timezone)
	if tz == "" {
		tz = u.Timezone
	}
	if err := db.UpdateProfile(ctx, u.ID, display, email, tz); err != nil {
		return err
	}
	if acc.PasswordHash != "" {
		if err := db.SetPasswordHash(ctx, u.ID, acc.PasswordHash); err != nil {
			return err
		}
	}
	if err := db.RestoreTOTP(ctx, u.ID, acc.TOTPSecret, acc.TOTPEnabled); err != nil {
		res.skip("authenticator")
	}
	if err := db.ReplaceAppPasswordBackups(ctx, u.ID, acc.AppPasswords); err != nil {
		return err
	}
	if err := db.ReplaceConnectedAccountBackups(ctx, u.ID, acc.ConnectedAccounts); err != nil {
		return err
	}
	return restoreSharesFromBackup(ctx, db, u.ID, acc.Shares, res)
}

func restoreSharesFromBackup(ctx context.Context, db *storage.DB, userID int64, shares []ShareBackup, res *Result) error {
	byCal := map[string][]storage.ShareGrant{}
	for _, s := range shares {
		slug := strings.TrimSpace(s.CalendarSlug)
		grantee, err := db.UserByUsername(ctx, s.Grantee)
		if err != nil {
			res.skip("share " + s.Grantee)
			continue
		}
		if grantee.ID == userID {
			continue
		}
		byCal[slug] = append(byCal[slug], storage.ShareGrant{GranteeID: grantee.ID, Access: s.Access})
	}
	for slug, grants := range byCal {
		c, err := db.CalendarBySlug(ctx, userID, slug)
		if err != nil || !c.IsOwner() {
			res.skip("share calendar " + slug)
			continue
		}
		if err := db.ReplaceShares(ctx, c.ID, grants); err != nil {
			res.skip("shares " + slug)
		}
	}
	return nil
}

func (r *Result) skip(msg string) {
	if r == nil {
		return
	}
	if len(r.Skipped) >= 40 {
		return
	}
	r.Skipped = append(r.Skipped, msg)
}
