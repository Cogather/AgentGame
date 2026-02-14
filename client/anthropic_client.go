package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicClient Anthropic 协议客户端（原生转发）
type AnthropicClient struct {
	model      string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAnthropicClient 创建新的 Anthropic 客户端
func NewAnthropicClient(baseURL, apiKey, modelID string) *AnthropicClient {
	return &AnthropicClient{
		model:   modelID,
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 0, // 不设置超时，让流式请求可以持续
		},
	}
}

// Messages 发送 Anthropic /v1/messages 请求（非流式）
func (c *AnthropicClient) Messages(ctx context.Context, requestBody []byte) (*http.Response, error) {
	url := strings.TrimRight(c.baseURL, "/") + "/messages"
	
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "ClaudeCode/0.1.0")
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API 错误 %d: %s", resp.StatusCode, string(bodyBytes))
	}
	
	return resp, nil
}

// MessagesStream 发送 Anthropic /v1/messages 请求（流式）
func (c *AnthropicClient) MessagesStream(ctx context.Context, requestBody []byte) (*http.Response, error) {
	// 修改请求体，设置 stream: true
	var reqMap map[string]interface{}
	if err := json.Unmarshal(requestBody, &reqMap); err != nil {
		return nil, fmt.Errorf("解析请求体失败: %w", err)
	}
	reqMap["stream"] = true
	
	modifiedBody, err := json.Marshal(reqMap)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}
	
	url := strings.TrimRight(c.baseURL, "/") + "/messages"
	
	// 流式请求使用 background context 避免被取消
	reqCtx := ctx
	if _, ok := ctx.Deadline(); ok {
		reqCtx = context.Background()
	}
	
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(modifiedBody))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "ClaudeCode/0.1.0")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API 错误 %d: %s", resp.StatusCode, string(bodyBytes))
	}
	
	return resp, nil
}
