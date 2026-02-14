package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"ocProxy/client"
	"ocProxy/config"
	"ocProxy/service"

	"github.com/sashabaranov/go-openai"
)

// AnthropicHandler Anthropic 协议处理器
type AnthropicHandler struct {
	service        *service.ProxyService
	cfg            *config.Config
	chatClient     *client.OpenAIClient
	workClient     *client.OpenAIClient
	chatModelName  string
	workModelName  string
	chatModelID    string
	workModelID    string
}

// NewAnthropicHandler 创建新的 Anthropic 处理器
func NewAnthropicHandler(svc *service.ProxyService, cfg *config.Config) *AnthropicHandler {
	return &AnthropicHandler{
		service:       svc,
		cfg:           cfg,
		chatClient:    svc.GetChatClient(),
		workClient:    svc.GetWorkClient(),
		chatModelName: svc.GetChatModelName(),
		workModelName: svc.GetWorkModelName(),
		chatModelID:   svc.GetChatModelID(),
		workModelID:   svc.GetWorkModelID(),
	}
}

// Messages 处理 Anthropic /v1/messages 请求
func (h *AnthropicHandler) Messages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 读取原始请求体
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[错误] 读取请求体失败: %v", err)
		client.WriteAnthropicError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// 解析 Anthropic 请求（用于判断路由和日志）
	var anthropicReq client.AnthropicMessageRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
		log.Printf("[错误] 解析 Anthropic 请求失败: %v", err)
		client.WriteAnthropicError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	log.Printf("[Anthropic] 收到请求: model=%s, stream=%v, messages=%d",
		anthropicReq.Model, anthropicReq.Stream, len(anthropicReq.Messages))

	// 根据模型名判断使用哪个客户端
	useWorkModel := h.isWorkModel(anthropicReq.Model)

	// 检查是否需要直通模式（Anthropic 格式直接转发）
	apiFormat := h.service.GetChatAPIFormat()
	if useWorkModel {
		apiFormat = h.service.GetWorkAPIFormat()
	}

	// 如果是 anthropic 格式，直接转发请求
	if apiFormat == "anthropic" {
		h.handleAnthropicDirect(ctx, w, bodyBytes, useWorkModel, anthropicReq.Stream)
		return
	}

	// 否则转换为 OpenAI 请求处理
	openaiReq, err := client.ConvertAnthropicToOpenAIRequest(&anthropicReq)
	if err != nil {
		log.Printf("[错误] 转换请求失败: %v", err)
		client.WriteAnthropicError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// 处理流式/非流式请求
	if anthropicReq.Stream {
		h.handleStreamRequest(ctx, w, openaiReq, useWorkModel, anthropicReq.Model)
	} else {
		h.handleNonStreamRequest(ctx, w, openaiReq, useWorkModel, anthropicReq.Model)
	}
}

