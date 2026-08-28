package dav_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devcoons/dcalcon/internal/dav"
	"github.com/devcoons/dcalcon/internal/storage"
)

func TestPrincipalSchedulingAndCTag(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(dav.New(db, "dCalCon", nil, ""))
	t.Cleanup(ts.Close)

	body := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop>
		<C:schedule-inbox-URL/><C:schedule-outbox-URL/><C:calendar-user-address-set/><C:calendar-home-set/>
	</D:prop></D:propfind>`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/dav/principals/alice/", strings.NewReader(body))
	req.Method = "PROPFIND"
	req.SetBasicAuth("alice", "secret-pass")
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "0")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 207 {
		t.Fatalf("principal PROPFIND %d %s", res.StatusCode, raw)
	}
	s := string(raw)
	if !strings.Contains(s, "schedule-inbox-URL") || !strings.Contains(s, "/dav/calendars/alice/inbox/") {
		t.Fatalf("missing inbox URL: %s", s)
	}
	if !strings.Contains(s, "mailto:alice@dcalcon.private") {
		t.Fatalf("missing local calendar-user-address: %s", s)
	}
	if !strings.Contains(s, "mailto:alice@example.com") {
		t.Fatalf("missing calendar-user-address-set: %s", s)
	}

	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VEVENT\r\nUID:meet-1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828T090000Z\r\nDTEND:20260828T100000Z\r\nSUMMARY:Meet\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	put, _ := http.NewRequest(http.MethodPut, ts.URL+"/dav/calendars/alice/personal/meet-1.ics", strings.NewReader(ics))
	put.SetBasicAuth("alice", "secret-pass")
	put.Header.Set("Content-Type", "text/calendar")
	pres, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	pbody, _ := io.ReadAll(pres.Body)
	pres.Body.Close()
	if pres.StatusCode != 201 && pres.StatusCode != 204 && pres.StatusCode != 200 {
		t.Fatalf("PUT event %d %s", pres.StatusCode, pbody)
	}

	pf := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:" xmlns:CS="http://calendarserver.org/ns/" xmlns:ICAL="http://apple.com/ns/ical/"><D:prop>
		<CS:getctag/><ICAL:calendar-color/><D:sync-token/><D:displayname/>
	</D:prop></D:propfind>`
	preq, _ := http.NewRequest("PROPFIND", ts.URL+"/dav/calendars/alice/personal/", strings.NewReader(pf))
	preq.SetBasicAuth("alice", "secret-pass")
	preq.Header.Set("Depth", "0")
	preq.Header.Set("Content-Type", "application/xml")
	pres2, err := http.DefaultClient.Do(preq)
	if err != nil {
		t.Fatal(err)
	}
	praw, _ := io.ReadAll(pres2.Body)
	pres2.Body.Close()
	if pres2.StatusCode != 207 {
		t.Fatalf("calendar PROPFIND %d %s", pres2.StatusCode, praw)
	}
	ps := string(praw)
	if !strings.Contains(strings.ToLower(ps), "getctag") {
		t.Fatalf("missing getctag: %s", ps)
	}
	if !strings.Contains(ps, "calendar-color") {
		t.Fatalf("missing calendar-color: %s", ps)
	}

	sync := `<?xml version="1.0"?><D:sync-collection xmlns:D="DAV:"><D:sync-token/><D:sync-level>1</D:sync-level><D:prop><D:getetag/></D:prop></D:sync-collection>`
	sreq, _ := http.NewRequest("REPORT", ts.URL+"/dav/calendars/alice/personal/", strings.NewReader(sync))
	sreq.SetBasicAuth("alice", "secret-pass")
	sreq.Header.Set("Content-Type", "application/xml")
	sreq.Header.Set("Depth", "1")
	sres, err := http.DefaultClient.Do(sreq)
	if err != nil {
		t.Fatal(err)
	}
	sraw, _ := io.ReadAll(sres.Body)
	sres.Body.Close()
	if sres.StatusCode != 207 {
		t.Fatalf("sync-collection %d %s", sres.StatusCode, sraw)
	}
	ss := string(sraw)
	if !strings.Contains(ss, "meet-1.ics") || !strings.Contains(ss, "sync-token") {
		t.Fatalf("sync body %s", ss)
	}
}

