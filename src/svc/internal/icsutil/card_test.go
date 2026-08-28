package icsutil

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestContactRoundTrip(t *testing.T) {
	in := ContactInput{
		FN:          "Ada Lovelace",
		GivenName:   "Ada",
		FamilyName:  "Lovelace",
		Nickname:    "Ada",
		Org:         "Analytical Engine",
		Title:       "Mathematician",
		BDAY:        "1815-12-10",
		Anniversary: "1835-07-08",
		Note:        "First programmer",
		Categories:  "science, history",
		Emails:      []TypedValue{{Value: "ada@example.com", Type: "work"}, {Value: "ada@home.test", Type: "home"}},
		Tels:        []TypedValue{{Value: "+44 20 0000", Type: "cell"}},
		URLs:        []TypedValue{{Value: "https://example.com/ada", Type: "work"}},
		Addresses: []AddressInput{{
			Type: "home", Street: "12 Great Street", City: "London", PostalCode: "SW1A 1AA", Country: "UK",
		}},
		Custom: []CustomField{{Name: "Department", Value: "Mathematics"}, {Name: "X-PRONOUNS", Value: "she/her"}},
	}
	raw, err := EncodeContact("ada-1", in, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "X-DEPARTMENT:Mathematics") {
		t.Fatalf("missing custom field:\n%s", raw)
	}
	if !strings.Contains(raw, "X-PRONOUNS:she/her") {
		t.Fatalf("missing pronouns:\n%s", raw)
	}
	got := ParseContact(raw)
	if got.FN != "Ada Lovelace" || got.Org != "Analytical Engine" || got.BDAY != "1815-12-10" {
		t.Fatalf("parsed %#v", got)
	}
	if len(got.Emails) != 2 || got.Email != "ada@example.com" {
		t.Fatalf("emails %#v", got.Emails)
	}
	if len(got.Addresses) != 1 || got.Addresses[0].City != "London" {
		t.Fatalf("addr %#v", got.Addresses)
	}
	if len(got.Custom) != 2 {
		t.Fatalf("custom %#v", got.Custom)
	}
}

