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

// SetupHouseRoutes 设置房屋管理路由
func (h *HouseHandler) SetupHouseRoutes(r *mux.Router) {
	// 查询房屋列表
	r.HandleFunc("/api/houses", h.GetHouses).Methods("GET")
	// 查询附近房屋（必须在 {id} 之前注册，避免被匹配为ID）
	r.HandleFunc("/api/houses/nearby", h.GetNearbyHouses).Methods("GET")
	// 获取统计信息（必须在 {id} 之前注册，避免被匹配为ID）
	r.HandleFunc("/api/houses/stats", h.GetHouseStatistics).Methods("GET")
	// 根据ID获取详情（放在最后，避免捕获其他路径）
	r.HandleFunc("/api/houses/{id}", h.GetHouseByID).Methods("GET")
}

// GetHouses 查询房屋列表
// 支持多种筛选条件
func (h *HouseHandler) GetHouses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := parseHouseQuery(r)

	// 执行查询
	houses, total := h.houseManager.QueryWithPagination(query)

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

// GetHouseByID 根据ID获取房屋详情
func (h *HouseHandler) GetHouseByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]

	house := h.houseManager.GetByID(id)
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

// GetNearbyHouses 查询地标附近房屋
func (h *HouseHandler) GetNearbyHouses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

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

	// 查询附近房屋
	houses := h.houseManager.FindNearby(landmark, maxDistance)

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

// GetHouseStatistics 获取房屋统计信息
func (h *HouseHandler) GetHouseStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := h.houseManager.GetStatistics()

	json.NewEncoder(w).Encode(HouseHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    stats,
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
