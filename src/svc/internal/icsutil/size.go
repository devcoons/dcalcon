package icsutil

import (
	"fmt"

	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/emersion/go-vcard"
)

func CheckICSSize(raw string) error {
	if len(raw) > limits.MaxICSBytes {
		return fmt.Errorf("calendar object exceeds %d bytes", limits.MaxICSBytes)
	}
	return nil
}

func CheckVCardSize(raw string, card vcard.Card) error {
	if len(raw) > limits.MaxVCardBytes {
		return fmt.Errorf("vCard exceeds %d bytes", limits.MaxVCardBytes)
	}
	if n := photoBytes(card); n > limits.MaxPhotoBytes {
		return fmt.Errorf("PHOTO exceeds %d bytes", limits.MaxPhotoBytes)
	}
	return nil
}

func photoBytes(card vcard.Card) int {
	if card == nil {
		return 0
	}
	n := 0
	for _, key := range []string{vcard.FieldPhoto, vcard.FieldLogo} {
		for _, p := range card[key] {
			n += len(p.Value)
		}
	}
	return n
}
