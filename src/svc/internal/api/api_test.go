package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devcoons/dcalcon/internal/api"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/devcoons/dcalcon/internal/otp"
	"github.com/devcoons/dcalcon/internal/providers"
	"github.com/devcoons/dcalcon/internal/storage"
)

func newTestServer(t *testing.T) (*httptest.Server, *storage.DB) {
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
	cfg.Auth.SessionTTL = time.Hour
	cfg.Auth.TokenKey = "test-token-key-for-aes"
	ts := httptest.NewServer(api.New(db, cfg))
	t.Cleanup(ts.Close)
	return ts, db
}

func client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 8 * time.Second}
}

func postJSON(t *testing.T, c *http.Client, rawURL string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	res, err := c.Post(rawURL, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func doJSON(t *testing.T, c *http.Client, method, rawURL string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, rdr)
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
	return res
}

func decode(t *testing.T, res *http.Response, dest any) {
	t.Helper()
	defer res.Body.Close()
	if dest == nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return
	}
	if err := json.NewDecoder(res.Body).Decode(dest); err != nil {
		t.Fatal(err)
	}
}

func login(t *testing.T, ts *httptest.Server, c *http.Client, user, pass string) int {
	t.Helper()
	res := postJSON(t, c, ts.URL+"/api/v1/auth/login", map[string]string{"username": user, "password": pass})
	status := res.StatusCode
	decode(t, res, nil)
	return status
}

func loginTOTP(t *testing.T, ts *httptest.Server, c *http.Client, user, pass, code string) int {
	t.Helper()
	res := postJSON(t, c, ts.URL+"/api/v1/auth/login", map[string]string{"username": user, "password": pass, "totp": code})
	status := res.StatusCode
	decode(t, res, nil)
	return status
}

func TestLoginAndMe(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "wrong-password"); st != http.StatusUnauthorized {
		t.Fatalf("bad password: %d", st)
	}
	if st := login(t, ts, c, "admin", "changeme1"); st != http.StatusOK {
		t.Fatalf("login: %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	var me storage.User
	decode(t, res, &me)
	if res.StatusCode != 200 || me.Username != "admin" || me.Role != "admin" {
		t.Fatalf("me: %d %+v", res.StatusCode, me)
	}
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	ts, _ := newTestServer(t)
	keep := client(t)
	other := client(t)
	if st := login(t, ts, keep, "admin", "changeme1"); st != 200 {
		t.Fatalf("login keep %d", st)
	}
	if st := login(t, ts, other, "admin", "changeme1"); st != 200 {
		t.Fatalf("login other %d", st)
	}
	res := postJSON(t, keep, ts.URL+"/api/v1/me/password", map[string]string{
		"current_password": "changeme1", "new_password": "changed99",
	})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("change password %d", res.StatusCode)
	}
	res, err := other.Get(ts.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, res, nil)
	if res.StatusCode != 401 {
		t.Fatalf("other session should die, got %d", res.StatusCode)
	}
	res, err = keep.Get(ts.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("current session should stay, got %d", res.StatusCode)
	}
}

func TestWebcalOnlyPersonalCalendars(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	var inbox int64
	for _, cal := range cals {
		if cal.Kind == "inbox" {
			inbox = cal.ID
		}
	}
	if inbox == 0 {
		t.Fatal("missing inbox calendar")
	}
	res = postJSON(t, c, ts.URL+"/api/v1/calendars/"+itoa(inbox)+"/webcal", map[string]string{})
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("inbox webcal want 400 got %d", res.StatusCode)
	}
}

func TestUserWorkflowAndCalDAVSetup(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := client(t)
	if st := login(t, ts, admin, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}

	res := postJSON(t, admin, ts.URL+"/api/v1/admin/users", map[string]string{
		"username": "bob", "email": "not-an-email", "password": "password1",
	})
	if res.StatusCode != 400 {
		t.Fatalf("expected invalid email, got %d", res.StatusCode)
	}
	decode(t, res, nil)

	res = postJSON(t, admin, ts.URL+"/api/v1/admin/users", map[string]string{
		"username": "bob", "email": "bob@example.com", "password": "short",
	})
	if res.StatusCode != 400 {
		t.Fatalf("expected short password, got %d", res.StatusCode)
	}
	decode(t, res, nil)

	res = postJSON(t, admin, ts.URL+"/api/v1/admin/users", map[string]any{
		"username": "bob", "email": "bob@example.com", "password": "s3cret-bob",
		"display_name": "Bob", "role": "user", "timezone": "Europe/Athens",
	})
	if res.StatusCode != 201 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("create user %d %s", res.StatusCode, b)
	}
	var created struct {
		User  storage.User      `json:"user"`
		Setup map[string]string `json:"setup"`
	}
	decode(t, res, &created)
	if created.User.Username != "bob" || created.Setup["calendar_home"] == "" {
		t.Fatalf("created payload %+v", created)
	}
	if created.Setup["calendar_home"] != "http://cal.example.test/dav/calendars/bob/" {
		t.Fatalf("calendar home %s", created.Setup["calendar_home"])
	}

	bob := client(t)
	if st := login(t, ts, bob, "bob", "s3cret-bob"); st != 200 {
		t.Fatalf("bob login %d", st)
	}

	res = doJSON(t, bob, http.MethodPatch, ts.URL+"/api/v1/me", map[string]string{
		"display_name": "Robert", "email": "robert@example.com", "timezone": "Europe/Berlin",
	})
	var me storage.User
	decode(t, res, &me)
	if res.StatusCode != 200 || me.DisplayName != "Robert" || me.Timezone != "Europe/Berlin" {
		t.Fatalf("patch me %d %+v", res.StatusCode, me)
	}

	res = postJSON(t, bob, ts.URL+"/api/v1/me/password", map[string]string{
		"current_password": "s3cret-bob", "new_password": "s3cret-bob2",
	})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("password %d", res.StatusCode)
	}
	bob2 := client(t)
	if st := login(t, ts, bob2, "bob", "s3cret-bob2"); st != 200 {
		t.Fatalf("relogin %d", st)
	}

	res, err := bob2.Get(ts.URL + "/api/v1/overview")
	if err != nil {
		t.Fatal(err)
	}
	var ov map[string]any
	decode(t, res, &ov)
	if res.StatusCode != 200 || ov["calendars"].(float64) < 1 {
		t.Fatalf("overview %+v", ov)
	}

	res = postJSON(t, bob2, ts.URL+"/api/v1/calendars", map[string]string{"name": "Work", "color": "#334155"})
	var cal storage.Calendar
	decode(t, res, &cal)
	if res.StatusCode != 201 {
		t.Fatalf("calendar %d", res.StatusCode)
	}

	res = postJSON(t, bob2, ts.URL+"/api/v1/calendars/"+itoa(cal.ID)+"/events", map[string]string{
		"summary": "Standup", "dtstart": "20260828T090000Z", "dtend": "20260828T093000Z",
	})
	var ev map[string]string
	decode(t, res, &ev)
	if res.StatusCode != 201 || ev["href"] == "" {
		t.Fatalf("event %d %v", res.StatusCode, ev)
	}

	res, err = bob2.Get(ts.URL + "/api/v1/addressbooks")
	if err != nil {
		t.Fatal(err)
	}
	var books []storage.AddressBook
	decode(t, res, &books)
	if len(books) == 0 {
		t.Fatal("no address book")
	}
	res = postJSON(t, bob2, ts.URL+"/api/v1/addressbooks/"+itoa(contactsBook(t, books).ID)+"/contacts", map[string]string{
		"fn": "Ada Lovelace", "email": "ada@example.com", "bday": "1815-12-10",
	})
	decode(t, res, nil)
	if res.StatusCode != 201 {
		t.Fatalf("contact %d", res.StatusCode)
	}

	res = doJSON(t, bob2, http.MethodPut, ts.URL+"/api/v1/settings/important-dates", map[string]any{
		"enabled": true, "include_birthdays": true, "include_anniversaries": true, "alarm_offsets": []string{"-P1D", "-P7D"},
	})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("important dates %d", res.StatusCode)
	}

	res = postJSON(t, bob2, ts.URL+"/api/v1/events/"+itoa(cal.ID)+"/invite", map[string]any{
		"href": ev["href"], "usernames": []string{"admin"},
	})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("invite %d", res.StatusCode)
	}

	res, err = admin.Get(ts.URL + "/api/v1/invitations")
	if err != nil {
		t.Fatal(err)
	}
	var inv []map[string]any
	decode(t, res, &inv)
	if len(inv) == 0 {
		t.Fatal("admin should have an invitation")
	}
	id := int(inv[0]["id"].(float64))
	res = postJSON(t, admin, ts.URL+"/api/v1/invitations/"+itoa(int64(id))+"/accept", map[string]string{})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("accept %d", res.StatusCode)
	}

	res = doJSON(t, admin, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+itoa(created.User.ID), map[string]string{"status": "disabled"})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("disable %d", res.StatusCode)
	}
	if st := login(t, ts, client(t), "bob", "s3cret-bob2"); st != 401 {
		t.Fatalf("disabled user still logs in: %d", st)
	}

	res = doJSON(t, admin, http.MethodPatch, ts.URL+"/api/v1/admin/users/1", map[string]string{"status": "disabled"})
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("should not disable last/self admin: %d", res.StatusCode)
	}

	u, _ := url.Parse(ts.URL)
	if len(admin.Jar.Cookies(u)) == 0 {
		t.Fatal("expected session cookie")
	}
}

