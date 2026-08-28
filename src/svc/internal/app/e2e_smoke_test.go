package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devcoons/dcalcon/internal/app"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
)

func combinedServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateUser(t.Context(), "admin", "admin@localhost", "changeme1", "Administrator", "admin", "UTC"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.HTTP.PublicURL = "http://cal.example.test"
	cfg.SchedulingDomain = schedule.DefaultLocalDomain
	cfg.Auth.TokenKey = "test-token-key-for-aes-32b"
	schedule.SetLocalDomain(cfg.SchedulingDomain)
	ts := httptest.NewServer(app.CombinedHandler(db, cfg))
	t.Cleanup(ts.Close)
	return ts
}

func jarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func decodeJSON(t *testing.T, res *http.Response, dest any) []byte {
	t.Helper()
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if dest != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, dest); err != nil {
			t.Fatalf("json %s: %v", raw, err)
		}
	}
	return raw
}

func apiJSON(t *testing.T, c *http.Client, method, url string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return res, raw
}

func mustLogin(t *testing.T, c *http.Client, base, user, pass string) {
	t.Helper()
	res, raw := apiJSON(t, c, http.MethodPost, base+"/api/v1/auth/login", map[string]string{"username": user, "password": pass})
	if res.StatusCode != 200 {
		t.Fatalf("login %s: %d %s", user, res.StatusCode, raw)
	}
}

func davDo(t *testing.T, method, url, body, user, pass string, hdr map[string]string) (*http.Response, []byte) {
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
	res, err := (&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return res, raw
}

func asMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("map %s: %v", raw, err)
	}
	return m
}

func asList(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("list %s: %v", raw, err)
	}
	return list
}

