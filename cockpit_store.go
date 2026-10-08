package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type secureEnvelope struct {
	Version    int    `json:"version"`
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func decryptAccountJSON(content, keyText string) (map[string]any, error) {
	var envelope secureEnvelope
	if err := json.Unmarshal([]byte(content), &envelope); err != nil || envelope.Algorithm == "" {
		var plain map[string]any
		if err := json.Unmarshal([]byte(content), &plain); err != nil {
			return nil, fmt.Errorf("account file is not JSON")
		}
		return plain, nil
	}
	if envelope.Version != 1 || envelope.Algorithm != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported account encryption")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyText))
	if err != nil {
		return nil, fmt.Errorf("invalid storage key")
	}
	nonce, err := base64.StdEncoding.DecodeString(strings.TrimSpace(envelope.Nonce))
	if err != nil || len(nonce) != 12 {
		return nil, fmt.Errorf("invalid account nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(envelope.Ciphertext))
	if err != nil {
		return nil, fmt.Errorf("invalid account ciphertext")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt account failed")
	}
	var account map[string]any
	if err := json.Unmarshal(plain, &account); err != nil {
		return nil, err
	}
	return account, nil
}

func loadCockpitAccount(platform, query, targetName string) (Account, error) {
	doc, err := loadMapping()
	if err != nil {
		return Account{}, err
	}
	name, spec, err := platformSpec(doc, platform)
	if err != nil {
		return Account{}, err
	}
	_, target, err := targetFrom(spec, targetName)
	if err != nil {
		return Account{}, err
	}
	indexPath := asString(target.values["account_index"])
	dir := asString(target.values["account_dir"])
	keyPath := asString(target.values["storage_key"])
	if indexPath == "" || dir == "" || keyPath == "" {
		return Account{}, fmt.Errorf("no %s account matched %q", name, query)
	}
	text, ok, err := readText(target, indexPath)
	if err != nil || !ok {
		return Account{}, fmt.Errorf("no %s account matched %q", name, query)
	}
	var index map[string]any
	if err := json.Unmarshal([]byte(text), &index); err != nil {
		return Account{}, err
	}
	var matched map[string]any
	for _, item := range asSlice(index["accounts"]) {
		account := asMap(item)
		if asString(account["id"]) == query || strings.EqualFold(asString(account["email"]), query) {
			matched = account
			break
		}
	}
	if matched == nil {
		return Account{}, fmt.Errorf("no %s account matched %q", name, query)
	}
	body, ok, err := readText(target, filepath.ToSlash(filepath.Join(dir, asString(matched["id"])+".json")))
	if err != nil || !ok {
		return Account{}, fmt.Errorf("account detail missing for %s", asString(matched["email"]))
	}
	keyText, ok, err := readText(target, keyPath)
	if err != nil || !ok {
		return Account{}, fmt.Errorf("account storage key missing")
	}
	raw, err := decryptAccountJSON(body, keyText)
	if err != nil {
		return Account{}, err
	}
	records := recordsFromPayload(name, spec, []any{raw}, "")
	if len(records) == 0 {
		return Account{}, fmt.Errorf("account %s has no usable credential", asString(matched["email"]))
	}
	return records[0], nil
}

func markIndexCurrent(target runtimeTarget, accountID string) error {
	indexPath := asString(target.values["account_index"])
	if indexPath == "" || !target.local {
		return nil
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	var index map[string]any
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}
	index["current_account_id"] = accountID
	body, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, append(body, '\n'), 0o600)
}
