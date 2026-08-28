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

func davServer(t *testing.T) (*httptest.Server, *storage.DB) {
	t.Helper()
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
	return ts, db
}

func davDo(t *testing.T, method, url, body, user, pass string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDAVHeaderHasAutoSchedule(t *testing.T) {
	ts, _ := davServer(t)
	res := davDo(t, http.MethodOptions, ts.URL+"/dav/principals/alice/", "", "alice", "secret-pass", nil)
	defer res.Body.Close()
	dav := res.Header.Get("DAV")
	if !strings.Contains(dav, "calendar-auto-schedule") {
		t.Fatalf("must advertise calendar-auto-schedule: %s", dav)
	}
	if !strings.Contains(dav, "calendar-access") {
		t.Fatalf("missing calendar-access: %s", dav)
	}
}

func TestIfMatchPrecondition(t *testing.T) {
	ts, _ := davServer(t)
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VEVENT\r\nUID:etag-1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828T090000Z\r\nDTEND:20260828T100000Z\r\nSUMMARY:Meet\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	url := ts.URL + "/dav/calendars/alice/personal/etag-1.ics"
	res := davDo(t, http.MethodPut, url, ics, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 201 && res.StatusCode != 204 && res.StatusCode != 200 {
		t.Fatalf("create %d", res.StatusCode)
	}
	etag := res.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag on create")
	}

	stale := davDo(t, http.MethodPut, url, strings.ReplaceAll(ics, "Meet", "Nope"), "alice", "secret-pass", map[string]string{
		"Content-Type": "text/calendar",
		"If-Match":     `"deadbeef"`,
	})
	io.Copy(io.Discard, stale.Body)
	stale.Body.Close()
	if stale.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("stale If-Match want 412, got %d", stale.StatusCode)
	}

	ok := davDo(t, http.MethodPut, url, strings.ReplaceAll(ics, "Meet", "Updated"), "alice", "secret-pass", map[string]string{
		"Content-Type": "text/calendar",
		"If-Match":     etag,
	})
	io.Copy(io.Discard, ok.Body)
	ok.Body.Close()
	if ok.StatusCode != 201 && ok.StatusCode != 204 && ok.StatusCode != 200 {
		t.Fatalf("matching If-Match %d", ok.StatusCode)
	}

	none := davDo(t, http.MethodPut, url, ics, "alice", "secret-pass", map[string]string{
		"Content-Type":  "text/calendar",
		"If-None-Match": "*",
	})
	io.Copy(io.Discard, none.Body)
	none.Body.Close()
	if none.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("If-None-Match * want 412, got %d", none.StatusCode)
	}
}

func TestCardDAVIfMatch(t *testing.T) {
	ts, _ := davServer(t)
	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:etag-c1\r\nFN:Ada\r\nEND:VCARD\r\n"
	url := ts.URL + "/dav/addressbooks/alice/contacts/etag-c1.vcf"
	res := davDo(t, http.MethodPut, url, vcard, "alice", "secret-pass", map[string]string{"Content-Type": "text/vcard"})
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("create %d", res.StatusCode)
	}
	etag := res.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	stale := davDo(t, http.MethodPut, url, strings.ReplaceAll(vcard, "Ada", "Nope"), "alice", "secret-pass", map[string]string{
		"Content-Type": "text/vcard",
		"If-Match":     `"deadbeef"`,
	})
	io.Copy(io.Discard, stale.Body)
	stale.Body.Close()
	if stale.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("stale If-Match want 412, got %d", stale.StatusCode)
	}
	ok := davDo(t, http.MethodPut, url, strings.ReplaceAll(vcard, "Ada", "Ada Lovelace"), "alice", "secret-pass", map[string]string{
		"Content-Type": "text/vcard",
		"If-Match":     etag,
	})
	io.Copy(io.Discard, ok.Body)
	ok.Body.Close()
	if ok.StatusCode >= 300 {
		t.Fatalf("matching If-Match %d", ok.StatusCode)
	}
}

func TestDAVRequiresAppPasswordWhenTOTP(t *testing.T) {
	ts, db := davServer(t)
	if err := db.EnableTOTP(t.Context(), 1, "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}
	res := davDo(t, http.MethodOptions, ts.URL+"/dav/principals/alice/", "", "alice", "secret-pass", nil)
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("account password with TOTP on: %d", res.StatusCode)
	}
	_, secret, err := db.CreateAppPassword(t.Context(), 1, "phone")
	if err != nil {
		t.Fatal(err)
	}
	ok := davDo(t, http.MethodOptions, ts.URL+"/dav/principals/alice/", "", "alice", secret, nil)
	io.Copy(io.Discard, ok.Body)
	ok.Body.Close()
	if ok.StatusCode != http.StatusNoContent && ok.StatusCode != http.StatusOK {
		t.Fatalf("app password with TOTP on: %d", ok.StatusCode)
	}
}

