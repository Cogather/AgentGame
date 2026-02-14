package service

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"ocProxy/client"
	"ocProxy/config"

	"github.com/sashabaranov/go-openai"
)

// StreamResponse 流式响应包装器
type StreamResponse struct {
	Response  *http.Response
	APIFormat string // "openai" 或 "anthropic"
}

// ProxyService 代理服务
type ProxyService struct {
	chatClient          *client.OpenAIClient
	workClient          *client.OpenAIClient
	chatAnthropicClient *client.AnthropicClient
	workAnthropicClient *client.AnthropicClient
	workModelBaseURL    string // 工作模型 base URL，用于判断是否需要 reasoning_content
	chatModelID         string // 请求远端使用的模型 ID
	workModelID         string // 请求远端使用的模型 ID
	chatModelName       string // 用户请求名，用于路由判断
	workModelName       string // 用户请求名，用于路由判断
	chatAPIFormat       string // chat 模型 API 格式
	workAPIFormat       string // work 模型 API 格式
	preprocessEnabled   bool   // 是否启用前处理
}

// NewProxyService 创建新的代理服务
func NewProxyService(cfg *config.Config) *ProxyService {
	chatModelID := cfg.ChatModel.ModelID
	workModelID := cfg.WorkModel.ModelID
	chatModelName := cfg.ChatModel.ModelName
	workModelName := cfg.WorkModel.ModelName
	chatAPIFormat := cfg.ChatModel.APIFormat
	workAPIFormat := cfg.WorkModel.APIFormat

	if chatModelName == "" {
		chatModelName = chatModelID
	}
	if workModelName == "" {
		workModelName = workModelID
	}
	if chatAPIFormat == "" {
		chatAPIFormat = "openai"
	}
	if workAPIFormat == "" {
		workAPIFormat = "openai"
	}

	preprocessEnabled := cfg.PreprocessEnabled

	svc := &ProxyService{
		chatClient: client.NewOpenAIClient(
			cfg.ChatModel.BaseURL,
			cfg.ChatModel.APIKey,
			chatModelID,
		),
		workClient: client.NewOpenAIClient(
			cfg.WorkModel.BaseURL,
			cfg.WorkModel.APIKey,
			workModelID,
		),
		workModelBaseURL:  cfg.WorkModel.BaseURL,
		chatModelID:       chatModelID,
		workModelID:       workModelID,
		chatModelName:     chatModelName,
		workModelName:     workModelName,
		chatAPIFormat:     chatAPIFormat,
		workAPIFormat:     workAPIFormat,
		preprocessEnabled: preprocessEnabled,
	}

	// 初始化 Anthropic 客户端（如果配置了 anthropic 格式）
	if chatAPIFormat == "anthropic" {
		svc.chatAnthropicClient = client.NewAnthropicClient(
			cfg.ChatModel.BaseURL,
			cfg.ChatModel.APIKey,
			chatModelID,
		)
	}
	if workAPIFormat == "anthropic" {
		svc.workAnthropicClient = client.NewAnthropicClient(
			cfg.WorkModel.BaseURL,
			cfg.WorkModel.APIKey,
			workModelID,
		)
	}

	return svc
}

// DetermineModelType 根据请求的模型名称判断使用哪个模型（chat/work）
// 使用 model_name 进行匹配，而非 model_id
func (s *ProxyService) DetermineModelType(requestModel string) bool {
	return requestModel == s.workModelName
}

// GetChatClient 获取聊天模型客户端
func (s *ProxyService) GetChatClient() *client.OpenAIClient {
	return s.chatClient
}

// GetWorkClient 获取工作模型客户端
func (s *ProxyService) GetWorkClient() *client.OpenAIClient {
	return s.workClient
}

// GetChatModelName 获取聊天模型名称
func (s *ProxyService) GetChatModelName() string {
	return s.chatModelName
}

// GetWorkModelName 获取工作模型名称
func (s *ProxyService) GetWorkModelName() string {
	return s.workModelName
}

