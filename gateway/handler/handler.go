package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"ocProxy/fake_app"
	gamerank "ocProxy/game/rank"
	gameuser "ocProxy/game/user"
	"ocProxy/gateway/config"
	"ocProxy/gateway/internal/logger"
	"ocProxy/gateway/internal/skill"
	"ocProxy/gateway/service"

	"github.com/gorilla/mux"
	"github.com/sashabaranov/go-openai"
)

// Handler HTTP 请求处理器
type Handler struct {
	service         *service.ProxyService
	promptLogger    *logger.PromptLogger
	responseLogger  *logger.ResponseLogger
	skillDirs       []string // 技能目录列表，每个目录下 SKILL.md 内容作为一条 user 消息注入 system 之后
	userManager     *gameuser.UserManager
	userHandler     *gameuser.Handler
	rankManager     *gamerank.RankManager
	rankHandler     *gamerank.Handler
	landmarkManager *fake_app.LandmarkManager
	landmarkHandler *LandmarkHandler
	houseManager    *fake_app.HouseManager
	houseHandler    *HouseHandler
}

// NewHandler 创建新的处理器。若配置中未指定日志文件名，则不创建对应 logger，不保存 prompt/response。
func NewHandler(svc *service.ProxyService, cfg *config.Config) (*Handler, error) {
	var promptLogger *logger.PromptLogger
	var responseLogger *logger.ResponseLogger

	if cfg != nil && strings.TrimSpace(cfg.Logging.PromptLogFile) != "" {
		var err error
		promptLogger, err = logger.NewPromptLogger(strings.TrimSpace(cfg.Logging.PromptLogFile))
		if err != nil {
			return nil, fmt.Errorf("创建 PromptLogger 失败: %w", err)
		}
	}
	if cfg != nil && strings.TrimSpace(cfg.Logging.ResponseLogFile) != "" {
		var err error
		responseLogger, err = logger.NewResponseLogger(strings.TrimSpace(cfg.Logging.ResponseLogFile))
		if err != nil {
			if promptLogger != nil {
				promptLogger.Close()
			}
			return nil, fmt.Errorf("创建 ResponseLogger 失败: %w", err)
		}
	}

	var skillDirs []string
	if cfg != nil {
		for _, d := range cfg.SkillDirs {
			if s := strings.TrimSpace(d); s != "" {
				skillDirs = append(skillDirs, s)
			}
		}
	}

	// 初始化用户管理器
	userManager, err := gameuser.NewUserManager("workspace")
	if err != nil {
		return nil, fmt.Errorf("初始化用户管理器失败: %w", err)
	}
	userHandler := gameuser.NewHandler(userManager)

	// 初始化排行榜管理器
	rankManager, err := gamerank.NewRankManager("rankdata")
	if err != nil {
		return nil, fmt.Errorf("初始化排行榜管理器失败: %w", err)
	}
	rankHandler := gamerank.NewHandler(rankManager)

	// 初始化地标数据管理器（可选，失败不影响其他功能）
	var landmarkManager *fake_app.LandmarkManager
	var landmarkHandler *LandmarkHandler
	landmarkManager, err = fake_app.NewLandmarkManager("fake_app/data")
	if err != nil {
		log.Printf("[警告] 初始化地标管理器失败: %v，地标查询功能不可用", err)
	} else {
		landmarkHandler = NewLandmarkHandler(landmarkManager)
		log.Printf("[LandmarkManager] 初始化完成，共 %d 个地标", len(landmarkManager.GetAll()))
	}

	// 初始化房屋管理器（可选，失败不影响其他功能）
	var houseManager *fake_app.HouseManager
	var houseHandler *HouseHandler
	houseManager, err = fake_app.NewHouseManager("fake_app/data")
	if err != nil {
		log.Printf("[警告] 初始化房屋管理器失败: %v，房屋查询功能不可用", err)
	} else {
		houseHandler = NewHouseHandler(houseManager, landmarkManager)
		log.Printf("[HouseManager] 初始化完成，共 %d 套房源", len(houseManager.GetAll("")))
	}

	return &Handler{
		service:         svc,
		promptLogger:    promptLogger,
		responseLogger:  responseLogger,
		skillDirs:       skillDirs,
		userManager:     userManager,
		userHandler:     userHandler,
		rankManager:     rankManager,
		rankHandler:     rankHandler,
		landmarkManager: landmarkManager,
		landmarkHandler: landmarkHandler,
		houseManager:    houseManager,
		houseHandler:    houseHandler,
	}, nil
}

