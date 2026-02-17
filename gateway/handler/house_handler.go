// Package handler 提供房屋管理的 HTTP 接口
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ocProxy/fake_app"

	"github.com/gorilla/mux"
)

// HouseHandler 房屋管理的 HTTP 处理器
type HouseHandler struct {
	houseManager    *fake_app.HouseManager
	landmarkManager *fake_app.LandmarkManager
}

// NewHouseHandler 创建新的房屋管理 HTTP 处理器
func NewHouseHandler(houseManager *fake_app.HouseManager, landmarkManager *fake_app.LandmarkManager) *HouseHandler {
	return &HouseHandler{
		houseManager:    houseManager,
		landmarkManager: landmarkManager,
	}
}

// HouseHTTPResponse 统一HTTP响应结构
type HouseHTTPResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// HouseListResponse 房屋列表响应
type HouseListResponse struct {
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Items    []*fake_app.House `json:"items"`
}

// HouseNearbyResponse 附近房屋响应
type HouseNearbyResponse struct {
	Landmark struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Longitude float64 `json:"longitude"`
		Latitude  float64 `json:"latitude"`
	} `json:"landmark"`
	Total int                           `json:"total"`
	Items []*fake_app.HouseWithDistance `json:"items"`
}

// userIDFromRequest 从请求头 X-User-ID 取当前用户，用于按用户隔离房源状态
func userIDFromRequest(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-ID"))
}

// requireUserID 校验 X-User-ID 必填；若为空则写 400 并返回 true，否则返回 false
func (h *HouseHandler) requireUserID(w http.ResponseWriter, userID string) bool {
	if userID != "" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    400,
		Message: "请提供请求头 X-User-ID 以标识当前用户",
	})
	return true
}

// SetupHouseRoutes 设置房屋管理路由
func (h *HouseHandler) SetupHouseRoutes(r *mux.Router) {
	// 初始化指定用户的房源数据：清空该用户状态覆盖，评测/比赛每启动新题目时调用（须在 {id} 前注册）
	r.HandleFunc("/api/houses/init", h.InitHouses).Methods("POST")
	// 查询房屋列表
	r.HandleFunc("/api/houses", h.GetHouses).Methods("GET")
	// 按小区名查房源（指代、地铁信息等，支撑评测集）
	r.HandleFunc("/api/houses/by_community", h.GetHousesByCommunity).Methods("GET")
	// 某小区周边某类地标（商超/公园，支撑评测集）
	r.HandleFunc("/api/houses/nearby_landmarks", h.GetNearbyLandmarks).Methods("GET")
	// 查询附近房屋（必须在 {id} 之前注册，避免被匹配为ID）
	r.HandleFunc("/api/houses/nearby", h.GetNearbyHouses).Methods("GET")
	// 获取统计信息（必须在 {id} 之前注册，避免被匹配为ID）
	r.HandleFunc("/api/houses/stats", h.GetHouseStatistics).Methods("GET")
	// 更新当前用户视角下某房源状态（租赁/下架等），仅影响该用户
	r.HandleFunc("/api/houses/{id}/status", h.UpdateHouseStatus).Methods("PUT", "PATCH")
	// 根据ID获取详情（放在最后，避免捕获其他路径）
	r.HandleFunc("/api/houses/{id}", h.GetHouseByID).Methods("GET")
}

// GetHouses 查询房屋列表
// 支持多种筛选条件；请求头 X-User-ID 必填，按该用户视角返回状态
func (h *HouseHandler) GetHouses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := userIDFromRequest(r)
	if h.requireUserID(w, userID) {
		return
	}
	query := parseHouseQuery(r)

	// 执行查询
	houses, total := h.houseManager.QueryWithPagination(query, userID)

	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data: HouseListResponse{
			Total:    total,
			Page:     query.Page,
			PageSize: query.PageSize,
			Items:    houses,
		},
	})
}

// InitHouses 初始化指定用户的房源数据：清空该用户的状态覆盖（租赁/退租等），使该用户视角恢复为初始数据。评测或比赛每启动新题目时调用。
// POST /api/houses/init，请求头必填 X-User-ID，仅重置该用户。
func (h *HouseHandler) InitHouses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := userIDFromRequest(r)
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    400,
			Message: "请提供请求头 X-User-ID 以指定要初始化的用户",
		})
		return
	}
	h.houseManager.ResetUser(userID)
	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    map[string]string{"action": "reset_user", "user_id": userID, "message": "该用户状态覆盖已清空，房源恢复为初始状态"},
	})
}

