package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func listRuntimeAccounts(platform, targetName string) error {
	doc, err := loadMapping()
	if err != nil {
		return err
	}
	name, spec, err := platformSpec(doc, platform)
	if err != nil {
		return err
	}
	chosen, target, err := targetFrom(spec, targetName)
	if err != nil {
		return err
	}
	indexPath := asString(target.values["account_index"])
	if indexPath == "" {
		return fmt.Errorf("%s target %s has no account_index in the mapping", name, chosen)
	}
	text, ok, err := readText(target, indexPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("account index not found: %s", indexPath)
	}
	var index map[string]any
	if err := json.Unmarshal([]byte(text), &index); err != nil {
		return err
	}
	currentID := asString(index["current_account_id"])
	currentEmail := ""
	if authPath := asString(target.values["auth_json"]); authPath != "" {
		currentEmail = emailFromAuthFile(target, authPath)
	}
	shown := 0
	for _, item := range asSlice(index["accounts"]) {
		account := asMap(item)
		email := asString(account["email"])
		id := asString(account["id"])
		line := fmt.Sprintf("%s %s id=%s", name, fallback(email, "-"), fallback(id, "-"))
		if plan := asString(account["plan_type"]); plan != "" {
			line += " plan=" + plan
		}
		if id != "" && id == currentID {
			line += " current"
		} else if currentEmail != "" && strings.EqualFold(email, currentEmail) {
			line += " current"
		}
		fmt.Println(line)
		shown++
	}
	if shown == 0 {
		fmt.Println("no accounts")
	}
	return nil
}

func emailFromAuthFile(target runtimeTarget, path string) string {
	text, ok, err := readText(target, path)
	if err != nil || !ok {
		return ""
	}
	var auth map[string]any
	if err := json.Unmarshal([]byte(text), &auth); err != nil {
		return ""
	}
	if email := asString(auth["email"]); email != "" {
		return email
	}
	tokens := asMap(auth["tokens"])
	return emailFromJWT(asString(tokens["id_token"]))
}
