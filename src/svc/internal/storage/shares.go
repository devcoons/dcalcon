package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type CalendarShare struct {
	ID          int64  `json:"id"`
	CalendarID  int64  `json:"calendar_id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Access      string `json:"access"`
	CreatedAt   string `json:"created_at"`
}

func (db *DB) listSharedCalendars(ctx context.Context, granteeID int64) ([]Calendar, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT c.id, c.user_id, c.slug, c.name, c.description, c.color, c.timezone, c.kind,
		       c.read_only, c.ctag, c.sync_token, u.username, s.access
		FROM calendar_shares s
		JOIN calendars c ON c.id = s.calendar_id
		JOIN users u ON u.id = c.user_id
		WHERE s.grantee_user_id = ? AND c.kind NOT IN ('inbox', 'outbox')
		ORDER BY u.username, c.name`, granteeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Calendar
	for rows.Next() {
		c, err := scanSharedCalendar(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (db *DB) sharedCalendar(ctx context.Context, granteeID, calendarID int64) (*Calendar, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT c.id, c.user_id, c.slug, c.name, c.description, c.color, c.timezone, c.kind,
		       c.read_only, c.ctag, c.sync_token, u.username, s.access
		FROM calendar_shares s
		JOIN calendars c ON c.id = s.calendar_id
		JOIN users u ON u.id = c.user_id
		WHERE s.grantee_user_id = ? AND s.calendar_id = ? AND c.kind NOT IN ('inbox', 'outbox')`,
		granteeID, calendarID)
	c, err := scanSharedCalendar(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func scanSharedCalendar(scan func(dest ...any) error) (*Calendar, error) {
	c := &Calendar{}
	var ro int
	if err := scan(&c.ID, &c.UserID, &c.Slug, &c.Name, &c.Description, &c.Color, &c.Timezone, &c.Kind, &ro, &c.CTag, &c.SyncToken, &c.OwnerUsername, &c.Access); err != nil {
		return nil, err
	}
	c.Shared = true
	if c.Access == "read" || ro == 1 {
		c.ReadOnly = true
	}
	return c, nil
}

func (db *DB) ListShares(ctx context.Context, calendarID int64) ([]CalendarShare, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT s.id, s.calendar_id, s.grantee_user_id, u.username, u.display_name, s.access, s.created_at
		FROM calendar_shares s
		JOIN users u ON u.id = s.grantee_user_id
		WHERE s.calendar_id = ?
		ORDER BY u.username`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CalendarShare
	for rows.Next() {
		var s CalendarShare
		if err := rows.Scan(&s.ID, &s.CalendarID, &s.UserID, &s.Username, &s.DisplayName, &s.Access, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) UpsertShare(ctx context.Context, calendarID, granteeID int64, access string) error {
	access = strings.ToLower(strings.TrimSpace(access))
	if access != "read" && access != "write" {
		return errors.New("access must be read or write")
	}
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO calendar_shares (calendar_id, grantee_user_id, access)
		VALUES (?, ?, ?)
		ON CONFLICT(calendar_id, grantee_user_id) DO UPDATE SET access = excluded.access`,
		calendarID, granteeID, access)
	return err
}

type ShareGrant struct {
	GranteeID int64
	Access    string
}

func (db *DB) ReplaceShares(ctx context.Context, calendarID int64, grants []ShareGrant) error {
	cur, err := db.ListShares(ctx, calendarID)
	if err != nil {
		return err
	}
	want := map[int64]string{}
	for _, g := range grants {
		access := strings.ToLower(strings.TrimSpace(g.Access))
		if access != "read" && access != "write" {
			continue
		}
		if g.GranteeID < 1 {
			continue
		}
		want[g.GranteeID] = access
	}
	for _, s := range cur {
		if _, ok := want[s.UserID]; !ok {
			if err := db.DeleteShare(ctx, calendarID, s.UserID); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}
	for id, access := range want {
		if err := db.UpsertShare(ctx, calendarID, id, access); err != nil {
			return err
		}
	}
	return db.bumpCalendar(ctx, calendarID)
}

func (db *DB) DeleteShare(ctx context.Context, calendarID, granteeID int64) error {
	res, err := db.conn(ctx).ExecContext(ctx, `
		DELETE FROM calendar_shares WHERE calendar_id = ? AND grantee_user_id = ?`, calendarID, granteeID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
