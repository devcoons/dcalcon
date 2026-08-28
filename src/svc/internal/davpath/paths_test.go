package davpath

import (
	"strings"
	"testing"
)

func TestCheckObjectHref(t *testing.T) {
	if err := CheckObjectHref("meet-1.ics"); err != nil {
		t.Fatal(err)
	}
	bads := []string{"", ".", "..", "../inbox/x.ics", "foo/bar.ics", `foo\bar.ics`, "a%2f../b.ics", "a%2e%2e/b.ics"}
	for _, h := range bads {
		if err := CheckObjectHref(h); err == nil {
			t.Errorf("expected reject %q", h)
		}
	}
}

func TestSlugifyAndZip(t *testing.T) {
	if got := Slugify("../../etc", "My Cal"); got != "etc" {
		t.Fatalf("slug %q", got)
	}
	if got := Slugify("", "Work Calendar"); got != "work-calendar" {
		t.Fatalf("got %q", got)
	}
	if !ValidSlug("x-share-12") || !ValidSlug("important-dates") || !ValidSlug("personal") {
		t.Fatal("system slugs must stay valid")
	}
	if ValidSlug("../x") || ValidSlug("a/b") || ValidSlug("-nope") {
		t.Fatal("bad slugs accepted")
	}
	if got := ZipSegment("../evil.txt"); got != "evil.txt" {
		t.Fatalf("zip segment %q", got)
	}
	if strings.Contains(ZipPath("calendars", "../tmp", "a.ics"), "..") {
		t.Fatal("zip path still has ..")
	}
	if ObjectFileHref("../x", ".ics") == "../x.ics" {
		t.Fatal("unsafe uid must not become href")
	}
}
