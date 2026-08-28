package schedule

import (
	"context"
	"errors"
	"strings"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
)

const PeopleBookSlug = "people"

func PeopleHref(username string) string {
	return strings.ToLower(strings.TrimSpace(username)) + ".vcf"
}

func PeopleUID(username string) string {
	return "dcalcon-user-" + strings.ToLower(strings.TrimSpace(username))
}

func PeopleCard(u storage.DirectoryUser) (string, string, error) {
	fn := strings.TrimSpace(u.DisplayName)
	if fn == "" {
		fn = u.Username
	}
	raw, err := icsutil.EncodeContact(PeopleUID(u.Username), icsutil.ContactInput{
		FN: fn,
		Emails: []icsutil.TypedValue{{
			Value: LocalMailbox(u.Username),
			Type:  "work",
		}},
		Note:       "dCalCon user on this server. Invite this address from any calendar app.",
		Categories: "dCalCon",
		Custom:     []icsutil.CustomField{{Name: "X-DCALCON-USERNAME", Value: u.Username}},
	}, "")
	if err != nil {
		return "", "", err
	}
	return raw, fn, nil
}

func RefreshPeopleBook(ctx context.Context, db *storage.DB, ownerID int64) error {
	if err := db.EnsurePeopleBook(ctx, ownerID); err != nil {
		return err
	}
	book, err := db.AddressBookBySlug(ctx, ownerID, PeopleBookSlug)
	if err != nil {
		return err
	}
	users, err := db.Directory(ctx, ownerID)
	if err != nil {
		return err
	}
	want := map[string]struct {
		raw, fn, uid string
	}{}
	for _, u := range users {
		raw, fn, err := PeopleCard(u)
		if err != nil {
			return err
		}
		want[PeopleHref(u.Username)] = struct {
			raw, fn, uid string
		}{raw: raw, fn: fn, uid: PeopleUID(u.Username)}
	}
	existing, err := db.ListAddressObjects(ctx, book.ID)
	if err != nil {
		return err
	}
	have := map[string]storage.AddressObject{}
	for _, o := range existing {
		have[o.Href] = o
	}
	for href, card := range want {
		if o, ok := have[href]; ok && o.VCard == card.raw {
			continue
		}
		etag := icsutil.ETag(card.raw)
		if err := db.UpsertAddressObject(ctx, book.ID, href, card.uid, etag, card.raw, card.fn, "", ""); err != nil {
			return err
		}
	}
	for href := range have {
		if _, ok := want[href]; ok {
			continue
		}
		if err := db.DeleteAddressObject(ctx, book.ID, href); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
	}
	return nil
}
