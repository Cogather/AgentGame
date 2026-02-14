package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// AnthropicMessages 处理 Anthropic /v1/messages 请求
func (h *Handler) AnthropicMessages(w http.ResponseWriter, r *http.Request) {
	anthropicHandler := NewAnthropicHandler(h.service, nil)
	anthropicHandler.Messages(w, r)
}

// proxyAnthropicStreamToOpenAI 将 Anthropic 流式响应转换为 OpenAI 格式
func (h *Handler) proxyAnthropicStreamToOpenAI(ctx context.Context, w http.ResponseWriter, streamResp *http.Response, flusher http.Flusher) {
	defer streamResp.Body.Close()

	reader := bufio.NewReader(streamResp.Body)
	var messageID string

	// 跟踪当前 tool_use 块的状态
	type toolUseBlock struct {
		id        string
		name      string
		jsonAccum strings.Builder
	}
	var currentToolUse *toolUseBlock
	toolCallIndex := 0

	for {
		if ctx.Err() != nil {
			return
		}

		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}
		if err != nil {
			log.Printf("[错误] 读取 Anthropic 流: %v", err)
			return
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}

		data := bytes.TrimPrefix(line, []byte("data:"))
		data = bytes.TrimSpace(data)

		if bytes.Equal(data, []byte("[DONE]")) {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(data, &event); err != nil {
			log.Printf("[Anthropic流] JSON解析失败: %v, 数据: %s", err, string(data))
			continue
		}

		eventType, _ := event["type"].(string)
		log.Printf("[Anthropic流] 收到事件类型: %s", eventType)

		switch eventType {
		case "message_start":
			if msg, ok := event["message"].(map[string]interface{}); ok {
				messageID, _ = msg["id"].(string)
			}
			openaiEvent := map[string]interface{}{
				"id":      messageID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   "kimi-for-coding",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role": "assistant",
						},
						"finish_reason": nil,
					},
				},
			}
			eventData, _ := json.Marshal(openaiEvent)
			fmt.Fprintf(w, "data: %s\n\n", eventData)
			flusher.Flush()

		case "content_block_start":
			if cb, ok := event["content_block"].(map[string]interface{}); ok {
				cbType, _ := cb["type"].(string)
				if cbType == "tool_use" {
					id, _ := cb["id"].(string)
					name, _ := cb["name"].(string)
					currentToolUse = &toolUseBlock{id: id, name: name}
					log.Printf("[Anthropic流] tool_use 开始: id=%s, name=%s", id, name)

					openaiEvent := map[string]interface{}{
						"id":      messageID,
						"object":  "chat.completion.chunk",
						"created": time.Now().Unix(),
						"model":   "kimi-for-coding",
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]interface{}{
									"tool_calls": []map[string]interface{}{
										{
											"index": toolCallIndex,
											"id":    id,
											"type":  "function",
											"function": map[string]interface{}{
												"name":      name,
												"arguments": "",
											},
										},
									},
								},
								"finish_reason": nil,
							},
						},
					}
					eventData, _ := json.Marshal(openaiEvent)
					log.Printf("[Anthropic流] 写入 tool_call 起始: %s", string(eventData))
					fmt.Fprintf(w, "data: %s\n\n", eventData)
					flusher.Flush()
				}
			}

		case "content_block_delta":
			delta, _ := event["delta"].(map[string]interface{})
			deltaType, _ := delta["type"].(string)
			log.Printf("[Anthropic流] content_block_delta 类型: %s", deltaType)

			if deltaType == "text_delta" {
				text, _ := delta["text"].(string)
				log.Printf("[Anthropic流] 收到文本: %q", text)
				if text != "" {
					openaiEvent := map[string]interface{}{
						"id":      messageID,
						"object":  "chat.completion.chunk",
						"created": time.Now().Unix(),
						"model":   "kimi-for-coding",
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]interface{}{
									"content": text,
								},
								"finish_reason": nil,
							},
						},
					}
					eventData, _ := json.Marshal(openaiEvent)
					fmt.Fprintf(w, "data: %s\n\n", eventData)
					flusher.Flush()
				}
			} else if deltaType == "input_json_delta" {
				partialJSON, _ := delta["partial_json"].(string)
				if currentToolUse != nil && partialJSON != "" {
					currentToolUse.jsonAccum.WriteString(partialJSON)

					openaiEvent := map[string]interface{}{
						"id":      messageID,
						"object":  "chat.completion.chunk",
						"created": time.Now().Unix(),
						"model":   "kimi-for-coding",
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]interface{}{
									"tool_calls": []map[string]interface{}{
										{
											"index": toolCallIndex,
											"function": map[string]interface{}{
												"arguments": partialJSON,
											},
										},
									},
								},
								"finish_reason": nil,
							},
						},
					}
					eventData, _ := json.Marshal(openaiEvent)
					fmt.Fprintf(w, "data: %s\n\n", eventData)
					flusher.Flush()
				}
			}

		case "content_block_stop":
			if currentToolUse != nil {
				log.Printf("[Anthropic流] tool_use 完成: id=%s, name=%s, args=%s",
					currentToolUse.id, currentToolUse.name, currentToolUse.jsonAccum.String())
				currentToolUse = nil
				toolCallIndex++
			}

		case "message_delta":
			finishReason := "stop"
			if d, ok := event["delta"].(map[string]interface{}); ok {
				if sr, ok := d["stop_reason"].(string); ok {
					switch sr {
					case "tool_use":
						finishReason = "tool_calls"
					case "max_tokens":
						finishReason = "length"
					default:
						finishReason = sr
					}
				}
			}

			openaiEvent := map[string]interface{}{
				"id":      messageID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   "kimi-for-coding",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": finishReason,
					},
				},
			}
			eventData, _ := json.Marshal(openaiEvent)
			log.Printf("[Anthropic流] 写入结束: %s", string(eventData))
			fmt.Fprintf(w, "data: %s\n\n", eventData)
			flusher.Flush()
		}
	}
}
