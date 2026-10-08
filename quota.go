package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type liveQuota struct {
	email     string
	plan      string
	used      float64
	hasUsed   bool
	limited   bool
	credits   string
	accountID string
}

func queryLiveQuota(account Account) (liveQuota, error) {
	access := asString(account.Credentials["access_token"])
	if access == "" {
		return liveQuota{}, fmt.Errorf("%s has no access token", account.Email)
	}
	accountID := asString(account.Credentials["account_id"])
	request, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		return liveQuota{}, err
	}
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("ChatGPT-Account-Id", accountID)
	request.Header.Set("User-Agent", "codex_cli_rs")
	request.Header.Set("originator", "codex_cli_rs")
	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return liveQuota{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return liveQuota{}, err
	}
	if response.StatusCode >= 300 {
		return liveQuota{}, fmt.Errorf("%s usage lookup returned %s", account.Email, response.Status)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return liveQuota{}, err
	}
	rate := asMap(payload["rate_limit"])
	window := asMap(rate["primary_window"])
	result := liveQuota{
		email:     account.Email,
		plan:      asString(payload["plan_type"]),
		limited:   asBool(rate["limit_reached"]),
		credits:   asString(asMap(payload["credits"])["balance"]),
		accountID: account.ID,
	}
	if _, ok := window["used_percent"]; ok {
		result.used = float64(asInt64(window["used_percent"]))
		if number, ok := window["used_percent"].(float64); ok {
			result.used = number
		}
		result.hasUsed = true
	}
	return result, nil
}

func switchAvailable(platform, targetName string, restart bool) (Account, []liveQuota, error) {
	doc, err := loadMapping()
	if err != nil {
		return Account{}, nil, err
	}
	_, spec, err := platformSpec(doc, platform)
	if err != nil {
		return Account{}, nil, err
	}
	_, target, err := targetFrom(spec, targetName)
	if err != nil {
		return Account{}, nil, err
	}
	text, ok, err := readText(target, asString(target.values["account_index"]))
	if err != nil || !ok {
		return Account{}, nil, fmt.Errorf("account index not found")
	}
	var index map[string]any
	if err := json.Unmarshal([]byte(text), &index); err != nil {
		return Account{}, nil, err
	}
	var reports []liveQuota
	var best *liveQuota
	var bestAccount Account
	for _, item := range asSlice(index["accounts"]) {
		row := asMap(item)
		email := asString(row["email"])
		account, err := loadCockpitAccount(platform, email, targetName)
		if err != nil {
			return Account{}, reports, err
		}
		report, err := queryLiveQuota(account)
		if err != nil {
			fmt.Printf("%s quota lookup failed\n", email)
			continue
		}
		reports = append(reports, report)
		line := fmt.Sprintf("%s plan=%s", report.email, fallback(report.plan, "-"))
		if report.hasUsed {
			line += fmt.Sprintf(" used=%g%%", report.used)
		}
		line += fmt.Sprintf(" limit_reached=%t credits=%s", report.limited, fallback(report.credits, "-"))
		fmt.Println(line)
		if report.limited {
			continue
		}
		if best == nil || (report.hasUsed && (!best.hasUsed || report.used < best.used)) {
			copied := report
			best = &copied
			bestAccount = account
		}
	}
	if best == nil {
		return Account{}, reports, fmt.Errorf("no account has remaining quota")
	}
	current := emailFromAuthFile(target, asString(target.values["auth_json"]))
	if strings.EqualFold(current, best.email) {
		fmt.Printf("already using %s\n", best.email)
		return bestAccount, reports, nil
	}
	written, err := switchAccount(bestAccount, targetName, restart, "")
	if err != nil {
		return bestAccount, reports, err
	}
	if err := markIndexCurrent(target, bestAccount.ID); err != nil {
		fmt.Printf("auth updated, index current not saved: %s\n", err)
	}
	fmt.Printf("switched %s -> %s\n", platform, best.email)
	for _, path := range written {
		fmt.Println("wrote", path)
	}
	return bestAccount, reports, nil
}