// UpdateHouseStatus 更新当前用户视角下某房源状态（如租赁后改为 rented），仅影响该用户
// PUT/PATCH /api/houses/{id}/status，请求体 JSON: {"status": "rented"|"available"|"offline"}
func (h *HouseHandler) UpdateHouseStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := userIDFromRequest(r)
	if h.requireUserID(w, userID) {
		return
	}
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    400,
			Message: "缺少房源 id",
		})
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    400,
			Message: "请求体需为 JSON，且包含 status 字段，如 {\"status\": \"rented\"}",
		})
		return
	}

	status := fake_app.HouseStatus(body.Status)
	if err := h.houseManager.UpdateStatusForUser(userID, id, status); err != nil {
		if strings.Contains(err.Error(), "不存在") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(HouseHTTPResponse{
				Code:    404,
				Message: err.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	// 返回修改后的房源详细信息，便于评测集构造与前端展示
	house := h.houseManager.GetByID(id, userID)
	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    house,
	})
}

// GetHouseByID 根据ID获取房屋详情；请求头 X-User-ID 必填，返回该用户视角下的状态
func (h *HouseHandler) GetHouseByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := userIDFromRequest(r)
	if h.requireUserID(w, userID) {
		return
	}
	vars := mux.Vars(r)
	id := vars["id"]
	house := h.houseManager.GetByID(id, userID)
	if house == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    404,
			Message: "未找到房屋: " + id,
		})
		return
	}

	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    house,
	})
}

// GetNearbyHouses 查询地标附近房屋；请求头 X-User-ID 必填
func (h *HouseHandler) GetNearbyHouses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := userIDFromRequest(r)
	if h.requireUserID(w, userID) {
		return
	}
	q := r.URL.Query()
	landmarkID := q.Get("landmark_id")
	if landmarkID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    400,
			Message: "请提供landmark_id参数",
		})
		return
	}

	// 获取地标
	landmark := h.landmarkManager.GetByID(landmarkID)
	if landmark == nil {
		// 尝试按名称查找
		landmark = h.landmarkManager.GetByName(landmarkID)
	}
	if landmark == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    404,
			Message: "未找到地标: " + landmarkID,
		})
		return
	}

	// 获取最大距离参数
	maxDistance := 2000.0 // 默认2km
	if d := q.Get("max_distance"); d != "" {
		if dist, err := strconv.ParseFloat(d, 64); err == nil && dist > 0 {
			maxDistance = dist
		}
	}

	// 查询附近房屋（按当前用户视角筛选可租）
	houses := h.houseManager.FindNearby(landmark, maxDistance, userID)

	response := HouseNearbyResponse{
		Total: len(houses),
		Items: houses,
	}
	response.Landmark.ID = landmark.ID
	response.Landmark.Name = landmark.Name
	response.Landmark.Longitude = landmark.Longitude
	response.Landmark.Latitude = landmark.Latitude

	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    response,
	})
}

// GetHouseStatistics 获取房屋统计信息；请求头 X-User-ID 必填，按该用户视角统计状态
func (h *HouseHandler) GetHouseStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := userIDFromRequest(r)
	if h.requireUserID(w, userID) {
		return
	}
	stats := h.houseManager.GetStatistics(userID)

	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    stats,
	})
}

// GetHousesByCommunity 按小区名查询可租房源（支撑评测：指代、查地铁信息等）；请求头 X-User-ID 必填
// GET /api/houses/by_community?community=建清园(南区)
func (h *HouseHandler) GetHousesByCommunity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := userIDFromRequest(r)
	if h.requireUserID(w, userID) {
		return
	}
	community := r.URL.Query().Get("community")
	if community == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    400,
			Message: "请提供 community 参数",
		})
		return
	}
	houses := h.houseManager.GetByCommunity(community, userID)

	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data: HouseListResponse{
			Total:    len(houses),
			Page:     1,
			PageSize: len(houses),
			Items:    houses,
		},
	})
}

// NearbyLandmarksResponse 某小区周边地标响应
type NearbyLandmarksResponse struct {
	Community string                           `json:"community"`
	Type      string                           `json:"type"`
	Total     int                              `json:"total"`
	Items     []*fake_app.LandmarkWithDistance `json:"items"`
}

