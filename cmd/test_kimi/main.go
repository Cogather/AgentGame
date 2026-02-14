// test_kimi 测试 Kimi API，验证带 tool_calls 的 assistant 消息如何正确发送
// 复现 "reasoning_content is missing in assistant tool call message" 错误并测试修复方案
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ocProxy/client"
	"ocProxy/config"

	"github.com/sashabaranov/go-openai"
)

func main() {
	cfgPath := "config.yaml"
	if wd, err := os.Getwd(); err == nil && filepath.Base(wd) == "test_kimi" {
		cfgPath = filepath.Join("..", "..", "config.yaml")
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		os.Exit(1)
	}

	baseURL := strings.TrimRight(cfg.WorkModel.BaseURL, "/")
	apiKey := cfg.WorkModel.APIKey
	modelID := cfg.WorkModel.ModelID
	url := baseURL + "/chat/completions"

	// 构建会触发错误的请求：包含 assistant 消息 + tool_calls（无 reasoning_content）
	// 模拟多轮对话：user -> assistant(tool_calls) -> tool -> user
	messages := []map[string]interface{}{
		{"role": "user", "content": "你好"},
		{
			"role": "assistant",
			"content": "",
			"tool_calls": []map[string]interface{}{
				{
					"id":       "call_xxx1",
					"type":     "function",
					"function": map[string]string{"name": "get_weather", "arguments": `{"city":"北京"}`},
				},
			},
		},
		{"role": "tool", "content": "晴，25度", "tool_call_id": "call_xxx1"},
		{"role": "user", "content": "继续"},
		{
			"role": "assistant",
			"content": "",
			"tool_calls": []map[string]interface{}{
				{
					"id":       "call_xxx2",
					"type":     "function",
					"function": map[string]string{"name": "search", "arguments": `{"q":"天气预报"}`},
				},
			},
		},
		{"role": "tool", "content": "无更多结果", "tool_call_id": "call_xxx2"},
		{"role": "user", "content": "总结一下"},
	}

	fmt.Println(strings.Repeat("=", 52))
	fmt.Println("Kimi API 测试 - 带 tool_calls 的 assistant 消息")
	fmt.Println(strings.Repeat("=", 52))

	// 方案1: 原始请求（预期失败）
	fmt.Println("\n方案1: 原始请求（无 reasoning_content）")
	tryRequest(url, apiKey, modelID, messages, nil, nil)

	// 方案2: 仅 reasoning: false
	fmt.Println("\n方案2: 添加 reasoning: false")
	tryRequest(url, apiKey, modelID, messages, func(req map[string]interface{}) {
		req["reasoning"] = false
	}, nil)

	// 方案3: 为 assistant+tool_calls 添加 reasoning_content: ""
	fmt.Println("\n方案3: 为 assistant+tool_calls 添加 reasoning_content: \"\"")
	tryRequest(url, apiKey, modelID, messages, nil, func(msgs []interface{}) {
		for _, m := range msgs {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			if msg["role"] != "assistant" {
				continue
			}
			if tc, ok := msg["tool_calls"].([]interface{}); ok && len(tc) > 0 {
				msg["reasoning_content"] = ""
			}
		}
	})

	// 方案4: reasoning: false + reasoning_content: ""
	fmt.Println("\n方案4: reasoning: false + reasoning_content: \"\"")
	tryRequest(url, apiKey, modelID, messages, func(req map[string]interface{}) {
		req["reasoning"] = false
	}, func(msgs []interface{}) {
		for _, m := range msgs {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			if msg["role"] != "assistant" {
				continue
			}
			if tc, ok := msg["tool_calls"].([]interface{}); ok && len(tc) > 0 {
				msg["reasoning_content"] = ""
			}
		}
	})

	// 方案5: 使用 openai.ChatCompletionRequest + PrepareMoonshotRequest（与主代码相同流程）
	fmt.Println("\n方案5: openai 结构体 + PrepareMoonshotRequest（主代码流程）")
	tryOpenAIRequest(url, apiKey, modelID)

	fmt.Println("\n测试完成")
}

func tryOpenAIRequest(url, apiKey, modelID string) {
	req := openai.ChatCompletionRequest{
		Model:      modelID,
		Messages:   buildOpenAIMessages(),
		Stream:     false,
		MaxTokens:  50,
	}
	body, err := client.PrepareMoonshotRequest(req)
	if err != nil {
		fmt.Printf("  ❌ PrepareMoonshotRequest 失败: %v\n", err)
		return
	}

	httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		fmt.Printf("  ❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("  ❌ 错误 %d: %s\n", resp.StatusCode, string(b))
		return
	}
	fmt.Printf("  ✅ 成功!\n")
}

func buildOpenAIMessages() []openai.ChatCompletionMessage {
	return []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "你好"},
		{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "",
			ToolCalls: []openai.ToolCall{
				{ID: "call_1", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "get_weather", Arguments: `{"city":"北京"}`}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "晴，25度", ToolCallID: "call_1"},
		{Role: openai.ChatMessageRoleUser, Content: "继续"},
		{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "",
			ToolCalls: []openai.ToolCall{
				{ID: "call_2", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "search", Arguments: `{"q":"天气"}`}},
			},
		},
		{Role: openai.ChatMessageRoleTool, Content: "无", ToolCallID: "call_2"},
		{Role: openai.ChatMessageRoleUser, Content: "总结"},
	}
}

func tryRequest(url, apiKey, modelID string, messages []map[string]interface{},
	modifyReq func(map[string]interface{}), modifyMsgs func([]interface{})) {
	req := map[string]interface{}{
		"model":    modelID,
		"messages": messages,
		"stream":   false,
		"max_tokens": 50,
	}
	if modifyReq != nil {
		modifyReq(req)
	}
	if modifyMsgs != nil {
		msgs := make([]interface{}, len(messages))
		for i, m := range messages {
			msgs[i] = m
		}
		modifyMsgs(msgs)
		req["messages"] = msgs
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("  ❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("  ❌ 错误 %d: %s\n", resp.StatusCode, string(b))
		return
	}
	var result map[string]interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		fmt.Printf("  ❌ 解析失败: %v\n", err)
		return
	}
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		c := choices[0].(map[string]interface{})
		if msg, ok := c["message"].(map[string]interface{}); ok {
			if content, ok := msg["content"].(string); ok {
				fmt.Printf("  ✅ 成功! 响应: %s\n", truncate(content, 100))
				return
			}
		}
	}
	fmt.Printf("  ✅ 成功! (raw): %s\n", truncate(string(b), 200))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
