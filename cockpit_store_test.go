package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDecryptAccountRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"id":"codex-1","email":"a@example.com","tokens":{"id_token":"h.e30.s","access_token":"access","refresh_token":"refresh","account_id":"acct"}}`)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	envelope := map[string]any{
		"version":      1,
		"kind":         "codex",
		"algorithm":    "AES-256-GCM",
		"key_id":       "local-secure-account-storage-v1",
		"nonce":        base64.StdEncoding.EncodeToString(nonce),
		"ciphertext":   base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
		"encrypted_at": 1,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decryptAccountJSON(string(body), base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	if asString(asMap(got["tokens"])["access_token"]) != "access" {
		t.Fatalf("token missing after decrypt")
	}
}
