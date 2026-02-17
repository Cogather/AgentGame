// Package fake_app 提供租房信息查询功能
package fake_app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	CommuteToXierqi  int      `json:"commute_to_xierqi"` // 到西二旗通勤时间（分钟）
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

	// 水电与通勤
	UtilitiesType      string // 水电类型，如 民水民电
	AvailableFromBefore string // 可入住日期上限，格式 2006-01-02
	CommuteToXierqiMax  int    // 到西二旗通勤时间上限（分钟）

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
// 基础数据来自 database_*.json；userStatusOverrides 按用户隔离「租赁/下架」等状态，互不影响。
type HouseManager struct {
	dataDir             string
	houses              map[string]*House
	mu                  sync.RWMutex
	userStatusOverrides map[string]map[string]string // userID -> houseID -> status
	overridesMu         sync.RWMutex
}

// NewHouseManager 创建房屋管理器
func NewHouseManager(dataDir string) (*HouseManager, error) {
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("数据目录不存在: %s", dataDir)
	}

	hm := &HouseManager{
		dataDir:             dataDir,
		houses:              make(map[string]*House),
		userStatusOverrides: make(map[string]map[string]string),
	}

	if err := hm.loadHouses(); err != nil {
		return nil, fmt.Errorf("加载房屋数据失败: %w", err)
	}

	log.Printf("[HouseManager] 初始化完成，已加载 %d 套房源", len(hm.houses))
	return hm, nil
}

// ResetUser 清空指定用户的状态覆盖（租赁/退租等），使该用户视角下的房源恢复为初始状态。评测或比赛每启动新题目时对该用户调用。
func (hm *HouseManager) ResetUser(userID string) {
	if userID == "" {
		return
	}
	hm.overridesMu.Lock()
	defer hm.overridesMu.Unlock()
	delete(hm.userStatusOverrides, userID)
	log.Printf("[HouseManager] 已重置用户 %s 的状态覆盖，该用户视角房源恢复为初始状态", userID)
}

// Reload 从磁盘重新加载房源数据并清空用户状态覆盖，用于完整初始化。
func (hm *HouseManager) Reload() error {
	hm.mu.Lock()
	hm.houses = make(map[string]*House)
	if err := hm.loadHouses(); err != nil {
		hm.mu.Unlock()
		return err
	}
	hm.overridesMu.Lock()
	hm.userStatusOverrides = make(map[string]map[string]string)
	hm.overridesMu.Unlock()
	hm.mu.Unlock()
	log.Printf("[HouseManager] 已从磁盘重新加载 %d 套房源并重置用户状态", len(hm.houses))
	return nil
}

