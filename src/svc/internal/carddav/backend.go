package carddav

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/davcond"
	"github.com/devcoons/dcalcon/internal/davext"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	gocarddav "github.com/emersion/go-webdav/carddav"
)

type Backend struct {
	Store *storage.DB
}

func (b *Backend) principal(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, webdav.NewHTTPError(http.StatusUnauthorized, errors.New("unauthenticated"))
	}
	return p, nil
}

func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return "", err
	}
	return davpath.PrincipalPath(p.Username), nil
}

func (b *Backend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return "", err
	}
	return davpath.AddressBookHome(p.Username), nil
}

func (b *Backend) toAB(p auth.Principal, a storage.AddressBook) *gocarddav.AddressBook {
	return &gocarddav.AddressBook{
		Path:            davpath.AddressBookPath(p.Username, a.Slug),
		Name:            a.Name,
		Description:     a.Description,
		MaxResourceSize: 1 << 20,
		SupportedAddressData: []gocarddav.AddressDataType{
			{ContentType: "text/vcard", Version: "3.0"},
			{ContentType: "text/vcard", Version: "4.0"},
		},
	}
}

func (b *Backend) ListAddressBooks(ctx context.Context) ([]gocarddav.AddressBook, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	if err := b.Store.EnsurePeopleBook(ctx, p.ID); err != nil {
		return nil, err
	}
	list, err := b.Store.ListAddressBooks(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := make([]gocarddav.AddressBook, 0, len(list))
	for _, a := range list {
		out = append(out, *b.toAB(p, a))
	}
	return out, nil
}

func (b *Backend) refreshPeople(ctx context.Context, userID int64, slug string) error {
	if slug != schedule.PeopleBookSlug {
		return nil
	}
	return schedule.RefreshPeopleBook(ctx, b.Store, userID)
}

func (b *Backend) GetAddressBook(ctx context.Context, abPath string) (*gocarddav.AddressBook, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	slug := davpath.AddressBookSlug(abPath, p.Username)
	if err := b.refreshPeople(ctx, p.ID, slug); err != nil {
		return nil, err
	}
	a, err := b.Store.AddressBookBySlug(ctx, p.ID, slug)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusNotFound, err)
	}
	return b.toAB(p, *a), nil
}

func (b *Backend) CreateAddressBook(ctx context.Context, addressBook *gocarddav.AddressBook) error {
	p, err := b.principal(ctx)
	if err != nil {
		return err
	}
	_ = addressBook
	_ = p
	return webdav.NewHTTPError(http.StatusForbidden, errors.New("address book creation via CardDAV is not enabled yet"))
}

func (b *Backend) DeleteAddressBook(ctx context.Context, _ string) error {
	return webdav.NewHTTPError(http.StatusForbidden, errors.New("address book deletion via CardDAV is not enabled yet"))
}

func (b *Backend) bookForPath(ctx context.Context, p auth.Principal, objPath string) (*storage.AddressBook, string, error) {
	slug := davpath.AddressBookSlug(objPath, p.Username)
	if slug == "" {
		return nil, "", webdav.NewHTTPError(http.StatusNotFound, storage.ErrNotFound)
	}
	if err := b.refreshPeople(ctx, p.ID, slug); err != nil {
		return nil, "", err
	}
	a, err := b.Store.AddressBookBySlug(ctx, p.ID, slug)
	if err != nil {
		return nil, "", webdav.NewHTTPError(http.StatusNotFound, err)
	}
	href := davpath.ObjectHref(objPath, davpath.AddressBookPath(p.Username, a.Slug))
	if err := davpath.CheckObjectHref(href); err != nil {
		return nil, "", webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	return a, href, nil
}

func (b *Backend) toObject(p auth.Principal, a storage.AddressBook, o storage.AddressObject) (*gocarddav.AddressObject, error) {
	card, err := icsutil.ParseCard(o.VCard)
	if err != nil {
		return nil, err
	}
	return &gocarddav.AddressObject{
		Path:          davpath.ObjectPath(davpath.AddressBookPath(p.Username, a.Slug), o.Href),
		ModTime:       o.UpdatedAt,
		ContentLength: int64(len(o.VCard)),
		ETag:          o.ETag,
		Card:          card,
	}, nil
}

func (b *Backend) GetAddressObject(ctx context.Context, objPath string, _ *gocarddav.AddressDataRequest) (*gocarddav.AddressObject, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	a, href, err := b.bookForPath(ctx, p, objPath)
	if err != nil {
		return nil, err
	}
	o, err := b.Store.AddressObjectByHref(ctx, a.ID, href)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusNotFound, err)
	}
	return b.toObject(p, *a, *o)
}