// GetChatModelID 获取聊天模型ID
func (s *ProxyService) GetChatModelID() string {
	return s.chatModelID
}

// GetWorkModelID 获取工作模型ID
func (s *ProxyService) GetWorkModelID() string {
	return s.workModelID
}

// GetChatAPIFormat 获取聊天模型API格式
func (s *ProxyService) GetChatAPIFormat() string {
	return s.chatAPIFormat
}

// GetWorkAPIFormat 获取工作模型API格式
func (s *ProxyService) GetWorkAPIFormat() string {
	return s.workAPIFormat
}

// GetChatAnthropicClient 获取聊天模型Anthropic客户端
func (s *ProxyService) GetChatAnthropicClient() *client.AnthropicClient {
	return s.chatAnthropicClient
}

// GetWorkAnthropicClient 获取工作模型Anthropic客户端
func (s *ProxyService) GetWorkAnthropicClient() *client.AnthropicClient {
	return s.workAnthropicClient
}

// callWorkModel 调用工作模型（支持 OpenAI 和 Anthropic 格式）
func (s *ProxyService) callWorkModel(ctx context.Context, workReq openai.ChatCompletionRequest) (interface{}, error) {
	log.Printf("[调用工作模型] 模型=%s, 流式=%v, 消息数=%d, apiFormat=%s", workReq.Model, workReq.Stream, len(workReq.Messages), s.workAPIFormat)

	// 如果是 anthropic 格式，需要转换请求和响应
	if s.workAPIFormat == "anthropic" && s.workAnthropicClient != nil {
		return s.callWorkModelAnthropic(ctx, workReq)
	}

	// 否则使用 OpenAI 格式
	if workReq.Stream {
		resp, err := s.workClient.ChatStream(ctx, workReq)
		if err != nil {
			log.Printf("[错误] 工作模型流式调用失败: %v", err)
			return nil, fmt.Errorf("工作模型流式调用失败: %w", err)
		}
		return &StreamResponse{Response: resp, APIFormat: "openai"}, nil
	}
	resp, err := s.workClient.Chat(ctx, workReq)
	if err != nil {
		log.Printf("[错误] 工作模型非流式调用失败: %v", err)
		return nil, fmt.Errorf("工作模型非流式调用失败: %w", err)
	}
	return resp, nil
}

// ProcessRequest 处理请求
func (s *ProxyService) ProcessRequest(ctx context.Context, req openai.ChatCompletionRequest, useWorkModel bool) (interface{}, error) {
	// 检查最后一条消息的 role
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("消息列表为空")
	}

	lastMessage := req.Messages[len(req.Messages)-1]
	isLastUserMessage := lastMessage.Role == openai.ChatMessageRoleUser

	// 前处理：请求工作模型且最后一条是 user 且启用前处理时，改为使用聊天模型（是否调用工具由 chat 模型自行处理）
	usedPreprocess := useWorkModel && isLastUserMessage && s.preprocessEnabled
	if usedPreprocess {
		useWorkModel = false
	}

	if useWorkModel {
		req.Model = s.workModelID
		log.Printf("[调用] 模型=%s, 流式=%v, 前处理=%v", s.workModelID, req.Stream, usedPreprocess)
		return s.callWorkModel(ctx, req)
	}

	req.Model = s.chatModelID
	log.Printf("[调用] 模型=%s, 流式=%v, 前处理=%v", s.chatModelID, req.Stream, usedPreprocess)
	if req.Stream {
		stream, err := s.chatClient.ChatStream(ctx, req)
		if err != nil {
			log.Printf("[错误] 创建流失败 (模型=%s): %v", s.chatModelID, err)
			return nil, fmt.Errorf("创建流失败: %w", err)
		}
		return &StreamResponse{Response: stream, APIFormat: s.chatAPIFormat}, nil
	}
	resp, err := s.chatClient.Chat(ctx, req)
	if err != nil {
		log.Printf("[错误] 调用模型失败 (模型=%s): %v", s.chatModelID, err)
		return nil, fmt.Errorf("调用模型失败: %w", err)
	}
	return resp, nil
}
