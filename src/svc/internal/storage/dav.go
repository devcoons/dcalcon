package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Calendar struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Color         string `json:"color"`
	Timezone      string `json:"timezone"`
	Kind          string `json:"kind"`
	ReadOnly      bool   `json:"read_only"`
	CTag          int64  `json:"ctag"`
	SyncToken     string `json:"sync_token"`
	Path          string `json:"path"`
	OwnerUsername string `json:"owner_username,omitempty"`
	Shared        bool   `json:"shared"`
	Access        string `json:"access"`
}

func (c Calendar) CanWrite() bool {
	if c.ReadOnly || c.Kind == "inbox" || c.Kind == "outbox" || c.Kind == "important_dates" {
		return false
	}
	if c.Shared && c.Access != "write" {
		return false
	}
	if c.Access == "read" {
		return false
	}
	return true
}

func (c Calendar) IsOwner() bool {
	return !c.Shared && (c.Access == "owner" || c.Access == "")
}

const shareSlugPrefix = "x-share-"

func ShareSlug(calendarID int64) string {
	return shareSlugPrefix + strconv.FormatInt(calendarID, 10)
}

func ParseShareSlug(slug string) (int64, bool) {
	if !strings.HasPrefix(slug, shareSlugPrefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(slug, shareSlugPrefix), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

type CalendarObject struct {
	ID         int64
	CalendarID int64
	Href       string
	UID        string
	ETag       string
	Component  string
	ICS        string
	DTStart    string
	DTEnd      string
	Summary    string
	UpdatedAt  time.Time
}

type AddressBook struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"read_only"`
	CTag        int64  `json:"ctag"`
	SyncToken   string `json:"sync_token"`
	Path        string `json:"path"`
}

type AddressObject struct {
	ID            int64
	AddressBookID int64
	Href          string
	UID           string
	ETag          string
	VCard         string
	FN            string
	BDAY          string
	Anniversary   string
	UpdatedAt     time.Time
}

func scanCalendar(scan func(dest ...any) error) (*Calendar, error) {
	c := &Calendar{}
	var ro int
	if err := scan(&c.ID, &c.UserID, &c.Slug, &c.Name, &c.Description, &c.Color, &c.Timezone, &c.Kind, &ro, &c.CTag, &c.SyncToken); err != nil {
		return nil, err
	}
	c.ReadOnly = ro == 1
	if c.Access == "" {
		c.Access = "owner"
	}
	return c, nil
}

func (db *DB) ListCalendars(ctx context.Context, userID int64) ([]Calendar, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, user_id, slug, name, description, color, timezone, kind, read_only, ctag, sync_token
		FROM calendars WHERE user_id = ? ORDER BY CASE kind WHEN 'personal' THEN 0 WHEN 'important_dates' THEN 1 ELSE 2 END, name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Calendar
	for rows.Next() {
		c, err := scanCalendar(rows.Scan)
		if err != nil {
			return nil, err
		}
		c.Access = "owner"
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	shared, err := db.listSharedCalendars(ctx, userID)
	if err != nil {
		return nil, err
	}
	return append(out, shared...), nil
}

func (db *DB) CalendarBySlug(ctx context.Context, userID int64, slug string) (*Calendar, error) {
	if id, ok := ParseShareSlug(slug); ok {
		c, err := db.sharedCalendar(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		c.Slug = slug
		return c, nil
	}
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, user_id, slug, name, description, color, timezone, kind, read_only, ctag, sync_token
		FROM calendars WHERE user_id = ? AND slug = ?`, userID, slug)
	c, err := scanCalendar(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Access = "owner"
	return c, nil
}

func (db *DB) CalendarByID(ctx context.Context, userID, id int64) (*Calendar, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, user_id, slug, name, description, color, timezone, kind, read_only, ctag, sync_token
		FROM calendars WHERE user_id = ? AND id = ?`, userID, id)
	c, err := scanCalendar(row.Scan)
	if err == nil {
		c.Access = "owner"
		return c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return db.sharedCalendar(ctx, userID, id)
}

func (db *DB) CreateCalendar(ctx context.Context, userID int64, slug, name, description, color, kind string, readOnly bool) (*Calendar, error) {
	if kind == "" {
		kind = "personal"
	}
	if color == "" {
		color = "#E72625"
	}
	if _, ok := ParseShareSlug(slug); ok {
		return nil, fmt.Errorf("reserved calendar slug")
	}
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO calendars (user_id, slug, name, description, color, kind, read_only)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, userID, slug, name, description, color, kind, boolInt(readOnly))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return db.CalendarBySlug(ctx, userID, slug)
}

func (db *DB) EnsureCalendar(ctx context.Context, userID int64, slug, name, kind string, readOnly bool) (*Calendar, error) {
	c, err := db.CalendarBySlug(ctx, userID, slug)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return db.CreateCalendar(ctx, userID, slug, name, "", "#B45309", kind, readOnly)
}

func (db *DB) bumpCalendar(ctx context.Context, id int64) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE calendars SET ctag = ctag + 1, sync_token = CAST(ctag + 1 AS TEXT), updated_at = ?
		WHERE id = ?`, nowUTC(), id)
	return err
}