func TestNonAdminForbidden(t *testing.T) {
	ts, db := newTestServer(t)
	if _, err := db.CreateUser(t.Context(), "cara", "cara@example.com", "password1", "Cara", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	c := client(t)
	if st := login(t, ts, c, "cara", "password1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/admin/users")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, res, nil)
	if res.StatusCode != 403 {
		t.Fatalf("want 403 got %d", res.StatusCode)
	}
}

func TestRecoveryAndEdit(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := client(t)
	if st := login(t, ts, admin, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}

	res := postJSON(t, admin, ts.URL+"/api/v1/admin/users", map[string]any{
		"username": "dana", "email": "dana@example.com", "password": "password1",
		"display_name": "Dana", "role": "user", "timezone": "UTC",
	})
	var created struct {
		User storage.User `json:"user"`
	}
	decode(t, res, &created)
	if res.StatusCode != 201 {
		t.Fatalf("create %d", res.StatusCode)
	}

	res = postJSON(t, client(t), ts.URL+"/api/v1/auth/recover", map[string]string{"email": "nobody@example.com"})
	var rec map[string]any
	decode(t, res, &rec)
	if res.StatusCode != 200 || rec["ok"] != true {
		t.Fatalf("unknown email recover %d %v", res.StatusCode, rec)
	}
	if _, ok := rec["recovery_url"]; ok {
		t.Fatal("public recover must not return the URL")
	}
	if _, ok := rec["emailed"]; ok {
		t.Fatal("public recover must not say whether mail was sent")
	}

	res, err := admin.Get(ts.URL + "/api/v1/admin/recovery-outbox")
	if err != nil {
		t.Fatal(err)
	}
	var box []map[string]any
	decode(t, res, &box)
	if res.StatusCode != 200 {
		t.Fatalf("outbox %d", res.StatusCode)
	}
	for _, m := range box {
		if _, ok := m["recovery_url"]; ok {
			t.Fatalf("outbox must not include recovery_url: %v", m)
		}
		if m["username"] == "" && m["user_id"] == nil {
			t.Fatalf("outbox row missing user: %v", m)
		}
	}

	res = postJSON(t, admin, ts.URL+"/api/v1/admin/users/"+itoa(created.User.ID)+"/recovery", map[string]string{})
	var sent struct {
		OK          bool   `json:"ok"`
		RecoveryURL string `json:"recovery_url"`
	}
	decode(t, res, &sent)
	if res.StatusCode != 200 || sent.RecoveryURL == "" {
		t.Fatalf("admin recovery %d %+v", res.StatusCode, sent)
	}
	token := sent.RecoveryURL[strings.LastIndex(sent.RecoveryURL, "/")+1:]

	res = postJSON(t, client(t), ts.URL+"/api/v1/auth/reset", map[string]string{"token": token, "password": "newpass99"})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("reset %d", res.StatusCode)
	}
	if st := login(t, ts, client(t), "dana", "password1"); st != 401 {
		t.Fatalf("old password still works: %d", st)
	}
	if st := login(t, ts, client(t), "dana", "newpass99"); st != 200 {
		t.Fatalf("new password %d", st)
	}

	res = postJSON(t, client(t), ts.URL+"/api/v1/auth/reset", map[string]string{"token": token, "password": "another99"})
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("reused token %d", res.StatusCode)
	}

	dana := client(t)
	if st := login(t, ts, dana, "dana", "newpass99"); st != 200 {
		t.Fatalf("dana login %d", st)
	}
	res, err = dana.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	var personal storage.Calendar
	for _, c := range cals {
		if c.Kind == "personal" {
			personal = c
		}
	}
	if personal.ID == 0 {
		t.Fatal("no personal calendar")
	}
	res = postJSON(t, dana, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/events", map[string]string{
		"summary": "Draft", "dtstart": "20260901T100000Z", "dtend": "20260901T110000Z",
	})
	var ev map[string]string
	decode(t, res, &ev)
	if res.StatusCode != 201 {
		t.Fatalf("event %d", res.StatusCode)
	}
	res = doJSON(t, dana, http.MethodPut, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/events/"+ev["href"], map[string]string{
		"summary": "Renamed", "dtstart": "20260901T100000Z", "dtend": "20260901T113000Z", "description": "notes",
	})
	var updated map[string]any
	decode(t, res, &updated)
	if res.StatusCode != 200 || updated["summary"] != "Renamed" {
		t.Fatalf("update event %d %v", res.StatusCode, updated)
	}

	res, err = dana.Get(ts.URL + "/api/v1/addressbooks")
	if err != nil {
		t.Fatal(err)
	}
	var books []storage.AddressBook
	decode(t, res, &books)
	bookID := contactsBook(t, books).ID
	res = postJSON(t, dana, ts.URL+"/api/v1/addressbooks/"+itoa(bookID)+"/contacts", map[string]string{
		"fn": "Pat", "email": "pat@example.com",
	})
	var ct map[string]string
	decode(t, res, &ct)
	res = doJSON(t, dana, http.MethodPut, ts.URL+"/api/v1/addressbooks/"+itoa(bookID)+"/contacts/"+ct["href"], map[string]string{
		"fn": "Patricia", "email": "patricia@example.com", "tel": "555",
	})
	var ct2 map[string]any
	decode(t, res, &ct2)
	if res.StatusCode != 200 || ct2["fn"] != "Patricia" {
		t.Fatalf("update contact %d %v", res.StatusCode, ct2)
	}

	res = postJSON(t, dana, ts.URL+"/api/v1/addressbooks/"+itoa(bookID)+"/contacts", map[string]any{
		"fn": "Grace Hopper", "given_name": "Grace", "family_name": "Hopper",
		"org": "US Navy", "title": "Rear Admiral",
		"emails":    []map[string]string{{"value": "grace@navy.test", "type": "work"}},
		"addresses": []map[string]string{{"type": "work", "city": "Arlington", "country": "US"}},
		"custom":    []map[string]string{{"name": "Department", "value": "Computing"}},
		"note":      "COBOL",
	})
	var rich map[string]string
	decode(t, res, &rich)
	if res.StatusCode != 201 {
		t.Fatalf("rich contact %d", res.StatusCode)
	}
	res, err = dana.Get(ts.URL + "/api/v1/addressbooks/" + itoa(bookID) + "/contacts/" + rich["href"])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	decode(t, res, &got)
	if res.StatusCode != 200 || got["org"] != "US Navy" || got["note"] != "COBOL" {
		t.Fatalf("get rich %d %v", res.StatusCode, got)
	}
	customs, _ := got["custom"].([]any)
	if len(customs) != 1 {
		t.Fatalf("custom %v", got["custom"])
	}
}

