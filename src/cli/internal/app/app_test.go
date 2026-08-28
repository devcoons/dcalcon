package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devcoons/dcalcon/cli/internal/client"
	"github.com/devcoons/dcalcon/cli/internal/config"
)

func TestHelp(t *testing.T) {
	out, err := runCLI(t, nil, "help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dcalcon-cli") || !strings.Contains(out, "calendar list") || !strings.Contains(out, "task get") {
		t.Fatalf("help:\n%s", out)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, err := runCLI(t, nil, "nope")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("got %v", err)
	}
}

func TestCalendarListRequiresLogin(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cli.json")
	_, err := runCLI(t, nil, "--config", cfg, "calendar", "list")
	if err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("got %v", err)
	}
}

func TestLoginStoresSessionAndListsCalendars(t *testing.T) {
	var sawBearer bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login":
			if r.Header.Get("Origin") != "" {
				t.Errorf("CLI must not send Origin, got %q", r.Header.Get("Origin"))
			}
			http.SetCookie(w, &http.Cookie{Name: client.SessionCookie, Value: "sess-abc", Path: "/"})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":1,"username":"admin","display_name":"Admin","role":"admin","email":"admin@local","status":"active"}`))
		case r.URL.Path == "/api/v1/calendars":
			if got := r.Header.Get("Authorization"); got != "Bearer sess-abc" {
				t.Errorf("authorization %q", got)
			}
			c, _ := r.Cookie(client.SessionCookie)
			if c == nil || c.Value != "sess-abc" {
				t.Errorf("cookie %+v", c)
			}
			sawBearer = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"slug":"personal","name":"Personal","kind":"calendar","access":"owner"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.json")
	out, err := runCLI(t, nil, "--url", srv.URL, "--config", cfgPath, "login", "--user", "admin", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "signed in as admin") {
		t.Fatalf("login out %q", out)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var f config.File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Session != "sess-abc" || f.Username != "admin" || f.URL != srv.URL {
		t.Fatalf("config %+v", f)
	}
	st, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}

	out, err = runCLI(t, nil, "--config", cfgPath, "calendar", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !sawBearer {
		t.Fatal("calendar list did not hit API")
	}
	if !strings.Contains(out, "personal") || !strings.Contains(out, "Personal") {
		t.Fatalf("list:\n%s", out)
	}

	out, err = runCLI(t, nil, "--config", cfgPath, "calendar", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"slug": "personal"`) {
		t.Fatalf("json:\n%s", out)
	}
}

func TestJSONFlagBeforeOrAfterCommand(t *testing.T) {
	rest, _, _, jsonOut, _, _, err := extractGlobals([]string{"calendar", "list", "--json"})
	if err != nil || !jsonOut || strings.Join(rest, " ") != "calendar list" {
		t.Fatalf("after: rest=%v json=%v err=%v", rest, jsonOut, err)
	}
	rest, url, _, jsonOut, _, _, err := extractGlobals([]string{"--url", "http://127.0.0.1:8080", "--json", "login", "--user", "a"})
	if err != nil || !jsonOut || url != "http://127.0.0.1:8080" || strings.Join(rest, " ") != "login --user a" {
		t.Fatalf("before: rest=%v url=%s json=%v err=%v", rest, url, jsonOut, err)
	}
}

func runCLI(t *testing.T, stdin *strings.Reader, args ...string) (string, error) {
	t.Helper()
	t.Setenv("DCALCON_SESSION", "")
	t.Setenv("DCALCON_URL", "")
	var stdout, stderr bytes.Buffer
	in := ioReader(stdin)
	e := &Env{
		Args:   append([]string{"dcalcon-cli"}, args...),
		Stdin:  in,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := e.Run()
	if err != nil && stderr.Len() > 0 {
		t.Log(stderr.String())
	}
	return stdout.String(), err
}

func TestSaveDownload(t *testing.T) {
	var stdout bytes.Buffer
	e := &Env{Stdout: &stdout}
	if err := e.saveDownload([]byte("hello"), "x.ics", "-"); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "hello" {
		t.Fatalf("stdout %q", stdout.String())
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "out.ics")
	stdout.Reset()
	if err := e.saveDownload([]byte("ics"), "cal.ics", path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "ics" {
		t.Fatalf("file %q", raw)
	}
	if !strings.Contains(stdout.String(), "wrote") {
		t.Fatalf("notice %q", stdout.String())
	}
}

func ioReader(r *strings.Reader) *strings.Reader {
	if r == nil {
		return strings.NewReader("")
	}
	return r
}
