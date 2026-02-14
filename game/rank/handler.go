// Package rank 提供排行榜的HTTP查询接口
package rank

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// Handler 排行榜HTTP处理器
type Handler struct {
	manager *RankManager
}

// NewHandler 创建新的排行榜HTTP处理器
func NewHandler(manager *RankManager) *Handler {
	return &Handler{manager: manager}
}

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// RankItemResponse 排行项响应（格式化时间）
type RankItemResponse struct {
	Rank           int    `json:"rank"`
	TeamName       string `json:"team_name"`
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
	Score          int    `json:"score"`
	CompletedTasks int    `json:"completed_tasks"`
	UpdateTime     string `json:"update_time"`
}

// SetupRoutes 设置排行榜路由
func (h *Handler) SetupRoutes(r *mux.Router) {
	// 对外查询接口
	r.HandleFunc("/api/rank", h.GetRankList).Methods("GET")
	r.HandleFunc("/api/rank/{user_id}", h.GetUserRank).Methods("GET")
}

// GetRankList 获取排行榜列表（按得分从高到低排序）
func (h *Handler) GetRankList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 获取limit参数，默认返回全部
	limitStr := r.URL.Query().Get("limit")
	limit := 0
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(Response{
				Code:    400,
				Message: "limit参数无效",
			})
			return
		}
	}

	items := h.manager.GetRankList(limit)

	// 转换为响应格式（格式化时间）
	responses := make([]*RankItemResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, &RankItemResponse{
			Rank:           item.Rank,
			TeamName:       item.TeamName,
			UserID:         item.UserID,
			Username:       item.Username,
			Score:          item.Score,
			CompletedTasks: item.CompletedTasks,
			UpdateTime:     item.UpdateTime.Format("2006-01-02 15:04:05"),
		})
	}

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    responses,
	})
}

// GetUserRank 获取单个用户排行信息
func (h *Handler) GetUserRank(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	userID := vars["user_id"]

	item, err := h.manager.GetUserRank(userID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(Response{
			Code:    404,
			Message: err.Error(),
		})
		return
	}

	response := &RankItemResponse{
		Rank:           item.Rank,
		TeamName:       item.TeamName,
		UserID:         item.UserID,
		Username:       item.Username,
		Score:          item.Score,
		CompletedTasks: item.CompletedTasks,
		UpdateTime:     item.UpdateTime.Format("2006-01-02 15:04:05"),
	}

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    response,
	})
}
