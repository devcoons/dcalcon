package icsutil_test

import (
	"strings"
	"testing"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/limits"
)

func TestCheckICSSize(t *testing.T) {
	if err := icsutil.CheckICSSize(strings.Repeat("a", limits.MaxICSBytes+1)); err == nil {
		t.Fatal("expected ICS oversize error")
	}
	if err := icsutil.CheckICSSize("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckVCardPhoto(t *testing.T) {
	okRaw := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:x\r\nFN:X\r\nEND:VCARD\r\n"
	card, err := icsutil.ParseCard(okRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := icsutil.CheckVCardSize(okRaw, card); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("A", limits.MaxPhotoBytes+8)
	raw := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:x\r\nFN:X\r\nPHOTO;ENCODING=b;TYPE=JPEG:" + big + "\r\nEND:VCARD\r\n"
	card, err = icsutil.ParseCard(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := icsutil.CheckVCardSize(raw, card); err == nil {
		t.Fatal("expected PHOTO oversize error")
	}
}
