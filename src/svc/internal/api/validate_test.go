package api

import "testing"

func TestValidUsername(t *testing.T) {
	ok := []string{"ab", "alice", "bob.smith", "user_1", "A9"}
	for _, s := range ok {
		if !validUsername(s) {
			t.Errorf("expected valid: %s", s)
		}
	}
	bad := []string{"", "a", "has space", "x/y", "toolongtoolongtoolongtoolongtoolong"}
	for _, s := range bad {
		if validUsername(s) {
			t.Errorf("expected invalid: %s", s)
		}
	}
}

func TestValidEmail(t *testing.T) {
	if !validEmail("ada@example.com") || validEmail("nope") || validEmail("") {
		t.Fatal("email validation")
	}
}