func TestTOTPEnrollLoginResetAndCalendarShare(t *testing.T) {
	ts, db := newTestServer(t)
	admin := client(t)
	if st := login(t, ts, admin, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}

	res := postJSON(t, admin, ts.URL+"/api/v1/me/totp/enable", map[string]string{"code": "123456"})
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("enable without setup %d", res.StatusCode)
	}

	res = postJSON(t, admin, ts.URL+"/api/v1/me/totp/setup", map[string]string{})
	var setup struct {
		Secret  string `json:"secret"`
		Otpauth string `json:"otpauth"`
	}
	decode(t, res, &setup)
	if res.StatusCode != 200 || setup.Secret == "" || setup.Otpauth == "" {
		t.Fatalf("setup %d %+v", res.StatusCode, setup)
	}

	res = postJSON(t, admin, ts.URL+"/api/v1/me/totp/enable", map[string]string{"code": "000000"})
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("bad code should not enable: %d", res.StatusCode)
	}

	code, err := otp.Code(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	res = postJSON(t, admin, ts.URL+"/api/v1/me/totp/enable", map[string]string{"code": code})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("enable %d", res.StatusCode)
	}
	var stored string
	if err := db.SQL.QueryRowContext(t.Context(), `SELECT totp_secret FROM users WHERE username = 'admin'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == setup.Secret || !strings.HasPrefix(stored, "s1:") {
		t.Fatalf("totp secret stored in plaintext: %q", stored)
	}

	res, err = admin.Get(ts.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	var me storage.User
	decode(t, res, &me)
	if !me.TOTPEnabled {
		t.Fatal("expected totp enabled")
	}

	totpClient := client(t)
	code, err = otp.Code(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	res = postJSON(t, totpClient, ts.URL+"/api/v1/auth/login", map[string]string{"username": "admin", "totp": code})
	decode(t, res, nil)
	if res.StatusCode != 401 {
		t.Fatalf("totp-only login should fail: %d", res.StatusCode)
	}

	res = postJSON(t, client(t), ts.URL+"/api/v1/auth/login", map[string]string{"username": "admin", "password": "changeme1"})
	var totpErr struct {
		Error string `json:"error"`
	}
	decode(t, res, &totpErr)
	if res.StatusCode != 401 || totpErr.Error != "authenticator code required" {
		t.Fatalf("password without totp: %d %s", res.StatusCode, totpErr.Error)
	}

	code, err = otp.Code(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st := loginTOTP(t, ts, client(t), "admin", "changeme1", code); st != 200 {
		t.Fatalf("password+totp login %d", st)
	}

	code, err = otp.Code(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	res = postJSON(t, client(t), ts.URL+"/api/v1/auth/reset-totp", map[string]string{
		"username": "admin", "code": code, "password": "newadmin99",
	})
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("reset-totp %d", res.StatusCode)
	}
	if st := login(t, ts, client(t), "admin", "changeme1"); st != 401 {
		t.Fatalf("old password still works %d", st)
	}
	admin = client(t)
	code, err = otp.Code(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st := loginTOTP(t, ts, admin, "admin", "newadmin99", code); st != 200 {
		t.Fatalf("new password+totp %d", st)
	}

	res = postJSON(t, admin, ts.URL+"/api/v1/admin/users", map[string]any{
		"username": "erin", "email": "erin@example.com", "password": "password1",
		"display_name": "Erin", "role": "user", "timezone": "UTC",
	})
	var created struct {
		User storage.User `json:"user"`
	}
	decode(t, res, &created)
	if res.StatusCode != 201 {
		t.Fatalf("create erin %d", res.StatusCode)
	}

	res, err = admin.Get(ts.URL + "/api/v1/directory")
	if err != nil {
		t.Fatal(err)
	}
	var dir []storage.DirectoryUser
	decode(t, res, &dir)
	if res.StatusCode != 200 || len(dir) == 0 {
		t.Fatalf("directory %d %v", res.StatusCode, dir)
	}

	res, err = admin.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	var personal storage.Calendar
	for _, c := range cals {
		if c.Kind == "personal" && !c.Shared {
			personal = c
		}
	}
	if personal.ID == 0 {
		t.Fatal("no owned personal calendar")
	}

	res = postJSON(t, admin, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/shares", map[string]string{
		"username": "erin", "access": "write",
	})
	var shares []storage.CalendarShare
	decode(t, res, &shares)
	if res.StatusCode != 200 || len(shares) != 1 {
		t.Fatalf("share %d %v", res.StatusCode, shares)
	}

	erin := client(t)
	if st := login(t, ts, erin, "erin", "password1"); st != 200 {
		t.Fatalf("erin login %d", st)
	}
	res, err = erin.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, res, &cals)
	var shared storage.Calendar
	for _, c := range cals {
		if c.Shared && c.ID == personal.ID {
			shared = c
		}
	}
	if shared.ID == 0 || !shared.CanWrite() {
		t.Fatalf("erin should see writable share %+v", shared)
	}

	res = postJSON(t, erin, ts.URL+"/api/v1/calendars/"+itoa(shared.ID)+"/events", map[string]any{
		"summary": "Shared standup", "dtstart": "20260828T090000", "dtend": "20260828T093000",
		"location": "Room A", "description": "weekly",
	})
	var ev map[string]string
	decode(t, res, &ev)
	if res.StatusCode != 201 {
		t.Fatalf("shared write %d %v", res.StatusCode, ev)
	}

	res, err = admin.Get(ts.URL + "/api/v1/calendars/" + itoa(personal.ID) + "/events")
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	decode(t, res, &events)
	if len(events) == 0 || events[0]["location"] != "Room A" {
		t.Fatalf("owner should see shared write %v", events)
	}
	if _, ok := events[0]["ics"]; ok {
		t.Fatal("event lists should omit raw ics")
	}

	res = postJSON(t, admin, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/shares", map[string]string{
		"username": "erin", "access": "read",
	})
	decode(t, res, &shares)
	if res.StatusCode != 200 {
		t.Fatalf("downgrade %d", res.StatusCode)
	}
	res = postJSON(t, erin, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/events", map[string]string{
		"summary": "Nope", "dtstart": "20260829T090000",
	})
	decode(t, res, nil)
	if res.StatusCode != 403 {
		t.Fatalf("read share must not write: %d", res.StatusCode)
	}

	_, _ = db, created
}

func TestMailAccountsAndInvites(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}

	res, err := c.Get(ts.URL + "/api/v1/mail")
	if err != nil {
		t.Fatal(err)
	}
	var mail map[string]any
	decode(t, res, &mail)
	if res.StatusCode != 200 || mail["token_key"] != true {
		t.Fatalf("mail status %d %+v", res.StatusCode, mail)
	}

	res = postJSON(t, c, ts.URL+"/api/v1/accounts", map[string]any{"provider": "google", "origin": ts.URL})
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("oauth without client id: %d", res.StatusCode)
	}

	res = postJSON(t, c, ts.URL+"/api/v1/accounts", map[string]any{
		"provider": "smtp",
		"email":    "ada@example.com",
		"host":     "smtp.example.com",
		"port":     587,
		"username": "ada@example.com",
		"password": "secret",
	})
	var acc map[string]any
	decode(t, res, &acc)
	if res.StatusCode != 201 || acc["email"] != "ada@example.com" {
		t.Fatalf("smtp %d %+v", res.StatusCode, acc)
	}
	id := int64(acc["id"].(float64))

	res, err = c.Get(ts.URL + "/api/v1/accounts")
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	decode(t, res, &list)
	if len(list) != 1 || list[0]["provider"] != "smtp" {
		t.Fatalf("list %+v", list)
	}

	res = doJSON(t, c, http.MethodDelete, ts.URL+"/api/v1/accounts/"+itoa(id), nil)
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("delete %d", res.StatusCode)
	}

	res = postJSON(t, c, ts.URL+"/api/v1/calendars", map[string]string{"name": "Mail"})
	var cal storage.Calendar
	decode(t, res, &cal)
	if res.StatusCode != 201 {
		t.Fatalf("calendar %d", res.StatusCode)
	}
	res = postJSON(t, c, ts.URL+"/api/v1/calendars/"+itoa(cal.ID)+"/events", map[string]string{
		"summary": "External", "dtstart": "20260828T090000Z", "dtend": "20260828T100000Z",
	})
	var ev map[string]any
	decode(t, res, &ev)
	if res.StatusCode != 201 {
		t.Fatalf("event %d %+v", res.StatusCode, ev)
	}
	res = postJSON(t, c, ts.URL+"/api/v1/events/"+itoa(cal.ID)+"/invite", map[string]any{
		"href": ev["href"], "emails": []string{"guest@example.net"},
	})
	var invErr map[string]any
	decode(t, res, &invErr)
	if res.StatusCode != 400 {
		t.Fatalf("invite without mail %d %+v", res.StatusCode, invErr)
	}

	res = doJSON(t, c, http.MethodPatch, ts.URL+"/api/v1/calendars/"+itoa(cal.ID), map[string]string{"name": "Mailing", "color": "#334155"})
	decode(t, res, &cal)
	if res.StatusCode != 200 || cal.Name != "Mailing" {
		t.Fatalf("rename %d %+v", res.StatusCode, cal)
	}
	res = doJSON(t, c, http.MethodDelete, ts.URL+"/api/v1/calendars/"+itoa(cal.ID), nil)
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("delete extra calendar %d", res.StatusCode)
	}
	res, err = c.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var remaining []storage.Calendar
	decode(t, res, &remaining)
	var personalID int64
	for _, ccal := range remaining {
		if ccal.Kind == "personal" && !ccal.Shared {
			personalID = ccal.ID
			break
		}
	}
	if personalID == 0 {
		t.Fatal("expected a remaining personal calendar")
	}
	res = doJSON(t, c, http.MethodDelete, ts.URL+"/api/v1/calendars/"+itoa(personalID), nil)
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("last personal calendar should stay: %d", res.StatusCode)
	}

	var outboxID int64
	for _, ccal := range remaining {
		if ccal.Kind == "outbox" {
			outboxID = ccal.ID
			break
		}
	}
	if outboxID == 0 {
		t.Fatal("expected outbox")
	}
	res = postJSON(t, c, ts.URL+"/api/v1/calendars/"+itoa(outboxID)+"/events", map[string]string{
		"summary": "Nope", "dtstart": "20260828T090000Z",
	})
	decode(t, res, nil)
	if res.StatusCode != 403 {
		t.Fatalf("outbox writes should be forbidden: %d", res.StatusCode)
	}
}

func TestOAuthStartAndCallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"email": "ada@gmail.com"})
	})
	oauth := httptest.NewServer(mux)
	t.Cleanup(oauth.Close)

	prev := providers.Google
	providers.Google = providers.Endpoints{
		Auth: oauth.URL + "/auth", Token: oauth.URL + "/token",
		Userinfo: oauth.URL + "/userinfo", Send: oauth.URL + "/send",
	}
	t.Cleanup(func() { providers.Google = prev })

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
	cfg.Auth.SessionTTL = time.Hour
	cfg.Auth.TokenKey = "test-token-key-for-aes"
	cfg.OAuth.GoogleClientID = "cid"
	cfg.OAuth.GoogleClientSecret = "csecret"
	h := api.NewHandler(db, cfg)
	h.HTTP = oauth.Client()
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)

	c := client(t)
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}

	res := postJSON(t, c, ts.URL+"/api/v1/accounts", map[string]any{"provider": "google", "origin": ts.URL})
	var out map[string]string
	decode(t, res, &out)
	if res.StatusCode != 200 || !strings.Contains(out["authorize_url"], "client_id=cid") {
		t.Fatalf("authorize %d %v", res.StatusCode, out)
	}
	u, err := url.Parse(out["authorize_url"])
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	if state == "" {
		t.Fatal("missing state")
	}

	res, err = c.Get(ts.URL + "/api/v1/oauth/google/callback?code=ok&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	decode(t, res, nil)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("callback %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "mail=ok") {
		t.Fatalf("redirect %s", loc)
	}

	res, err = c.Get(ts.URL + "/api/v1/accounts")
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	decode(t, res, &list)
	if len(list) != 1 || list[0]["email"] != "ada@gmail.com" || list[0]["provider"] != "google" {
		t.Fatalf("connected %+v", list)
	}

	res, err = c.Get(ts.URL + "/api/v1/oauth/google/callback?code=x&state=nope")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, res, nil)
	if res.StatusCode != http.StatusFound || !strings.Contains(res.Header.Get("Location"), "mail=err") {
		t.Fatalf("bad state %d %s", res.StatusCode, res.Header.Get("Location"))
	}
}

func contactsBook(t *testing.T, books []storage.AddressBook) storage.AddressBook {
	t.Helper()
	for _, b := range books {
		if b.Slug == "contacts" {
			return b
		}
	}
	if len(books) == 0 {
		t.Fatal("no address book")
	}
	t.Fatalf("no contacts book in %+v", books)
	return storage.AddressBook{}
}

func itoa(n int64) string {
	return jsonNumber(n)
}

func jsonNumber(n int64) string {
	return string(mustMarshal(n))
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestLoginLockoutAndAppPasswords(t *testing.T) {
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
	cfg.Auth.MaxAttempts = 2
	cfg.Auth.Lockout = time.Minute
	cfg.Auth.SessionTTL = time.Hour
	h := api.NewHandler(db, cfg)
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)
	c := client(t)
	if st := login(t, ts, c, "admin", "wrong-password"); st != http.StatusUnauthorized {
		t.Fatalf("fail 1: %d", st)
	}
	if st := login(t, ts, c, "admin", "wrong-password"); st != http.StatusTooManyRequests && st != http.StatusUnauthorized {
		t.Fatalf("fail 2: %d", st)
	}
	if st := login(t, ts, c, "admin", "wrong-password"); st != http.StatusTooManyRequests {
		t.Fatalf("locked: %d", st)
	}

	ts2, _ := newTestServer(t)
	c2 := client(t)
	if st := login(t, ts2, c2, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res := postJSON(t, c2, ts2.URL+"/api/v1/me/app-passwords", map[string]string{"name": "phone"})
	var created struct {
		ID       int64  `json:"id"`
		Password string `json:"password"`
		Prefix   string `json:"prefix"`
	}
	decode(t, res, &created)
	if res.StatusCode != 201 || !strings.HasPrefix(created.Password, "dcc_") {
		t.Fatalf("create app password %d %+v", res.StatusCode, created)
	}
	listRes, err := c2.Get(ts2.URL + "/api/v1/me/app-passwords")
	if err != nil {
		t.Fatal(err)
	}
	var list []storage.AppPassword
	decode(t, listRes, &list)
	if listRes.StatusCode != 200 || len(list) != 1 {
		t.Fatalf("list %d %+v", listRes.StatusCode, list)
	}
	denied := client(t)
	if st := login(t, ts2, denied, "admin", created.Password); st != http.StatusUnauthorized {
		t.Fatalf("app password must not login to dashboard, got %d", st)
	}
	del := doJSON(t, c2, http.MethodDelete, ts2.URL+"/api/v1/me/app-passwords/"+itoa(created.ID), nil)
	decode(t, del, nil)
	if del.StatusCode != 200 {
		t.Fatalf("revoke %d", del.StatusCode)
	}
}

func postVCard(t *testing.T, c *http.Client, rawURL, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/vcard")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func readRaw(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestContactCRUDAndImportExport(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/addressbooks")
	if err != nil {
		t.Fatal(err)
	}
	var books []storage.AddressBook
	decode(t, res, &books)
	if res.StatusCode != 200 || len(books) == 0 {
		t.Fatalf("books %d %+v", res.StatusCode, books)
	}
	base := ts.URL + "/api/v1/addressbooks/" + itoa(contactsBook(t, books).ID) + "/contacts"

	anon := client(t)
	unauth, err := anon.Get(base + "/export")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, unauth, nil)
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth export %d", unauth.StatusCode)
	}
	missing, err := c.Get(ts.URL + "/api/v1/addressbooks/99999/contacts")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, missing, nil)
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing book %d", missing.StatusCode)
	}

	empty := postVCard(t, c, base+"/import", "not a vcard")
	decode(t, empty, nil)
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty import %d", empty.StatusCode)
	}

	created := postJSON(t, c, base, map[string]string{"fn": "Local Person", "email": "local@example.com"})
	var local struct {
		Href string `json:"href"`
		UID  string `json:"uid"`
	}
	decode(t, created, &local)
	if created.StatusCode != http.StatusCreated || local.Href == "" || !strings.HasSuffix(local.Href, ".vcf") {
		t.Fatalf("create %d %+v", created.StatusCode, local)
	}

	got, err := c.Get(base + "/" + local.Href)
	if err != nil {
		t.Fatal(err)
	}
	var dto map[string]any
	decode(t, got, &dto)
	if got.StatusCode != 200 || dto["fn"] != "Local Person" {
		t.Fatalf("get json %d %v", got.StatusCode, dto)
	}

	one, err := c.Get(base + "/" + local.Href + "/vcard")
	if err != nil {
		t.Fatal(err)
	}
	oneBody := readRaw(t, one)
	if one.StatusCode != 200 || !strings.Contains(oneBody, "FN:Local Person") {
		t.Fatalf("export one %d %s", one.StatusCode, oneBody)
	}
	if !strings.Contains(one.Header.Get("Content-Type"), "text/vcard") {
		t.Fatalf("content-type %s", one.Header.Get("Content-Type"))
	}

	upd := doJSON(t, c, http.MethodPut, base+"/"+local.Href, map[string]string{
		"fn": "Local Updated", "email": "local@example.com",
	})
	decode(t, upd, &dto)
	if upd.StatusCode != 200 || dto["fn"] != "Local Updated" {
		t.Fatalf("update %d %v", upd.StatusCode, dto)
	}

	bundle := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:import-ada\r\nFN:Ada Lovelace\r\nEMAIL:ada@example.com\r\nEND:VCARD\r\nBEGIN:VCARD\r\nVERSION:3.0\r\nUID:import-al\r\nFN:Al\r\nTEL:+1\r\nEND:VCARD\r\nBEGIN:VCARD\r\nVERSION:3.0\r\nFN:No UID Yet\r\nEND:VCARD\r\n"
	imp := postVCard(t, c, base+"/import", bundle)
	var stats struct {
		Created int      `json:"created"`
		Updated int      `json:"updated"`
		Skipped int      `json:"skipped"`
		Errors  []string `json:"errors"`
	}
	decode(t, imp, &stats)
	if imp.StatusCode != 200 || stats.Created != 3 {
		t.Fatalf("import %d %+v", imp.StatusCode, stats)
	}

	again := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:import-ada\r\nFN:Ada Updated\r\nEMAIL:ada@example.com\r\nEND:VCARD\r\n"
	imp2 := postVCard(t, c, base+"/import", again)
	decode(t, imp2, &stats)
	if imp2.StatusCode != 200 || stats.Updated != 1 || stats.Created != 0 {
		t.Fatalf("reimport %d %+v", imp2.StatusCode, stats)
	}

	ada, err := c.Get(base + "/import-ada.vcf")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, ada, &dto)
	if ada.StatusCode != 200 || dto["fn"] != "Ada Updated" {
		t.Fatalf("get imported %d %v", ada.StatusCode, dto)
	}

	huge := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:too-big\r\nFN:Too Big\r\nPHOTO;ENCODING=b;TYPE=JPEG:" +
		strings.Repeat("A", limits.MaxPhotoBytes+8) + "\r\nEND:VCARD\r\n"
	skip := postVCard(t, c, base+"/import", huge)
	decode(t, skip, &stats)
	if skip.StatusCode != 200 || stats.Skipped != 1 || stats.Created != 0 {
		t.Fatalf("skip oversized %d %+v", skip.StatusCode, stats)
	}

	exp, err := c.Get(base + "/export")
	if err != nil {
		t.Fatal(err)
	}
	body := readRaw(t, exp)
	if exp.StatusCode != 200 || !strings.Contains(body, "Ada Updated") || !strings.Contains(body, "FN:Al") || !strings.Contains(body, "Local Updated") {
		t.Fatalf("export %d %s", exp.StatusCode, body)
	}
	if !strings.Contains(exp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition %s", exp.Header.Get("Content-Disposition"))
	}

	oneImp, err := c.Get(base + "/import-ada.vcf/vcard")
	if err != nil {
		t.Fatal(err)
	}
	oneImpBody := readRaw(t, oneImp)
	if oneImp.StatusCode != 200 || !strings.Contains(oneImpBody, "Ada Updated") {
		t.Fatalf("one imported %d %s", oneImp.StatusCode, oneImpBody)
	}

	list, err := c.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	var contacts []map[string]any
	decode(t, list, &contacts)
	if list.StatusCode != 200 || len(contacts) != 4 {
		t.Fatalf("list %d n=%d", list.StatusCode, len(contacts))
	}

	del := doJSON(t, c, http.MethodDelete, base+"/"+local.Href, nil)
	decode(t, del, nil)
	if del.StatusCode != 200 {
		t.Fatalf("delete %d", del.StatusCode)
	}
	gone, err := c.Get(base + "/" + local.Href)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, gone, nil)
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted still there %d", gone.StatusCode)
	}
}

func TestContactZipImport(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/addressbooks")
	if err != nil {
		t.Fatal(err)
	}
	var books []storage.AddressBook
	decode(t, res, &books)
	base := ts.URL + "/api/v1/addressbooks/" + itoa(contactsBook(t, books).ID) + "/contacts"

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"folder/ada.vcf": "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:zip-ada\r\nFN:Zip Ada\r\nEND:VCARD\r\n",
		"bob.vcf":        "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:zip-bob\r\nFN:Zip Bob\r\nEND:VCARD\r\n",
		"readme.txt":     "not a card",
	} {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, base+"/import", bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/zip")
	imp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var stats struct {
		Created int `json:"created"`
		Skipped int `json:"skipped"`
	}
	decode(t, imp, &stats)
	if imp.StatusCode != 200 || stats.Created != 2 {
		t.Fatalf("zip import %d %+v", imp.StatusCode, stats)
	}

	list, err := c.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	var contacts []map[string]any
	decode(t, list, &contacts)
	if list.StatusCode != 200 || len(contacts) != 2 {
		t.Fatalf("list %d n=%d", list.StatusCode, len(contacts))
	}

	emptyZip := bytes.Buffer{}
	emptyW := zip.NewWriter(&emptyZip)
	f, err := emptyW.Create("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(f, "hello")
	_ = emptyW.Close()
	req, _ = http.NewRequest(http.MethodPost, base+"/import", bytes.NewReader(emptyZip.Bytes()))
	req.Header.Set("Content-Type", "application/zip")
	empty, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, empty, nil)
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty zip %d", empty.StatusCode)
	}
}

func TestPeopleBookAndPrivateInvite(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := client(t)
	if st := login(t, ts, admin, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res := postJSON(t, admin, ts.URL+"/api/v1/admin/users", map[string]any{
		"username": "zoe", "email": "zoe@example.com", "password": "password1",
		"display_name": "Zoe", "role": "user", "timezone": "UTC",
	})
	decode(t, res, nil)
	if res.StatusCode != 201 {
		t.Fatalf("create zoe %d", res.StatusCode)
	}

	res, err := admin.Get(ts.URL + "/api/v1/setup")
	if err != nil {
		t.Fatal(err)
	}
	var setup map[string]any
	decode(t, res, &setup)
	if setup["scheduling_address"] != "admin@dcalcon.private" {
		t.Fatalf("setup %+v", setup)
	}

	res, err = admin.Get(ts.URL + "/api/v1/directory")
	if err != nil {
		t.Fatal(err)
	}
	var dir []storage.DirectoryUser
	decode(t, res, &dir)
	if len(dir) == 0 || dir[0].LocalEmail == "" {
		t.Fatalf("directory %+v", dir)
	}

	res, err = admin.Get(ts.URL + "/api/v1/addressbooks")
	if err != nil {
		t.Fatal(err)
	}
	var books []storage.AddressBook
	decode(t, res, &books)
	var people *storage.AddressBook
	for i := range books {
		if books[i].Slug == "people" {
			people = &books[i]
		}
	}
	if people == nil || !people.ReadOnly {
		t.Fatalf("people book %+v", books)
	}
	res, err = admin.Get(ts.URL + "/api/v1/addressbooks/" + itoa(people.ID) + "/contacts")
	if err != nil {
		t.Fatal(err)
	}
	var cards []map[string]any
	decode(t, res, &cards)
	if len(cards) == 0 {
		t.Fatal("expected people contacts")
	}
	blocked := postJSON(t, admin, ts.URL+"/api/v1/addressbooks/"+itoa(people.ID)+"/contacts", map[string]string{
		"fn": "Nope", "email": "nope@example.com",
	})
	decode(t, blocked, nil)
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("people book should be read-only, got %d", blocked.StatusCode)
	}

	res, err = admin.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	var personal storage.Calendar
	for _, c := range cals {
		if c.Kind == "personal" && !c.Shared {
			personal = c
			break
		}
	}
	res = postJSON(t, admin, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/events", map[string]string{
		"summary": "Team", "dtstart": "20260828T090000Z", "dtend": "20260828T100000Z",
	})
	var ev map[string]any
	decode(t, res, &ev)
	if res.StatusCode != 201 {
		t.Fatalf("event %d %+v", res.StatusCode, ev)
	}
	res = postJSON(t, admin, ts.URL+"/api/v1/events/"+itoa(personal.ID)+"/invite", map[string]any{
		"href": ev["href"], "emails": []string{"zoe@dcalcon.private"},
	})
	var inv map[string]any
	decode(t, res, &inv)
	if res.StatusCode != 200 {
		t.Fatalf("invite %d %+v", res.StatusCode, inv)
	}
	if n, _ := inv["local"].(float64); n != 1 {
		t.Fatalf("want 1 local invite, got %+v", inv)
	}

	zoe := client(t)
	if st := login(t, ts, zoe, "zoe", "password1"); st != 200 {
		t.Fatalf("zoe login %d", st)
	}
	res, err = zoe.Get(ts.URL + "/api/v1/invitations")
	if err != nil {
		t.Fatal(err)
	}
	var inbox []map[string]any
	decode(t, res, &inbox)
	if len(inbox) == 0 {
		t.Fatal("zoe should see the invitation")
	}
}

func TestISOEventTimesAndPartialTask(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	var personal storage.Calendar
	for _, cal := range cals {
		if cal.Kind == "personal" && !cal.Shared {
			personal = cal
			break
		}
	}
	if personal.ID == 0 {
		t.Fatal("no personal calendar")
	}
	res = postJSON(t, c, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/events", map[string]any{
		"summary": "ISO", "dtstart": "2026-08-28T15:04", "dtend": "2026-08-28T16:04",
	})
	var ev map[string]any
	decode(t, res, &ev)
	if res.StatusCode != 201 {
		t.Fatalf("iso event %d %+v", res.StatusCode, ev)
	}
	res, err = c.Get(ts.URL + "/api/v1/calendars/" + itoa(personal.ID) + "/events/" + ev["href"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	decode(t, res, &got)
	ds := fmt.Sprint(got["dtstart"])
	if !strings.Contains(ds, "T") {
		t.Fatalf("dtstart should be compact ICS with T, got %q", ds)
	}

	due := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	res, err = c.Get(ts.URL + "/api/v1/overview")
	if err != nil {
		t.Fatal(err)
	}
	var ovBefore map[string]any
	decode(t, res, &ovBefore)
	soonBefore, _ := ovBefore["events_soon"].(float64)

	res = postJSON(t, c, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/tasks", map[string]string{
		"summary": "Pay rent", "due": due, "status": "NEEDS-ACTION",
	})
	var task map[string]any
	decode(t, res, &task)
	if res.StatusCode != 201 {
		t.Fatalf("task %d %+v", res.StatusCode, task)
	}
	res, err = c.Get(ts.URL + "/api/v1/calendars/" + itoa(personal.ID) + "/tasks/" + task["href"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var gotTask map[string]any
	decode(t, res, &gotTask)
	if res.StatusCode != 200 || gotTask["summary"] != "Pay rent" {
		t.Fatalf("get task %d %+v", res.StatusCode, gotTask)
	}

	res, err = c.Get(ts.URL + "/api/v1/overview")
	if err != nil {
		t.Fatal(err)
	}
	var ov map[string]any
	decode(t, res, &ov)
	if res.StatusCode != 200 {
		t.Fatalf("overview %d %+v", res.StatusCode, ov)
	}
	soonAfter, _ := ov["events_soon"].(float64)
	if soonAfter != soonBefore {
		t.Fatalf("events_soon should ignore tasks: before %v after %v", soonBefore, soonAfter)
	}
	foundTask := false
	upcoming, _ := ov["upcoming"].([]any)
	for _, raw := range upcoming {
		u, _ := raw.(map[string]any)
		if u["kind"] == "task" && u["summary"] == "Pay rent" {
			foundTask = true
			ds := fmt.Sprint(u["dtstart"])
			want := strings.ReplaceAll(due, "-", "")
			if !strings.HasPrefix(ds, want) {
				t.Fatalf("task dtstart %q want prefix %q", ds, want)
			}
		}
	}
	if !foundTask {
		t.Fatalf("open due task missing from overview: %+v", ov["upcoming"])
	}

	res = doJSON(t, c, http.MethodPut, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/tasks/"+task["href"].(string), map[string]string{
		"status": "COMPLETED",
	})
	var updated map[string]any
	decode(t, res, &updated)
	if res.StatusCode != 200 || updated["status"] != "COMPLETED" || updated["summary"] != "Pay rent" {
		t.Fatalf("partial task update %d %+v", res.StatusCode, updated)
	}

	res, err = c.Get(ts.URL + "/api/v1/overview")
	if err != nil {
		t.Fatal(err)
	}
	var ovDone map[string]any
	decode(t, res, &ovDone)
	upcomingDone, _ := ovDone["upcoming"].([]any)
	for _, raw := range upcomingDone {
		u, _ := raw.(map[string]any)
		if u["kind"] == "task" && u["summary"] == "Pay rent" {
			t.Fatal("completed task should not appear on overview")
		}
	}
}

func TestEventAndTaskAttachments(t *testing.T) {
	ts, db := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	var personal storage.Calendar
	for _, cal := range cals {
		if cal.Kind == "personal" && !cal.Shared {
			personal = cal
			break
		}
	}
	if personal.ID == 0 {
		t.Fatal("no personal calendar")
	}
	base := ts.URL + "/api/v1/calendars/" + itoa(personal.ID)

	res = postJSON(t, c, base+"/events", map[string]any{
		"summary": "With file", "dtstart": "20260828T090000", "dtend": "20260828T100000",
	})
	var ev map[string]any
	decode(t, res, &ev)
	if res.StatusCode != 201 {
		t.Fatalf("event %d %+v", res.StatusCode, ev)
	}
	href := ev["href"].(string)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "agenda.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("bring slides"))
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, base+"/events/"+href+"/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var added []storage.Attachment
	decode(t, res, &added)
	if res.StatusCode != 201 || len(added) != 1 || added[0].Filename != "agenda.txt" || added[0].PublicID == "" {
		t.Fatalf("upload %d %+v", res.StatusCode, added)
	}
	eventAtt := added[0]

	res, err = c.Get(base + "/events/" + href)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	decode(t, res, &got)
	atts, _ := got["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("event dto attachments %+v", got["attachments"])
	}

	res, err = c.Get(base + "/attachments/" + eventAtt.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(body) != "bring slides" {
		t.Fatalf("download %d %q", res.StatusCode, body)
	}
	if !strings.Contains(res.Header.Get("Content-Disposition"), "agenda.txt") {
		t.Fatalf("disposition %s", res.Header.Get("Content-Disposition"))
	}

	buf.Reset()
	w = multipart.NewWriter(&buf)
	part, err = w.CreateFormFile("file", "note.html")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("<html><script>alert(1)</script>"))
	_ = w.Close()
	req, _ = http.NewRequest(http.MethodPost, base+"/events/"+href+"/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var htmlAtts []storage.Attachment
	decode(t, res, &htmlAtts)
	if res.StatusCode != 201 || len(htmlAtts) != 1 {
		t.Fatalf("html upload %d %+v", res.StatusCode, htmlAtts)
	}
	res, err = c.Get(base + "/attachments/" + htmlAtts[0].PublicID)
	if err != nil {
		t.Fatal(err)
	}
	htmlBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Contains(htmlBody, []byte("<script>")) {
		t.Fatalf("html download %d", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("html must not be served as HTML, got %s", res.Header.Get("Content-Type"))
	}
	if !strings.Contains(res.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("html disposition %s", res.Header.Get("Content-Disposition"))
	}
	if res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache %s", res.Header.Get("Cache-Control"))
	}

	obj, err := db.CalendarObjectByHref(t.Context(), personal.ID, href)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(obj.ICS, "MANAGED-ID="+eventAtt.PublicID) {
		t.Fatalf("ICS should reference attachment:\n%s", obj.ICS)
	}

	res = postJSON(t, c, base+"/tasks", map[string]string{"summary": "Read PDF", "due": "2026-09-01"})
	var task map[string]any
	decode(t, res, &task)
	if res.StatusCode != 201 {
		t.Fatalf("task %d %+v", res.StatusCode, task)
	}
	thref := task["href"].(string)
	buf.Reset()
	w = multipart.NewWriter(&buf)
	part, _ = w.CreateFormFile("file", "spec.pdf")
	_, _ = part.Write([]byte("%PDF"))
	_ = w.Close()
	req, _ = http.NewRequest(http.MethodPost, base+"/tasks/"+thref+"/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var taskAtts []storage.Attachment
	decode(t, res, &taskAtts)
	if res.StatusCode != 201 || len(taskAtts) != 1 || taskAtts[0].Filename != "spec.pdf" {
		t.Fatalf("task upload %d %+v", res.StatusCode, taskAtts)
	}

	res = doJSON(t, c, http.MethodDelete, base+"/events/"+href+"/attachments/"+eventAtt.PublicID, nil)
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("delete %d", res.StatusCode)
	}
	res = doJSON(t, c, http.MethodDelete, base+"/events/"+href+"/attachments/"+htmlAtts[0].PublicID, nil)
	decode(t, res, nil)
	if res.StatusCode != 200 {
		t.Fatalf("delete html %d", res.StatusCode)
	}
	res, _ = c.Get(base + "/events/" + href)
	got = map[string]any{}
	decode(t, res, &got)
	atts, _ = got["attachments"].([]any)
	if len(atts) != 0 {
		t.Fatalf("expected no attachments after delete, got %+v", atts)
	}
}

func TestRejectUnsafeEventUID(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	res, err := c.Get(ts.URL + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	var personal storage.Calendar
	for _, cal := range cals {
		if cal.Kind == "personal" && !cal.Shared {
			personal = cal
			break
		}
	}
	res = postJSON(t, c, ts.URL+"/api/v1/calendars/"+itoa(personal.ID)+"/events", map[string]any{
		"summary": "x", "dtstart": "20260828T090000", "dtend": "20260828T100000", "uid": "../inbox/x",
	})
	decode(t, res, nil)
	if res.StatusCode != 400 {
		t.Fatalf("unsafe uid want 400 got %d", res.StatusCode)
	}
}

func personalCal(t *testing.T, c *http.Client, base string) storage.Calendar {
	t.Helper()
	res, err := c.Get(base + "/api/v1/calendars")
	if err != nil {
		t.Fatal(err)
	}
	var cals []storage.Calendar
	decode(t, res, &cals)
	for _, cal := range cals {
		if cal.Kind == "personal" && !cal.Shared {
			return cal
		}
	}
	t.Fatal("no personal calendar")
	return storage.Calendar{}
}

func postBackupZip(t *testing.T, c *http.Client, rawURL string, zipData []byte, password string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "backup.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(zipData); err != nil {
		t.Fatal(err)
	}
	if password != "" {
		if err := w.WriteField("password", password); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestUserBackupDataRoundTrip(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}
	cal := personalCal(t, c, ts.URL)
	res := postJSON(t, c, ts.URL+"/api/v1/calendars/"+itoa(cal.ID)+"/events", map[string]string{
		"summary": "Backup meet", "dtstart": "20260828T090000Z", "dtend": "20260828T100000Z",
	})
	var ev map[string]string
	decode(t, res, &ev)
	if res.StatusCode != 201 || ev["href"] == "" {
		t.Fatalf("event %d %v", res.StatusCode, ev)
	}
	booksRes, err := c.Get(ts.URL + "/api/v1/addressbooks")
	if err != nil {
		t.Fatal(err)
	}
	var books []storage.AddressBook
	decode(t, booksRes, &books)
	cres := postJSON(t, c, ts.URL+"/api/v1/addressbooks/"+itoa(contactsBook(t, books).ID)+"/contacts", map[string]string{
		"fn": "Backup Person", "email": "backup@example.com",
	})
	decode(t, cres, nil)
	if cres.StatusCode != 201 {
		t.Fatalf("contact %d", cres.StatusCode)
	}

	exp, err := c.Get(ts.URL + "/api/v1/me/backup?kind=data")
	if err != nil {
		t.Fatal(err)
	}
	zipData, err := io.ReadAll(exp.Body)
	exp.Body.Close()
	if exp.StatusCode != 200 || !bytes.HasPrefix(zipData, []byte("PK")) {
		t.Fatalf("export %d prefix=%q", exp.StatusCode, zipData[:min(len(zipData), 8)])
	}
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatal(err)
	}
	foundMan := false
	for _, f := range zr.File {
		if f.Name == "dcalcon.json" {
			foundMan = true
			break
		}
	}
	if !foundMan {
		t.Fatal("data zip missing dcalcon.json")
	}

	del := doJSON(t, c, http.MethodDelete, ts.URL+"/api/v1/calendars/"+itoa(cal.ID)+"/events/"+ev["href"], nil)
	decode(t, del, nil)
	if del.StatusCode != 200 {
		t.Fatalf("delete %d", del.StatusCode)
	}

	imp := postBackupZip(t, c, ts.URL+"/api/v1/me/backup", zipData, "")
	var result map[string]any
	decode(t, imp, &result)
	if imp.StatusCode != 200 {
		t.Fatalf("import %d %+v", imp.StatusCode, result)
	}

	got, err := c.Get(ts.URL + "/api/v1/calendars/" + itoa(cal.ID) + "/events")
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	decode(t, got, &events)
	found := false
	for _, e := range events {
		if e["summary"] == "Backup meet" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("event missing after restore: %+v", events)
	}
}

func TestUserBackupFullRoundTrip(t *testing.T) {
	ts, db := newTestServer(t)
	c := client(t)
	if st := login(t, ts, c, "admin", "changeme1"); st != 200 {
		t.Fatalf("login %d", st)
	}

	denied := postJSON(t, c, ts.URL+"/api/v1/me/backup/export", map[string]string{"kind": "full"})
	decode(t, denied, nil)
	if denied.StatusCode != http.StatusUnauthorized {
		t.Fatalf("full export without password want 401 got %d", denied.StatusCode)
	}
	fullGET, err := c.Get(ts.URL + "/api/v1/me/backup?kind=full")
	if err != nil {
		t.Fatal(err)
	}
	decode(t, fullGET, nil)
	if fullGET.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET full want 400 got %d", fullGET.StatusCode)
	}

	exp := postJSON(t, c, ts.URL+"/api/v1/me/backup/export", map[string]string{"kind": "full", "password": "changeme1"})
	zipData, err := io.ReadAll(exp.Body)
	exp.Body.Close()
	if exp.StatusCode != 200 || !bytes.HasPrefix(zipData, []byte("PK")) {
		t.Fatalf("full export %d prefix=%q", exp.StatusCode, zipData[:min(len(zipData), 8)])
	}

	chg := postJSON(t, c, ts.URL+"/api/v1/me/password", map[string]string{
		"current_password": "changeme1", "new_password": "newpass99",
	})
	decode(t, chg, nil)
	if chg.StatusCode != 200 {
		t.Fatalf("change password %d", chg.StatusCode)
	}

	noPass := postBackupZip(t, c, ts.URL+"/api/v1/me/backup", zipData, "")
	decode(t, noPass, nil)
	if noPass.StatusCode != http.StatusUnauthorized {
		t.Fatalf("full import without password want 401 got %d", noPass.StatusCode)
	}

	imp := postBackupZip(t, c, ts.URL+"/api/v1/me/backup", zipData, "newpass99")
	var result map[string]any
	decode(t, imp, &result)
	if imp.StatusCode != 200 {
		t.Fatalf("full import %d %+v", imp.StatusCode, result)
	}
	if _, err := db.Authenticate(t.Context(), "admin", "changeme1"); err != nil {
		t.Fatalf("old password should work after full restore: %v", err)
	}

	evil := bytes.Buffer{}
	zw := zip.NewWriter(&evil)
	f, err := zw.Create("../account.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(`{"format":"dcalcon.user-backup"}`))
	_ = zw.Close()
	bad := postBackupZip(t, c, ts.URL+"/api/v1/me/backup", evil.Bytes(), "changeme1")
	decode(t, bad, nil)
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("zip slip want 400 got %d", bad.StatusCode)
	}
}
