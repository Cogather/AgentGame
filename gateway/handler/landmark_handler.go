package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"ocProxy/fake_app"

	"github.com/gorilla/mux"
)

// LandmarkHandler 地标管理的 HTTP 处理器
type LandmarkHandler struct {
	manager *fake_app.LandmarkManager
}

// NewLandmarkHandler 创建新的地标管理 HTTP 处理器
func NewLandmarkHandler(manager *fake_app.LandmarkManager) *LandmarkHandler {
	return &LandmarkHandler{manager: manager}
}

// LandmarkResponse 地标响应结构
type LandmarkResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Category  string                 `json:"category"`
	District  string                 `json:"district"`
	Longitude float64                `json:"longitude"`
	Latitude  float64                `json:"latitude"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// LandmarkListResponse 地标列表响应
type LandmarkListResponse struct {
	Total int                 `json:"total"`
	Items []*LandmarkResponse `json:"items"`
}

// LandmarkHTTPResponse 统一HTTP响应结构
type LandmarkHTTPResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// SetupLandmarkRoutes 设置地标管理路由
func (h *LandmarkHandler) SetupLandmarkRoutes(r *mux.Router) {
	// 获取全部地标（支持类别筛选）
	r.HandleFunc("/api/landmarks", h.GetAllLandmarks).Methods("GET")
	// 根据名称精确查询
	r.HandleFunc("/api/landmarks/name/{name}", h.GetByName).Methods("GET")
	// 关键词搜索（模糊匹配）
	r.HandleFunc("/api/landmarks/search", h.SearchByKeyword).Methods("GET")
	// 根据ID获取详情
	r.HandleFunc("/api/landmarks/{id}", h.GetByID).Methods("GET")
	// 获取统计信息
	r.HandleFunc("/api/landmarks/stats", h.GetStatistics).Methods("GET")
}

// convertToLandmarkResponse 将 Landmark 转换为响应结构
func convertToLandmarkResponse(lm *fake_app.Landmark) *LandmarkResponse {
	if lm == nil {
		return nil
	}
	return &LandmarkResponse{
		ID:        lm.ID,
		Name:      lm.Name,
		Category:  string(lm.Category),
		District:  lm.District,
		Longitude: lm.Longitude,
		Latitude:  lm.Latitude,
		Details:   lm.RawData,
	}
}

// GetAllLandmarks 获取全部地标信息
// 支持查询参数：category=subway|company|landmark，district=行政区名
func (h *LandmarkHandler) GetAllLandmarks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query()
	category := query.Get("category")
	district := query.Get("district")

	var landmarks []*fake_app.Landmark

	// 根据筛选条件查询
	if category != "" {
		// 按类别筛选
		cat := fake_app.LandmarkCategory(category)
		landmarks = h.manager.GetByCategory(cat)
	} else if district != "" {
		// 按行政区筛选
		landmarks = h.manager.GetByDistrict(district)
	} else {
		// 获取全部
		landmarks = h.manager.GetAll()
	}

	// 转换为响应结构
	items := make([]*LandmarkResponse, 0, len(landmarks))
	for _, lm := range landmarks {
		items = append(items, convertToLandmarkResponse(lm))
	}

	json.NewEncoder(w).Encode(LandmarkHTTPResponse{
		Code:    0,
		Message: "success",
		Data: LandmarkListResponse{
			Total: len(items),
			Items: items,
		},
	})
}

// GetByName 根据名称精确查询地标
// URL 格式: /api/landmarks/name/{name}
// 示例: /api/landmarks/name/西二旗站
func (h *LandmarkHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	name := vars["name"]

	// URL解码
	name, _ = urlDecodeLandmark(name)

	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LandmarkHTTPResponse{
			Code:    400,
			Message: "地标名称不能为空",
		})
		return
	}

	landmark := h.manager.GetByName(name)
	if landmark == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(LandmarkHTTPResponse{
			Code:    404,
			Message: "未找到地标: " + name,
		})
		return
	}

	json.NewEncoder(w).Encode(LandmarkHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    convertToLandmarkResponse(landmark),
	})
}

// SearchByKeyword 根据关键词搜索地标（模糊匹配）
// 支持查询参数: q=关键词, category=类别（可选）
// 示例: /api/landmarks/search?q=百度&category=company
func (h *LandmarkHandler) SearchByKeyword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query()
	keyword := query.Get("q")
	category := query.Get("category")

	if keyword == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LandmarkHTTPResponse{
			Code:    400,
			Message: "搜索关键词不能为空（使用 q 参数）",
		})
		return
	}

	// 执行搜索
	results := h.manager.SearchByKeyword(keyword)

	// 如果指定了类别，进行过滤
	if category != "" {
		filtered := make([]*fake_app.Landmark, 0)
		for _, lm := range results {
			if string(lm.Category) == category {
				filtered = append(filtered, lm)
			}
		}
		results = filtered
	}

	// 转换为响应结构
	items := make([]*LandmarkResponse, 0, len(results))
	for _, lm := range results {
		items = append(items, convertToLandmarkResponse(lm))
	}

	json.NewEncoder(w).Encode(LandmarkHTTPResponse{
		Code:    0,
		Message: "success",
		Data: LandmarkListResponse{
			Total: len(items),
			Items: items,
		},
	})
}

// GetByID 根据ID获取地标详情
// URL 格式: /api/landmarks/{id}
// 示例: /api/landmarks/SS_001
func (h *LandmarkHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LandmarkHTTPResponse{
			Code:    400,
			Message: "地标ID不能为空",
		})
		return
	}

	landmark := h.manager.GetByID(id)
	if landmark == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(LandmarkHTTPResponse{
			Code:    404,
			Message: "未找到地标ID: " + id,
		})
		return
	}

	json.NewEncoder(w).Encode(LandmarkHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    convertToLandmarkResponse(landmark),
	})
}

// GetStatistics 获取地标数据统计信息
func (h *LandmarkHandler) GetStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := h.manager.GetStatistics()

	json.NewEncoder(w).Encode(LandmarkHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    stats,
	})
}

// urlDecodeLandmark 简单的URL解码（处理常见编码字符）
func urlDecodeLandmark(s string) (string, error) {
	// 处理常见的URL编码
	s = strings.ReplaceAll(s, "%20", " ")
	s = strings.ReplaceAll(s, "%2F", "/")
	s = strings.ReplaceAll(s, "%3A", ":")
	s = strings.ReplaceAll(s, "%3F", "?")
	s = strings.ReplaceAll(s, "%26", "&")
	s = strings.ReplaceAll(s, "%3D", "=")
	s = strings.ReplaceAll(s, "%2B", "+")
	return s, nil
}
