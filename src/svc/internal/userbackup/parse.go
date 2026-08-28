package userbackup

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/devcoons/dcalcon/internal/storage"
)

type Bundle struct {
	Manifest Manifest
	Dates    storage.ImportantDatesSettings
	Cals     []calendarBundle
	Books    []bookBundle
	Account  *Account
}

type calendarBundle struct {
	Meta  CalendarMeta
	ICS   map[string]string // href -> ics
	Files map[string][]fileBlob
}

type fileBlob struct {
	Ref  FileRef
	Data []byte
}

type bookBundle struct {
	Meta  BookMeta
	Cards map[string]string // href -> vcard
}

func Open(data []byte) (*Bundle, error) {
	if len(data) > limits.MaxBackupZip {
		return nil, ErrTooLarge
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrNotBackup
	}
	if len(zr.File) > limits.MaxImportZipFiles {
		return nil, ErrTooLarge
	}
	files := map[string][]byte{}
	var total int64
	maxUncompressed := int64(limits.MaxBackupZip) * 2
	for _, f := range zr.File {
		name, err := zipName(f.Name)
		if err != nil {
			return nil, err
		}
		if f.UncompressedSize64 > uint64(limits.MaxICSBytes) && f.UncompressedSize64 > uint64(limits.MaxAttachmentBytes) {
			return nil, ErrTooLarge
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		raw, err := io.ReadAll(io.LimitReader(rc, int64(limits.MaxICSBytes)+int64(limits.MaxAttachmentBytes)+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		total += int64(len(raw))
		if total > maxUncompressed {
			return nil, ErrTooLarge
		}
		files[name] = raw
	}
	raw, ok := files["dcalcon.json"]
	if !ok {
		return nil, ErrNotBackup
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return nil, ErrNotBackup
	}
	if man.Format != Format {
		return nil, ErrNotBackup
	}
	if man.Version != Version {
		return nil, ErrUnsupported
	}
	kind, err := NormalizeKind(man.Kind)
	if err != nil {
		return nil, err
	}
	man.Kind = kind
	out := &Bundle{Manifest: man, Dates: storage.ImportantDatesSettings{
		IncludeBirthdays: true, IncludeAnniversaries: true, AlarmOffsets: []string{"-P1D"},
	}}
	if raw, ok := files["settings/important-dates.json"]; ok {
		_ = json.Unmarshal(raw, &out.Dates)
	}
	if raw, ok := files["account.json"]; ok && kind == KindFull {
		var acc Account
		if err := json.Unmarshal(raw, &acc); err != nil {
			return nil, fmt.Errorf("account.json: %w", err)
		}
		out.Account = &acc
	}
	for name, raw := range files {
		if !strings.HasPrefix(name, "calendars/") || !strings.HasSuffix(name, "/calendar.json") {
			continue
		}
		root := strings.TrimSuffix(name, "/calendar.json")
		var meta CalendarMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		cb := calendarBundle{Meta: meta, ICS: map[string]string{}, Files: map[string][]fileBlob{}}
		for _, item := range meta.Items {
			p, ok := zipChild(root, item.File)
			if !ok {
				continue
			}
			if body, ok := files[p]; ok {
				cb.ICS[item.Href] = string(body)
			}
		}
		for _, fref := range meta.Files {
			p, ok := zipChild(root, fref.File)
			if !ok {
				continue
			}
			body, ok := files[p]
			if !ok {
				continue
			}
			cb.Files[fref.Href] = append(cb.Files[fref.Href], fileBlob{Ref: fref, Data: body})
		}
		out.Cals = append(out.Cals, cb)
	}
	for name, raw := range files {
		if !strings.HasPrefix(name, "contacts/") || !strings.HasSuffix(name, "/book.json") {
			continue
		}
		root := strings.TrimSuffix(name, "/book.json")
		var meta BookMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		bb := bookBundle{Meta: meta, Cards: map[string]string{}}
		for _, item := range meta.Items {
			p, ok := zipChild(root, item.File)
			if !ok {
				continue
			}
			if body, ok := files[p]; ok {
				bb.Cards[item.Href] = string(body)
			}
		}
		out.Books = append(out.Books, bb)
	}
	return out, nil
}

func zipName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	if unsafeZipName(name) {
		return "", ErrUnsafeZip
	}
	cleaned := path.Clean(name)
	if unsafeZipName(cleaned) || cleaned == "." {
		return "", ErrUnsafeZip
	}
	return cleaned, nil
}

func zipChild(root, rel string) (string, bool) {
	rel = strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "/")
	if unsafeZipName(rel) {
		return "", false
	}
	p := path.Clean(root + "/" + rel)
	if !strings.HasPrefix(p, root+"/") {
		return "", false
	}
	return p, true
}

func unsafeZipName(name string) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") {
		return true
	}
	if strings.Contains(name, ":") {
		return true
	}
	for _, p := range strings.Split(name, "/") {
		if p == ".." {
			return true
		}
	}
	return false
}
