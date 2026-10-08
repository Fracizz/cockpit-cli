package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func importFile(path, platformHint string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	doc, err := loadMapping()
	if err != nil {
		return nil, err
	}
	records, err := parseDocument(doc, raw, platformHint)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no codex, cursor, or antigravity accounts in %s", path)
	}
	saved := make([]map[string]any, 0, len(records))
	for _, record := range records {
		view, err := upsertAccount(record)
		if err != nil {
			return nil, err
		}
		saved = append(saved, view)
	}
	return saved, nil
}

func parseDocument(doc map[string]any, raw any, platformHint string) ([]Account, error) {
	bundle := asMap(doc["bundle"])
	if obj, ok := raw.(map[string]any); ok && obj["schema"] == bundle["schema"] {
		if int(asInt64(obj["version"])) != int(asInt64(bundle["version"])) {
			return nil, fmt.Errorf("unsupported share bundle version: %v", obj["version"])
		}
		platforms := asMap(obj[asString(bundle["platforms_key"])])
		if platforms == nil {
			return nil, fmt.Errorf("share bundle is missing platforms")
		}
		var records []Account
		for platform, rawSpec := range asMap(doc["platforms"]) {
			spec := asMap(rawSpec)
			for _, key := range asSlice(spec["bundle_keys"]) {
				name, _ := key.(string)
				section, exists := platforms[name]
				if !exists || section == nil {
					continue
				}
				product := asString(asMap(spec["product_by_bundle_key"])[name])
				items, err := unwrapPayload(section, asSlice(bundle["payload_keys"]))
				if err != nil {
					return nil, err
				}
				records = append(records, recordsFromPayload(platform, spec, items, product)...)
			}
		}
		return records, nil
	}
	if platformHint != "" {
		platform, spec, err := platformSpec(doc, platformHint)
		if err != nil {
			return nil, err
		}
		product := ""
		if strings.EqualFold(strings.TrimSpace(platformHint), "antigravity_ide") {
			product = "ide"
		}
		items := []any{raw}
		if list, ok := raw.([]any); ok {
			items = list
		}
		return recordsFromPayload(platform, spec, items, product), nil
	}
	switch value := raw.(type) {
	case []any:
		return detectRecords(doc, value), nil
	case map[string]any:
		return detectRecords(doc, []any{value}), nil
	default:
		return nil, fmt.Errorf("share JSON must be an object or array")
	}
}

func unwrapPayload(section any, keys []any) ([]any, error) {
	if text, ok := section.(string); ok {
		if err := json.Unmarshal([]byte(text), &section); err != nil {
			return nil, err
		}
	}
	if list, ok := section.([]any); ok {
		return list, nil
	}
	obj, ok := section.(map[string]any)
	if !ok {
		return nil, nil
	}
	for _, key := range keys {
		name, _ := key.(string)
		value, exists := obj[name]
		if !exists {
			continue
		}
		if text, ok := value.(string); ok {
			if err := json.Unmarshal([]byte(text), &value); err != nil {
				return nil, err
			}
		}
		switch typed := value.(type) {
		case []any:
			return typed, nil
		case map[string]any:
			return []any{typed}, nil
		}
	}
	return []any{obj}, nil
}

func detectRecords(doc map[string]any, items []any) []Account {
	grouped := map[string][]any{}
	for platform := range asMap(doc["platforms"]) {
		grouped[platform] = nil
	}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if platform := detectPlatform(obj); platform != "" {
			grouped[platform] = append(grouped[platform], obj)
		}
	}
	var records []Account
	for platform, rawSpec := range asMap(doc["platforms"]) {
		records = append(records, recordsFromPayload(platform, asMap(rawSpec), grouped[platform], "")...)
	}
	return records
}

func detectPlatform(item map[string]any) string {
	if _, ok := item["id_token"]; ok {
		return "codex"
	}
	if _, ok := item["plan_type"]; ok {
		return "codex"
	}
	if _, ok := item["tokens"].(map[string]any); ok {
		return "codex"
	}
	if item["type"] == "codex" {
		return "codex"
	}
	if token, ok := item["token"].(map[string]any); ok && token["refresh_token"] != nil && token["access_token"] != nil {
		return "antigravity"
	}
	for _, key := range []string{"access_token", "accessToken", "cursor_auth_raw", "membership_type"} {
		if _, ok := item[key]; ok {
			return "cursor"
		}
	}
	return ""
}

func recordsFromPayload(platform string, spec map[string]any, items []any, product string) []Account {
	var records []Account
	canonical := asMap(spec["canonical"])
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		credentials := map[string]any{}
		for name, paths := range canonical {
			if value := firstValue(obj, asSlice(paths)); value != nil {
				credentials[name] = value
			}
		}
		email := asString(credentials["email"])
		if platform == "codex" && email == "" {
			email = emailFromJWT(asString(credentials["id_token"]))
			if email == "" {
				email = emailFromJWT(asString(credentials["access_token"]))
			}
			if email != "" {
				credentials["email"] = email
			}
		}
		if asString(credentials["access_token"]) == "" && asString(credentials["openai_api_key"]) == "" && asString(credentials["personal_access_token"]) == "" {
			continue
		}
		if platform != "codex" && asString(credentials["access_token"]) == "" {
			continue
		}
		if platform == "antigravity" && asString(credentials["refresh_token"]) == "" {
			continue
		}
		given := ""
		if text, ok := credentials["id"].(string); ok {
			given = text
		}
		delete(credentials, "id")
		delete(credentials, "email")
		records = append(records, Account{
			Schema:      "cockpit-cli.account",
			Version:     1,
			Platform:    platform,
			ID:          stableID(platform, fallback(email, "unknown"), product, given),
			Email:       email,
			Product:     product,
			ImportedAt:  time.Now().Unix(),
			Credentials: credentials,
		})
	}
	return records
}

func fallback(value, other string) string {
	if value == "" {
		return other
	}
	return value
}

func emailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload := parts[1]
	if pad := len(payload) % 4; pad != 0 {
		payload += strings.Repeat("=", 4-pad)
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return ""
	}
	return asString(claims["email"])
}
