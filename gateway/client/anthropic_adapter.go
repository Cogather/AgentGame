package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ocProxy/tools"

	"github.com/sashabaranov/go-openai"
)

// AnthropicMessageRequest Anthropic /v1/messages 请求格式
type AnthropicMessageRequest struct {
	Model       string                 `json:"model"`
	Messages    []AnthropicMessage     `json:"messages"`
	MaxTokens   int                    `json:"max_tokens"`
	System      string                 `json:"system,omitempty"`
	Tools       []AnthropicTool        `json:"tools,omitempty"`
	ToolChoice  interface{}            `json:"tool_choice,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Temperature *float32               `json:"temperature,omitempty"`
	TopP        *float32               `json:"top_p,omitempty"`
}

// AnthropicMessage Anthropic 消息格式
type AnthropicMessage struct {
	Role    string                   `json:"role"`
	Content interface{}              `json:"content"` // string 或 []AnthropicContentBlock
}

// AnthropicContentBlock 内容块（支持文本和工具调用）
type AnthropicContentBlock struct {
	Type         string                   `json:"type"` // text, tool_use, tool_result
	Text         string                   `json:"text,omitempty"`
	ID           string                   `json:"id,omitempty"`
	Name         string                   `json:"name,omitempty"`
	Input        map[string]interface{}   `json:"input,omitempty"`
	ToolUseID    string                   `json:"tool_use_id,omitempty"`
	Content      interface{}              `json:"content,omitempty"`
	IsError      bool                     `json:"is_error,omitempty"`
}

// AnthropicTool Anthropic 工具定义
type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// AnthropicMessageResponse 非流式响应
type AnthropicMessageResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Model        string                  `json:"model"`
	Content      []AnthropicContentBlock `json:"content"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence string                  `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicUsage 用量统计
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicStreamEvent 流式事件
type AnthropicStreamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta json.RawMessage `json:"delta,omitempty"`
	Usage *AnthropicUsage `json:"usage,omitempty"`
	ID    string          `json:"id,omitempty"`
	Role  string          `json:"role,omitempty"`
	Model string          `json:"model,omitempty"`
	StopReason string     `json:"stop_reason,omitempty"`
}

// AnthropicDelta 流式 delta 内容
type AnthropicDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// --- 转换函数 ---

// ConvertAnthropicToOpenAIRequest 将 Anthropic 请求转换为 OpenAI 请求
func ConvertAnthropicToOpenAIRequest(anthropicReq *AnthropicMessageRequest) (*openai.ChatCompletionRequest, error) {
	openaiReq := &openai.ChatCompletionRequest{
		Model:     anthropicReq.Model,
		MaxTokens: anthropicReq.MaxTokens,
		Stream:    anthropicReq.Stream,
	}

	if anthropicReq.Temperature != nil {
		openaiReq.Temperature = *anthropicReq.Temperature
	}
	if anthropicReq.TopP != nil {
		openaiReq.TopP = *anthropicReq.TopP
	}

	// 转换 messages
	messages := make([]openai.ChatCompletionMessage, 0)

	// 添加 system prompt
	if anthropicReq.System != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: anthropicReq.System,
		})
	}

	// 转换对话消息
	for _, msg := range anthropicReq.Messages {
		openaiMsg, err := convertAnthropicMessageToOpenAI(msg)
		if err != nil {
			return nil, fmt.Errorf("转换消息失败: %w", err)
		}
		messages = append(messages, openaiMsg...)
	}

	openaiReq.Messages = messages

	// 转换 tools
	if len(anthropicReq.Tools) > 0 {
		tools := make([]openai.Tool, 0, len(anthropicReq.Tools))
		for _, tool := range anthropicReq.Tools {
			openaiTool := openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.InputSchema,
				},
			}
			tools = append(tools, openaiTool)
		}
		openaiReq.Tools = tools
	}

	// 转换 tool_choice
	if anthropicReq.ToolChoice != nil {
		openaiReq.ToolChoice = convertAnthropicToolChoice(anthropicReq.ToolChoice)
	}

	return openaiReq, nil
}

// convertAnthropicMessageToOpenAI 转换单条 Anthropic 消息为 OpenAI 消息列表
func convertAnthropicMessageToOpenAI(msg AnthropicMessage) ([]openai.ChatCompletionMessage, error) {
	var result []openai.ChatCompletionMessage

	switch content := msg.Content.(type) {
	case string:
		// 简单文本消息
		result = append(result, openai.ChatCompletionMessage{
			Role:    convertAnthropicRole(msg.Role),
			Content: content,
		})
	case []interface{}:
		// 多模态/工具消息
		openaiMsg := openai.ChatCompletionMessage{
			Role: convertAnthropicRole(msg.Role),
		}

		var textContent strings.Builder
		var toolCalls []openai.ToolCall
		var toolResults []ToolMessageContent

		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}

			blockType, _ := blockMap["type"].(string)

			switch blockType {
			case "text":
				if text, ok := blockMap["text"].(string); ok {
					textContent.WriteString(text)
				}

			case "tool_use":
				// 工具调用
				toolCall := openai.ToolCall{
					Type: openai.ToolTypeFunction,
					ID:   tools.GetString(blockMap,"id"),
					Function: openai.FunctionCall{
						Name:      tools.GetString(blockMap,"name"),
						Arguments: tools.MarshalToString(blockMap["input"]),
					},
				}
				toolCalls = append(toolCalls, toolCall)

			case "tool_result":
				// 工具结果 - 需要单独的消息
				toolResult := ToolMessageContent{
					ToolCallID: tools.GetString(blockMap,"tool_use_id"),
					Content:    tools.GetToolResultContent(blockMap["content"]),
					IsError:    tools.GetBool(blockMap,"is_error"),
				}
				toolResults = append(toolResults, toolResult)
			}
		}

		// 设置文本内容
		if textContent.Len() > 0 {
			openaiMsg.Content = textContent.String()
		}

		// 设置工具调用
		if len(toolCalls) > 0 {
			openaiMsg.ToolCalls = toolCalls
		}

		result = append(result, openaiMsg)

		// 添加工具结果消息
		for _, tr := range toolResults {
			result = append(result, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    tr.Content,
				ToolCallID: tr.ToolCallID,
			})
		}

	default:
		return nil, fmt.Errorf("不支持的消息内容类型: %T", msg.Content)
	}

	return result, nil
}

// ToolMessageContent 工具消息内容辅助结构
type ToolMessageContent struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// convertAnthropicRole 转换角色名称
func convertAnthropicRole(role string) string {
	switch role {
	case "user":
		return openai.ChatMessageRoleUser
	case "assistant":
		return openai.ChatMessageRoleAssistant
	default:
		return role
	}
}

// convertAnthropicToolChoice 转换 tool_choice
func convertAnthropicToolChoice(choice interface{}) string {
	switch c := choice.(type) {
	case string:
		switch c {
		case "auto":
			return "auto"
		case "any":
			return "required"
		case "none":
			return "none"
		default:
			return "auto"
		}
	case map[string]interface{}:
		// 特定工具选择 {"type": "tool", "name": "xxx"}
		if toolName, ok := c["name"].(string); ok {
			return fmt.Sprintf(`{"type": "function", "function": {"name": "%s"}}`, toolName)
		}
	}
	return "auto"
}

// --- OpenAI 转 Anthropic 响应 ---

// ConvertOpenAIToAnthropicResponse 将 OpenAI 响应转换为 Anthropic 格式
func ConvertOpenAIToAnthropicResponse(openaiResp *openai.ChatCompletionResponse, model string) *AnthropicMessageResponse {
	if len(openaiResp.Choices) == 0 {
		return &AnthropicMessageResponse{
			ID:   openaiResp.ID,
			Type: "message",
			Role: "assistant",
			Model: model,
			Content: []AnthropicContentBlock{},
		}
	}

	choice := openaiResp.Choices[0]
	message := choice.Message

	// 构建 content blocks
	content := make([]AnthropicContentBlock, 0)

	// 文本内容
	if message.Content != "" {
		content = append(content, AnthropicContentBlock{
			Type: "text",
			Text: message.Content,
		})
	}

	// 工具调用
	for _, tc := range message.ToolCalls {
		if tc.Type == openai.ToolTypeFunction {
			// 解析参数
			var input map[string]interface{}
			json.Unmarshal([]byte(tc.Function.Arguments), &input)

			content = append(content, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
	}

	// 转换 stop_reason
	stopReason := convertOpenAIStopReason(string(choice.FinishReason))

	return &AnthropicMessageResponse{
		ID:         openaiResp.ID,
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    content,
		StopReason: stopReason,
		Usage: AnthropicUsage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
	}
}

// convertOpenAIStopReason 转换停止原因
func convertOpenAIStopReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}

// --- 流式响应转换 ---

// AnthropicStreamWriter 流式响应写入器
type AnthropicStreamWriter struct {
	writer      http.ResponseWriter
	model       string
	messageID   string
	role        string
	index       int
	textBuffer  strings.Builder
	flusher     http.Flusher
}

// NewAnthropicStreamWriter 创建新的流式写入器
func NewAnthropicStreamWriter(w http.ResponseWriter, model string) *AnthropicStreamWriter {
	flusher, _ := w.(http.Flusher)
	return &AnthropicStreamWriter{
		writer:    w,
		model:     model,
		messageID: tools.GenerateMessageID(),
		role:      "assistant",
		flusher:   flusher,
	}
}

// WriteEvent 写入流式事件
func (w *AnthropicStreamWriter) WriteEvent(eventType string, data []byte) error {
	_, err := fmt.Fprintf(w.writer, "event: %s\ndata: %s\n\n", eventType, string(data))
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return err
}

// SendMessageStart 发送消息开始事件
func (w *AnthropicStreamWriter) SendMessageStart() error {
	event := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":         w.messageID,
			"type":       "message",
			"role":       w.role,
			"model":      w.model,
			"content":    []interface{}{},
			"stop_reason": nil,
		},
	}
	data, _ := json.Marshal(event)
	return w.WriteEvent("message_start", data)
}

// SendContentBlockStart 发送内容块开始
func (w *AnthropicStreamWriter) SendContentBlockStart(blockType string) error {
	event := map[string]interface{}{
		"type":       "content_block_start",
		"index":      w.index,
		"content_block": map[string]interface{}{
			"type": blockType,
		},
	}
	if blockType == "tool_use" {
		event["content_block"].(map[string]interface{})["id"] = tools.GenerateToolCallID()
		event["content_block"].(map[string]interface{})["name"] = ""
		event["content_block"].(map[string]interface{})["input"] = map[string]interface{}{}
	}
	data, _ := json.Marshal(event)
	return w.WriteEvent("content_block_start", data)
}

// SendContentBlockDelta 发送内容增量
func (w *AnthropicStreamWriter) SendContentBlockDelta(delta map[string]interface{}) error {
	event := map[string]interface{}{
		"type":  "content_block_delta",
		"index": w.index,
		"delta": delta,
	}
	data, _ := json.Marshal(event)
	return w.WriteEvent("content_block_delta", data)
}

// SendContentBlockStop 发送内容块结束
func (w *AnthropicStreamWriter) SendContentBlockStop() error {
	event := map[string]interface{}{
		"type":  "content_block_stop",
		"index": w.index,
	}
	data, _ := json.Marshal(event)
	err := w.WriteEvent("content_block_stop", data)
	w.index++
	return err
}

// SendMessageDelta 发送消息增量（用量等）
func (w *AnthropicStreamWriter) SendMessageDelta(usage *AnthropicUsage, stopReason string) error {
	delta := map[string]interface{}{}
	if stopReason != "" {
		delta["stop_reason"] = stopReason
	}

	event := map[string]interface{}{
		"type": "message_delta",
		"delta": delta,
	}

	if usage != nil {
		event["usage"] = usage
	}

	data, _ := json.Marshal(event)
	return w.WriteEvent("message_delta", data)
}

// SendMessageStop 发送消息结束
func (w *AnthropicStreamWriter) SendMessageStop() error {
	event := map[string]string{
		"type": "message_stop",
	}
	data, _ := json.Marshal(event)
	return w.WriteEvent("message_stop", data)
}

// ParseAnthropicRequest 从 HTTP 请求体解析 Anthropic 请求
func ParseAnthropicRequest(body io.Reader) (*AnthropicMessageRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("读取请求体失败: %w", err)
	}

	var req AnthropicMessageRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	return &req, nil
}

// WriteAnthropicError 写入 Anthropic 格式的错误响应
func WriteAnthropicError(w http.ResponseWriter, statusCode int, errType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errType,
			"message": message,
		},
	}

	json.NewEncoder(w).Encode(errorResp)
}

// ProxyOpenAIStreamToAnthropic 将 OpenAI 流式响应代理转换为 Anthropic 格式
func ProxyOpenAIStreamToAnthropic(openaiResp *http.Response, w http.ResponseWriter, model string) error {
	// 设置 Anthropic 流式响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	writer := NewAnthropicStreamWriter(w, model)

	// 发送消息开始
	if err := writer.SendMessageStart(); err != nil {
		return err
	}

	// 开始内容块
	if err := writer.SendContentBlockStart("text"); err != nil {
		return err
	}

	// 读取并转换 OpenAI SSE 流
	reader := io.Reader(openaiResp.Body)
	buffer := make([]byte, 4096)
	var totalTokens int

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			// 这里需要解析 OpenAI SSE 格式并转换为 Anthropic 格式
			// 简化实现：直接将文本内容转发
			lines := bytes.Split(buffer[:n], []byte("\n"))
			for _, line := range lines {
				line = bytes.TrimSpace(line)
				if len(line) == 0 {
					continue
				}

				// 解析 data: 行
				if bytes.HasPrefix(line, []byte("data: ")) {
					data := bytes.TrimPrefix(line, []byte("data: "))

					// 检查 [DONE]
					if bytes.Equal(data, []byte("[DONE]")) {
						continue
					}

					// 解析 OpenAI 流式响应
					var streamResp openai.ChatCompletionStreamResponse
					if jsonErr := json.Unmarshal(data, &streamResp); jsonErr == nil {
						if len(streamResp.Choices) > 0 {
							delta := streamResp.Choices[0].Delta
							if delta.Content != "" {
								deltaEvent := map[string]string{
									"type": "text_delta",
									"text": delta.Content,
								}
								deltaData, _ := json.Marshal(deltaEvent)
								writer.WriteEvent("content_block_delta", deltaData)
								totalTokens++
							}
						}
					}
				}
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// 结束内容块
	if err := writer.SendContentBlockStop(); err != nil {
		return err
	}

	// 发送消息增量（用量）
	usage := &AnthropicUsage{
		OutputTokens: totalTokens,
	}
	if err := writer.SendMessageDelta(usage, "end_turn"); err != nil {
		return err
	}

	// 发送消息结束
	if err := writer.SendMessageStop(); err != nil {
		return err
	}

	flusher.Flush()
	return nil
}
