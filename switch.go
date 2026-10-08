package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func switchAccount(account Account, targetName string, restart bool, product string) ([]string, error) {
	doc, err := loadMapping()
	if err != nil {
		return nil, err
	}
	_, spec, err := platformSpec(doc, account.Platform)
	if err != nil {
		return nil, err
	}
	runtime := asMap(spec["runtime"])
	chosen, target, err := targetFrom(spec, targetName)
	if err != nil {
		return nil, err
	}
	var written []string
	switch asString(runtime["kind"]) {
	case "codex_auth_json":
		path, err := switchCodex(account, target, runtime)
		if err != nil {
			return nil, err
		}
		written = []string{path}
		if restart && asString(target.values["restart"]) != "" {
			if err := restartCodex(asString(target.values["restart"])); err != nil {
				return written, err
			}
		}
	case "vscdb_items":
		path, err := switchCursor(account, target, runtime)
		if err != nil {
			return nil, err
		}
		written = []string{path}
	case "antigravity_unified_state":
		if product == "" {
			product = account.Product
		}
		written, err = switchAntigravity(account, target, runtime, product)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported runtime kind %s", asString(runtime["kind"]))
	}
	if err := markCurrent(account.Platform, account.ID, chosen); err != nil {
		return written, err
	}
	return written, nil
}

func switchCodex(account Account, target runtimeTarget, runtime map[string]any) (string, error) {
	authSpec := asMap(runtime["auth_json"])
	authPath := asString(target.values["auth_json"])
	existing := map[string]any{}
	if text, ok, err := readText(target, authPath); err != nil {
		return "", err
	} else if ok && strings.TrimSpace(text) != "" {
		if err := json.Unmarshal([]byte(text), &existing); err != nil {
			return "", fmt.Errorf("existing auth.json is not an object: %w", err)
		}
	}
	auth, err := buildCodexAuth(account, authSpec)
	if err != nil {
		return "", err
	}
	for key, value := range existing {
		if _, replaced := auth[key]; !replaced {
			auth[key] = value
		}
	}
	if _, ok := auth["tokens"]; ok {
		for _, key := range asSlice(authSpec["oauth_clears"]) {
			delete(auth, asString(key))
		}
	}
	body, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return "", err
	}
	written, err := writeText(target, authPath, string(body)+"\n")
	if err != nil {
		return "", err
	}
	if err := alignForcedLogin(target, account.Credentials, authSpec); err != nil {
		return written, err
	}
	return written, nil
}

func buildCodexAuth(account Account, authSpec map[string]any) (map[string]any, error) {
	creds := account.Credentials
	access := asString(creds["access_token"])
	idToken := asString(creds["id_token"])
	if asString(creds["openai_api_key"]) != "" && access == "" && idToken == "" {
		return map[string]any{
			"auth_mode":      asString(authSpec["api_key_auth_mode"]),
			"OPENAI_API_KEY": asString(creds["openai_api_key"]),
		}, nil
	}
	if identity, ok := creds["agent_identity"].(map[string]any); ok {
		return map[string]any{"auth_mode": "agentIdentity", "agent_identity": identity}, nil
	}
	if access == "" {
		label := account.Email
		if label == "" {
			label = account.ID
		}
		return nil, fmt.Errorf("%s has no Codex access token", label)
	}
	if strings.TrimSpace(idToken) == "" && strings.TrimSpace(asString(creds["refresh_token"])) == "" {
		pat := asString(creds["personal_access_token"])
		if pat == "" {
			pat = access
		}
		return map[string]any{"OPENAI_API_KEY": nil, "personal_access_token": pat}, nil
	}
	accountID := asString(creds["account_id"])
	if accountID == "" {
		accountID = accountIDFromAccessToken(access, asString(authSpec["account_id_claim"]))
	}
	mode := asString(creds["auth_mode"])
	if mode == "" {
		mode = asString(authSpec["auth_mode"])
	}
	tokens := map[string]any{
		"id_token":      idToken,
		"access_token":  access,
		"refresh_token": asString(creds["refresh_token"]),
	}
	if accountID != "" {
		tokens["account_id"] = accountID
	}
	return map[string]any{
		"auth_mode":      mode,
		"OPENAI_API_KEY": nil,
		"tokens":         tokens,
		"last_refresh":   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}

func accountIDFromAccessToken(token, claimPath string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return ""
	}
	var current any = claims
	for _, part := range strings.Split(claimPath, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[part]
	}
	return asString(current)
}

func alignForcedLogin(target runtimeTarget, creds map[string]any, authSpec map[string]any) error {
	configPath := asString(target.values["config_toml"])
	if configPath == "" {
		return nil
	}
	text, ok, err := readText(target, configPath)
	if err != nil || !ok || strings.TrimSpace(text) == "" {
		return err
	}
	key := asString(authSpec["forced_login_method_key"])
	pattern := regexp.MustCompile(`(?m)^(\s*` + regexp.QuoteMeta(key) + `\s*=\s*)['"]([^'"]+)['"]`)
	match := pattern.FindStringSubmatchIndex(text)
	if match == nil {
		return nil
	}
	desired := asString(authSpec["forced_login_method_oauth"])
	if asString(creds["openai_api_key"]) != "" && asString(creds["id_token"]) == "" {
		desired = asString(authSpec["forced_login_method_api"])
	}
	current := text[match[4]:match[5]]
	if strings.EqualFold(strings.TrimSpace(current), desired) {
		return nil
	}
	updated := text[:match[4]] + desired + text[match[5]:]
	_, err = writeText(target, configPath, updated)
	return err
}

