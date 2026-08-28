package providers

import (
	"net/url"
	"strings"
	"testing"
)

func TestAuthURLPKCE(t *testing.T) {
	v, ch, err := NewVerifier()
	if err != nil || v == "" || ch == "" {
		t.Fatalf("verifier %v", err)
	}
	a := OAuthApp{
		ClientID:    "cid",
		RedirectURI: "http://localhost:3000/api/v1/oauth/google/callback",
		Scopes:      GoogleScopes(),
		Endpoints:   Google,
		Offline:     true,
		Prompt:      "consent",
	}
	u, err := url.Parse(a.AuthURL("st", ch, false))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" {
		t.Fatalf("%v", q)
	}
	if !strings.Contains(q.Get("scope"), "gmail.send") {
		t.Fatalf("scope %s", q.Get("scope"))
	}
}

func TestMicrosoftTenant(t *testing.T) {
	e := MicrosoftWithTenant("contoso")
	if !strings.Contains(e.Auth, "/contoso/") || !strings.Contains(e.Token, "/contoso/") {
		t.Fatalf("%+v", e)
	}
}
