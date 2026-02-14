package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GetString 从 map 中安全取 string 值
func GetString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// GetBool 从 map 中安全取 bool 值
func GetBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

// MarshalToString 将任意值序列化为 JSON 字符串，失败返回 "{}"
func MarshalToString(v interface{}) string {
	if v == nil {
		return "{}"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// GetToolResultContent 从 Anthropic tool_result 的 content 字段提取文本
func GetToolResultContent(v interface{}) string {
	switch content := v.(type) {
	case string:
		return content
	case []interface{}:
		var texts []string
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if text, ok := itemMap["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}