func TestAppPasswordDAVAuth(t *testing.T) {
	ts, db := davServer(t)
	pw, secret, err := db.CreateAppPassword(t.Context(), 1, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if pw.Prefix == "" || !strings.HasPrefix(secret, "dcc_") {
		t.Fatalf("secret %s prefix %s", secret, pw.Prefix)
	}
	res := davDo(t, http.MethodOptions, ts.URL+"/dav/principals/alice/", "", "alice", secret, nil)
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		t.Fatalf("app password DAV %d", res.StatusCode)
	}
}

func TestInteropClientSequence(t *testing.T) {
	ts, _ := davServer(t)
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/.well-known/caldav", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("alice", "secret-pass")
	well, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, well.Body)
	well.Body.Close()
	if well.StatusCode != http.StatusMovedPermanently && well.StatusCode != http.StatusPermanentRedirect && well.StatusCode != http.StatusFound {
		t.Fatalf("well-known %d", well.StatusCode)
	}

	body := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav" xmlns:CR="urn:ietf:params:xml:ns:carddav"><D:prop>
		<D:current-user-principal/><C:calendar-home-set/><CR:addressbook-home-set/>
	</D:prop></D:propfind>`
	pf := davDo(t, "PROPFIND", ts.URL+"/dav/principals/alice/", body, "alice", "secret-pass", map[string]string{
		"Content-Type": "application/xml", "Depth": "0",
	})
	raw, _ := io.ReadAll(pf.Body)
	pf.Body.Close()
	if pf.StatusCode != 207 {
		t.Fatalf("principal %d %s", pf.StatusCode, raw)
	}
	s := string(raw)
	if !strings.Contains(s, "calendar-home-set") || !strings.Contains(s, "addressbook-home-set") {
		t.Fatalf("homes %s", s)
	}

	weekly := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:week-interop\r\nDTSTAMP:20260105T000000Z\r\nDTSTART:20260105T090000Z\r\nDTEND:20260105T100000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:Standup\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	put := davDo(t, http.MethodPut, ts.URL+"/dav/calendars/alice/personal/week-interop.ics", weekly, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	io.Copy(io.Discard, put.Body)
	put.Body.Close()
	if put.StatusCode >= 300 {
		t.Fatalf("PUT weekly %d", put.StatusCode)
	}

	query := `<?xml version="1.0"?><C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:getetag/><C:calendar-data><C:expand start="20260801T000000Z" end="20260901T000000Z"/></C:calendar-data></D:prop><C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VEVENT"><C:time-range start="20260801T000000Z" end="20260901T000000Z"/></C:comp-filter></C:comp-filter></C:filter></C:calendar-query>`
	rep := davDo(t, "REPORT", ts.URL+"/dav/calendars/alice/personal/", query, "alice", "secret-pass", map[string]string{"Content-Type": "application/xml", "Depth": "1"})
	qraw, _ := io.ReadAll(rep.Body)
	rep.Body.Close()
	if rep.StatusCode != 207 {
		t.Fatalf("calendar-query %d %s", rep.StatusCode, qraw)
	}
	qs := string(qraw)
	if !strings.Contains(qs, "week-interop.ics") {
		t.Fatalf("query missed weekly event: %s", qs)
	}

	vcard := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:c1\r\nFN:Ada Lovelace\r\nEMAIL:ada@example.com\r\nEND:VCARD\r\n"
	cput := davDo(t, http.MethodPut, ts.URL+"/dav/addressbooks/alice/contacts/c1.vcf", vcard, "alice", "secret-pass", map[string]string{"Content-Type": "text/vcard"})
	io.Copy(io.Discard, cput.Body)
	cput.Body.Close()
	if cput.StatusCode >= 300 {
		t.Fatalf("PUT vcard %d", cput.StatusCode)
	}
	aq := `<?xml version="1.0"?><CR:addressbook-query xmlns:D="DAV:" xmlns:CR="urn:ietf:params:xml:ns:carddav"><D:prop><D:getetag/><CR:address-data/></D:prop><CR:filter><CR:prop-filter name="FN"><CR:text-match>Ada</CR:text-match></CR:prop-filter></CR:filter></CR:addressbook-query>`
	ares := davDo(t, "REPORT", ts.URL+"/dav/addressbooks/alice/contacts/", aq, "alice", "secret-pass", map[string]string{"Content-Type": "application/xml"})
	araw, _ := io.ReadAll(ares.Body)
	ares.Body.Close()
	if ares.StatusCode != 207 {
		t.Fatalf("addressbook-query %d %s", ares.StatusCode, araw)
	}
	if !strings.Contains(string(araw), "Ada") {
		t.Fatalf("addressbook-query body %s", araw)
	}

	for i := 0; i < 25; i++ {
		rep := davDo(t, "REPORT", ts.URL+"/dav/calendars/alice/personal/", query, "alice", "secret-pass", map[string]string{"Content-Type": "application/xml", "Depth": "1"})
		io.Copy(io.Discard, rep.Body)
		rep.Body.Close()
		if rep.StatusCode != 207 {
			t.Fatalf("calendar-query load %d: %d", i, rep.StatusCode)
		}
	}
}

