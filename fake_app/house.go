// Package fake_app 提供租房信息查询功能
package fake_app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// HouseStatus 房屋状态
type HouseStatus string

const (
	HouseStatusAvailable HouseStatus = "available" // 可租
	HouseStatusRented    HouseStatus = "rented"    // 已租
	HouseStatusOffline   HouseStatus = "offline"   // 下架
)

// House 房屋结构体
type House struct {
	HouseID          string   `json:"house_id"`
	Community        string   `json:"community"`
	District         string   `json:"district"`
	Area             string   `json:"area"`
	Address          string   `json:"address"`
	Bedrooms         int      `json:"bedrooms"`
	Livingrooms      int      `json:"livingrooms"`
	Bathrooms        int      `json:"bathrooms"`
	AreaSqm          float64  `json:"area_sqm"`
	Floor            string   `json:"floor"`
	TotalFloors      int      `json:"total_floors"`
	Orientation      string   `json:"orientation"`
	Decoration       string   `json:"decoration"`
	Price            int      `json:"price"`
	PriceUnit        string   `json:"price_unit"`
	RentalType       string   `json:"rental_type"`
	PropertyType     string   `json:"property_type"`
	UtilitiesType    string   `json:"utilities_type"`
	Elevator         bool     `json:"elevator"`
	Subway           string   `json:"subway"`
	SubwayDistance   int      `json:"subway_distance"`
	SubwayStation    string   `json:"subway_station"`
	AvailableFrom    string   `json:"available_from"`
	ListingPlatform  string   `json:"listing_platform"`
	ListingURL       string   `json:"listing_url"`
	Tags             []string `json:"tags"`
	HiddenNoiseLevel string   `json:"hidden_noise_level"`
	Status           string   `json:"status"`
	Longitude        float64  `json:"longitude"`
	Latitude         float64  `json:"latitude"`
	CoordinateSystem string   `json:"coordinate_system"`
}

// HouseWithDistance 带距离信息的房屋
type HouseWithDistance struct {
	House
	DistanceToLandmark float64 `json:"distance_to_landmark"` // 直线距离（米）
	WalkingDistance    float64 `json:"walking_distance"`     // 估算步行距离（米）
	WalkingDuration    int     `json:"walking_duration"`     // 估算步行时间（分钟）
}

// HouseQuery 房屋查询条件
type HouseQuery struct {
	// 基础筛选
	Districts  []string // 行政区列表
	Areas      []string // 商圈列表
	MinPrice   int      // 最低价格
	MaxPrice   int      // 最高价格
	Bedrooms   []int    // 卧室数列表
	RentalType string   // 租赁类型

	// 房屋属性
	Decoration   string // 装修
	Elevator     *bool  // 是否有电梯
	Orientation  string // 朝向
	MinArea      int    // 最小面积
	MaxArea      int    // 最大面积
	PropertyType string // 物业类型

	// 地铁相关
	SubwayLine    string // 地铁线路
	MaxSubwayDist int    // 最大地铁距离
	SubwayStation string // 指定地铁站

	// 地标距离
	NearLandmarkID string  // 附近地标ID
	MaxDistance    float64 // 最大距离

	// 排序
	SortBy    string // 排序字段
	SortOrder string // 排序顺序

	// 分页
	Page     int // 页码（从1开始）
	PageSize int // 每页数量
}

// HouseStatistics 房屋统计信息
type HouseStatistics struct {
	Total      int            `json:"total"`
	ByStatus   map[string]int `json:"by_status"`
	ByDistrict map[string]int `json:"by_district"`
	ByBedrooms map[string]int `json:"by_bedrooms"`
	PriceRange PriceRange     `json:"price_range"`
}

// PriceRange 价格范围
type PriceRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
	Avg int `json:"avg"`
}

// HouseManager 房屋管理器
type HouseManager struct {
	dataDir string
	houses  map[string]*House
	mu      sync.RWMutex
}

// NewHouseManager 创建房屋管理器
func NewHouseManager(dataDir string) (*HouseManager, error) {
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("数据目录不存在: %s", dataDir)
	}

	hm := &HouseManager{
		dataDir: dataDir,
		houses:  make(map[string]*House),
	}

	if err := hm.loadHouses(); err != nil {
		return nil, fmt.Errorf("加载房屋数据失败: %w", err)
	}

	log.Printf("[HouseManager] 初始化完成，已加载 %d 套房源", len(hm.houses))
	return hm, nil
}