func (db *DB) ListCalendarObjects(ctx context.Context, calendarID int64) ([]CalendarObject, error) {
	return db.listCalendarObjects(ctx, calendarID, "")
}

func (db *DB) ListCalendarObjectsByComponent(ctx context.Context, calendarID int64, component string) ([]CalendarObject, error) {
	return db.listCalendarObjects(ctx, calendarID, component)
}

type ObjectRef struct {
	Href string
	ETag string
}

func (db *DB) ListCalendarObjectRefs(ctx context.Context, calendarID int64, hrefPrefix string) ([]ObjectRef, error) {
	q := `SELECT href, etag FROM calendar_objects WHERE calendar_id = ?`
	args := []any{calendarID}
	if hrefPrefix != "" {
		q += ` AND href LIKE ?`
		args = append(args, hrefPrefix+"%")
	}
	rows, err := db.conn(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanObjectRefs(rows)
}

func (db *DB) ListAddressObjectRefs(ctx context.Context, bookID int64) ([]ObjectRef, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `SELECT href, etag FROM address_objects WHERE addressbook_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	return scanObjectRefs(rows)
}

func scanObjectRefs(rows *sql.Rows) ([]ObjectRef, error) {
	defer rows.Close()
	var out []ObjectRef
	for rows.Next() {
		var r ObjectRef
		if err := rows.Scan(&r.Href, &r.ETag); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) listCalendarObjects(ctx context.Context, calendarID int64, component string) ([]CalendarObject, error) {
	q := `
		SELECT id, calendar_id, href, uid, etag, component, ics, COALESCE(dtstart,''), COALESCE(dtend,''), COALESCE(summary,''), updated_at
		FROM calendar_objects WHERE calendar_id = ?`
	args := []any{calendarID}
	if component != "" {
		q += ` AND component = ?`
		args = append(args, strings.ToUpper(strings.TrimSpace(component)))
	}
	q += ` ORDER BY dtstart, id`
	rows, err := db.conn(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CalendarObject
	for rows.Next() {
		o, err := scanCalObject(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (db *DB) CalendarObjectByHref(ctx context.Context, calendarID int64, href string) (*CalendarObject, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, calendar_id, href, uid, etag, component, ics, COALESCE(dtstart,''), COALESCE(dtend,''), COALESCE(summary,''), updated_at
		FROM calendar_objects WHERE calendar_id = ? AND href = ?`, calendarID, href)
	o, err := scanCalObject(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

func (db *DB) CalendarObjectByUID(ctx context.Context, calendarID int64, uid string) (*CalendarObject, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, calendar_id, href, uid, etag, component, ics, COALESCE(dtstart,''), COALESCE(dtend,''), COALESCE(summary,''), updated_at
		FROM calendar_objects WHERE calendar_id = ? AND uid = ? LIMIT 1`, calendarID, uid)
	o, err := scanCalObject(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

func scanCalObject(scan func(dest ...any) error) (*CalendarObject, error) {
	o := &CalendarObject{}
	var updated string
	if err := scan(&o.ID, &o.CalendarID, &o.Href, &o.UID, &o.ETag, &o.Component, &o.ICS, &o.DTStart, &o.DTEnd, &o.Summary, &updated); err != nil {
		return nil, err
	}
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", updated); err == nil {
		o.UpdatedAt = t
	} else {
		o.UpdatedAt = time.Now().UTC()
	}
	return o, nil
}

func (db *DB) UpsertCalendarObject(ctx context.Context, calendarID int64, href, uid, etag, component, ics, dtstart, dtend, summary string) error {
	if uid != "" {
		if other, err := db.CalendarObjectByUID(ctx, calendarID, uid); err == nil && other.Href != href {
			if err := db.DeleteCalendarObject(ctx, calendarID, other.Href); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO calendar_objects (calendar_id, href, uid, etag, component, ics, dtstart, dtend, summary, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(calendar_id, href) DO UPDATE SET
			uid = excluded.uid, etag = excluded.etag, component = excluded.component, ics = excluded.ics,
			dtstart = excluded.dtstart, dtend = excluded.dtend, summary = excluded.summary, updated_at = excluded.updated_at`,
		calendarID, href, uid, etag, component, ics, dtstart, dtend, summary, nowUTC())
	if err != nil {
		return err
	}
	if err := db.recordChange(ctx, "calendar", calendarID, href, false); err != nil {
		return err
	}
	return db.bumpCalendar(ctx, calendarID)
}

func (db *DB) DeleteCalendarObject(ctx context.Context, calendarID int64, href string) error {
	res, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM calendar_objects WHERE calendar_id = ? AND href = ?`, calendarID, href)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := db.recordChange(ctx, "calendar", calendarID, href, true); err != nil {
		return err
	}
	return db.bumpCalendar(ctx, calendarID)
}

func (db *DB) DeleteCalendarObjectsByPrefix(ctx context.Context, calendarID int64, hrefPrefix string) error {
	rows, err := db.conn(ctx).QueryContext(ctx, `SELECT href FROM calendar_objects WHERE calendar_id = ? AND href LIKE ?`, calendarID, hrefPrefix+"%")
	if err != nil {
		return err
	}
	var hrefs []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return err
		}
		hrefs = append(hrefs, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(hrefs) == 0 {
		return nil
	}
	_, err = db.conn(ctx).ExecContext(ctx, `DELETE FROM calendar_objects WHERE calendar_id = ? AND href LIKE ?`, calendarID, hrefPrefix+"%")
	if err != nil {
		return err
	}
	for _, h := range hrefs {
		if err := db.recordChange(ctx, "calendar", calendarID, h, true); err != nil {
			return err
		}
	}
	return db.bumpCalendar(ctx, calendarID)
}

func (db *DB) PatchCalendar(ctx context.Context, userID int64, slug string, name, description, color *string) error {
	c, err := db.CalendarBySlug(ctx, userID, slug)
	if err != nil {
		return err
	}
	n, d, col := c.Name, c.Description, c.Color
	if name != nil && strings.TrimSpace(*name) != "" {
		n = strings.TrimSpace(*name)
	}
	if description != nil {
		d = *description
	}
	if color != nil && strings.TrimSpace(*color) != "" {
		col = strings.TrimSpace(*color)
	}
	return db.writeCalendarMeta(ctx, c.ID, n, d, col)
}

func (db *DB) PatchCalendarMeta(ctx context.Context, id int64, name, description, color string) error {
	c, err := db.calendarByIDOnly(ctx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) != "" {
		c.Name = strings.TrimSpace(name)
	}
	c.Description = description
	if strings.TrimSpace(color) != "" {
		c.Color = strings.TrimSpace(color)
	}
	return db.writeCalendarMeta(ctx, c.ID, c.Name, c.Description, c.Color)
}

func (db *DB) writeCalendarMeta(ctx context.Context, id int64, name, description, color string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE calendars SET name = ?, description = ?, color = ?, updated_at = ? WHERE id = ?`,
		name, description, color, nowUTC(), id)
	return err
}

