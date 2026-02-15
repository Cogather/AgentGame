// Package fake_app 提供租房信息查询功能
// 本文件实现地标数据加载与查询功能
// - 支持地铁站、世界500强企业、商圈地标三类数据
// - 启动时全量加载到内存
// - 支持按名称查询、按类别查询、模糊搜索
package fake_app

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LandmarkCategory 地标类别
type LandmarkCategory string

const (
	CategorySubway   LandmarkCategory = "subway"   // 地铁站
	CategoryCompany  LandmarkCategory = "company"  // 世界500强企业
	CategoryLandmark LandmarkCategory = "landmark" // 商圈地标
)

// Landmark 地标结构体
type Landmark struct {
	ID        string                 `json:"id"`        // 地标唯一ID
	Name      string                 `json:"name"`      // 地标名称
	Category  LandmarkCategory       `json:"category"`  // 类别
	District  string                 `json:"district"`  // 所属行政区
	Longitude float64                `json:"longitude"` // 经度
	Latitude  float64                `json:"latitude"`  // 纬度
	RawData   map[string]interface{} `json:"details"`   // 原始详细数据
}

// SubwayStation 地铁站详细信息
type SubwayStation struct {
	StationID string   `json:"station_id"`
	Name      string   `json:"name"`
	Lines     []string `json:"lines"`
	District  string   `json:"district"`
	Longitude float64  `json:"longitude"`
	Latitude  float64  `json:"latitude"`
	Type      string   `json:"type"`     // normal/transfer
	Category  string   `json:"category"` // subway
}

// Fortune500Company 世界500强企业详细信息
type Fortune500Company struct {
	CompanyID    string  `json:"company_id"`
	Name         string  `json:"name"`
	NameEN       string  `json:"name_en"`
	ShortName    string  `json:"short_name"`
	Industry     string  `json:"industry"`
	Rank2024     int     `json:"rank_2024"`
	Address      string  `json:"address"`
	District     string  `json:"district"`
	Longitude    float64 `json:"longitude"`
	Latitude     float64 `json:"latitude"`
	Category     string  `json:"category"`
	NearbySubway string  `json:"nearby_subway"`
}

// BusinessLandmark 商圈地标详细信息
type BusinessLandmark struct {
	LandmarkID   string  `json:"landmark_id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`      // shopping/park/landmark/transport/culture
	TypeName     string  `json:"type_name"` // 类型中文名
	District     string  `json:"district"`
	Address      string  `json:"address"`
	Longitude    float64 `json:"longitude"`
	Latitude     float64 `json:"latitude"`
	Category     string  `json:"category"`
	NearbySubway string  `json:"nearby_subway"`
}

// LandmarkManager 地标数据管理器
type LandmarkManager struct {
	dataDir   string               // 数据目录路径
	landmarks map[string]*Landmark // 内存中的地标缓存，key为ID
	byName    map[string]string    // 名称到ID的索引，key为名称，value为ID
	mu        sync.RWMutex         // 读写锁
}

// NewLandmarkManager 创建新的地标管理器，启动时将所有地标数据加载到内存
func NewLandmarkManager(dataDir string) (*LandmarkManager, error) {
	// 检查数据目录
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("数据目录不存在: %s", dataDir)
	}

	lm := &LandmarkManager{
		dataDir:   dataDir,
		landmarks: make(map[string]*Landmark),
		byName:    make(map[string]string),
	}

	// 从磁盘加载所有地标数据到内存
	if err := lm.loadAllData(); err != nil {
		return nil, fmt.Errorf("加载地标数据失败: %w", err)
	}

	log.Printf("[LandmarkManager] 初始化完成，已加载 %d 个地标", len(lm.landmarks))
	return lm, nil
}

// loadAllData 加载所有地标数据
func (lm *LandmarkManager) loadAllData() error {
	if err := lm.loadSubwayStations(); err != nil {
		return fmt.Errorf("加载地铁站数据失败: %w", err)
	}
	if err := lm.loadCompanies(); err != nil {
		return fmt.Errorf("加载企业数据失败: %w", err)
	}
	if err := lm.loadLandmarks(); err != nil {
		return fmt.Errorf("加载地标数据失败: %w", err)
	}
	return nil
}

