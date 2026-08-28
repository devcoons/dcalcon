package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrNoKey = errors.New("token encryption key is not set")

func Key(configured string) ([]byte, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return nil, ErrNoKey
	}
	if b, err := hex.DecodeString(configured); err == nil && (len(b) == 16 || len(b) == 24 || len(b) == 32) {
		return b, nil
	}
	sum := sha256.Sum256([]byte(configured))
	return sum[:], nil
}

func EnsureKey(configured, sqlitePath string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return strings.TrimSpace(configured), nil
	}
	if sqlitePath == "" || sqlitePath == ":memory:" || strings.HasPrefix(sqlitePath, "file::memory:") {
		return "", nil
	}
	path := sqlitePath + ".token-key"
	if raw, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(raw)), nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	hexKey := hex.EncodeToString(b)
	if err := os.WriteFile(path, []byte(hexKey+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write token key: %w", err)
	}
	return hexKey, nil
}

func Seal(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, nil, ErrNoKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func Open(key, ciphertext, nonce []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, ErrNoKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce")
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