func (db *DB) CountPersonalCalendars(ctx context.Context, userID int64) (int, error) {
	var n int
	err := db.conn(ctx).QueryRowContext(ctx, `
		SELECT COUNT(1) FROM calendars WHERE user_id = ? AND kind = 'personal'`, userID).Scan(&n)
	return n, err
}

func (db *DB) DeleteCalendar(ctx context.Context, userID, id int64) error {
	res, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM calendars WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) CountContacts(ctx context.Context, userID int64) (int, error) {
	var n int
	err := db.conn(ctx).QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM address_objects o
		JOIN addressbooks b ON b.id = o.addressbook_id
		WHERE b.user_id = ? AND b.slug != 'people'`, userID).Scan(&n)
	return n, err
}

func (db *DB) CountPendingInbox(ctx context.Context, userID int64) (int, error) {
	var n int
	err := db.conn(ctx).QueryRowContext(ctx, `
		SELECT COUNT(1) FROM schedule_items WHERE user_id = ? AND collection = 'inbox' AND status = 'pending'`, userID).Scan(&n)
	return n, err
}

func (db *DB) ClosePendingInbox(ctx context.Context, userID int64, uid, status string) error {
	if status == "" {
		status = "cancelled"
	}
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE schedule_items SET status = ?, updated_at = ?
		WHERE user_id = ? AND uid = ? AND collection = 'inbox' AND status = 'pending'`,
		status, nowUTC(), userID, uid)
	return err
}