// GetNearbyLandmarks 查询某小区周边某类地标（商超/公园等，支撑评测集）；请求头 X-User-ID 必填
// GET /api/houses/nearby_landmarks?community=保利锦上(二期)&type=shopping&max_distance_m=3000
func (h *HouseHandler) GetNearbyLandmarks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := userIDFromRequest(r)
	if h.requireUserID(w, userID) {
		return
	}
	if h.landmarkManager == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    503,
			Message: "地标服务不可用",
		})
		return
	}
	q := r.URL.Query()
	community := q.Get("community")
	if community == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    400,
			Message: "请提供 community 参数",
		})
		return
	}
	houses := h.houseManager.GetByCommunity(community, userID)
	if len(houses) == 0 {
		json.NewEncoder(w).Encode(HouseHTTPResponse{
			Code:    0,
			Message: "success",
			Data: NearbyLandmarksResponse{
				Community: community,
				Type:      q.Get("type"),
				Total:     0,
				Items:     []*fake_app.LandmarkWithDistance{},
			},
		})
		return
	}

	// 以该小区第一套房源坐标为基准点
	lat, lng := houses[0].Latitude, houses[0].Longitude
	maxDist := 3000.0
	if d := q.Get("max_distance_m"); d != "" {
		if v, err := strconv.ParseFloat(d, 64); err == nil && v > 0 {
			maxDist = v
		}
	}
	typeFilter := q.Get("type") // shopping | park

	items := h.landmarkManager.FindLandmarksNearPoint(lat, lng, maxDist, typeFilter)

	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data: NearbyLandmarksResponse{
			Community: community,
			Type:      typeFilter,
			Total:     len(items),
			Items:     items,
		},
	})
}

// parseHouseQuery 解析查询参数
func parseHouseQuery(r *http.Request) *fake_app.HouseQuery {
	q := r.URL.Query()

	query := &fake_app.HouseQuery{
		Page:     1,
		PageSize: 20,
	}

	// 行政区
	if d := q.Get("district"); d != "" {
		query.Districts = strings.Split(d, ",")
	}

	// 商圈
	if a := q.Get("area"); a != "" {
		query.Areas = strings.Split(a, ",")
	}

	// 价格范围
	if p := q.Get("min_price"); p != "" {
		query.MinPrice, _ = strconv.Atoi(p)
	}
	if p := q.Get("max_price"); p != "" {
		query.MaxPrice, _ = strconv.Atoi(p)
	}

	// 卧室数
	if b := q.Get("bedrooms"); b != "" {
		parts := strings.Split(b, ",")
		for _, part := range parts {
			if n, err := strconv.Atoi(part); err == nil {
				query.Bedrooms = append(query.Bedrooms, n)
			}
		}
	}

	// 租赁类型
	query.RentalType = q.Get("rental_type")

	// 装修
	query.Decoration = q.Get("decoration")

	// 电梯
	if e := q.Get("elevator"); e != "" {
		elevator := e == "true"
		query.Elevator = &elevator
	}

	// 朝向
	query.Orientation = q.Get("orientation")

	// 面积范围
	if a := q.Get("min_area"); a != "" {
		query.MinArea, _ = strconv.Atoi(a)
	}
	if a := q.Get("max_area"); a != "" {
		query.MaxArea, _ = strconv.Atoi(a)
	}

	// 物业类型
	query.PropertyType = q.Get("property_type")

	// 地铁相关
	query.SubwayLine = q.Get("subway_line")
	if d := q.Get("max_subway_dist"); d != "" {
		query.MaxSubwayDist, _ = strconv.Atoi(d)
	}
	query.SubwayStation = q.Get("subway_station")

	// 水电与通勤（支撑评测集）
	query.UtilitiesType = q.Get("utilities_type")
	query.AvailableFromBefore = q.Get("available_from_before")
	if c := q.Get("commute_to_xierqi_max"); c != "" {
		query.CommuteToXierqiMax, _ = strconv.Atoi(c)
	}

	// 排序
	query.SortBy = q.Get("sort_by")
	query.SortOrder = q.Get("sort_order")

	// 分页
	if p := q.Get("page"); p != "" {
		query.Page, _ = strconv.Atoi(p)
		if query.Page < 1 {
			query.Page = 1
		}
	}
	if ps := q.Get("page_size"); ps != "" {
		query.PageSize, _ = strconv.Atoi(ps)
		if query.PageSize < 1 {
			query.PageSize = 20
		}
		if query.PageSize > 100 {
			query.PageSize = 100
		}
	}

	return query
}
