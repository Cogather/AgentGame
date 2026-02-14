package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// OpenAIClient OpenAI 协议客户端封装
type OpenAIClient struct {
	model      string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIClient 创建新的 OpenAI 客户端
func NewOpenAIClient(baseURL, apiKey, modelID string) *OpenAIClient {
	return &OpenAIClient{
		model:   modelID,
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 0, // 不设置超时，让流式请求可以持续
		},
	}
}

// doRequest 统一发起 POST /chat/completions 请求。stream 为 true 时使用 context.Background() 避免流被取消。
func (c *OpenAIClient) doRequest(ctx context.Context, body []byte, stream bool) (*http.Response, error) {
	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	reqCtx := ctx
	if stream {
		reqCtx = context.Background()
	}
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	// 添加 User-Agent，Kimi Code 需要特定标识
	httpReq.Header.Set("User-Agent", "ClaudeCode/0.1.0")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
		httpReq.Header.Set("Cache-Control", "no-cache")
		httpReq.Header.Set("Connection", "keep-alive")
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[错误] HTTP 请求失败: %v", err)
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("[错误] API 错误: %d, %s", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("API 错误 %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return resp, nil
}

// bodyForRequest 根据 baseURL 生成请求体：Moonshot 用 PrepareMoonshotRequest，否则标准 JSON
func (c *OpenAIClient) bodyForRequest(req openai.ChatCompletionRequest, stream bool) ([]byte, error) {
	reqCopy := req
	if reqCopy.Model == "" {
		reqCopy.Model = c.model
	}
	reqCopy.Stream = stream
	if IsMoonshotAPI(c.baseURL) {
		return PrepareMoonshotRequest(reqCopy)
	}
	return json.Marshal(reqCopy)
}

// Chat 非流式聊天（Moonshot 时自动带 reasoning_content）
func (c *OpenAIClient) Chat(ctx context.Context, req openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	body, err := c.bodyForRequest(req, false)
	if err != nil {
		return nil, err
	}
	model := req.Model
	if model == "" {
		model = c.model
	}
	log.Printf("[客户端] 调用模型 %s (非流式), 消息数=%d", model, len(req.Messages))
	resp, err := c.doRequest(ctx, body, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var chatResp openai.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &chatResp, nil
}

// ChatStream 流式聊天，直接返回 *http.Response（Moonshot 时自动带 reasoning_content）
func (c *OpenAIClient) ChatStream(ctx context.Context, req openai.ChatCompletionRequest) (*http.Response, error) {
	body, err := c.bodyForRequest(req, true)
	if err != nil {
		return nil, err
	}
	model := req.Model
	if model == "" {
		model = c.model
	}
	log.Printf("[客户端] 调用模型 %s (流式), 消息数=%d", model, len(req.Messages))
	return c.doRequest(ctx, body, true)
}

// --- Moonshot / reasoning_content 扩展（仅请求体增加思考内容字段）---

// PrepareMoonshotRequest 为 Moonshot 等 thinking 模型准备请求体：补全 reasoning_content、设置 reasoning: false
func PrepareMoonshotRequest(req openai.ChatCompletionRequest) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析请求失败: %w", err)
	}
	messages, ok := raw["messages"].([]interface{})
	if ok {
		for i, m := range messages {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			if role != openai.ChatMessageRoleAssistant {
				continue
			}
			if _, has := msg["reasoning_content"]; has {
				continue
			}
			msg["reasoning_content"] = " "
			messages[i] = msg
		}
		raw["messages"] = messages
	}
	raw["reasoning"] = false
	return json.Marshal(raw)
}

// IsMoonshotAPI 判断 baseURL 是否为 Moonshot API（需要 reasoning_content）
func IsMoonshotAPI(baseURL string) bool {
	return strings.Contains(baseURL, "moonshot")
}