func (db *DB) calendarByIDOnly(ctx context.Context, id int64) (*Calendar, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, user_id, slug, name, description, color, timezone, kind, read_only, ctag, sync_token
		FROM calendars WHERE id = ?`, id)
	c, err := scanCalendar(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func scanAB(scan func(dest ...any) error) (*AddressBook, error) {
	a := &AddressBook{}
	var ro int
	if err := scan(&a.ID, &a.UserID, &a.Slug, &a.Name, &a.Description, &ro, &a.CTag, &a.SyncToken); err != nil {
		return nil, err
	}
	a.ReadOnly = ro == 1
	return a, nil
}

func (db *DB) ListAddressBooks(ctx context.Context, userID int64) ([]AddressBook, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, user_id, slug, name, description, read_only, ctag, sync_token
		FROM addressbooks WHERE user_id = ?
		ORDER BY CASE slug WHEN 'contacts' THEN 0 WHEN 'people' THEN 2 ELSE 1 END, name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AddressBook
	for rows.Next() {
		a, err := scanAB(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (db *DB) AddressBookBySlug(ctx context.Context, userID int64, slug string) (*AddressBook, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, user_id, slug, name, description, read_only, ctag, sync_token
		FROM addressbooks WHERE user_id = ? AND slug = ?`, userID, slug)
	a, err := scanAB(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (db *DB) AddressBookByID(ctx context.Context, userID, id int64) (*AddressBook, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, user_id, slug, name, description, read_only, ctag, sync_token
		FROM addressbooks WHERE user_id = ? AND id = ?`, userID, id)
	a, err := scanAB(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (db *DB) bumpAddressBook(ctx context.Context, id int64) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE addressbooks SET ctag = ctag + 1, sync_token = CAST(ctag + 1 AS TEXT), updated_at = ?
		WHERE id = ?`, nowUTC(), id)
	return err
}

func (db *DB) ListAddressObjects(ctx context.Context, bookID int64) ([]AddressObject, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, addressbook_id, href, uid, etag, vcard, COALESCE(fn,''), COALESCE(bday,''), COALESCE(anniversary,''), updated_at
		FROM address_objects WHERE addressbook_id = ? ORDER BY fn COLLATE NOCASE, id`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AddressObject
	for rows.Next() {
		o, err := scanAddrObject(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (db *DB) AddressObjectByHref(ctx context.Context, bookID int64, href string) (*AddressObject, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, addressbook_id, href, uid, etag, vcard, COALESCE(fn,''), COALESCE(bday,''), COALESCE(anniversary,''), updated_at
		FROM address_objects WHERE addressbook_id = ? AND href = ?`, bookID, href)
	o, err := scanAddrObject(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

func (db *DB) AddressObjectByUID(ctx context.Context, bookID int64, uid string) (*AddressObject, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, ErrNotFound
	}
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, addressbook_id, href, uid, etag, vcard, COALESCE(fn,''), COALESCE(bday,''), COALESCE(anniversary,''), updated_at
		FROM address_objects WHERE addressbook_id = ? AND uid = ? ORDER BY id LIMIT 1`, bookID, uid)
	o, err := scanAddrObject(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

func scanAddrObject(scan func(dest ...any) error) (*AddressObject, error) {
	o := &AddressObject{}
	var updated string
	if err := scan(&o.ID, &o.AddressBookID, &o.Href, &o.UID, &o.ETag, &o.VCard, &o.FN, &o.BDAY, &o.Anniversary, &updated); err != nil {
		return nil, err
	}
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", updated); err == nil {
		o.UpdatedAt = t
	} else {
		o.UpdatedAt = time.Now().UTC()
	}
	return o, nil
}

func (db *DB) UpsertAddressObject(ctx context.Context, bookID int64, href, uid, etag, vcard, fn, bday, anniversary string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO address_objects (addressbook_id, href, uid, etag, vcard, fn, bday, anniversary, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(addressbook_id, href) DO UPDATE SET
			uid = excluded.uid, etag = excluded.etag, vcard = excluded.vcard,
			fn = excluded.fn, bday = excluded.bday, anniversary = excluded.anniversary, updated_at = excluded.updated_at`,
		bookID, href, uid, etag, vcard, fn, bday, anniversary, nowUTC())
	if err != nil {
		return err
	}
	if err := db.recordChange(ctx, "addressbook", bookID, href, false); err != nil {
		return err
	}
	return db.bumpAddressBook(ctx, bookID)
}