func TestColorPropPatch(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(dav.New(db, "dCalCon", nil, ""))
	t.Cleanup(ts.Close)

	xml := `<?xml version="1.0"?><D:propertyupdate xmlns:D="DAV:" xmlns:ICAL="http://apple.com/ns/ical/"><D:set><D:prop>
		<ICAL:calendar-color>#FF00AA</ICAL:calendar-color>
	</D:prop></D:set></D:propertyupdate>`
	req, _ := http.NewRequest("PROPPATCH", ts.URL+"/dav/calendars/alice/personal/", strings.NewReader(xml))
	req.SetBasicAuth("alice", "secret-pass")
	req.Header.Set("Content-Type", "application/xml")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 207 {
		t.Fatalf("PROPPATCH %d %s", res.StatusCode, raw)
	}
	c, err := db.CalendarBySlug(t.Context(), 1, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if c.Color != "#FF00AA" {
		t.Fatalf("color %s", c.Color)
	}
}

func TestPeopleBookCardDAVReadOnly(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(t.Context(), "bob", "bob@example.com", "secret-pass", "Bob", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(dav.New(db, "dCalCon", nil, ""))
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/dav/addressbooks/alice/people/bob.vcf", nil)
	req.SetBasicAuth("alice", "secret-pass")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("DELETE people card %d %s", res.StatusCode, raw)
	}
}

func TestMkCalendarColorAndVJournalRejected(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	alice, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(dav.New(db, "dCalCon", nil, ""))
	t.Cleanup(ts.Close)

	xml := `<?xml version="1.0"?><C:mkcalendar xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav" xmlns:ICAL="http://apple.com/ns/ical/">
		<D:set><D:prop>
			<D:displayname>Work</D:displayname>
			<ICAL:calendar-color>#112233</ICAL:calendar-color>
		</D:prop></D:set>
	</C:mkcalendar>`
	req, _ := http.NewRequest("MKCALENDAR", ts.URL+"/dav/calendars/alice/work/", strings.NewReader(xml))
	req.SetBasicAuth("alice", "secret-pass")
	req.Header.Set("Content-Type", "application/xml")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("MKCALENDAR %d %s", res.StatusCode, raw)
	}
	c, err := db.CalendarBySlug(t.Context(), alice.ID, "work")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Work" || c.Color != "#112233" {
		t.Fatalf("calendar %+v", c)
	}

	pf := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop>
		<C:supported-calendar-component-set/><D:current-user-privilege-set/><D:acl/>
	</D:prop></D:propfind>`
	preq, _ := http.NewRequest("PROPFIND", ts.URL+"/dav/calendars/alice/work/", strings.NewReader(pf))
	preq.SetBasicAuth("alice", "secret-pass")
	preq.Header.Set("Depth", "0")
	preq.Header.Set("Content-Type", "application/xml")
	pres, err := http.DefaultClient.Do(preq)
	if err != nil {
		t.Fatal(err)
	}
	praw, _ := io.ReadAll(pres.Body)
	pres.Body.Close()
	if pres.StatusCode != 207 {
		t.Fatalf("PROPFIND %d %s", pres.StatusCode, praw)
	}
	ps := string(praw)
	if !strings.Contains(ps, "VEVENT") || !strings.Contains(ps, "VTODO") {
		t.Fatalf("missing component set: %s", ps)
	}
	if strings.Contains(ps, "VJOURNAL") {
		t.Fatalf("VJOURNAL advertised: %s", ps)
	}
	if !strings.Contains(ps, "current-user-privilege-set") {
		t.Fatalf("missing privilege set: %s", ps)
	}
	if !strings.Contains(ps, "acl") {
		t.Fatalf("missing ACL: %s", ps)
	}

	journal := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VJOURNAL\r\nUID:j1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828T090000Z\r\nSUMMARY:Note\r\nEND:VJOURNAL\r\nEND:VCALENDAR\r\n"
	jreq, _ := http.NewRequest(http.MethodPut, ts.URL+"/dav/calendars/alice/work/j1.ics", strings.NewReader(journal))
	jreq.SetBasicAuth("alice", "secret-pass")
	jreq.Header.Set("Content-Type", "text/calendar")
	jres, err := http.DefaultClient.Do(jreq)
	if err != nil {
		t.Fatal(err)
	}
	jbody, _ := io.ReadAll(jres.Body)
	jres.Body.Close()
	if jres.StatusCode != http.StatusForbidden {
		t.Fatalf("VJOURNAL PUT %d %s", jres.StatusCode, jbody)
	}
}

func TestACLShare(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	alice, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(t.Context(), "bob", "bob@example.com", "secret-pass", "Bob", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(dav.New(db, "dCalCon", nil, ""))
	t.Cleanup(ts.Close)

	xml := `<?xml version="1.0"?><D:acl xmlns:D="DAV:">
		<D:ace><D:principal><D:href>/dav/principals/alice/</D:href></D:principal><D:grant><D:privilege><D:all/></D:privilege></D:grant><D:protected/></D:ace>
		<D:ace><D:principal><D:href>/dav/principals/bob/</D:href></D:principal><D:grant><D:privilege><D:read/></D:privilege></D:grant></D:ace>
	</D:acl>`
	req, _ := http.NewRequest("ACL", ts.URL+"/dav/calendars/alice/personal/", strings.NewReader(xml))
	req.SetBasicAuth("alice", "secret-pass")
	req.Header.Set("Content-Type", "application/xml")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ACL %d %s", res.StatusCode, raw)
	}
	c, err := db.CalendarBySlug(t.Context(), alice.ID, "personal")
	if err != nil {
		t.Fatal(err)
	}
	shares, err := db.ListShares(t.Context(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 || shares[0].Username != "bob" || shares[0].Access != "read" {
		t.Fatalf("shares %+v", shares)
	}
}
