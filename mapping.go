package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed mappings/platforms.json
var embeddedMapping []byte

func loadMapping() (map[string]any, error) {
	raw, err := mappingBytes()
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc["schema"] != "cockpit-cli.platform-map" {
		return nil, fmt.Errorf("unsupported mapping schema")
	}
	return doc, nil
}

func mappingBytes() ([]byte, error) {
	if path := os.Getenv("COCKPIT_CLI_MAP"); path != "" {
		return os.ReadFile(path)
	}
	if exe, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(exe), "mappings", "platforms.json")
		if b, err := os.ReadFile(beside); err == nil {
			return b, nil
		}
	}
	return embeddedMapping, nil
}

func platformSpec(doc map[string]any, name string) (string, map[string]any, error) {
	platforms, _ := doc["platforms"].(map[string]any)
	key := strings.ToLower(strings.TrimSpace(name))
	for platform, raw := range platforms {
		spec, _ := raw.(map[string]any)
		aliases := []string{platform}
		for _, item := range asSlice(spec["aliases"]) {
			if s, ok := item.(string); ok {
				aliases = append(aliases, s)
			}
		}
		for _, alias := range aliases {
			if strings.ToLower(alias) == key {
				return platform, spec, nil
			}
		}
	}
	return "", nil, fmt.Errorf("unknown platform %q", name)
}

func lookup(obj map[string]any, path string) any {
	var current any = obj
	for _, part := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		next, ok := m[part]
		if !ok || next == nil {
			return nil
		}
		current = next
	}
	switch v := current.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
	case []any:
		if len(v) == 0 {
			return nil
		}
	case map[string]any:
		if len(v) == 0 {
			return nil
		}
	}
	return current
}

func firstValue(obj map[string]any, paths []any) any {
	for _, item := range paths {
		path, ok := item.(string)
		if !ok {
			continue
		}
		if value := lookup(obj, path); value != nil {
			return value
		}
	}
	return nil
}

func asSlice(v any) []any {
	items, _ := v.([]any)
	return items
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}

func asInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		var n int64
		fmt.Sscan(t, &n)
		return n
	default:
		return 0
	}
}
