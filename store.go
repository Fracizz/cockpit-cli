package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Account struct {
	Schema      string         `json:"schema"`
	Version     int            `json:"version"`
	Platform    string         `json:"platform"`
	ID          string         `json:"id"`
	Email       string         `json:"email"`
	Product     string         `json:"product,omitempty"`
	ImportedAt  int64          `json:"imported_at"`
	Credentials map[string]any `json:"credentials"`
	Raw         map[string]any `json:"-"`
}

type Index struct {
	Schema   string           `json:"schema"`
	Version  int              `json:"version"`
	Accounts []map[string]any `json:"accounts"`
	Current  map[string]any   `json:"current"`
}

func storeHome() string {
	if home := os.Getenv("COCKPIT_CLI_HOME"); home != "" {
		return home
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return ".cockpit-cli"
	}
	return filepath.Join(dir, ".cockpit-cli")
}

func loadIndex() (Index, error) {
	path := filepath.Join(storeHome(), "index.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Index{Schema: "cockpit-cli.index", Version: 1, Accounts: []map[string]any{}, Current: map[string]any{}}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, err
	}
	if index.Accounts == nil {
		index.Accounts = []map[string]any{}
	}
	if index.Current == nil {
		index.Current = map[string]any{}
	}
	return index, nil
}

func saveIndex(index Index) error {
	dir := storeHome()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.json"), append(body, '\n'), 0o600)
}

func stableID(platform, email, product, given string) string {
	if given != "" {
		return given
	}
	sum := sha256.Sum256([]byte(platform + "|" + product + "|" + email))
	return "local-" + hex.EncodeToString(sum[:8])
}

func publicView(account Account) map[string]any {
	plan := asString(account.Credentials["plan_type"])
	if plan == "" {
		plan = asString(account.Credentials["membership_type"])
	}
	item := map[string]any{
		"platform":    account.Platform,
		"id":          account.ID,
		"email":       account.Email,
		"imported_at": account.ImportedAt,
	}
	if account.Product != "" {
		item["product"] = account.Product
	}
	if plan != "" {
		item["plan_type"] = plan
	}
	return item
}

func upsertAccount(account Account) (map[string]any, error) {
	dir := filepath.Join(storeHome(), "accounts", account.Platform)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, account.ID+".json")
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		return nil, err
	}
	index, err := loadIndex()
	if err != nil {
		return nil, err
	}
	view := publicView(account)
	kept := make([]map[string]any, 0, len(index.Accounts))
	for _, item := range index.Accounts {
		if asString(item["platform"]) == account.Platform && asString(item["id"]) == account.ID {
			continue
		}
		kept = append(kept, item)
	}
	index.Accounts = append(kept, view)
	if err := saveIndex(index); err != nil {
		return nil, err
	}
	return view, nil
}

func loadAccount(platform, id string) (Account, error) {
	path := filepath.Join(storeHome(), "accounts", platform, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Account{}, fmt.Errorf("account file missing: %s/%s", platform, id)
	}
	var account Account
	if err := json.Unmarshal(data, &account); err != nil {
		return Account{}, err
	}
	if account.Credentials == nil {
		account.Credentials = map[string]any{}
	}
	return account, nil
}

func findAccount(platform, query, product string) (Account, error) {
	index, err := loadIndex()
	if err != nil {
		return Account{}, err
	}
	var matches []map[string]any
	for _, item := range index.Accounts {
		if asString(item["platform"]) != platform {
			continue
		}
		if product != "" && asString(item["product"]) != product {
			continue
		}
		if asString(item["id"]) == query || strings.EqualFold(asString(item["email"]), query) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return Account{}, fmt.Errorf("no %s account matched %q", platform, query)
	}
	if len(matches) > 1 {
		return Account{}, fmt.Errorf("multiple accounts matched %q", query)
	}
	return loadAccount(platform, asString(matches[0]["id"]))
}

func markCurrent(platform, id, target string) error {
	index, err := loadIndex()
	if err != nil {
		return err
	}
	index.Current[platform] = map[string]any{
		"id":          id,
		"target":      target,
		"switched_at": time.Now().Unix(),
	}
	now := time.Now().Unix()
	for _, item := range index.Accounts {
		if asString(item["platform"]) == platform && asString(item["id"]) == id {
			item["last_used"] = now
		}
	}
	return saveIndex(index)
}
