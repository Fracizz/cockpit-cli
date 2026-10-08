package main

import "encoding/base64"

func encodeVarint(value uint64) []byte {
	var out []byte
	for value >= 0x80 {
		out = append(out, byte(value&0x7F)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}

func readVarint(data []byte, offset int) (uint64, int, error) {
	var result uint64
	var shift uint
	pos := offset
	for {
		if pos >= len(data) {
			return 0, 0, errTruncated
		}
		b := data[pos]
		result |= uint64(b&0x7F) << shift
		pos++
		if b&0x80 == 0 {
			return result, pos, nil
		}
		shift += 7
		if shift > 63 {
			return 0, 0, errTruncated
		}
	}
}

func skipField(data []byte, offset int, wireType byte) (int, error) {
	switch wireType {
	case 0:
		_, pos, err := readVarint(data, offset)
		return pos, err
	case 1:
		return offset + 8, nil
	case 2:
		length, pos, err := readVarint(data, offset)
		if err != nil {
			return 0, err
		}
		return pos + int(length), nil
	case 5:
		return offset + 4, nil
	default:
		return 0, errTruncated
	}
}

func encodeLenDelim(fieldNum int, payload []byte) []byte {
	tag := uint64(fieldNum<<3) | 2
	out := encodeVarint(tag)
	out = append(out, encodeVarint(uint64(len(payload)))...)
	return append(out, payload...)
}

func encodeString(fieldNum int, value string) []byte {
	return encodeLenDelim(fieldNum, []byte(value))
}

func encodeVarintField(fieldNum int, value uint64) []byte {
	return append(encodeVarint(uint64(fieldNum<<3)), encodeVarint(value)...)
}

func createOAuthInfo(spec map[string]any, credentials map[string]any) []byte {
	fields := asMap(spec["oauth_token_info"])
	expiry := uint64(asInt64(credentials["expiry_timestamp"]))
	timestamp := append(encodeVarintField(1, expiry), encodeVarintField(2, 0)...)
	body := append([]byte{}, encodeString(int(asInt64(fields["access_token"])), asString(credentials["access_token"]))...)
	body = append(body, encodeString(int(asInt64(fields["token_type"])), asString(fields["token_type_value"]))...)
	body = append(body, encodeString(int(asInt64(fields["refresh_token"])), asString(credentials["refresh_token"]))...)
	body = append(body, encodeLenDelim(int(asInt64(fields["expiry"])), timestamp)...)
	if idToken := stringsTrim(asString(credentials["id_token"])); idToken != "" {
		body = append(body, encodeString(int(asInt64(fields["id_token"])), idToken)...)
	}
	if asBool(credentials["is_gcp_tos"]) {
		body = append(body, encodeVarintField(int(asInt64(fields["is_gcp_tos"])), 1)...)
	}
	return body
}

func createUnifiedTopicEntry(spec map[string]any, sentinel string, payload []byte) []byte {
	row := encodeString(int(asInt64(spec["row_value_field"])), base64.StdEncoding.EncodeToString(payload))
	entry := append(encodeString(int(asInt64(spec["sentinel_key_field"])), sentinel), encodeLenDelim(int(asInt64(spec["row_field"])), row)...)
	return encodeLenDelim(int(asInt64(spec["unified_topic_entry_field"])), entry)
}

func entryKey(data []byte, spec map[string]any) (string, bool) {
	offset := 0
	want := int(asInt64(spec["sentinel_key_field"]))
	for offset < len(data) {
		tag, pos, err := readVarint(data, offset)
		if err != nil {
			return "", false
		}
		wireType := byte(tag & 7)
		fieldNum := int(tag >> 3)
		if fieldNum == want && wireType == 2 {
			length, content, err := readVarint(data, pos)
			if err != nil || content+int(length) > len(data) {
				return "", false
			}
			return string(data[content : content+int(length)]), true
		}
		next, err := skipField(data, pos, wireType)
		if err != nil {
			return "", false
		}
		offset = next
	}
	return "", false
}

func removeUnifiedTopicEntry(data []byte, targetKey string, spec map[string]any) ([]byte, error) {
	var result []byte
	offset := 0
	entryField := int(asInt64(spec["unified_topic_entry_field"]))
	for offset < len(data) {
		start := offset
		tag, pos, err := readVarint(data, offset)
		if err != nil {
			return nil, err
		}
		wireType := byte(tag & 7)
		fieldNum := int(tag >> 3)
		next, err := skipField(data, pos, wireType)
		if err != nil {
			return nil, err
		}
		drop := false
		if fieldNum == entryField && wireType == 2 {
			length, content, err := readVarint(data, pos)
			if err != nil {
				return nil, err
			}
			if key, ok := entryKey(data[content:content+int(length)], spec); ok && key == targetKey {
				drop = true
			}
		}
		if !drop {
			result = append(result, data[start:next]...)
		}
		offset = next
	}
	return result, nil
}

func buildOAuthTopic(spec map[string]any, existing []byte, credentials map[string]any) ([]byte, error) {
	topic := existing
	for _, key := range []string{"oauth_sentinel", "auth_state_sentinel"} {
		next, err := removeUnifiedTopicEntry(topic, asString(spec[key]), spec)
		if err != nil {
			return nil, err
		}
		topic = next
	}
	payload := createOAuthInfo(spec, credentials)
	return append(topic, createUnifiedTopicEntry(spec, asString(spec["oauth_sentinel"]), payload)...), nil
}

func buildEnterpriseTopic(spec map[string]any, projectID string) []byte {
	payload := encodeString(int(asInt64(spec["string_value_field"])), projectID)
	return createUnifiedTopicEntry(spec, asString(spec["enterprise_sentinel"]), payload)
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
