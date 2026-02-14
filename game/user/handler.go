// Package user 提供用户管理的 HTTP 接口
package user

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// Handler 用户管理的 HTTP 处理器
type Handler struct {
	manager *UserManager
}

// NewHandler 创建新的用户管理 HTTP 处理器
func NewHandler(manager *UserManager) *Handler {
	return &Handler{manager: manager}
}

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// AddUserRequest 添加用户请求
type AddUserRequest struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	TeamName  string `json:"team_name"`
	AgentIP   string `json:"agent_ip"`
	AgentPort int    `json:"agent_port"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Username  string `json:"username,omitempty"`
	TeamName  string `json:"team_name,omitempty"`
	AgentIP   string `json:"agent_ip,omitempty"`
	AgentPort int    `json:"agent_port,omitempty"`
}

// SetupRoutes 设置用户管理路由
func (h *Handler) SetupRoutes(r *mux.Router) {
	r.HandleFunc("/api/users", h.AddUser).Methods("POST")
	r.HandleFunc("/api/users", h.GetAllUsers).Methods("GET")
	r.HandleFunc("/api/users/{user_id}", h.GetUser).Methods("GET")
	r.HandleFunc("/api/users/{user_id}", h.UpdateUser).Methods("PUT")
	r.HandleFunc("/api/users/{user_id}", h.DeleteUser).Methods("DELETE")
}

// AddUser 添加用户
func (h *Handler) AddUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req AddUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Code:    400,
			Message: "请求参数格式错误: " + err.Error(),
		})
		return
	}

	// 验证必填字段
	if req.UserID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Code:    400,
			Message: "用户工号不能为空",
		})
		return
	}
	if req.Username == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Code:    400,
			Message: "用户名不能为空",
		})
		return
	}
	if req.AgentIP == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Code:    400,
			Message: "Agent IP 不能为空",
		})
		return
	}
	if req.AgentPort <= 0 || req.AgentPort > 65535 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Code:    400,
			Message: "Agent 端口无效",
		})
		return
	}

	user := &User{
		UserID:    req.UserID,
		Username:  req.Username,
		TeamName:  req.TeamName,
		AgentIP:   req.AgentIP,
		AgentPort: req.AgentPort,
	}

	if err := h.manager.AddUser(user); err != nil {
		// 检查是否是重复用户
		if h.manager.UserExists(req.UserID) {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(Response{
				Code:    409,
				Message: "用户工号已存在: " + req.UserID,
			})
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Code:    500,
			Message: "添加用户失败: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "用户添加成功",
		Data:    user,
	})
}

// GetUser 获取单个用户信息
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	userID := vars["user_id"]

	user, err := h.manager.GetUser(userID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(Response{
			Code:    404,
			Message: "用户不存在: " + userID,
		})
		return
	}

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    user,
	})
}

// GetAllUsers 获取所有用户
func (h *Handler) GetAllUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	users := h.manager.GetAllUsers()

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    users,
	})
}

// UpdateUser 更新用户信息
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	userID := vars["user_id"]

	// 检查用户是否存在
	if !h.manager.UserExists(userID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(Response{
			Code:    404,
			Message: "用户不存在: " + userID,
		})
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Code:    400,
			Message: "请求参数格式错误: " + err.Error(),
		})
		return
	}

	// 验证端口范围
	if req.AgentPort < 0 || req.AgentPort > 65535 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Code:    400,
			Message: "Agent 端口无效",
		})
		return
	}

	updates := &User{
		Username:  req.Username,
		TeamName:  req.TeamName,
		AgentIP:   req.AgentIP,
		AgentPort: req.AgentPort,
	}

	if err := h.manager.UpdateUser(userID, updates); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Code:    500,
			Message: "更新用户失败: " + err.Error(),
		})
		return
	}

	// 获取更新后的用户信息
	user, _ := h.manager.GetUser(userID)

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "用户更新成功",
		Data:    user,
	})
}

// DeleteUser 删除用户
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	userID := vars["user_id"]

	// 检查用户是否存在
	if !h.manager.UserExists(userID) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(Response{
			Code:    404,
			Message: "用户不存在: " + userID,
		})
		return
	}

	if err := h.manager.DeleteUser(userID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Code:    500,
			Message: "删除用户失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "用户删除成功",
	})
}