func restartCodex(command string) error {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return fmt.Errorf("empty restart command")
	}
	cmd := exec.Command(fields[0], fields[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			return err
		}
		return fmt.Errorf("%s", detail)
	}
	if text := strings.TrimSpace(string(out)); text != "" {
		fmt.Println(text)
	}
	return nil
}

func switchCursor(account Account, target runtimeTarget, runtime map[string]any) (string, error) {
	if !target.local {
		return "", fmt.Errorf("cursor switch only writes a local state.vscdb")
	}
	dbPath := expandPath(asString(target.values["state_db"]))
	if _, err := osStat(dbPath); err != nil {
		return "", fmt.Errorf("Cursor state database not found: %s", dbPath)
	}
	table := asString(runtime["table"])
	if !identPattern.MatchString(table) {
		return "", fmt.Errorf("invalid table name")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	creds := map[string]any{}
	for key, value := range account.Credentials {
		creds[key] = value
	}
	creds["email"] = account.Email
	for _, raw := range asSlice(runtime["items"]) {
		item := asMap(raw)
		value := asString(creds[asString(item["from"])])
		if value == "" {
			if asBool(item["required"]) {
				return "", fmt.Errorf("cursor account is missing %s", asString(item["from"]))
			}
			continue
		}
		query := fmt.Sprintf("INSERT OR REPLACE INTO %s (key, value) VALUES (?, ?)", table)
		if _, err := db.Exec(query, asString(item["key"]), value); err != nil {
			return "", err
		}
	}
	return dbPath, nil
}

func switchAntigravity(account Account, target runtimeTarget, runtime map[string]any, product string) ([]string, error) {
	if !target.local {
		return nil, fmt.Errorf("antigravity switch only writes a local state.vscdb")
	}
	creds := map[string]any{}
	for key, value := range account.Credentials {
		creds[key] = value
	}
	if asString(creds["access_token"]) == "" || asString(creds["refresh_token"]) == "" {
		return nil, fmt.Errorf("antigravity account needs access_token and refresh_token")
	}
	if asInt64(creds["expiry_timestamp"]) == 0 {
		expires := asInt64(creds["expires_in"])
		if expires == 0 {
			expires = 3600
		}
		creds["expiry_timestamp"] = time.Now().Unix() + expires
	}
	spec := asMap(runtime["protobuf"])
	projectID := asString(creds["project_id"])
	if projectID == asString(spec["ignored_project_id"]) {
		projectID = ""
	}
	var written []string
	for _, dir := range antigravityDirs(target, runtime, product) {
		dbPath := filepath.Join(dir, filepath.FromSlash(asString(runtime["state_db_relative"])))
		if _, err := osStat(dbPath); err != nil {
			continue
		}
		if err := writeAntigravityDB(dbPath, runtime, spec, creds, projectID); err != nil {
			return nil, err
		}
		written = append(written, dbPath)
	}
	if len(written) == 0 {
		return nil, fmt.Errorf("no Antigravity state.vscdb found for the selected product")
	}
	return written, nil
}

func writeAntigravityDB(dbPath string, runtime, spec, creds map[string]any, projectID string) error {
	table := asString(runtime["table"])
	if !identPattern.MatchString(table) {
		return fmt.Errorf("invalid table name")
	}
	items := asMap(runtime["items"])
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	var existing string
	row := db.QueryRow(fmt.Sprintf("SELECT value FROM %s WHERE key = ?", table), asString(items["oauth_token"]))
	_ = row.Scan(&existing)
	var topic []byte
	if existing != "" {
		topic, err = base64.StdEncoding.DecodeString(existing)
		if err != nil {
			return err
		}
	}
	updated, err := buildOAuthTopic(spec, topic, creds)
	if err != nil {
		return err
	}
	if err := putItem(db, table, asString(items["oauth_token"]), base64.StdEncoding.EncodeToString(updated)); err != nil {
		return err
	}
	if err := putItem(db, table, asString(items["onboarding"]), "true"); err != nil {
		return err
	}
	enterpriseKey := asString(items["enterprise_preferences"])
	if projectID != "" {
		enterprise := buildEnterpriseTopic(spec, projectID)
		return putItem(db, table, enterpriseKey, base64.StdEncoding.EncodeToString(enterprise))
	}
	_, err = db.Exec(fmt.Sprintf("DELETE FROM %s WHERE key = ?", table), enterpriseKey)
	return err
}

func putItem(db *sql.DB, table, key, value string) error {
	_, err := db.Exec(fmt.Sprintf("INSERT OR REPLACE INTO %s (key, value) VALUES (?, ?)", table), key, value)
	return err
}

func antigravityDirs(target runtimeTarget, runtime map[string]any, product string) []string {
	var names []string
	if product != "" {
		for _, item := range asSlice(asMap(runtime["products"])[product]) {
			names = append(names, asString(item))
		}
	}
	var dirs []string
	for _, item := range asSlice(target.values["user_data_dirs"]) {
		path := expandPath(asString(item))
		if len(names) > 0 && !contains(names, filepath.Base(path)) {
			continue
		}
		dirs = append(dirs, path)
	}
	return dirs
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
