package main

import (
	"encoding/json"
	"fmt"
	"os"
	gorun "runtime"
	"strings"
	"time"
)

func exportFile(path, platformName, targetName string) ([]Account, error) {
	doc, err := loadMapping()
	if err != nil {
		return nil, err
	}
	accounts, err := collectExportAccounts(doc, platformName, targetName)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts to export")
	}
	body, err := json.MarshalIndent(exportBundle(doc, accounts), "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		return nil, err
	}
	return accounts, nil
}

func collectExportAccounts(doc map[string]any, platformName, targetName string) ([]Account, error) {
	var platforms []string
	if platformName != "" {
		name, _, err := platformSpec(doc, platformName)
		if err != nil {
			return nil, err
		}
		platforms = []string{name}
	} else {
		for name := range asMap(doc["platforms"]) {
			platforms = append(platforms, name)
		}
	}
	var accounts []Account
	index, err := loadIndex()
	if err != nil {
		return nil, err
	}
	for _, row := range index.Accounts {
		platform := asString(row["platform"])
		if !contains(platforms, platform) {
			continue
		}
		account, err := loadAccount(platform, asString(row["id"]))
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	cockpit, err := loadCockpitAccounts(doc, platforms, targetName)
	if err != nil {
		return nil, err
	}
	return mergeAccounts(accounts, cockpit), nil
}

func mergeAccounts(local, cockpit []Account) []Account {
	seen := map[string]int{}
	var merged []Account
	add := func(account Account) {
		key := account.Platform + "|" + account.Product + "|" + strings.ToLower(account.Email)
		if i, ok := seen[key]; ok {
			merged[i] = account
			return
		}
		seen[key] = len(merged)
		merged = append(merged, account)
	}
	for _, account := range local {
		add(account)
	}
	for _, account := range cockpit {
		add(account)
	}
	return merged
}

func loadCockpitAccounts(doc map[string]any, platforms []string, targetName string) ([]Account, error) {
	root, remote, ok, err := cockpitRoot(doc, targetName)
	if err != nil || !ok {
		return nil, err
	}
	store := asMap(doc["cockpit_store"])
	keyFile := asString(store["key_file"])
	var accounts []Account
	for _, platform := range platforms {
		_, spec, err := platformSpec(doc, platform)
		if err != nil {
			return nil, err
		}
		catalog := asMap(spec["catalog"])
		if catalog == nil {
			continue
		}
		indexText, found, err := readText(remote, joinRemote(root, asString(catalog["index"])))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		keyText, found, err := readText(remote, joinRemote(root, keyFile))
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("account storage key missing")
		}
		var index map[string]any
		if err := json.Unmarshal([]byte(indexText), &index); err != nil {
			return nil, err
		}
		for _, item := range asSlice(index["accounts"]) {
			row := asMap(item)
			id := asString(row["id"])
			body, found, err := readText(remote, joinRemote(root, asString(catalog["dir"])+"/"+id+".json"))
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, fmt.Errorf("account detail missing for %s", fallback(asString(row["email"]), id))
			}
			raw, err := decryptAccountJSON(body, keyText)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fallback(asString(row["email"]), id), err)
			}
			parsed := recordsFromPayload(platform, spec, []any{raw}, "")
			for i := range parsed {
				parsed[i].Raw = raw
			}
			accounts = append(accounts, parsed...)
		}
	}
	return accounts, nil
}

func cockpitRoot(doc map[string]any, targetName string) (string, runtimeTarget, bool, error) {
	store := asMap(doc["cockpit_store"])
	targets := asMap(store["targets"])
	name := targetName
	if name == "" {
		if gorunGOOS() == "linux" {
			name = "linux"
		} else {
			return "", runtimeTarget{}, false, nil
		}
	}
	if name == "local" {
		return "", runtimeTarget{}, false, nil
	}
	raw, ok := targets[name].(map[string]any)
	if !ok {
		return "", runtimeTarget{}, false, fmt.Errorf("unknown cockpit store target %q", name)
	}
	target := runtimeTarget{values: raw, local: asString(raw["distro"]) == "", distro: asString(raw["distro"]), user: asString(raw["user"])}
	return asString(raw["path"]), target, true, nil
}

func exportBundle(doc map[string]any, accounts []Account) map[string]any {
	bundle := asMap(doc["bundle"])
	grouped := map[string][]any{}
	for _, account := range accounts {
		_, spec, err := platformSpec(doc, account.Platform)
		if err != nil {
			continue
		}
		key := exportBundleKey(spec, account)
		grouped[key] = append(grouped[key], accountToExportItem(account))
	}
	platforms := map[string]any{}
	total := 0
	for key, items := range grouped {
		total += len(items)
		platforms[key] = map[string]any{
			"account_count": len(items),
			"exported_data": items,
		}
	}
	return map[string]any{
		"schema":      bundle["schema"],
		"version":     bundle["version"],
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"summary": map[string]any{
			"platform_count": len(grouped),
			"account_count":  total,
		},
		"platforms": platforms,
	}
}

func exportBundleKey(spec map[string]any, account Account) string {
	products := asMap(spec["product_by_bundle_key"])
	for key, product := range products {
		if asString(product) == account.Product && account.Product != "" {
			return key
		}
	}
	for _, item := range asSlice(spec["bundle_keys"]) {
		if text, ok := item.(string); ok {
			return text
		}
	}
	return account.Platform
}

func accountToExportItem(account Account) map[string]any {
	if account.Raw != nil {
		return account.Raw
	}
	item := map[string]any{"id": account.ID, "email": account.Email}
	creds := account.Credentials
	switch account.Platform {
	case "cursor":
		for _, key := range []string{"access_token", "refresh_token", "auth_id", "membership_type", "subscription_status", "sign_up_type"} {
			if value := asString(creds[key]); value != "" {
				item[key] = value
			}
		}
	case "antigravity":
		token := map[string]any{}
		for _, key := range []string{"access_token", "refresh_token", "expiry_timestamp", "expires_in", "id_token", "project_id"} {
			if _, ok := creds[key]; ok && asString(creds[key]) != "" {
				token[key] = creds[key]
			}
		}
		if _, ok := creds["is_gcp_tos"]; ok {
			token["is_gcp_tos"] = asBool(creds["is_gcp_tos"])
		}
		item["token"] = token
	default:
		if plan := asString(creds["plan_type"]); plan != "" {
			item["plan_type"] = plan
		}
		if mode := asString(creds["auth_mode"]); mode != "" {
			item["auth_mode"] = mode
		}
		tokens := map[string]any{}
		for _, key := range []string{"id_token", "access_token", "refresh_token", "account_id"} {
			if value := asString(creds[key]); value != "" {
				tokens[key] = value
			}
		}
		if len(tokens) > 0 {
			item["tokens"] = tokens
		}
		if value := asString(creds["openai_api_key"]); value != "" {
			item["openai_api_key"] = value
		}
		if value := asString(creds["personal_access_token"]); value != "" {
			item["personal_access_token"] = value
		}
		if identity, ok := creds["agent_identity"]; ok {
			item["agent_identity"] = identity
		}
	}
	return item
}

func joinRemote(root, name string) string {
	return strings.TrimRight(root, "/") + "/" + strings.TrimLeft(name, "/")
}

func gorunGOOS() string {
	return gorun.GOOS
}
