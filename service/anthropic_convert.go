package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"ocProxy/client"
	"ocProxy/tools"

	"github.com/sashabaranov/go-openai"
)

// callWorkModelAnthropic 使用 Anthropic 格式调用工作模型
func (s *ProxyService) callWorkModelAnthropic(ctx context.Context, workReq openai.ChatCompletionRequest) (interface{}, error) {
	// 将 OpenAI 请求转换为 Anthropic 请求
	anthropicReq := s.convertOpenAIToAnthropicRequest(workReq)

	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("序列化 Anthropic 请求失败: %w", err)
	}

	log.Printf("[Anthropic请求体] %s", string(reqBody))

	if workReq.Stream {
		// 流式请求
		resp, err := s.workAnthropicClient.MessagesStream(ctx, reqBody)
		if err != nil {
			log.Printf("[错误] Anthropic 流式调用失败: %v", err)
			return nil, fmt.Errorf("Anthropic 流式调用失败: %w", err)
		}
		return &StreamResponse{Response: resp, APIFormat: "anthropic"}, nil
	}

	// 非流式请求
	log.Printf("[Anthropic请求] 发送请求体: %s", string(reqBody))
	resp, err := s.workAnthropicClient.Messages(ctx, reqBody)
	if err != nil {
		log.Printf("[错误] Anthropic 调用失败: %v", err)
		return nil, fmt.Errorf("Anthropic 调用失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取 Anthropic 响应
	var anthropicResp client.AnthropicMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("解析 Anthropic 响应失败: %w", err)
	}

	// 转换为 OpenAI 响应
	return s.convertAnthropicToOpenAIResponse(&anthropicResp, workReq.Model), nil
}

// convertOpenAIToAnthropicRequest 将 OpenAI 请求转换为 Anthropic 请求
func (s *ProxyService) convertOpenAIToAnthropicRequest(req openai.ChatCompletionRequest) *client.AnthropicMessageRequest {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192 // 默认 max_tokens
	}

	anthropicReq := &client.AnthropicMessageRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Stream:    req.Stream,
	}

	if req.Temperature > 0 {
		temp := float32(req.Temperature)
		anthropicReq.Temperature = &temp
	}
	if req.TopP > 0 {
		topP := float32(req.TopP)
		anthropicReq.TopP = &topP
	}

	// 转换消息
	var system string
	for i, msg := range req.Messages {
		log.Printf("[消息%d] role=%s, tool_call_id=%s", i, msg.Role, msg.ToolCallID)

		if msg.Role == openai.ChatMessageRoleSystem {
			system = msg.Content
			continue
		}

		// 处理 tool 角色消息（转换为 user 角色的 tool_result）
		if msg.Role == openai.ChatMessageRoleTool {
			log.Printf("[消息%d] 转换 tool 消息, tool_call_id=%s", i, msg.ToolCallID)
			toolResultContent := []client.AnthropicContentBlock{
				{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				},
			}
			anthropicMsg := client.AnthropicMessage{
				Role:    "user",
				Content: toolResultContent,
			}
			anthropicReq.Messages = append(anthropicReq.Messages, anthropicMsg)
			continue
		}

		// 处理 assistant 消息，如果包含 ToolCalls，需要转换为 content blocks
		if msg.Role == openai.ChatMessageRoleAssistant && len(msg.ToolCalls) > 0 {
			log.Printf("[消息%d] 转换 assistant 消息，包含 %d 个 tool_calls", i, len(msg.ToolCalls))
			contentBlocks := make([]client.AnthropicContentBlock, 0)

			// 先添加文本内容（如果有）
			if msg.Content != "" {
				contentBlocks = append(contentBlocks, client.AnthropicContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}

			// 添加 tool_use 块
			for _, tc := range msg.ToolCalls {
				if tc.Type == openai.ToolTypeFunction {
					var input map[string]interface{}
					if tc.Function.Arguments != "" {
						json.Unmarshal([]byte(tc.Function.Arguments), &input)
					}
					contentBlocks = append(contentBlocks, client.AnthropicContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
					log.Printf("[消息%d] 添加 tool_use: id=%s, name=%s", i, tc.ID, tc.Function.Name)
				}
			}

			anthropicMsg := client.AnthropicMessage{
				Role:    msg.Role,
				Content: contentBlocks,
			}
			anthropicReq.Messages = append(anthropicReq.Messages, anthropicMsg)
			continue
		}

		// 处理 MultiContent（当客户端发送数组格式的 content 时）
		if len(msg.MultiContent) > 0 {
			var contentBlocks []client.AnthropicContentBlock
			for _, part := range msg.MultiContent {
				switch part.Type {
				case openai.ChatMessagePartTypeText:
					contentBlocks = append(contentBlocks, client.AnthropicContentBlock{
						Type: "text",
						Text: part.Text,
					})
				case openai.ChatMessagePartTypeImageURL:
					if part.ImageURL != nil {
						contentBlocks = append(contentBlocks, client.AnthropicContentBlock{
							Type: "image",
							Text: fmt.Sprintf("[image: %s]", part.ImageURL.URL),
						})
					}
				}
			}
			anthropicMsg := client.AnthropicMessage{
				Role:    msg.Role,
				Content: contentBlocks,
			}
			anthropicReq.Messages = append(anthropicReq.Messages, anthropicMsg)
		} else {
			anthropicMsg := client.AnthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			}
			anthropicReq.Messages = append(anthropicReq.Messages, anthropicMsg)
		}
	}
	anthropicReq.System = system

	// 转换 tools
	for _, tool := range req.Tools {
		if tool.Type == openai.ToolTypeFunction && tool.Function != nil {
			var inputSchema map[string]interface{}
			if params, ok := tool.Function.Parameters.(map[string]interface{}); ok {
				inputSchema = params
			}
			anthropicTool := client.AnthropicTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				InputSchema: inputSchema,
			}
			anthropicReq.Tools = append(anthropicReq.Tools, anthropicTool)
		}
	}

	return anthropicReq
}

// convertAnthropicToOpenAIResponse 将 Anthropic 响应转换为 OpenAI 响应
func (s *ProxyService) convertAnthropicToOpenAIResponse(anthropicResp *client.AnthropicMessageResponse, model string) *openai.ChatCompletionResponse {
	var content string
	var toolCalls []openai.ToolCall

	for _, block := range anthropicResp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			toolCall := openai.ToolCall{
				Type: openai.ToolTypeFunction,
				ID:   block.ID,
				Function: openai.FunctionCall{
					Name:      block.Name,
					Arguments: tools.MarshalToString(block.Input),
				},
			}
			toolCalls = append(toolCalls, toolCall)
		}
	}

	// 转换 stop_reason
	finishReason := "stop"
	switch anthropicResp.StopReason {
	case "tool_use":
		finishReason = "tool_calls"
	case "max_tokens":
		finishReason = "length"
	}

	return &openai.ChatCompletionResponse{
		ID:    anthropicResp.ID,
		Model: model,
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:      openai.ChatMessageRoleAssistant,
					Content:   content,
					ToolCalls: toolCalls,
				},
				FinishReason: openai.FinishReason(finishReason),
			},
		},
		Usage: openai.Usage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
		},
	}
}