// loadJSON 加载JSON文件
func (lm *LandmarkManager) loadJSON(filename string) (map[string]interface{}, error) {
	filepath := filepath.Join(lm.dataDir, filename)
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	return result, nil
}

// loadSubwayStations 加载地铁站数据
func (lm *LandmarkManager) loadSubwayStations() error {
	data, err := lm.loadJSON("subway_stations.json")
	if err != nil {
		return err
	}

	stations, ok := data["stations"].([]interface{})
	if !ok {
		return fmt.Errorf("地铁站数据格式错误")
	}

	for _, s := range stations {
		stationData, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := stationData["station_id"].(string)
		name, _ := stationData["name"].(string)
		district, _ := stationData["district"].(string)
		longitude, _ := stationData["longitude"].(float64)
		latitude, _ := stationData["latitude"].(float64)

		if id == "" || name == "" {
			continue
		}

		landmark := &Landmark{
			ID:        id,
			Name:      name,
			Category:  CategorySubway,
			District:  district,
			Longitude: longitude,
			Latitude:  latitude,
			RawData:   stationData,
		}

		lm.landmarks[id] = landmark
		lm.byName[name] = id
	}

	log.Printf("[LandmarkManager] 加载 %d 个地铁站", len(stations))
	return nil
}

// loadCompanies 加载世界500强企业数据
func (lm *LandmarkManager) loadCompanies() error {
	data, err := lm.loadJSON("fortune500_companies.json")
	if err != nil {
		return err
	}

	companies, ok := data["companies"].([]interface{})
	if !ok {
		return fmt.Errorf("企业数据格式错误")
	}

	for _, c := range companies {
		companyData, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := companyData["company_id"].(string)
		name, _ := companyData["name"].(string)
		district, _ := companyData["district"].(string)
		longitude, _ := companyData["longitude"].(float64)
		latitude, _ := companyData["latitude"].(float64)

		if id == "" || name == "" {
			continue
		}

		landmark := &Landmark{
			ID:        id,
			Name:      name,
			Category:  CategoryCompany,
			District:  district,
			Longitude: longitude,
			Latitude:  latitude,
			RawData:   companyData,
		}

		lm.landmarks[id] = landmark
		lm.byName[name] = id

		// 同时索引简称和英文名
		if shortName, ok := companyData["short_name"].(string); ok && shortName != "" {
			lm.byName[shortName] = id
		}
		if nameEN, ok := companyData["name_en"].(string); ok && nameEN != "" {
			lm.byName[nameEN] = id
		}
	}

	log.Printf("[LandmarkManager] 加载 %d 家企业", len(companies))
	return nil
}

// loadLandmarks 加载商圈地标数据
func (lm *LandmarkManager) loadLandmarks() error {
	data, err := lm.loadJSON("landmarks.json")
	if err != nil {
		return err
	}

	landmarks, ok := data["landmarks"].([]interface{})
	if !ok {
		return fmt.Errorf("地标数据格式错误")
	}

	for _, l := range landmarks {
		landmarkData, ok := l.(map[string]interface{})
		if !ok {
			continue
		}

		id, _ := landmarkData["landmark_id"].(string)
		name, _ := landmarkData["name"].(string)
		district, _ := landmarkData["district"].(string)
		longitude, _ := landmarkData["longitude"].(float64)
		latitude, _ := landmarkData["latitude"].(float64)

		if id == "" || name == "" {
			continue
		}

		landmark := &Landmark{
			ID:        id,
			Name:      name,
			Category:  CategoryLandmark,
			District:  district,
			Longitude: longitude,
			Latitude:  latitude,
			RawData:   landmarkData,
		}

		lm.landmarks[id] = landmark
		lm.byName[name] = id
	}

	log.Printf("[LandmarkManager] 加载 %d 个地标", len(landmarks))
	return nil
}

// GetByName 根据名称查询地标（精确匹配）
func (lm *LandmarkManager) GetByName(name string) *Landmark {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if id, exists := lm.byName[name]; exists {
		if landmark, ok := lm.landmarks[id]; ok {
			// 返回副本
			copy := *landmark
			return &copy
		}
	}
	return nil
}

