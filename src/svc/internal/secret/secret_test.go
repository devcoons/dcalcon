package secret

import "testing"

func TestSealOpen(t *testing.T) {
	key, err := Key("test-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	ct, nonce, err := Seal(key, []byte(`{"refresh_token":"abc"}`))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Open(key, ct, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != `{"refresh_token":"abc"}` {
		t.Fatalf("got %s", plain)
	}
	if _, err := Open(key, ct, nonce[:4]); err == nil {
		t.Fatal("expected auth failure")
	}
}

func TestKeyHex(t *testing.T) {
	k, err := Key("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil || len(k) != 32 {
		t.Fatalf("%v %d", err, len(k))
	}
}