// handleAnthropicDirect 直接转发 Anthropic 请求（不转换格式）
func (h *AnthropicHandler) handleAnthropicDirect(ctx context.Context, w http.ResponseWriter, bodyBytes []byte, useWorkModel bool, stream bool) {
	var anthropicClient *client.AnthropicClient
	modelID := h.chatModelID
	modelType := "聊天"
	
	if useWorkModel {
		anthropicClient = h.service.GetWorkAnthropicClient()
		modelID = h.workModelID
		modelType = "工作"
	} else {
		anthropicClient = h.service.GetChatAnthropicClient()
	}

	if anthropicClient == nil {
		log.Printf("[错误] %s 模型的 Anthropic 客户端未初始化", modelType)
		client.WriteAnthropicError(w, http.StatusInternalServerError, "server_error", "Anthropic client not initialized")
		return
	}

	log.Printf("[Anthropic] 直接转发到 %s 模型 (%s): %s", modelType, map[bool]string{true: "流式", false: "非流式"}[stream], modelID)

	// 修改请求体中的模型名
	var reqMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqMap); err != nil {
		client.WriteAnthropicError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	reqMap["model"] = modelID
	modifiedBody, _ := json.Marshal(reqMap)

	if stream {
		// 流式请求
		resp, err := anthropicClient.MessagesStream(ctx, modifiedBody)
		if err != nil {
			log.Printf("[错误] Anthropic 流式请求失败: %v", err)
			client.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}
		defer resp.Body.Close()

		// 直接转发 SSE 流
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("[错误] 读取流失败: %v", err)
				return
			}
			w.Write(line)
			flusher.Flush()
		}
	} else {
		// 非流式请求
		resp, err := anthropicClient.Messages(ctx, modifiedBody)
		if err != nil {
			log.Printf("[错误] Anthropic 请求失败: %v", err)
			client.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}
		defer resp.Body.Close()

		// 直接转发响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// isWorkModel 判断是否为工作模型
func (h *AnthropicHandler) isWorkModel(model string) bool {
	// Anthropic 格式的模型名可能是 "local-models/work" 或 "work"
	// 检查是否包含 work 模型名
	if strings.Contains(model, h.workModelName) {
		return true
	}
	// 如果模型名是 "local-models/chat" 等形式，检查是否为聊天模型
	if strings.Contains(model, h.chatModelName) {
		return false
	}
	// 默认使用工作模型
	return true
}

// handleNonStreamRequest 处理非流式请求
func (h *AnthropicHandler) handleNonStreamRequest(ctx context.Context, w http.ResponseWriter, openaiReq *openai.ChatCompletionRequest, useWorkModel bool, originalModel string) {
	// 选择客户端
	var targetClient *client.OpenAIClient
	var modelID string
	if useWorkModel {
		targetClient = h.workClient
		modelID = h.workModelID
	} else {
		targetClient = h.chatClient
		modelID = h.chatModelID
	}

	// 设置模型 ID
	openaiReq.Model = modelID

	log.Printf("[Anthropic] 调用 %s 模型 (非流式): %s", map[bool]string{true: "工作", false: "聊天"}[useWorkModel], modelID)

	// 调用 OpenAI 客户端
	resp, err := targetClient.Chat(ctx, *openaiReq)
	if err != nil {
		log.Printf("[错误] 调用模型失败: %v", err)
		client.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	// 转换为 Anthropic 响应
	anthropicResp := client.ConvertOpenAIToAnthropicResponse(resp, originalModel)

	// 返回响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

// handleStreamRequest 处理流式请求
func (h *AnthropicHandler) handleStreamRequest(ctx context.Context, w http.ResponseWriter, openaiReq *openai.ChatCompletionRequest, useWorkModel bool, originalModel string) {
	// 选择客户端
	var targetClient *client.OpenAIClient
	var modelID string
	if useWorkModel {
		targetClient = h.workClient
		modelID = h.workModelID
	} else {
		targetClient = h.chatClient
		modelID = h.chatModelID
	}

	// 设置模型 ID
	openaiReq.Model = modelID

	log.Printf("[Anthropic] 调用 %s 模型 (流式): %s", map[bool]string{true: "工作", false: "聊天"}[useWorkModel], modelID)

	// 调用 OpenAI 客户端获取流
	streamResp, err := targetClient.ChatStream(ctx, *openaiReq)
	if err != nil {
		log.Printf("[错误] 创建流失败: %v", err)
		client.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	defer streamResp.Body.Close()

	// 设置响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		client.WriteAnthropicError(w, http.StatusInternalServerError, "server_error", "Streaming not supported")
		return
	}

	// 创建流式写入器
	writer := client.NewAnthropicStreamWriter(w, originalModel)

	// 发送消息开始事件
	if err := writer.SendMessageStart(); err != nil {
		log.Printf("[错误] 发送消息开始事件失败: %v", err)
		return
	}

	// 解析并转发 SSE 流
	reader := bufio.NewReader(streamResp.Body)
	var contentBlockStarted bool
	var outputTokens int

	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[错误] 读取流失败: %v", err)
			return
		}

		// 处理 SSE 行
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// 解析 data: 行
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		data := bytes.TrimPrefix(line, []byte("data: "))

		// 检查 [DONE]
		if bytes.Equal(data, []byte("[DONE]")) {
			break
		}

		// 解析 OpenAI 流式响应
		var streamResp openai.ChatCompletionStreamResponse
		if jsonErr := json.Unmarshal(data, &streamResp); jsonErr != nil {
			log.Printf("[警告] 解析流式响应失败: %v", jsonErr)
			continue
		}

		if len(streamResp.Choices) == 0 {
			continue
		}

		choice := streamResp.Choices[0]
		delta := choice.Delta

		// 处理角色（只在开始时）
		if delta.Role != "" && !contentBlockStarted {
			// 开始文本内容块
			if err := writer.SendContentBlockStart("text"); err != nil {
				log.Printf("[错误] 发送内容块开始失败: %v", err)
				return
			}
			contentBlockStarted = true
		}

		// 处理文本内容增量
		if delta.Content != "" && contentBlockStarted {
			deltaEvent := map[string]string{
				"type": "text_delta",
				"text": delta.Content,
			}
			deltaData, _ := json.Marshal(deltaEvent)
			if err := writer.WriteEvent("content_block_delta", deltaData); err != nil {
				log.Printf("[错误] 发送内容增量失败: %v", err)
				return
			}
			outputTokens++
		}

		// 处理工具调用（Anthropic 也支持工具调用）
		if len(delta.ToolCalls) > 0 {
			// 简化处理：工具调用在流式响应中比较复杂
			// 这里只处理文本内容，工具调用需要更复杂的转换
			for _, tc := range delta.ToolCalls {
				if tc.Function.Arguments != "" {
					// 累积工具调用参数
					log.Printf("[Anthropic] 工具调用参数: %s", tc.Function.Arguments)
				}
			}
		}
	}

	// 结束内容块
	if contentBlockStarted {
		if err := writer.SendContentBlockStop(); err != nil {
			log.Printf("[错误] 发送内容块结束失败: %v", err)
			return
		}
	}

	// 发送消息增量（用量和停止原因）
	usage := &client.AnthropicUsage{
		OutputTokens: outputTokens,
	}
	if err := writer.SendMessageDelta(usage, "end_turn"); err != nil {
		log.Printf("[错误] 发送消息增量失败: %v", err)
		return
	}

	// 发送消息停止事件
	if err := writer.SendMessageStop(); err != nil {
		log.Printf("[错误] 发送消息停止事件失败: %v", err)
		return
	}

	flusher.Flush()
}

// 修复：添加缺少的 context 包导入修复
// 注意：需要在文件顶部添加 "context" 到 imports

// SetupAnthropicRoutes 设置 Anthropic 协议路由
func SetupAnthropicRoutes(r interface{}, h *AnthropicHandler) {
	// 此方法由调用者在 main.go 中设置路由
	// 路由: POST /v1/messages
}
