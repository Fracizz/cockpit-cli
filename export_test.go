package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportLocalRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("COCKPIT_CLI_HOME", home)
	t.Setenv("COCKPIT_CLI_MAP", writeTestMapping(t, home,
		filepath.Join(home, "auth.json"),
		filepath.Join(home, "config.toml"),
		filepath.Join(home, "state.vscdb"),
		home,
	))
	share := filepath.Join(home, "share.json")
	body := `{
	  "schema": "cockpit-tools.account-transfer",
	  "version": 1,
	  "platforms": {
	    "codex": {"exported_data": [{
	      "id": "codex-1", "email": "codex@example.com", "plan_type": "pro",
	      "tokens": {"id_token": "header.e30.sig", "access_token": "access-1", "refresh_token": "refresh-1", "account_id": "acct-1"}
	    }]},
	    "cursor": {"exported_data": [{
	      "id": "cursor-1", "email": "cursor@example.com", "access_token": "cursor-access", "refresh_token": "cursor-refresh"
	    }]},
	    "antigravity_ide": {"exported_data": [{
	      "id": "ag-1", "email": "ag@example.com",
	      "token": {"access_token": "ag-access", "refresh_token": "ag-refresh", "expiry_timestamp": 1800000000}
	    }]}
	  }
	}`
	if err := os.WriteFile(share, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := importFile(share, ""); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(home, "out.json")
	exported, err := exportFile(out, "", "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 3 {
		t.Fatalf("exported %d accounts", len(exported))
	}
	doc, err := loadMapping()
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	records, err := parseDocument(doc, raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("reimported %d accounts", len(records))
	}
	for _, record := range records {
		switch record.Platform {
		case "codex":
			if asString(record.Credentials["access_token"]) != "access-1" || record.Product != "" {
				t.Fatal("codex export did not round-trip")
			}
		case "cursor":
			if asString(record.Credentials["access_token"]) != "cursor-access" {
				t.Fatal("cursor export did not round-trip")
			}
		case "antigravity":
			if asString(record.Credentials["refresh_token"]) != "ag-refresh" || record.Product != "ide" {
				t.Fatal("antigravity export did not round-trip")
			}
		default:
			t.Fatalf("unexpected platform %s", record.Platform)
		}
	}
}

func TestExportCockpitStore(t *testing.T) {
	home := t.TempDir()
	store := t.TempDir()
	t.Setenv("COCKPIT_CLI_HOME", home)
	mapPath := writeTestMapping(t, home,
		filepath.Join(home, "auth.json"),
		filepath.Join(home, "config.toml"),
		filepath.Join(home, "state.vscdb"),
		home,
	)
	mapping, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(mapping, &doc); err != nil {
		t.Fatal(err)
	}
	targets := doc["cockpit_store"].(map[string]any)["targets"].(map[string]any)
	targets["linux"].(map[string]any)["path"] = store
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mapPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COCKPIT_CLI_MAP", mapPath)

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"id":"codex-9","email":"quota@example.com","plan_type":"pro","tokens":{"id_token":"h.e30.s","access_token":"live-access","refresh_token":"live-refresh","account_id":"acct-9"}}`)
	envelope, err := sealAccount(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store, "codex_accounts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "secure-account-storage.key"), []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "codex_accounts.json"), []byte(`{"accounts":[{"id":"codex-9","email":"quota@example.com"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "codex_accounts", "codex-9.json"), envelope, 0o600); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(home, "cockpit.json")
	exported, err := exportFile(out, "codex", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].Email != "quota@example.com" {
		t.Fatalf("exported %d cockpit accounts", len(exported))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadMapping()
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	records, err := parseDocument(loaded, raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || asString(records[0].Credentials["access_token"]) != "live-access" {
		t.Fatal("cockpit export did not round-trip")
	}
	if !containsBytes(string(data), "live-refresh") {
		t.Fatal("export file dropped the cockpit account payload")
	}
}

func sealAccount(key, plain []byte) ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
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
	return json.Marshal(envelope)
}