func TestEncodeContactPreservesPhoto(t *testing.T) {
	existing := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:p1\r\nFN:Pat\r\nPHOTO;VALUE=URI:https://example.com/p.jpg\r\nEMAIL:old@example.com\r\nX-NICK:skip\r\nEND:VCARD\r\n"
	raw, err := EncodeContact("p1", ContactInput{
		FN:     "Patricia",
		Emails: []TypedValue{{Value: "new@example.com"}},
		Custom: []CustomField{{Name: "Team", Value: "Ops"}},
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "https://example.com/p.jpg") {
		t.Fatalf("photo dropped:\n%s", raw)
	}
	if strings.Contains(raw, "old@example.com") {
		t.Fatalf("old email kept:\n%s", raw)
	}
	if strings.Contains(raw, "X-NICK") {
		t.Fatalf("removed custom kept:\n%s", raw)
	}
	if !strings.Contains(raw, "X-TEAM:Ops") {
		t.Fatalf("new custom missing:\n%s", raw)
	}
}

func TestEncodeContactPreservesUnknownWhenCustomOmitted(t *testing.T) {
	existing := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:p1\r\nFN:Pat\r\nPHOTO;VALUE=URI:https://example.com/p.jpg\r\nX-NICK:skip\r\nX-ABDATE;TYPE=ANNIVERSARY:2010-06-15\r\nEND:VCARD\r\n"
	raw, err := EncodeContact("p1", ContactInput{
		FN:     "Patricia",
		Emails: []TypedValue{{Value: "new@example.com"}},
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "X-NICK:skip") {
		t.Fatalf("omitted custom dropped X-NICK:\n%s", raw)
	}
	if !strings.Contains(raw, "X-ABDATE") || !strings.Contains(raw, "TYPE=ANNIVERSARY") {
		t.Fatalf("X-ABDATE params dropped:\n%s", raw)
	}
	if !strings.Contains(raw, "https://example.com/p.jpg") {
		t.Fatalf("photo dropped:\n%s", raw)
	}
}

func TestEncodeContactKeepsListedCustomAndHidden(t *testing.T) {
	existing := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:p1\r\nFN:Pat\r\nX-NICK:skip\r\nX-EMAIL:hidden@example.com\r\nEND:VCARD\r\n"
	raw, err := EncodeContact("p1", ContactInput{
		FN:     "Patricia",
		Custom: []CustomField{{Name: "X-NICK", Value: "skip"}},
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "X-NICK:skip") {
		t.Fatalf("listed X-NICK dropped:\n%s", raw)
	}
	if !strings.Contains(raw, "X-EMAIL:hidden@example.com") {
		t.Fatalf("dashboard-unknown X-* dropped:\n%s", raw)
	}
}

func TestCustomPropName(t *testing.T) {
	n, ok := CustomPropName("Department")
	if !ok || n != "X-DEPARTMENT" {
		t.Fatalf("%s %v", n, ok)
	}
	if _, ok := CustomPropName("FN"); ok {
		t.Fatal("FN must be rejected")
	}
}

func TestUpdateCardCompat(t *testing.T) {
	raw, err := UpdateCard("", "Pat", "pat@example.com", "555", "1990-01-02", "")
	if err != nil {
		t.Fatal(err)
	}
	got := ParseContact(raw)
	if got.FN != "Pat" || got.Email != "pat@example.com" || got.Tel != "555" || got.BDAY != "1990-01-02" {
		t.Fatalf("%#v", got)
	}
}

func TestSplitAndPrepareCards(t *testing.T) {
	raw := "BEGIN:VCARD\nVERSION:3.0\nUID:a1\nFN:Ada\nEMAIL:ada@example.com\nEND:VCARD\nBEGIN:VCARD\nVERSION:3.0\nFN:No UID\nEND:VCARD\n"
	blocks := SplitCardBlocks(raw)
	if len(blocks) != 2 {
		t.Fatalf("blocks %d", len(blocks))
	}
	first, err := PrepareImportedCard(blocks[0], "unused")
	if err != nil || first.UID != "a1" || first.FN != "Ada" {
		t.Fatalf("first %+v %v", first, err)
	}
	second, err := PrepareImportedCard(blocks[1], "generated-uid")
	if err != nil || second.UID != "generated-uid" || second.FN != "No UID" {
		t.Fatalf("second %+v %v", second, err)
	}
	if VCardHref("Ada Lovelace") != "Ada-Lovelace.vcf" {
		t.Fatalf("href %s", VCardHref("Ada Lovelace"))
	}
	joined := JoinCards([]string{first.Raw, second.Raw})
	if !strings.Contains(joined, "FN:Ada") || !strings.Contains(joined, "FN:No UID") {
		t.Fatalf("join %s", joined)
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportCardBlocksFromZip(t *testing.T) {
	card := func(uid, fn string) string {
		return "BEGIN:VCARD\nVERSION:3.0\nUID:" + uid + "\nFN:" + fn + "\nEND:VCARD\n"
	}
	raw := zipBytes(t, map[string]string{
		"contacts/ada.vcf":   card("zip-ada", "Ada"),
		"al.vcard":           card("zip-al", "Al"),
		"notes.txt":          "ignore me",
		"__MACOSX/._ada.vcf": "junk",
		"bundle.vcf":         card("zip-one", "One") + card("zip-two", "Two"),
		"../escape.vcf":      card("nope", "Nope"),
	})
	blocks, err := ImportCardBlocks(raw)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(blocks, "")
	if len(blocks) != 4 || !strings.Contains(joined, "FN:Ada") || !strings.Contains(joined, "FN:Al") || !strings.Contains(joined, "FN:Two") {
		t.Fatalf("blocks %d %s", len(blocks), joined)
	}
	if strings.Contains(joined, "FN:Nope") {
		t.Fatal("zip-slip path imported")
	}

	plain, err := ImportCardBlocks([]byte(card("p1", "Plain")))
	if err != nil || len(plain) != 1 {
		t.Fatalf("plain %+v %v", plain, err)
	}

	empty, err := ImportCardBlocks(zipBytes(t, map[string]string{"readme.txt": "hi"}))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty zip %d %v", len(empty), err)
	}
}
