package icsutil

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSplitAndWriteManagedAttach(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("hello-file"))
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:a1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828T090000Z\r\nSUMMARY:Meet\r\nATTACH;ENCODING=BASE64;VALUE=BINARY;FILENAME=notes.txt;FMTTYPE=text/plain:" + payload + "\r\nATTACH:https://example.com/agenda.pdf\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	split, err := SplitAttachments(raw, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(split.Inlines) != 1 || string(split.Inlines[0].Data) != "hello-file" {
		t.Fatalf("inline %+v", split.Inlines)
	}
	if split.Inlines[0].Filename != "notes.txt" {
		t.Fatalf("filename %q", split.Inlines[0].Filename)
	}
	if len(split.External) != 1 || !strings.Contains(split.External[0].Value, "example.com") {
		t.Fatalf("external %+v", split.External)
	}
	if strings.Contains(split.ICS, "hello-file") || strings.Contains(strings.ToUpper(split.ICS), "VALUE=BINARY") {
		t.Fatal("binary should be stripped from ICS")
	}

	out, err := WriteAttachments(split, []ManagedAttach{{
		PublicID: "11111111-1111-1111-1111-111111111111", Filename: "notes.txt", ContentType: "text/plain", Size: 10,
	}}, "http://cal.example.test", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "MANAGED-ID=11111111-1111-1111-1111-111111111111") {
		t.Fatalf("missing managed id:\n%s", out)
	}
	if !strings.Contains(out, "/dav/attachments/11111111-1111-1111-1111-111111111111") {
		t.Fatalf("missing dav uri:\n%s", out)
	}
	if !strings.Contains(out, "example.com/agenda.pdf") {
		t.Fatalf("lost external uri:\n%s", out)
	}

	again, err := SplitAttachments(out, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Inlines) != 0 {
		t.Fatalf("uri attach treated as inline: %+v", again.Inlines)
	}
	if len(again.ManagedIDs) != 1 || again.ManagedIDs[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("managed ids %+v", again.ManagedIDs)
	}
}

func TestAttachmentURI(t *testing.T) {
	if AttachmentURI("", "abc") != "/dav/attachments/abc" {
		t.Fatal(AttachmentURI("", "abc"))
	}
	if AttachmentURI("http://x/", "abc") != "http://x/dav/attachments/abc" {
		t.Fatal(AttachmentURI("http://x/", "abc"))
	}
}