// loadHouses 加载房屋数据
func (hm *HouseManager) loadHouses() error {
	dataFile := filepath.Join(hm.dataDir, "database.json")
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return fmt.Errorf("读取房屋数据文件失败: %w", err)
	}

	var result struct {
		Houses []*House `json:"houses"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("解析房屋数据失败: %w", err)
	}

	for _, house := range result.Houses {
		if house.HouseID != "" {
			hm.houses[house.HouseID] = house
		}
	}

	return nil
}

// GetAll 获取所有房屋
func (hm *HouseManager) GetAll() []*House {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make([]*House, 0, len(hm.houses))
	for _, house := range hm.houses {
		copy := *house
		result = append(result, &copy)
	}
	return result
}

// GetByID 根据ID获取房屋
func (hm *HouseManager) GetByID(id string) *House {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if house, exists := hm.houses[id]; exists {
		copy := *house
		return &copy
	}
	return nil
}

// Query 根据条件查询房屋
func (hm *HouseManager) Query(query *HouseQuery) []*House {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	var results []*House
	for _, house := range hm.houses {
		if hm.matchQuery(house, query) {
			copy := *house
			results = append(results, &copy)
		}
	}

	// 排序
	hm.sortResults(results, query.SortBy, query.SortOrder)

	return results
}

// QueryWithPagination 分页查询
func (hm *HouseManager) QueryWithPagination(query *HouseQuery) ([]*House, int) {
	results := hm.Query(query)

	// 设置默认分页
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	total := len(results)
	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= total {
		return []*House{}, total
	}
	if end > total {
		end = total
	}

	return results[start:end], total
}

// matchQuery 匹配查询条件
func (hm *HouseManager) matchQuery(house *House, query *HouseQuery) bool {
	// 状态筛选（默认只显示可租房源）
	if house.Status != string(HouseStatusAvailable) {
		return false
	}

	// 价格范围
	if query.MinPrice > 0 && house.Price < query.MinPrice {
		return false
	}
	if query.MaxPrice > 0 && house.Price > query.MaxPrice {
		return false
	}

	// 行政区
	if len(query.Districts) > 0 && !containsString(query.Districts, house.District) {
		return false
	}

	// 商圈
	if len(query.Areas) > 0 && !containsString(query.Areas, house.Area) {
		return false
	}

	// 卧室数
	if len(query.Bedrooms) > 0 && !containsInt(query.Bedrooms, house.Bedrooms) {
		return false
	}

	// 租赁类型
	if query.RentalType != "" && house.RentalType != query.RentalType {
		return false
	}

	// 装修
	if query.Decoration != "" && house.Decoration != query.Decoration {
		return false
	}

	// 电梯
	if query.Elevator != nil && house.Elevator != *query.Elevator {
		return false
	}

	// 朝向
	if query.Orientation != "" && house.Orientation != query.Orientation {
		return false
	}

	// 面积范围
	if query.MinArea > 0 && int(house.AreaSqm) < query.MinArea {
		return false
	}
	if query.MaxArea > 0 && int(house.AreaSqm) > query.MaxArea {
		return false
	}

	// 物业类型
	if query.PropertyType != "" && house.PropertyType != query.PropertyType {
		return false
	}

	// 地铁距离
	if query.MaxSubwayDist > 0 && house.SubwayDistance > query.MaxSubwayDist {
		return false
	}

	// 地铁线路
	if query.SubwayLine != "" && !strings.Contains(house.Subway, query.SubwayLine) {
		return false
	}

	// 地铁站
	if query.SubwayStation != "" && house.SubwayStation != query.SubwayStation {
		return false
	}

	return true
}

// sortResults 排序结果
func (hm *HouseManager) sortResults(results []*House, sortBy, sortOrder string) {
	if sortBy == "" {
		return
	}

	asc := sortOrder != "desc"

	switch sortBy {
	case "price":
		sort.Slice(results, func(i, j int) bool {
			if asc {
				return results[i].Price < results[j].Price
			}
			return results[i].Price > results[j].Price
		})
	case "area":
		sort.Slice(results, func(i, j int) bool {
			if asc {
				return results[i].AreaSqm < results[j].AreaSqm
			}
			return results[i].AreaSqm > results[j].AreaSqm
		})
	case "subway":
		sort.Slice(results, func(i, j int) bool {
			if asc {
				return results[i].SubwayDistance < results[j].SubwayDistance
			}
			return results[i].SubwayDistance > results[j].SubwayDistance
		})
	}
}

// GetStatistics 获取统计信息
func (hm *HouseManager) GetStatistics() *HouseStatistics {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	stats := &HouseStatistics{
		Total:      len(hm.houses),
		ByStatus:   make(map[string]int),
		ByDistrict: make(map[string]int),
		ByBedrooms: make(map[string]int),
	}

	var totalPrice int
	minPrice := int(^uint(0) >> 1) // MaxInt
	maxPrice := 0

	for _, house := range hm.houses {
		// 按状态统计
		stats.ByStatus[house.Status]++

		// 按行政区统计
		stats.ByDistrict[house.District]++

		// 按卧室数统计
		key := fmt.Sprintf("%d", house.Bedrooms)
		stats.ByBedrooms[key]++

		// 价格统计
		totalPrice += house.Price
		if house.Price < minPrice {
			minPrice = house.Price
		}
		if house.Price > maxPrice {
			maxPrice = house.Price
		}
	}

	if len(hm.houses) > 0 {
		stats.PriceRange = PriceRange{
			Min: minPrice,
			Max: maxPrice,
			Avg: totalPrice / len(hm.houses),
		}
	}

	return stats
}

// UpdateStatus 更新房屋状态
func (hm *HouseManager) UpdateStatus(houseID string, status HouseStatus) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	house, exists := hm.houses[houseID]
	if !exists {
		return fmt.Errorf("房屋不存在: %s", houseID)
	}

	// 验证状态值
	if status != HouseStatusAvailable && status != HouseStatusRented && status != HouseStatusOffline {
		return fmt.Errorf("无效的状态值: %s", status)
	}

	house.Status = string(status)
	return nil
}

// FindNearby 查询附近房屋
func (hm *HouseManager) FindNearby(landmark *Landmark, maxDistance float64) []*HouseWithDistance {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	var results []*HouseWithDistance
	for _, house := range hm.houses {
		// 只返回可租房源
		if house.Status != string(HouseStatusAvailable) {
			continue
		}

		distance := calcDistance(house.Latitude, house.Longitude, landmark.Latitude, landmark.Longitude)
		if distance <= maxDistance {
			walkingDist := estimateWalkingDistance(distance)
			results = append(results, &HouseWithDistance{
				House:              *house,
				DistanceToLandmark: distance,
				WalkingDistance:    walkingDist,
				WalkingDuration:    estimateWalkingDuration(walkingDist),
			})
		}
	}

	// 按距离排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].DistanceToLandmark < results[j].DistanceToLandmark
	})

	return results
}

// 辅助函数
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsInt(slice []int, item int) bool {
	for _, i := range slice {
		if i == item {
			return true
		}
	}
	return false
}
