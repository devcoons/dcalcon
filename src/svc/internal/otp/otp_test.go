package otp

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateAndValidate(t *testing.T) {
	secret, url, err := Generate("dCalCon", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || !strings.Contains(url, "otpauth://totp/") {
		t.Fatalf("secret/url %q %q", secret, url)
	}
	if Valid("000000", secret) {
		t.Fatal("zero code should not validate")
	}
	code, err := Code(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !Valid(code, secret) {
		t.Fatalf("expected %s to validate", code)
	}
}