// ChatCompletion 处理 OpenAI 标准的聊天完成请求
func (h *Handler) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 解析 OpenAI 标准请求体
	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[错误] 解析请求体失败: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("Invalid request: %v", err),
				"type":    "invalid_request_error",
				"code":    "invalid_request",
			},
		})
		return
	}

	// 在 system 消息之后注入各 skill_dirs 下 SKILL.md 内容（每条一条 user 消息）
	if len(h.skillDirs) > 0 {
		injected, injectErr := skill.InjectAfterSystem(req.Messages, h.skillDirs)
		if injectErr != nil {
			log.Printf("[警告] skill 注入失败: %v", injectErr)
		} else {
			req.Messages = injected
		}
	}

	// 保存请求到 prompt.jsonl
	if h.promptLogger != nil && len(req.Messages) > 0 {
		if err := h.promptLogger.Log(req.Messages); err != nil {
			log.Printf("[警告] 保存请求日志失败: %v", err)
		}
	}

	// 根据请求的 model 字段判断使用哪个模型
	useWorkModel := h.service.DetermineModelType(req.Model)

	// 直接处理请求
	result, err := h.service.ProcessRequest(ctx, req, useWorkModel)
	if err != nil {
		log.Printf("[错误] 处理请求失败 (模型=%s, 流式=%v): %v", req.Model, req.Stream, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "server_error",
				"code":    "internal_error",
			},
		})
		return
	}

	// 处理流式响应
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Streaming not supported",
					"type":    "server_error",
					"code":    "streaming_not_supported",
				},
			})
			return
		}

		// 立即发送响应头，确保连接已建立
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		if streamResp, ok := result.(*service.StreamResponse); ok {
			// StreamResponse：带 API 格式信息的流式响应
			if streamResp.APIFormat == "anthropic" {
				// 转换 Anthropic 流到 OpenAI 流
				h.proxyAnthropicStreamToOpenAI(ctx, w, streamResp.Response, flusher)
			} else {
				// 直接转发 Body（OpenAI 格式）
				defer streamResp.Response.Body.Close()
				reader := bufio.NewReader(streamResp.Response.Body)
				for {
					if ctx.Err() != nil {
						return
					}
					line, err := reader.ReadBytes('\n')
					if err == io.EOF {
						break
					}
					if err != nil {
						log.Printf("[错误] 读取流式响应: %v", err)
						return
					}
					if len(line) > 0 {
						if _, writeErr := w.Write(line); writeErr != nil {
							return
						}
					} else {
						w.Write([]byte("\n"))
					}
					flusher.Flush()
				}
			}
		} else if streamResp, ok := result.(*http.Response); ok {
			// 直接转发 *http.Response（兼容简化预处理返回的流）
			defer streamResp.Body.Close()
			reader := bufio.NewReader(streamResp.Body)
			for {
				if ctx.Err() != nil {
					return
				}
				line, err := reader.ReadBytes('\n')
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Printf("[错误] 读取流式响应: %v", err)
					return
				}
				if len(line) > 0 {
					if _, writeErr := w.Write(line); writeErr != nil {
						return
					}
				} else {
					w.Write([]byte("\n"))
				}
				flusher.Flush()
			}
		}
	} else {
		// 非流式响应
		w.Header().Set("Content-Type", "application/json")
		if resp, ok := result.(*openai.ChatCompletionResponse); ok {
			json.NewEncoder(w).Encode(resp)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid response type",
					"type":    "server_error",
					"code":    "invalid_response",
				},
			})
		}
	}
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// SetupRoutes 设置路由
func (h *Handler) SetupRoutes(r *mux.Router) {
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
	r.HandleFunc("/v1/chat/completions", h.ChatCompletion).Methods("POST")
	// Anthropic 协议支持
	r.HandleFunc("/v1/messages", h.AnthropicMessages).Methods("POST")

	// 用户管理路由
	if h.userHandler != nil {
		h.userHandler.SetupRoutes(r)
	}

	// 排行榜路由
	if h.rankHandler != nil {
		h.rankHandler.SetupRoutes(r)
	}

	// 地标数据路由
	if h.landmarkHandler != nil {
		h.landmarkHandler.SetupLandmarkRoutes(r)
	}

	// 房屋数据路由
	if h.houseHandler != nil {
		h.houseHandler.SetupHouseRoutes(r)
	}
}

// Close 关闭处理器，释放资源
func (h *Handler) Close() error {
	if h.promptLogger != nil {
		_ = h.promptLogger.Close()
	}
	if h.responseLogger != nil {
		return h.responseLogger.Close()
	}
	return nil
}

// GetRankManager 获取排行榜管理器（供内部业务逻辑使用）
func (h *Handler) GetRankManager() *gamerank.RankManager {
	return h.rankManager
}

// GetUserManager 获取用户管理器（供内部业务逻辑使用）
func (h *Handler) GetUserManager() *gameuser.UserManager {
	return h.userManager
}

// GetLandmarkManager 获取地标管理器（供内部业务逻辑使用）
func (h *Handler) GetLandmarkManager() *fake_app.LandmarkManager {
	return h.landmarkManager
}

// GetHouseManager 获取房屋管理器（供内部业务逻辑使用）
func (h *Handler) GetHouseManager() *fake_app.HouseManager {
	return h.houseManager
}