// SearchByKeyword 根据关键词搜索地标（模糊匹配）
func (lm *LandmarkManager) SearchByKeyword(keyword string) []*Landmark {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var results []*Landmark
	keywordLower := strings.ToLower(keyword)

	for _, landmark := range lm.landmarks {
		// 检查名称
		if strings.Contains(strings.ToLower(landmark.Name), keywordLower) {
			copy := *landmark
			results = append(results, &copy)
			continue
		}

		// 针对企业，检查简称和英文名
		if landmark.Category == CategoryCompany {
			if shortName, ok := landmark.RawData["short_name"].(string); ok {
				if strings.Contains(strings.ToLower(shortName), keywordLower) {
					copy := *landmark
					results = append(results, &copy)
					continue
				}
			}
			if nameEN, ok := landmark.RawData["name_en"].(string); ok {
				if strings.Contains(strings.ToLower(nameEN), keywordLower) {
					copy := *landmark
					results = append(results, &copy)
					continue
				}
			}
		}

		// 针对地铁站，检查线路名
		if landmark.Category == CategorySubway {
			if lines, ok := landmark.RawData["lines"].([]interface{}); ok {
				for _, line := range lines {
					if lineStr, ok := line.(string); ok {
						if strings.Contains(strings.ToLower(lineStr), keywordLower) {
							copy := *landmark
							results = append(results, &copy)
							break
						}
					}
				}
			}
		}
	}

	return results
}

// GetAll 获取全部地标信息
func (lm *LandmarkManager) GetAll() []*Landmark {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	results := make([]*Landmark, 0, len(lm.landmarks))
	for _, landmark := range lm.landmarks {
		copy := *landmark
		results = append(results, &copy)
	}
	return results
}

// GetByCategory 按类别获取地标
func (lm *LandmarkManager) GetByCategory(category LandmarkCategory) []*Landmark {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var results []*Landmark
	for _, landmark := range lm.landmarks {
		if landmark.Category == category {
			copy := *landmark
			results = append(results, &copy)
		}
	}
	return results
}

// GetByDistrict 按行政区获取地标
func (lm *LandmarkManager) GetByDistrict(district string) []*Landmark {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var results []*Landmark
	for _, landmark := range lm.landmarks {
		if landmark.District == district {
			copy := *landmark
			results = append(results, &copy)
		}
	}
	return results
}

// GetByID 根据ID获取地标
func (lm *LandmarkManager) GetByID(id string) *Landmark {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if landmark, exists := lm.landmarks[id]; exists {
		copy := *landmark
		return &copy
	}
	return nil
}

// GetStatistics 获取地标数据统计信息
func (lm *LandmarkManager) GetStatistics() map[string]interface{} {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	stats := map[string]interface{}{
		"total": len(lm.landmarks),
		"by_category": map[string]int{
			"subway":   0,
			"company":  0,
			"landmark": 0,
		},
		"by_district": make(map[string]int),
	}

	byCategory := stats["by_category"].(map[string]int)
	byDistrict := stats["by_district"].(map[string]int)

	for _, landmark := range lm.landmarks {
		byCategory[string(landmark.Category)]++
		byDistrict[landmark.District]++
	}

	return stats
}

// Reload 重新加载所有数据
func (lm *LandmarkManager) Reload() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// 清空现有数据
	lm.landmarks = make(map[string]*Landmark)
	lm.byName = make(map[string]string)

	// 重新加载
	if err := lm.loadAllData(); err != nil {
		return fmt.Errorf("重新加载数据失败: %w", err)
	}

	log.Printf("[LandmarkManager] 重新加载完成，当前 %d 个地标", len(lm.landmarks))
	return nil
}

// calcDistance 计算两点间的Haversine距离（米）
func calcDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000 // 地球半径（米）

	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*
			math.Sin(Δλ/2)*math.Sin(Δλ/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// estimateWalkingDistance 估算步行距离（米）
func estimateWalkingDistance(straightDist float64) float64 {
	// 城市道路系数：1.2-1.5，取平均值1.3
	coefficient := 1.3
	return straightDist * coefficient
}

// estimateWalkingDuration 估算步行时间（分钟）
func estimateWalkingDuration(walkingDist float64) int {
	// 步行速度：80米/分钟
	speed := 80.0
	return int(walkingDist / speed)
}
