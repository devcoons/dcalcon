package userbackup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/storage"
)

func Build(ctx context.Context, db *storage.DB, userID int64, kind string, w io.Writer) error {
	kind, err := NormalizeKind(kind)
	if err != nil {
		return err
	}
	u, err := db.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	defer zw.Close()

	man := Manifest{
		Format:     Format,
		Version:    Version,
		Kind:       kind,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Username:   u.Username,
	}
	if err := writeJSON(zw, "dcalcon.json", man); err != nil {
		return err
	}

	dates, err := db.GetImportantDates(ctx, userID)
	if err != nil {
		return err
	}
	if dates == nil {
		dates = &storage.ImportantDatesSettings{IncludeBirthdays: true, IncludeAnniversaries: true, AlarmOffsets: []string{"-P1D"}}
	}
	if err := writeJSON(zw, "settings/important-dates.json", dates); err != nil {
		return err
	}

	cals, err := db.ListCalendars(ctx, userID)
	if err != nil {
		return err
	}
	for _, c := range cals {
		if c.Shared || skipCalendarKind(c.Kind) {
			continue
		}
		if err := writeCalendar(ctx, db, zw, userID, c, kind); err != nil {
			return err
		}
	}

	books, err := db.ListAddressBooks(ctx, userID)
	if err != nil {
		return err
	}
	for _, b := range books {
		if b.Slug == "people" || b.ReadOnly {
			continue
		}
		if err := writeBook(ctx, db, zw, b); err != nil {
			return err
		}
	}

	if kind == KindFull {
		acc, err := buildAccount(ctx, db, u)
		if err != nil {
			return err
		}
		if err := writeJSON(zw, "account.json", acc); err != nil {
			return err
		}
	}
	return nil
}

func writeCalendar(ctx context.Context, db *storage.DB, zw *zip.Writer, userID int64, c storage.Calendar, kind string) error {
	root := "calendars/" + davpath.ZipSegment(c.Slug)
	meta := CalendarMeta{
		Slug:        c.Slug,
		Name:        c.Name,
		Description: c.Description,
		Color:       c.Color,
		Kind:        c.Kind,
		Timezone:    c.Timezone,
		Items:       []ObjectRef{},
		Files:       []FileRef{},
	}
	if kind == KindFull {
		if tok, err := db.WebcalForCalendar(ctx, userID, c.ID); err == nil {
			meta.WebcalToken = tok.Token
		}
	}
	objs, err := db.ListCalendarObjects(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, o := range objs {
		rel := "objects/" + davpath.ZipSegment(o.Href)
		if err := writeBytes(zw, root+"/"+rel, []byte(o.ICS)); err != nil {
			return err
		}
		meta.Items = append(meta.Items, ObjectRef{
			Href:      o.Href,
			UID:       o.UID,
			Component: o.Component,
			File:      rel,
		})
		atts, err := db.ListAttachments(ctx, c.ID, o.Href)
		if err != nil {
			return err
		}
		for _, metaAtt := range atts {
			full, err := db.AttachmentByPublicID(ctx, metaAtt.PublicID)
			if err != nil {
				continue
			}
			frel := "files/" + davpath.ZipSegment(full.PublicID)
			if err := writeBytes(zw, root+"/"+frel, full.Data); err != nil {
				return err
			}
			meta.Files = append(meta.Files, FileRef{
				Href:        o.Href,
				PublicID:    full.PublicID,
				Filename:    full.Filename,
				ContentType: full.ContentType,
				File:        frel,
			})
		}
	}
	return writeJSON(zw, root+"/calendar.json", meta)
}

func writeBook(ctx context.Context, db *storage.DB, zw *zip.Writer, b storage.AddressBook) error {
	root := "contacts/" + davpath.ZipSegment(b.Slug)
	meta := BookMeta{
		Slug:        b.Slug,
		Name:        b.Name,
		Description: b.Description,
		Items:       []ObjectRef{},
	}
	objs, err := db.ListAddressObjects(ctx, b.ID)
	if err != nil {
		return err
	}
	for _, o := range objs {
		rel := "objects/" + davpath.ZipSegment(o.Href)
		if err := writeBytes(zw, root+"/"+rel, []byte(o.VCard)); err != nil {
			return err
		}
		meta.Items = append(meta.Items, ObjectRef{Href: o.Href, UID: o.UID, File: rel})
	}
	return writeJSON(zw, root+"/book.json", meta)
}

func buildAccount(ctx context.Context, db *storage.DB, u *storage.User) (Account, error) {
	hash, err := db.PasswordHash(ctx, u.ID)
	if err != nil {
		return Account{}, err
	}
	secret, _, enabled, err := db.TOTPState(ctx, u.ID)
	if err != nil {
		return Account{}, err
	}
	apps, err := db.ListAppPasswordBackups(ctx, u.ID)
	if err != nil {
		return Account{}, err
	}
	accounts, err := db.ListConnectedAccountBackups(ctx, u.ID)
	if err != nil {
		return Account{}, err
	}
	cals, err := db.ListCalendars(ctx, u.ID)
	if err != nil {
		return Account{}, err
	}
	var shares []ShareBackup
	for _, c := range cals {
		if c.Shared || skipCalendarKind(c.Kind) || !c.IsOwner() {
			continue
		}
		list, err := db.ListShares(ctx, c.ID)
		if err != nil {
			return Account{}, err
		}
		for _, s := range list {
			shares = append(shares, ShareBackup{CalendarSlug: c.Slug, Grantee: s.Username, Access: s.Access})
		}
	}
	if shares == nil {
		shares = []ShareBackup{}
	}
	return Account{
		Username:          u.Username,
		Email:             u.Email,
		DisplayName:       u.DisplayName,
		Timezone:          u.Timezone,
		PasswordHash:      hash,
		TOTPEnabled:       enabled,
		TOTPSecret:        secret,
		AppPasswords:      apps,
		ConnectedAccounts: accounts,
		Shares:            shares,
	}, nil
}

func writeJSON(zw *zip.Writer, name string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeBytes(zw, name, raw)
}

func writeBytes(zw *zip.Writer, name string, data []byte) error {
	name = strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/")
	if unsafeZipName(name) {
		return fmt.Errorf("refusing to write %s", name)
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