func (db *DB) DeleteAddressObject(ctx context.Context, bookID int64, href string) error {
	res, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM address_objects WHERE addressbook_id = ? AND href = ?`, bookID, href)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := db.recordChange(ctx, "addressbook", bookID, href, true); err != nil {
		return err
	}
	return db.bumpAddressBook(ctx, bookID)
}

func (db *DB) ContactsWithDates(ctx context.Context, userID int64) ([]AddressObject, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT o.id, o.addressbook_id, o.href, o.uid, o.etag, o.vcard, COALESCE(o.fn,''), COALESCE(o.bday,''), COALESCE(o.anniversary,''), o.updated_at
		FROM address_objects o
		JOIN addressbooks a ON a.id = o.addressbook_id
		WHERE a.user_id = ? AND (o.bday IS NOT NULL AND o.bday != '' OR o.anniversary IS NOT NULL AND o.anniversary != '')`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AddressObject
	for rows.Next() {
		o, err := scanAddrObject(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

type ScheduleItem struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Collection string `json:"collection"`
	Method     string `json:"method"`
	UID        string `json:"uid"`
	Organizer  string `json:"organizer"`
	Attendee   string `json:"attendee"`
	Status     string `json:"status"`
	ICS        string `json:"ics"`
	CreatedAt  string `json:"created_at"`
}

func (db *DB) ListPendingInbox(ctx context.Context, userID int64, limit int) ([]ScheduleItem, error) {
	q := `
		SELECT id, user_id, collection, method, uid, COALESCE(organizer,''), COALESCE(attendee,''), status, ics, created_at
		FROM schedule_items WHERE user_id = ? AND collection = 'inbox' AND status = 'pending' ORDER BY id DESC`
	args := []any{userID}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.conn(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanScheduleItems(rows)
}

func (db *DB) ListInbox(ctx context.Context, userID int64) ([]ScheduleItem, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, user_id, collection, method, uid, COALESCE(organizer,''), COALESCE(attendee,''), status, ics, created_at
		FROM schedule_items WHERE user_id = ? AND collection = 'inbox' ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanScheduleItems(rows)
}

func scanScheduleItems(rows *sql.Rows) ([]ScheduleItem, error) {
	defer rows.Close()
	var out []ScheduleItem
	for rows.Next() {
		var s ScheduleItem
		if err := rows.Scan(&s.ID, &s.UserID, &s.Collection, &s.Method, &s.UID, &s.Organizer, &s.Attendee, &s.Status, &s.ICS, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) GetScheduleItem(ctx context.Context, userID, id int64) (*ScheduleItem, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, user_id, collection, method, uid, COALESCE(organizer,''), COALESCE(attendee,''), status, ics, created_at
		FROM schedule_items WHERE user_id = ? AND id = ?`, userID, id)
	s := &ScheduleItem{}
	if err := row.Scan(&s.ID, &s.UserID, &s.Collection, &s.Method, &s.UID, &s.Organizer, &s.Attendee, &s.Status, &s.ICS, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (db *DB) InsertSchedule(ctx context.Context, userID int64, collection, method, uid, organizer, attendee, status, ics string) error {
	res, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE schedule_items SET ics = ?,
			status = CASE WHEN status IN ('accepted', 'declined') THEN status ELSE ? END,
			organizer = ?, updated_at = ?
		WHERE user_id = ? AND collection = ? AND method = ? AND uid = ? AND COALESCE(attendee,'') = ?`,
		ics, status, organizer, nowUTC(), userID, collection, method, uid, attendee)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	_, err = db.conn(ctx).ExecContext(ctx, `
		INSERT INTO schedule_items (user_id, collection, method, uid, organizer, attendee, status, ics)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, userID, collection, method, uid, organizer, attendee, status, ics)
	return err
}

func (db *DB) ScheduleByUID(ctx context.Context, userID int64, collection, method, uid, attendee string) (*ScheduleItem, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, user_id, collection, method, uid, COALESCE(organizer,''), COALESCE(attendee,''), status, ics, created_at
		FROM schedule_items
		WHERE user_id = ? AND collection = ? AND method = ? AND uid = ? AND COALESCE(attendee,'') = ?
		ORDER BY id DESC LIMIT 1`, userID, collection, method, uid, attendee)
	s := &ScheduleItem{}
	if err := row.Scan(&s.ID, &s.UserID, &s.Collection, &s.Method, &s.UID, &s.Organizer, &s.Attendee, &s.Status, &s.ICS, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (db *DB) DeleteObjectsByUID(ctx context.Context, calendarID int64, uid string) error {
	rows, err := db.conn(ctx).QueryContext(ctx, `SELECT href FROM calendar_objects WHERE calendar_id = ? AND uid = ?`, calendarID, uid)
	if err != nil {
		return err
	}
	var hrefs []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return err
		}
		hrefs = append(hrefs, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, h := range hrefs {
		if err := db.DeleteCalendarObject(ctx, calendarID, h); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

func (db *DB) FindOwnedObjectByUID(ctx context.Context, userID int64, uid string) (*Calendar, *CalendarObject, error) {
	var calID int64
	err := db.conn(ctx).QueryRowContext(ctx, `
		SELECT o.calendar_id
		FROM calendar_objects o
		JOIN calendars c ON c.id = o.calendar_id
		WHERE c.user_id = ? AND o.uid = ? AND c.kind NOT IN ('inbox', 'outbox')
		LIMIT 1`, userID, uid).Scan(&calID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	c, err := db.CalendarByID(ctx, userID, calID)
	if err != nil {
		return nil, nil, err
	}
	o, err := db.CalendarObjectByUID(ctx, calID, uid)
	if err != nil {
		return nil, nil, err
	}
	return c, o, nil
}

func (db *DB) SetScheduleStatus(ctx context.Context, userID, id int64, status string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE schedule_items SET status = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		status, nowUTC(), userID, id)
	return err
}

type ConnectedAccount struct {
	ID        int64  `json:"id"`
	Provider  string `json:"provider"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	Scopes    string `json:"scopes,omitempty"`
	LastError string `json:"last_error,omitempty"`
	Cipher    []byte `json:"-"`
	Nonce     []byte `json:"-"`
}

type DAVChange struct {
	ID      int64
	Href    string
	Deleted bool
}

func SyncToken(id int64) string {
	return fmt.Sprintf("http://dcalcon/ns/sync/%d", id)
}

func ParseSyncToken(token string) (int64, bool) {
	token = strings.TrimSpace(token)
	const prefix = "http://dcalcon/ns/sync/"
	if token == "" {
		return 0, true
	}
	if !strings.HasPrefix(token, prefix) {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(token, prefix), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func (db *DB) recordChange(ctx context.Context, kind string, collectionID int64, href string, deleted bool) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO dav_changes (kind, collection_id, href, deleted) VALUES (?, ?, ?, ?)`,
		kind, collectionID, href, boolInt(deleted))
	return err
}

func (db *DB) LatestChangeID(ctx context.Context, kind string, collectionID int64) (int64, error) {
	var id sql.NullInt64
	err := db.conn(ctx).QueryRowContext(ctx, `
		SELECT MAX(id) FROM dav_changes WHERE kind = ? AND collection_id = ?`, kind, collectionID).Scan(&id)
	if err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, nil
	}
	return id.Int64, nil
}

func (db *DB) ChangesSince(ctx context.Context, kind string, collectionID, since int64) ([]DAVChange, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, href, deleted FROM dav_changes
		WHERE kind = ? AND collection_id = ? AND id > ?
		ORDER BY id`, kind, collectionID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	last := map[string]DAVChange{}
	var order []string
	for rows.Next() {
		var c DAVChange
		var del int
		if err := rows.Scan(&c.ID, &c.Href, &del); err != nil {
			return nil, err
		}
		c.Deleted = del == 1
		if _, ok := last[c.Href]; !ok {
			order = append(order, c.Href)
		}
		last[c.Href] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]DAVChange, 0, len(order))
	for _, href := range order {
		out = append(out, last[href])
	}
	return out, nil
}