func (b *Backend) ListAddressObjects(ctx context.Context, abPath string, _ *gocarddav.AddressDataRequest) ([]gocarddav.AddressObject, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	slug := davpath.AddressBookSlug(abPath, p.Username)
	if err := b.refreshPeople(ctx, p.ID, slug); err != nil {
		return nil, err
	}
	a, err := b.Store.AddressBookBySlug(ctx, p.ID, slug)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusNotFound, err)
	}
	list, err := b.Store.ListAddressObjects(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	out := make([]gocarddav.AddressObject, 0, len(list))
	for _, o := range list {
		ao, err := b.toObject(p, *a, o)
		if err != nil {
			continue
		}
		out = append(out, *ao)
	}
	return out, nil
}

func (b *Backend) QueryAddressObjects(ctx context.Context, abPath string, query *gocarddav.AddressBookQuery) ([]gocarddav.AddressObject, error) {
	list, err := b.ListAddressObjects(ctx, abPath, nil)
	if err != nil {
		return nil, err
	}
	if query == nil || len(query.PropFilters) == 0 {
		return list, nil
	}
	filtered := make([]gocarddav.AddressObject, 0, len(list))
	for _, o := range list {
		if matchCard(o.Card, query) {
			filtered = append(filtered, o)
		}
	}
	return filtered, nil
}

func matchCard(card vcard.Card, query *gocarddav.AddressBookQuery) bool {
	anyOf := query.FilterTest != gocarddav.FilterAllOf
	matched := 0
	for _, pf := range query.PropFilters {
		ok := false
		val := strings.ToLower(card.PreferredValue(pf.Name))
		if pf.IsNotDefined {
			ok = val == ""
		} else if len(pf.TextMatches) == 0 {
			ok = val != ""
		} else {
			for _, tm := range pf.TextMatches {
				if strings.Contains(val, strings.ToLower(tm.Text)) != tm.NegateCondition {
					ok = true
					break
				}
			}
		}
		if ok {
			matched++
			if anyOf {
				return true
			}
		} else if !anyOf {
			return false
		}
	}
	if anyOf {
		return matched > 0
	}
	return true
}

func (b *Backend) PutAddressObject(ctx context.Context, objPath string, card vcard.Card, opts *gocarddav.PutAddressObjectOptions) (*gocarddav.AddressObject, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	a, href, err := b.bookForPath(ctx, p, objPath)
	if err != nil {
		return nil, err
	}
	if a.ReadOnly {
		return nil, webdav.NewHTTPError(http.StatusForbidden, errors.New("address book is read-only"))
	}
	existing, err := b.Store.AddressObjectByHref(ctx, a.ID, href)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	etag := ""
	if existing != nil {
		etag = existing.ETag
	}
	var ifMatch, ifNone webdav.ConditionalMatch
	if opts != nil {
		ifMatch, ifNone = opts.IfMatch, opts.IfNoneMatch
	}
	if err := davcond.Check(etag, existing != nil, ifMatch, ifNone); err != nil {
		return nil, err
	}
	raw, err := icsutil.EncodeCard(card)
	if err != nil {
		return nil, err
	}
	if err := icsutil.CheckVCardSize(raw, card); err != nil {
		return nil, webdav.NewHTTPError(http.StatusRequestEntityTooLarge, err)
	}
	uid := icsutil.CardUID(card)
	if uid == "" {
		uid = strings.TrimSuffix(href, path.Ext(href))
	}
	etag = icsutil.ETag(raw)
	if err := b.Store.UpsertAddressObject(ctx, a.ID, href, uid, etag, raw, icsutil.CardFN(card), icsutil.CardBDAY(card), icsutil.CardAnniversary(card)); err != nil {
		return nil, err
	}
	o, err := b.Store.AddressObjectByHref(ctx, a.ID, href)
	if err != nil {
		return nil, err
	}
	return b.toObject(p, *a, *o)
}

func (b *Backend) DeleteAddressObject(ctx context.Context, objPath string) error {
	p, err := b.principal(ctx)
	if err != nil {
		return err
	}
	a, href, err := b.bookForPath(ctx, p, objPath)
	if err != nil {
		return err
	}
	if a.ReadOnly {
		return webdav.NewHTTPError(http.StatusForbidden, errors.New("address book is read-only"))
	}
	if err := b.Store.DeleteAddressObject(ctx, a.ID, href); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return webdav.NewHTTPError(http.StatusNotFound, err)
		}
		return err
	}
	return nil
}

func NewHandler(store *storage.DB) http.Handler {
	inner := &gocarddav.Handler{Backend: &Backend{Store: store}, Prefix: "/dav"}
	return davext.Wrap(inner, store)
}
