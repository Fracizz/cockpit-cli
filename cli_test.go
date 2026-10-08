package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestImportShareAndSwitch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("COCKPIT_CLI_HOME", home)
	authPath := filepath.Join(home, "codex", "auth.json")
	configPath := filepath.Join(home, "codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("forced_login_method = \"api\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cursorDB := filepath.Join(home, "Cursor", "User", "globalStorage", "state.vscdb")
	ideDB := filepath.Join(home, "Antigravity IDE", "User", "globalStorage", "state.vscdb")
	if err := createItemDB(cursorDB); err != nil {
		t.Fatal(err)
	}
	if err := createItemDB(ideDB); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COCKPIT_CLI_MAP", writeTestMapping(t, home, authPath, configPath, cursorDB, filepath.Dir(filepath.Dir(filepath.Dir(ideDB)))))

	share := filepath.Join(home, "share.json")
	body := `{
	  "schema": "cockpit-tools.account-transfer",
	  "version": 1,
	  "platforms": {
	    "codex": {"account_count": 1, "exported_data": [{
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
	saved, err := importFile(share, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 3 {
		t.Fatalf("imported %d accounts", len(saved))
	}

	codex, err := findAccount("codex", "codex@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := switchAccount(codex, "windows", false, ""); err != nil {
		t.Fatal(err)
	}
	authRaw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	var auth map[string]any
	if err := json.Unmarshal(authRaw, &auth); err != nil {
		t.Fatal(err)
	}
	tokens := auth["tokens"].(map[string]any)
	if tokens["access_token"] != "access-1" || tokens["account_id"] != "acct-1" {
		t.Fatalf("auth tokens = %#v", tokens)
	}
	if strings.Contains(string(authRaw), "refresh-1") && auth["auth_mode"] != "chatgpt" {
		t.Fatalf("auth mode = %v", auth["auth_mode"])
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `forced_login_method = "chatgpt"`) {
		t.Fatalf("config = %s", config)
	}

	cursor, err := findAccount("cursor", "cursor@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := switchAccount(cursor, "windows", false, ""); err != nil {
		t.Fatal(err)
	}
	if got := itemValue(t, cursorDB, "cursorAuth/cachedEmail"); got != "cursor@example.com" {
		t.Fatalf("cursor email = %s", got)
	}
	if got := itemValue(t, cursorDB, "cursorAuth/accessToken"); got != "cursor-access" {
		t.Fatalf("cursor token = %s", got)
	}

	ag, err := findAccount("antigravity", "ag@example.com", "ide")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := switchAccount(ag, "windows", false, "ide"); err != nil {
		t.Fatal(err)
	}
	raw := itemValue(t, ideDB, "antigravityUnifiedStateSync.oauthToken")
	decoded, err := decodeB64(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !containsBytes(string(decoded), "oauthTokenInfoSentinelKey") {
		t.Fatal("antigravity oauth topic missing sentinel")
	}
	if got := itemValue(t, ideDB, "antigravityOnboarding"); got != "true" {
		t.Fatalf("onboarding = %s", got)
	}
}

func TestOAuthTopicReplacesSentinel(t *testing.T) {
	doc, err := loadMapping()
	if err != nil {
		t.Fatal(err)
	}
	spec := asMap(asMap(asMap(doc["platforms"])["antigravity"])["runtime"])["protobuf"].(map[string]any)
	first, err := buildOAuthTopic(spec, nil, map[string]any{
		"access_token": "a1", "refresh_token": "r1", "expiry_timestamp": int64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildOAuthTopic(spec, first, map[string]any{
		"access_token": "a2", "refresh_token": "r2", "expiry_timestamp": int64(20),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(second), "oauthTokenInfoSentinelKey") != 1 {
		t.Fatalf("sentinel count = %d", strings.Count(string(second), "oauthTokenInfoSentinelKey"))
	}
	info := createOAuthInfo(spec, map[string]any{
		"access_token": "a2", "refresh_token": "r2", "expiry_timestamp": int64(20),
	})
	if !containsBytes(string(info), "r2") || containsBytes(string(info), "r1") {
		t.Fatal("oauth info did not keep the new refresh token")
	}
}

func writeTestMapping(t *testing.T, home, authPath, configPath, cursorDB, ideRoot string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(embeddedMapping, &doc); err != nil {
		t.Fatal(err)
	}
	platforms := doc["platforms"].(map[string]any)
	codexRuntime := platforms["codex"].(map[string]any)["runtime"].(map[string]any)
	codexRuntime["default_target"] = "windows"
	codexRuntime["targets"].(map[string]any)["windows"] = map[string]any{
		"auth_json":   authPath,
		"config_toml": configPath,
	}
	cursorRuntime := platforms["cursor"].(map[string]any)["runtime"].(map[string]any)
	cursorRuntime["targets"].(map[string]any)["windows"] = map[string]any{"state_db": cursorDB}
	agRuntime := platforms["antigravity"].(map[string]any)["runtime"].(map[string]any)
	agRuntime["targets"].(map[string]any)["windows"] = map[string]any{
		"user_data_dirs": []any{ideRoot},
	}
	path := filepath.Join(home, "platforms.json")
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func createItemDB(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT)`)
	return err
}

func itemValue(t *testing.T, path, key string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, key).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func containsBytes(s, part string) bool {
	return strings.Contains(s, part)
}

func decodeB64(value string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(value)
}
