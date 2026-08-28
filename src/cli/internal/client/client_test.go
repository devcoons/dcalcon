package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginReadsSessionCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			http.NotFound(w, r)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "tok", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"ada","role":"user"}`))
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "")
	u, sid, err := c.Login("ada", "pw", "")
	if err != nil {
		t.Fatal(err)
	}
	if sid != "tok" || c.Session != "tok" || u.Username != "ada" {
		t.Fatalf("user %+v sid %s", u, sid)
	}
}

func TestUnauthorizedMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := New(srv.URL, "x").Get("/api/v1/me", nil)
	if err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("got %v", err)
	}
}

func TestFilenameFromDisposition(t *testing.T) {
	if got := filenameFromDisposition(`attachment; filename="dcalcon-admin-data.zip"`, "download"); got != "dcalcon-admin-data.zip" {
		t.Fatalf("got %q", got)
	}
	if got := filenameFromDisposition("", "download"); got != "download" {
		t.Fatalf("fallback %q", got)
	}
}