func idOf(m map[string]any) int64 {
	switch v := m["id"].(type) {
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func findBy(list []map[string]any, key, want string) map[string]any {
	for _, it := range list {
		if fmt.Sprint(it[key]) == want {
			return it
		}
	}
	return nil
}

func TestCombinedFeatureSmoke(t *testing.T) {
	ts := combinedServer(t)
	base := ts.URL
	admin := jarClient(t)
	alice := jarClient(t)

	res, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	raw := decodeJSON(t, res, nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("ok")) {
		t.Fatalf("healthz %d %s", res.StatusCode, raw)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("security headers %v", res.Header)
	}
	res, err = http.Get(base + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	raw = decodeJSON(t, res, nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("ready")) {
		t.Fatalf("readyz %d %s", res.StatusCode, raw)
	}
	res, err = http.Get(base + "/version")
	if err != nil {
		t.Fatal(err)
	}
	raw = decodeJSON(t, res, nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("dCalCon")) && !bytes.Contains(raw, []byte("version")) {
		t.Fatalf("version %d %s", res.StatusCode, raw)
	}
	res, err = http.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	raw = decodeJSON(t, res, nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("dcalcon_schedule_errors_total")) {
		t.Fatalf("metrics %d %s", res.StatusCode, raw)
	}

	res, raw = davDo(t, http.MethodGet, base+"/.well-known/caldav", "", "", "", nil)
	if res.StatusCode != http.StatusMovedPermanently || !strings.Contains(res.Header.Get("Location"), "/dav/") {
		t.Fatalf("well-known caldav %d loc=%s %s", res.StatusCode, res.Header.Get("Location"), raw)
	}
	res, raw = davDo(t, http.MethodGet, base+"/.well-known/carddav", "", "", "", nil)
	if res.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("well-known carddav %d %s", res.StatusCode, raw)
	}

	mustLogin(t, admin, base, "admin", "changeme1")

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/me", nil)
	me := asMap(t, raw)
	if res.StatusCode != 200 || me["username"] != "admin" {
		t.Fatalf("me %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/setup", nil)
	setup := asMap(t, raw)
	if res.StatusCode != 200 || setup["scheduling_address"] != "admin@dcalcon.private" || setup["scheduling_domain"] != "dcalcon.private" {
		t.Fatalf("setup %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/overview", nil)
	if res.StatusCode != 200 {
		t.Fatalf("overview %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/calendars", nil)
	cals := asList(t, raw)
	if res.StatusCode != 200 {
		t.Fatalf("calendars %d %s", res.StatusCode, raw)
	}
	personal := findBy(cals, "kind", "personal")
	inbox := findBy(cals, "slug", "inbox")
	outbox := findBy(cals, "slug", "outbox")
	if personal == nil || inbox == nil || outbox == nil {
		t.Fatalf("expected personal/inbox/outbox, got %s", raw)
	}
	if inbox["read_only"] != true {
		t.Fatalf("inbox should be read-only: %+v", inbox)
	}
	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/webcal", idOf(inbox)), nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("inbox webcal want 400, got %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/calendars", map[string]string{"name": "Work", "color": "#334155"})
	work := asMap(t, raw)
	if res.StatusCode != 201 || idOf(work) == 0 {
		t.Fatalf("create calendar %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodPatch, base+"/api/v1/calendars/"+fmt.Sprint(idOf(work)), map[string]string{"name": "Office", "color": "#111111"})
	if res.StatusCode != 200 {
		t.Fatalf("patch calendar %d %s", res.StatusCode, raw)
	}

	pid := idOf(personal)
	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/events", pid), map[string]string{
		"summary": "Standup", "dtstart": "20260828T090000Z", "dtend": "20260828T100000Z", "location": "HQ",
	})
	ev := asMap(t, raw)
	if res.StatusCode != 201 || ev["href"] == "" {
		t.Fatalf("create event %d %s", res.StatusCode, raw)
	}
	href := fmt.Sprint(ev["href"])
	res, raw = apiJSON(t, admin, http.MethodGet, base+fmt.Sprintf("/api/v1/calendars/%d/events/%s", pid, href), nil)
	if res.StatusCode != 200 || asMap(t, raw)["summary"] != "Standup" {
		t.Fatalf("get event %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodPut, base+fmt.Sprintf("/api/v1/calendars/%d/events/%s", pid, href), map[string]string{
		"summary": "Standup 2", "dtstart": "20260828T090000Z", "dtend": "20260828T101500Z",
	})
	if res.StatusCode != 200 {
		t.Fatalf("update event %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/events", pid), map[string]any{
		"summary": "Weekly", "dtstart": "20260828T140000Z", "dtend": "20260828T150000Z",
		"rrule": "FREQ=WEEKLY;COUNT=4", "alarm_minutes": 10,
	})
	weekly := asMap(t, raw)
	if res.StatusCode != 201 || weekly["href"] == "" {
		t.Fatalf("rrule event %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodGet, base+fmt.Sprintf("/api/v1/calendars/%d/events/%s", pid, weekly["href"]), nil)
	weeklyGot := asMap(t, raw)
	if res.StatusCode != 200 || !strings.Contains(fmt.Sprint(weeklyGot["rrule"]), "FREQ=WEEKLY") || weeklyGot["alarm_minutes"] != float64(10) {
		t.Fatalf("rrule/alarm roundtrip %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/tasks", pid), map[string]string{
		"summary": "File taxes", "due": "20260830", "status": "NEEDS-ACTION",
	})
	task := asMap(t, raw)
	if res.StatusCode != 201 || task["href"] == "" {
		t.Fatalf("create task %d %s", res.StatusCode, raw)
	}
	taskHref := fmt.Sprint(task["href"])
	res, raw = apiJSON(t, admin, http.MethodGet, base+fmt.Sprintf("/api/v1/calendars/%d/events/%s", pid, taskHref), nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("task via events API want 404, got %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodPut, base+fmt.Sprintf("/api/v1/calendars/%d/tasks/%s", pid, taskHref), map[string]string{
		"summary": "File taxes", "due": "20260830", "status": "COMPLETED",
	})
	if res.StatusCode != 200 || asMap(t, raw)["status"] != "COMPLETED" {
		t.Fatalf("update task %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/tasks", nil)
	if res.StatusCode != 200 || findBy(asList(t, raw), "summary", "File taxes") == nil {
		t.Fatalf("list tasks %d %s", res.StatusCode, raw)
	}
	req, err := http.NewRequest(http.MethodGet, base+fmt.Sprintf("/api/v1/calendars/%d/export", pid), nil)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	expBody, _ := io.ReadAll(exp.Body)
	exp.Body.Close()
	if exp.StatusCode != 200 || !bytes.Contains(expBody, []byte("Standup")) {
		t.Fatalf("export %d %s", exp.StatusCode, expBody)
	}
	importICS := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VEVENT\r\nUID:imported-1\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260831T120000Z\r\nDTEND:20260831T130000Z\r\nSUMMARY:Imported\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	impReq, err := http.NewRequest(http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/import", pid), strings.NewReader(importICS))
	if err != nil {
		t.Fatal(err)
	}
	impReq.Header.Set("Content-Type", "text/calendar")
	imp, err := admin.Do(impReq)
	if err != nil {
		t.Fatal(err)
	}
	impBody, _ := io.ReadAll(imp.Body)
	imp.Body.Close()
	if imp.StatusCode != 200 || !bytes.Contains(impBody, []byte(`"created":1`)) && !bytes.Contains(impBody, []byte(`"created": 1`)) {
		t.Fatalf("import %d %s", imp.StatusCode, impBody)
	}
	takeReq, err := http.NewRequest(http.MethodGet, base+"/api/v1/me/export", nil)
	if err != nil {
		t.Fatal(err)
	}
	take, err := admin.Do(takeReq)
	if err != nil {
		t.Fatal(err)
	}
	takeBody, _ := io.ReadAll(take.Body)
	take.Body.Close()
	if take.StatusCode != 200 || !bytes.HasPrefix(takeBody, []byte("PK")) {
		t.Fatalf("takeout %d prefix=%q", take.StatusCode, takeBody[:min(len(takeBody), 8)])
	}
	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/webcal", pid), nil)
	web := asMap(t, raw)
	if res.StatusCode != 200 || web["url"] == "" {
		t.Fatalf("webcal %d %s", res.StatusCode, raw)
	}
	token := fmt.Sprint(web["token"])
	pub, err := http.Get(base + "/webcal/" + token + ".ics")
	if err != nil {
		t.Fatal(err)
	}
	pubBody, _ := io.ReadAll(pub.Body)
	pub.Body.Close()
	if pub.StatusCode != 200 || !bytes.Contains(pubBody, []byte("Standup")) {
		t.Fatalf("public webcal %d %s", pub.StatusCode, pubBody)
	}
	res, raw = apiJSON(t, admin, http.MethodGet, base+fmt.Sprintf("/api/v1/calendars/%d/webcal", pid), nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte(`"enabled":true`)) && !bytes.Contains(raw, []byte(`"enabled": true`)) {
		t.Fatalf("webcal get %d %s", res.StatusCode, raw)
	}
	if bytes.Contains(raw, []byte(token)) {
		t.Fatal("webcal GET re-displayed secret token")
	}
	res, raw = apiJSON(t, admin, http.MethodDelete, base+fmt.Sprintf("/api/v1/calendars/%d/webcal", pid), nil)
	if res.StatusCode != 200 {
		t.Fatalf("webcal delete %d %s", res.StatusCode, raw)
	}
	pubOff, err := http.Get(base + "/webcal/" + token + ".ics")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(pubOff.Body)
	pubOff.Body.Close()
	if pubOff.StatusCode != 404 {
		t.Fatalf("disabled webcal want 404, got %d", pubOff.StatusCode)
	}
	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/freebusy?users=admin&start=20260828T000000Z&end=20260829T000000Z", nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("start")) {
		t.Fatalf("freebusy %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/admin/users", map[string]any{
		"username": "alice", "email": "alice@example.com", "password": "alice-pass",
		"display_name": "Alice", "role": "user", "timezone": "UTC",
	})
	if res.StatusCode != 201 {
		t.Fatalf("create alice %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/directory", nil)
	dir := asList(t, raw)
	if res.StatusCode != 200 || findBy(dir, "username", "alice") == nil {
		t.Fatalf("directory %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/addressbooks", nil)
	books := asList(t, raw)
	people := findBy(books, "slug", "people")
	contacts := findBy(books, "slug", "contacts")
	if people == nil || people["read_only"] != true || contacts == nil {
		t.Fatalf("books %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodGet, base+fmt.Sprintf("/api/v1/addressbooks/%d/contacts", idOf(people)), nil)
	cards := asList(t, raw)
	if res.StatusCode != 200 || len(cards) == 0 {
		t.Fatalf("people contacts should refresh on access, got %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/addressbooks/%d/contacts", idOf(people)), map[string]string{"fn": "Nope"})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("people POST want 403, got %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/addressbooks/%d/contacts", idOf(contacts)), map[string]string{
		"fn": "Ada Lovelace", "email": "ada@example.com", "bday": "1815-12-10",
	})
	createdCard := asMap(t, raw)
	if res.StatusCode != 201 {
		t.Fatalf("create contact %d %s", res.StatusCode, raw)
	}
	cardHref := fmt.Sprint(createdCard["href"])
	res, raw = apiJSON(t, admin, http.MethodPut, base+fmt.Sprintf("/api/v1/addressbooks/%d/contacts/%s", idOf(contacts), cardHref), map[string]string{
		"fn": "Ada", "email": "ada@example.com",
	})
	if res.StatusCode != 200 {
		t.Fatalf("update contact %d %s", res.StatusCode, raw)
	}
	res, _ = apiJSON(t, admin, http.MethodGet, base+fmt.Sprintf("/api/v1/addressbooks/%d/contacts/export", idOf(contacts)), nil)
	if res.StatusCode != 200 {
		t.Fatalf("export contacts %d", res.StatusCode)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/events/%d/invite", pid), map[string]any{
		"href": href, "emails": []string{"alice@dcalcon.private"},
	})
	inv := asMap(t, raw)
	if res.StatusCode != 200 || inv["local"] != float64(1) {
		t.Fatalf("local invite %d %s", res.StatusCode, raw)
	}

	mustLogin(t, alice, base, "alice", "alice-pass")
	res, raw = apiJSON(t, alice, http.MethodPost, base+"/api/v1/calendars", map[string]string{"name": "Travel", "color": "#0F766E"})
	travel := asMap(t, raw)
	if res.StatusCode != 201 || idOf(travel) == 0 {
		t.Fatalf("alice travel calendar %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, alice, http.MethodGet, base+"/api/v1/invitations", nil)
	invites := asList(t, raw)
	pending := findBy(invites, "status", "pending")
	if pending == nil {
		t.Fatalf("alice pending invite %s", raw)
	}
	res, raw = apiJSON(t, alice, http.MethodPost, base+fmt.Sprintf("/api/v1/invitations/%d/accept", idOf(pending)), map[string]any{"calendar_id": idOf(travel)})
	if res.StatusCode != 200 {
		t.Fatalf("accept %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, alice, http.MethodGet, base+fmt.Sprintf("/api/v1/calendars/%d/events", idOf(travel)), nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("Standup")) {
		t.Fatalf("accepted event missing on travel calendar %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, alice, http.MethodGet, base+"/api/v1/invitations", nil)
	for _, it := range asList(t, raw) {
		if it["status"] == "pending" && strings.Contains(fmt.Sprint(it["summary"]), "Standup") {
			t.Fatalf("accepted invite still pending: %s", raw)
		}
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/events", pid), map[string]any{
		"summary": "Skip", "dtstart": "20260829T090000Z", "dtend": "20260829T100000Z",
		"invite": []string{"alice"},
	})
	if res.StatusCode != 201 {
		t.Fatalf("event with invite %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, alice, http.MethodGet, base+"/api/v1/invitations", nil)
	var skipID int64
	for _, it := range asList(t, raw) {
		if it["status"] == "pending" && it["summary"] == "Skip" {
			skipID = idOf(it)
		}
	}
	if skipID == 0 {
		t.Fatalf("missing skip invite %s", raw)
	}
	res, raw = apiJSON(t, alice, http.MethodPost, base+fmt.Sprintf("/api/v1/invitations/%d/decline", skipID), map[string]string{})
	if res.StatusCode != 200 {
		t.Fatalf("decline %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+fmt.Sprintf("/api/v1/calendars/%d/shares", pid), map[string]string{"username": "alice", "access": "write"})
	if res.StatusCode != 201 && res.StatusCode != 200 {
		t.Fatalf("share %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, alice, http.MethodGet, base+"/api/v1/calendars", nil)
	if findBy(asList(t, raw), "shared", "true") == nil && findBy(asList(t, raw), "access", "write") == nil {
		sharedOK := false
		for _, c := range asList(t, raw) {
			if c["shared"] == true {
				sharedOK = true
			}
		}
		if !sharedOK {
			t.Fatalf("alice should see shared calendar %s", raw)
		}
	}

	res, raw = apiJSON(t, admin, http.MethodPut, base+"/api/v1/settings/important-dates", map[string]any{
		"enabled": true, "include_birthdays": true, "include_anniversaries": true, "alarm_offsets": []string{"-P1D"},
	})
	if res.StatusCode != 200 {
		t.Fatalf("important dates %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/accounts", map[string]any{
		"provider": "smtp", "email": "admin@example.com", "host": "smtp.example.com",
		"port": 587, "username": "admin@example.com", "password": "secret",
	})
	if res.StatusCode != 201 {
		t.Fatalf("smtp account %d %s", res.StatusCode, raw)
	}
	accID := idOf(asMap(t, raw))
	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/mail", nil)
	if res.StatusCode != 200 {
		t.Fatalf("mail %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodDelete, base+fmt.Sprintf("/api/v1/accounts/%d", accID), nil)
	if res.StatusCode != 200 {
		t.Fatalf("delete account %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/me/app-passwords", map[string]string{"name": "phone"})
	appPW := asMap(t, raw)
	secret := fmt.Sprint(appPW["password"])
	if res.StatusCode != 201 || !strings.HasPrefix(secret, "dcc_") {
		t.Fatalf("app password %d %s", res.StatusCode, raw)
	}
	denied := jarClient(t)
	res, raw = apiJSON(t, denied, http.MethodPost, base+"/api/v1/auth/login", map[string]string{"username": "admin", "password": secret})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("app password must not login, got %d %s", res.StatusCode, raw)
	}

	res, raw = davDo(t, "PROPFIND", base+"/dav/principals/admin/", `<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><C:calendar-user-address-set/><C:schedule-inbox-URL/></D:prop></D:propfind>`, "admin", secret, map[string]string{"Content-Type": "application/xml", "Depth": "0"})
	if res.StatusCode != 207 || !bytes.Contains(raw, []byte("mailto:admin@dcalcon.private")) || !bytes.Contains(raw, []byte("/dav/calendars/admin/inbox/")) {
		t.Fatalf("principal PROPFIND %d %s", res.StatusCode, raw)
	}

	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//test//EN\r\nBEGIN:VEVENT\r\nUID:dav-meet\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260830T090000Z\r\nDTEND:20260830T100000Z\r\nSUMMARY:DAV Meet\r\nORGANIZER:mailto:admin@dcalcon.private\r\nATTENDEE;CN=Alice:mailto:alice@dcalcon.private\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	res, raw = davDo(t, http.MethodPut, base+"/dav/calendars/admin/personal/dav-meet.ics", ics, "admin", secret, map[string]string{"Content-Type": "text/calendar"})
	if res.StatusCode != 201 && res.StatusCode != 204 && res.StatusCode != 200 {
		t.Fatalf("caldav PUT %d %s", res.StatusCode, raw)
	}
	etag := res.Header.Get("ETag")
	res, raw = davDo(t, http.MethodPut, base+"/dav/calendars/admin/personal/dav-meet.ics", strings.ReplaceAll(ics, "DAV Meet", "Nope"), "admin", secret, map[string]string{"Content-Type": "text/calendar", "If-Match": `"deadbeef"`})
	if res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("If-Match want 412, got %d %s", res.StatusCode, raw)
	}
	_ = etag

	res, raw = davDo(t, http.MethodDelete, base+"/dav/addressbooks/admin/people/alice.vcf", "", "admin", secret, nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("people CardDAV DELETE want 403, got %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, alice, http.MethodGet, base+"/api/v1/invitations", nil)
	davInvite := false
	for _, it := range asList(t, raw) {
		if it["summary"] == "DAV Meet" && it["status"] == "pending" {
			davInvite = true
		}
	}
	if !davInvite {
		t.Fatalf("caldav PUT should deliver local invite, invitations=%s", raw)
	}

	fbReport := `<?xml version="1.0"?><C:free-busy-query xmlns:C="urn:ietf:params:xml:ns:caldav"><C:time-range start="20260828T000000Z" end="20260829T000000Z"/></C:free-busy-query>`
	res, raw = davDo(t, "REPORT", base+"/dav/calendars/admin/personal/", fbReport, "admin", secret, map[string]string{"Content-Type": "application/xml", "Depth": "0"})
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("BEGIN:VFREEBUSY")) {
		t.Fatalf("free-busy REPORT %d %s", res.StatusCode, raw)
	}
	res, raw = davDo(t, "REPORT", base+"/dav/calendars/admin/personal/", `<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><C:calendar-query xmlns:C="urn:ietf:params:xml:ns:caldav">&xxe;</C:calendar-query>`, "admin", secret, map[string]string{"Content-Type": "application/xml"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("entity REPORT want 400, got %d %s", res.StatusCode, raw)
	}
	outboxICS := "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nPRODID:-//dCalCon//test//EN\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:outbox-meet\r\nDTSTAMP:20260801T000000Z\r\nDTSTART:20260901T090000Z\r\nDTEND:20260901T100000Z\r\nSUMMARY:Outbox Meet\r\nORGANIZER:mailto:admin@dcalcon.private\r\nATTENDEE:mailto:alice@dcalcon.private\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	res, raw = davDo(t, http.MethodPost, base+"/dav/calendars/admin/outbox/", outboxICS, "admin", secret, map[string]string{"Content-Type": "text/calendar"})
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("schedule-response")) {
		t.Fatalf("outbox POST %d %s", res.StatusCode, raw)
	}
	res, raw = davDo(t, "PROPFIND", base+"/dav/principals/alice/", `<D:propfind xmlns:D="DAV:" xmlns:CS="http://calendarserver.org/ns/"><D:prop><CS:calendar-proxy-write-for/></D:prop></D:propfind>`, "alice", "alice-pass", map[string]string{"Content-Type": "application/xml", "Depth": "0"})
	if res.StatusCode != 207 || !bytes.Contains(raw, []byte("x-share-")) {
		t.Fatalf("alice calendar-proxy-write-for %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodDelete, base+fmt.Sprintf("/api/v1/calendars/%d/events/%s", pid, href), nil)
	if res.StatusCode != 200 {
		t.Fatalf("delete standup %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, alice, http.MethodGet, base+fmt.Sprintf("/api/v1/calendars/%d/events", idOf(travel)), nil)
	if res.StatusCode != 200 || bytes.Contains(raw, []byte("Standup")) {
		t.Fatalf("cancel should remove accepted copy %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/admin/audit", nil)
	if res.StatusCode != 200 || !bytes.Contains(raw, []byte("webcal.rotate")) {
		t.Fatalf("audit %d %s", res.StatusCode, raw)
	}

	otherSess := jarClient(t)
	mustLogin(t, otherSess, base, "admin", "changeme1")
	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/me/sessions/revoke", nil)
	if res.StatusCode != 200 {
		t.Fatalf("revoke sessions %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/me", nil)
	if res.StatusCode != 200 {
		t.Fatalf("current session should survive revoke %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, otherSess, http.MethodGet, base+"/api/v1/me", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("other session should die %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/me/totp/setup", map[string]string{})
	if res.StatusCode != 200 || asMap(t, raw)["secret"] == "" {
		t.Fatalf("totp setup %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodDelete, base+"/api/v1/me/totp/setup", nil)
	if res.StatusCode != 200 {
		t.Fatalf("totp cancel %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/auth/recover", map[string]string{"email": "admin@localhost"})
	if res.StatusCode != 200 {
		t.Fatalf("recover %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodDelete, base+fmt.Sprintf("/api/v1/addressbooks/%d/contacts/%s", idOf(contacts), cardHref), nil)
	if res.StatusCode != 200 {
		t.Fatalf("delete contact %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/admin/users", nil)
	if res.StatusCode != 200 || findBy(asList(t, raw), "username", "alice") == nil {
		t.Fatalf("admin users %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, alice, http.MethodGet, base+"/api/v1/admin/users", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("alice admin want 403, got %d %s", res.StatusCode, raw)
	}

	res, raw = apiJSON(t, admin, http.MethodPost, base+"/api/v1/auth/logout", nil)
	if res.StatusCode != 200 {
		t.Fatalf("logout %d %s", res.StatusCode, raw)
	}
	res, raw = apiJSON(t, admin, http.MethodGet, base+"/api/v1/me", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout want 401, got %d %s", res.StatusCode, raw)
	}
}
