package icsutil

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/emersion/go-vcard"
)

type ImportedCard struct {
	Raw         string
	UID         string
	FN          string
	BDAY        string
	Anniversary string
}

func toCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func SplitCardBlocks(raw string) []string {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	upper := strings.ToUpper(s)
	var out []string
	from := 0
	for {
		start := strings.Index(upper[from:], "BEGIN:VCARD")
		if start < 0 {
			break
		}
		start += from
		endRel := strings.Index(upper[start:], "END:VCARD")
		if endRel < 0 {
			break
		}
		end := start + endRel + len("END:VCARD")
		block := strings.TrimSpace(s[start:end])
		if block != "" {
			out = append(out, toCRLF(block)+"\r\n")
		}
		from = end
	}
	return out
}

func ImportCardBlocks(raw []byte) ([]string, error) {
	if looksLikeZip(raw) {
		return cardsFromZip(raw)
	}
	return SplitCardBlocks(string(raw)), nil
}

func looksLikeZip(raw []byte) bool {
	return len(raw) >= 4 && raw[0] == 'P' && raw[1] == 'K' && (raw[2] == 3 || raw[2] == 5 || raw[2] == 7)
}

func cardsFromZip(raw []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("could not read zip")
	}
	if len(zr.File) > limits.MaxImportZipFiles {
		return nil, fmt.Errorf("zip has too many files")
	}
	var (
		out   []string
		total int64
	)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !zipVCardName(f.Name) {
			continue
		}
		if f.UncompressedSize64 > uint64(limits.MaxHTTPBody) {
			return nil, fmt.Errorf("zip too large")
		}
		total += int64(f.UncompressedSize64)
		if total > int64(limits.MaxHTTPBody) {
			return nil, fmt.Errorf("zip too large")
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(rc, int64(limits.MaxHTTPBody)+1))
		_ = rc.Close()
		if err != nil || int64(len(body)) > int64(limits.MaxHTTPBody) {
			return nil, fmt.Errorf("zip too large")
		}
		out = append(out, SplitCardBlocks(string(body))...)
	}
	return out, nil
}

func zipVCardName(name string) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.Contains(name, "..") || strings.HasPrefix(name, "__MACOSX/") {
		return false
	}
	base := strings.ToLower(filepath.Base(name))
	if base == "" || strings.HasPrefix(base, ".") {
		return false
	}
	return strings.HasSuffix(base, ".vcf") || strings.HasSuffix(base, ".vcard")
}

func JoinCards(raws []string) string {
	var b strings.Builder
	for _, s := range raws {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		s = toCRLF(s)
		b.WriteString(s)
		if !strings.HasSuffix(s, "\r\n") {
			b.WriteString("\r\n")
		}
	}
	return b.String()
}

func PrepareImportedCard(raw, newUID string) (ImportedCard, error) {
	card, err := ParseCard(raw)
	if err != nil || card == nil {
		return ImportedCard{}, fmt.Errorf("could not parse vCard")
	}
	uid := CardUID(card)
	if uid == "" {
		uid = strings.TrimSpace(newUID)
		if uid == "" {
			return ImportedCard{}, fmt.Errorf("vCard has no UID")
		}
		card.SetValue(vcard.FieldUID, uid)
	}
	fn := CardFN(card)
	if fn == "" {
		fn = uid
		card.SetValue(vcard.FieldFormattedName, fn)
	}
	encoded, err := EncodeCard(card)
	if err != nil {
		return ImportedCard{}, err
	}
	if err := CheckVCardSize(encoded, card); err != nil {
		return ImportedCard{}, err
	}
	return ImportedCard{
		Raw: encoded, UID: uid, FN: fn,
		BDAY: CardBDAY(card), Anniversary: CardAnniversary(card),
	}, nil
}

func VCardHref(uid string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(uid) {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "contact"
	}
	if len(s) > 120 {
		s = s[:120]
	}
	if !strings.HasSuffix(strings.ToLower(s), ".vcf") {
		s += ".vcf"
	}
	return s
}

func VCardFileName(fn, uid string) string {
	s := VCardHref(fn)
	if s == "contact.vcf" && uid != "" {
		return VCardHref(uid)
	}
	return s
}