func TestFreeBusyReportAndOutbox(t *testing.T) {
	ts, _ := davServer(t)
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VEVENT\r\nUID:busy-1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828T090000Z\r\nDTEND:20260828T100000Z\r\nSUMMARY:Busy\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	put := davDo(t, http.MethodPut, ts.URL+"/dav/calendars/alice/personal/busy-1.ics", ics, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	io.Copy(io.Discard, put.Body)
	put.Body.Close()
	if put.StatusCode >= 300 {
		t.Fatalf("put %d", put.StatusCode)
	}
	report := `<?xml version="1.0"?><C:free-busy-query xmlns:C="urn:ietf:params:xml:ns:caldav"><C:time-range start="20260828T000000Z" end="20260829T000000Z"/></C:free-busy-query>`
	res := davDo(t, "REPORT", ts.URL+"/dav/calendars/alice/personal/", report, "alice", "secret-pass", map[string]string{"Content-Type": "application/xml"})
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("free-busy-query %d %s", res.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "FREEBUSY") && !strings.Contains(string(raw), "VFREEBUSY") {
		t.Fatalf("freebusy body %s", raw)
	}

	fb := "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nBEGIN:VFREEBUSY\r\nUID:fb-out\r\nDTSTART:20260828T000000Z\r\nDTEND:20260829T000000Z\r\nEND:VFREEBUSY\r\nEND:VCALENDAR\r\n"
	post := davDo(t, http.MethodPost, ts.URL+"/dav/calendars/alice/outbox/", fb, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	praw, _ := io.ReadAll(post.Body)
	post.Body.Close()
	if post.StatusCode != 200 {
		t.Fatalf("outbox POST %d %s", post.StatusCode, praw)
	}
	if !strings.Contains(string(praw), "schedule-response") {
		t.Fatalf("schedule-response %s", praw)
	}

	entity := `<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><C:free-busy-query xmlns:C="urn:ietf:params:xml:ns:caldav">&xxe;</C:free-busy-query>`
	bad := davDo(t, "REPORT", ts.URL+"/dav/calendars/alice/personal/", entity, "alice", "secret-pass", map[string]string{"Content-Type": "application/xml"})
	io.Copy(io.Discard, bad.Body)
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("entity body want 400, got %d", bad.StatusCode)
	}
}

func TestPrincipalListsShareProxy(t *testing.T) {
	ts, db := davServer(t)
	bob, err := db.CreateUser(t.Context(), "bob", "bob@example.com", "secret-pass", "Bob", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := db.CalendarBySlug(t.Context(), 1, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertShare(t.Context(), cal.ID, bob.ID, "read"); err != nil {
		t.Fatal(err)
	}
	body := `<?xml version="1.0"?><D:propfind xmlns:D="DAV:" xmlns:CS="http://calendarserver.org/ns/"><D:prop><CS:calendar-proxy-read-for/></D:prop></D:propfind>`
	pf := davDo(t, "PROPFIND", ts.URL+"/dav/principals/bob/", body, "bob", "secret-pass", map[string]string{"Content-Type": "application/xml", "Depth": "0"})
	raw, _ := io.ReadAll(pf.Body)
	pf.Body.Close()
	if pf.StatusCode != 207 {
		t.Fatalf("propfind %d %s", pf.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "x-share-") {
		t.Fatalf("proxy hrefs %s", raw)
	}
}

func TestRejectNestedHrefAndTodoQuery(t *testing.T) {
	ts, _ := davServer(t)
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VEVENT\r\nUID:meet\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828T090000Z\r\nDTEND:20260828T100000Z\r\nSUMMARY:Meet\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	todo := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VTODO\r\nUID:task-1\r\nDTSTAMP:20260801T000000Z\r\nDUE:20260830\r\nSUMMARY:File taxes\r\nSTATUS:NEEDS-ACTION\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	put := davDo(t, http.MethodPut, ts.URL+"/dav/calendars/alice/personal/meet.ics", ics, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	io.Copy(io.Discard, put.Body)
	put.Body.Close()
	if put.StatusCode >= 300 {
		t.Fatalf("put event %d", put.StatusCode)
	}
	put = davDo(t, http.MethodPut, ts.URL+"/dav/calendars/alice/personal/task-1.ics", todo, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	io.Copy(io.Discard, put.Body)
	put.Body.Close()
	if put.StatusCode >= 300 {
		t.Fatalf("put todo %d", put.StatusCode)
	}
	nested := davDo(t, http.MethodPut, ts.URL+"/dav/calendars/alice/personal/nested/evil.ics", ics, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	io.Copy(io.Discard, nested.Body)
	nested.Body.Close()
	if nested.StatusCode != http.StatusBadRequest {
		t.Fatalf("nested href want 400, got %d", nested.StatusCode)
	}
	q := `<?xml version="1.0"?><C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:getetag/><C:calendar-data/></D:prop><C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VTODO"/></C:comp-filter></C:filter></C:calendar-query>`
	rep := davDo(t, "REPORT", ts.URL+"/dav/calendars/alice/personal/", q, "alice", "secret-pass", map[string]string{"Content-Type": "application/xml", "Depth": "1"})
	raw, _ := io.ReadAll(rep.Body)
	rep.Body.Close()
	if rep.StatusCode != 207 {
		t.Fatalf("todo query %d %s", rep.StatusCode, raw)
	}
	body := string(raw)
	if !strings.Contains(body, "File taxes") {
		t.Fatalf("todo query missed VTODO: %s", body)
	}
	if strings.Contains(body, "SUMMARY:Meet") {
		t.Fatalf("todo query should not return VEVENT: %s", body)
	}
}

func TestDAVPutExtractsInlineAttachment(t *testing.T) {
	ts, db := davServer(t)
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VEVENT\r\nUID:att-1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260828T090000Z\r\nDTEND:20260828T100000Z\r\nSUMMARY:Meet\r\nATTACH;ENCODING=BASE64;VALUE=BINARY;FILENAME=notes.txt;FMTTYPE=text/plain:aGVsbG8=\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	put := davDo(t, http.MethodPut, ts.URL+"/dav/calendars/alice/personal/att-1.ics", ics, "alice", "secret-pass", map[string]string{"Content-Type": "text/calendar"})
	io.Copy(io.Discard, put.Body)
	put.Body.Close()
	if put.StatusCode != 201 && put.StatusCode != 204 && put.StatusCode != 200 {
		t.Fatalf("put %d", put.StatusCode)
	}
	u, err := db.UserByUsername(t.Context(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	cals, err := db.ListCalendars(t.Context(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	var personal storage.Calendar
	for _, c := range cals {
		if c.Kind == "personal" {
			personal = c
			break
		}
	}
	obj, err := db.CalendarObjectByHref(t.Context(), personal.ID, "att-1.ics")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(obj.ICS, "VALUE=BINARY") || strings.Contains(obj.ICS, "aGVsbG8=") {
		t.Fatalf("binary should be extracted:\n%s", obj.ICS)
	}
	atts, err := db.ListAttachments(t.Context(), personal.ID, "att-1.ics")
	if err != nil || len(atts) != 1 || atts[0].Filename != "notes.txt" {
		t.Fatalf("stored %+v %v", atts, err)
	}
	if !strings.Contains(obj.ICS, atts[0].PublicID) {
		t.Fatalf("ICS missing managed id:\n%s", obj.ICS)
	}
	got := davDo(t, http.MethodGet, ts.URL+"/dav/attachments/"+atts[0].PublicID, "", "alice", "secret-pass", nil)
	raw, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if got.StatusCode != 200 || string(raw) != "hello" {
		t.Fatalf("dav download %d %q", got.StatusCode, raw)
	}
}
