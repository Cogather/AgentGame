package tools

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// GetString 从 map 中获取字符串值
func GetString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// GetBool 从 map 中获取布尔值
func GetBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return false
}

// MarshalToString 将数据序列化为 JSON 字符串
func MarshalToString(v interface{}) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// GetToolResultContent 获取工具结果内容
func GetToolResultContent(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		// 处理内容块数组
		result, _ := json.Marshal(val)
		return string(result)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GenerateMessageID 生成消息 ID
func GenerateMessageID() string {
	return fmt.Sprintf("msg_%s", generateRandomID())
}

// GenerateToolCallID 生成工具调用 ID
func GenerateToolCallID() string {
	return fmt.Sprintf("tool_%s", generateRandomID())
}

// generateRandomID 生成随机 ID
func generateRandomID() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	result := make([]byte, 24)
	for i := range result {
		result[i] = letters[rand.Intn(len(letters))]
	}
	return string(result)
}