// loadHouses 加载房屋数据：自动发现并合并所有 database_数字.json（如 database_2000.json、database_4000.json），
// 按数字升序加载；若存在 database.json 则最后加载（兼容旧单文件）。同 house_id 时后加载的覆盖先加载的。
func (hm *HouseManager) loadHouses() error {
	entries, err := os.ReadDir(hm.dataDir)
	if err != nil {
		return fmt.Errorf("读取数据目录失败: %w", err)
	}

	type numberedFile struct {
		name string
		num  int // 用于排序，database.json 设为 -1 放最后
	}
	var files []numberedFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "database.json" {
			files = append(files, numberedFile{name, -1})
			continue
		}
		if strings.HasPrefix(name, "database_") && strings.HasSuffix(name, ".json") {
			mid := name[len("database_") : len(name)-len(".json")]
			n, err := strconv.Atoi(mid)
			if err != nil {
				continue
			}
			files = append(files, numberedFile{name, n})
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("未找到可用的房源数据文件（database_数字.json 或 database.json）")
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].num < files[j].num
	})

	for _, f := range files {
		dataFile := filepath.Join(hm.dataDir, f.name)
		data, err := os.ReadFile(dataFile)
		if err != nil {
			log.Printf("[HouseManager] 跳过 %s: %v", f.name, err)
			continue
		}
		var result struct {
			Houses []*House `json:"houses"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			log.Printf("[HouseManager] 解析失败 %s: %v", f.name, err)
			continue
		}
		for _, house := range result.Houses {
			if house.HouseID != "" {
				hm.houses[house.HouseID] = house
			}
		}
		log.Printf("[HouseManager] 已加载 %s，本文件 %d 条，累计 %d 条", f.name, len(result.Houses), len(hm.houses))
	}
	if len(hm.houses) == 0 {
		return fmt.Errorf("未从任何房源文件中加载到有效数据")
	}
	return nil
}

// effectiveStatus 返回某用户视角下某房源的展示状态（基础状态 + 该用户的状态覆盖）
func (hm *HouseManager) effectiveStatus(houseID, baseStatus, userID string) string {
	if userID == "" {
		return baseStatus
	}
	hm.overridesMu.RLock()
	defer hm.overridesMu.RUnlock()
	if m, ok := hm.userStatusOverrides[userID]; ok {
		if s, ok := m[houseID]; ok {
			return s
		}
	}
	return baseStatus
}

// UpdateStatusForUser 仅更新某用户视角下的房源状态，不影响其他用户
func (hm *HouseManager) UpdateStatusForUser(userID, houseID string, status HouseStatus) error {
	if userID == "" {
		return fmt.Errorf("需要提供 userID 以更新状态")
	}
	if status != HouseStatusAvailable && status != HouseStatusRented && status != HouseStatusOffline {
		return fmt.Errorf("无效的状态值: %s", status)
	}
	hm.mu.RLock()
	_, exists := hm.houses[houseID]
	hm.mu.RUnlock()
	if !exists {
		return fmt.Errorf("房屋不存在: %s", houseID)
	}
	hm.overridesMu.Lock()
	defer hm.overridesMu.Unlock()
	if hm.userStatusOverrides[userID] == nil {
		hm.userStatusOverrides[userID] = make(map[string]string)
	}
	hm.userStatusOverrides[userID][houseID] = string(status)
	return nil
}

// GetAll 获取所有房屋；userID 非空时返回该用户视角下的有效状态
func (hm *HouseManager) GetAll(userID string) []*House {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make([]*House, 0, len(hm.houses))
	for _, house := range hm.houses {
		copy := *house
		copy.Status = hm.effectiveStatus(house.HouseID, house.Status, userID)
		result = append(result, &copy)
	}
	return result
}

// GetByID 根据ID获取房屋；userID 非空时返回该用户视角下的有效状态
func (hm *HouseManager) GetByID(id, userID string) *House {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if house, exists := hm.houses[id]; exists {
		copy := *house
		copy.Status = hm.effectiveStatus(house.HouseID, house.Status, userID)
		return &copy
	}
	return nil
}

// Query 根据条件查询房屋；userID 非空时按该用户视角下的有效状态筛选与展示
func (hm *HouseManager) Query(query *HouseQuery, userID string) []*House {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	var results []*House
	for _, house := range hm.houses {
		effStatus := hm.effectiveStatus(house.HouseID, house.Status, userID)
		if hm.matchQuery(house, query, effStatus) {
			copy := *house
			copy.Status = effStatus
			results = append(results, &copy)
		}
	}

	// 排序
	hm.sortResults(results, query.SortBy, query.SortOrder)

	return results
}

// QueryWithPagination 分页查询；userID 非空时按该用户视角下的有效状态
func (hm *HouseManager) QueryWithPagination(query *HouseQuery, userID string) ([]*House, int) {
	results := hm.Query(query, userID)

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

// matchQuery 匹配查询条件；effectiveStatus 为该用户视角下的状态，非空时用于「可租」筛选
func (hm *HouseManager) matchQuery(house *House, query *HouseQuery, effectiveStatus string) bool {
	statusCheck := house.Status
	if effectiveStatus != "" {
		statusCheck = effectiveStatus
	}
	// 状态筛选（默认只显示可租房源）
	if statusCheck != string(HouseStatusAvailable) {
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

	// 水电类型
	if query.UtilitiesType != "" && house.UtilitiesType != query.UtilitiesType {
		return false
	}

	// 可入住日期上限（house.AvailableFrom <= query.AvailableFromBefore，字符串比较同格式 YYYY-MM-DD）
	if query.AvailableFromBefore != "" && house.AvailableFrom != "" && house.AvailableFrom > query.AvailableFromBefore {
		return false
	}

	// 到西二旗通勤时间上限（0 表示未填，视为通过）
	if query.CommuteToXierqiMax > 0 && house.CommuteToXierqi > query.CommuteToXierqiMax {
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

// GetStatistics 获取统计信息；userID 非空时按该用户视角下的有效状态统计 ByStatus
func (hm *HouseManager) GetStatistics(userID string) *HouseStatistics {
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
		effStatus := hm.effectiveStatus(house.HouseID, house.Status, userID)
		stats.ByStatus[effStatus]++

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

// FindNearby 查询附近房屋；userID 非空时按该用户视角下的有效状态筛选可租
func (hm *HouseManager) FindNearby(landmark *Landmark, maxDistance float64, userID string) []*HouseWithDistance {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	var results []*HouseWithDistance
	for _, house := range hm.houses {
		effStatus := hm.effectiveStatus(house.HouseID, house.Status, userID)
		if effStatus != string(HouseStatusAvailable) {
			continue
		}

		distance := calcDistance(house.Latitude, house.Longitude, landmark.Latitude, landmark.Longitude)
		if distance <= maxDistance {
			h := *house
			h.Status = effStatus
			walkingDist := estimateWalkingDistance(distance)
			results = append(results, &HouseWithDistance{
				House:              h,
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

// GetByCommunity 按小区名查询可租房源；userID 非空时按该用户视角下的有效状态
func (hm *HouseManager) GetByCommunity(community, userID string) []*House {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if community == "" {
		return nil
	}

	var results []*House
	for _, house := range hm.houses {
		effStatus := hm.effectiveStatus(house.HouseID, house.Status, userID)
		if effStatus != string(HouseStatusAvailable) {
			continue
		}
		if house.Community == community {
			copy := *house
			copy.Status = effStatus
			results = append(results, &copy)
		}
	}
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
