# 地标数据管理模块文档

## 1. 模块概述

`fake_app/landmark.go` 提供租房信息查询系统中的地标数据加载与查询功能，支持地铁站、世界500强企业、商圈地标三类数据的统一管理。

### 1.1 核心特性
- **启动时全量加载**：服务启动时将所有地标数据加载到内存，查询性能优异
- **多维度索引**：支持ID、名称、类别、行政区多种查询方式
- **模糊搜索**：支持关键词模糊匹配，包括名称、简称、英文名、地铁线路
- **线程安全**：使用读写锁保证并发访问安全
- **数据热重载**：支持运行时重新加载数据
- **HTTP接口**：提供RESTful API支持远端查询

---

## 2. 数据结构定义

### 2.1 地标类别枚举
```go
type LandmarkCategory string

const (
    CategorySubway   LandmarkCategory = "subway"   // 地铁站
    CategoryCompany  LandmarkCategory = "company"  // 世界500强企业
    CategoryLandmark LandmarkCategory = "landmark" // 商圈地标
)
```

### 2.2 地标基础结构
```go
type Landmark struct {
    ID        string                 // 地标唯一ID
    Name      string                 // 地标名称
    Category  LandmarkCategory       // 类别
    District  string                 // 所属行政区
    Longitude float64                // 经度（WGS84）
    Latitude  float64                // 纬度（WGS84）
    RawData   map[string]interface{} // 原始详细数据（HTTP 响应中序列化为 "details"）
}
```

### 2.3 详细数据结构

#### 地铁站 (SubwayStation)
| 字段 | 类型 | 说明 |
|------|------|------|
| station_id | string | 站点唯一ID |
| name | string | 站点名称 |
| lines | []string | 所属线路列表 |
| district | string | 所属行政区 |
| longitude | float64 | 经度 |
| latitude | float64 | 纬度 |
| type | string | 站点类型：normal/transfer |

#### 世界500强企业 (Fortune500Company)
| 字段 | 类型 | 说明 |
|------|------|------|
| company_id | string | 企业唯一ID |
| name | string | 企业中文名 |
| name_en | string | 企业英文名 |
| short_name | string | 简称（如"百度"） |
| industry | string | 所属行业 |
| rank_2024 | int | 2024年500强排名 |
| address | string | 详细地址 |
| district | string | 所属行政区 |
| nearby_subway | string | 附近地铁站 |

#### 商圈地标 (BusinessLandmark)
| 字段 | 类型 | 说明 |
|------|------|------|
| landmark_id | string | 地标唯一ID |
| name | string | 地标名称 |
| type | string | 类型：shopping/park/landmark/transport/culture |
| type_name | string | 类型中文名 |
| address | string | 详细地址 |
| nearby_subway | string | 附近地铁站 |

### 2.4 带距离的地标（LandmarkWithDistance）

用于「某点周边地标」查询结果，由 `FindLandmarksNearPoint` 返回。

```go
type LandmarkWithDistance struct {
    Landmark Landmark `json:"landmark"` // 地标信息
    Distance float64  `json:"distance"` // 与给定点的直线距离（米）
}
```

---

## 3. 核心接口（Go API）

### 3.1 初始化管理器

```go
// 创建地标管理器，自动加载所有数据
manager, err := fake_app.NewLandmarkManager("./fake_app/data")
if err != nil {
    log.Fatal(err)
}
```

### 3.2 按名称查询（精确匹配）

```go
// 根据地标名称精确查询
landmark := manager.GetByName("西二旗站")
if landmark != nil {
    fmt.Printf("找到: %s (%s)\n", landmark.Name, landmark.Category)
    fmt.Printf("坐标: (%.4f, %.4f)\n", landmark.Longitude, landmark.Latitude)
}
```

**支持的名称形式：**
- 地铁站：`"西二旗站"`、`"国贸站"`
- 企业：`"百度公司"`、`"百度"`（简称）、`"Baidu"`（英文名）
- 地标：`"三里屯太古里"`、`"故宫博物院"`

### 3.3 关键词搜索（模糊匹配）

```go
// 根据关键词模糊搜索
results := manager.SearchByKeyword("百度")
for _, lm := range results {
    fmt.Printf("- %s (%s)\n", lm.Name, lm.Category)
}
```

**搜索范围：**
- 地标名称
- 企业简称、英文名
- 地铁线路名

### 3.4 查询全部地标

```go
// 获取所有地标
allLandmarks := manager.GetAll()
fmt.Printf("共 %d 个地标\n", len(allLandmarks))

// 按类别筛选
subways := manager.GetByCategory(fake_app.CategorySubway)
companies := manager.GetByCategory(fake_app.CategoryCompany)
landmarks := manager.GetByCategory(fake_app.CategoryLandmark)
```

### 3.5 其他查询接口

```go
// 根据ID查询
landmark := manager.GetByID("SS_001")

// 按行政区查询
haidianLandmarks := manager.GetByDistrict("海淀")

// 获取统计信息
stats := manager.GetStatistics()
fmt.Printf("总计: %d\n", stats["total"])
fmt.Printf("地铁站: %d\n", stats["by_category"].(map[string]int)["subway"])
```

### 3.6 按点查询周边地标（FindLandmarksNearPoint）

根据给定经纬度，查询在指定距离内的某类地标（如商超、公园），结果按距离升序。仅遍历 `CategoryLandmark`（landmarks.json），并按 `RawData["type"]` 过滤；用于「某小区周边商超/公园」等场景（由房屋模块传入该小区房源坐标后调用）。

```go
// 查询 (lat, lng) 周边 3000 米内的 shopping 类地标
results := manager.FindLandmarksNearPoint(39.8667, 116.5103, 3000, "shopping")
for _, r := range results {
    fmt.Printf("%s: %.0f 米\n", r.Landmark.Name, r.Distance)
}

// typeFilter 为空则不过滤类型，仅按距离
allNearby := manager.FindLandmarksNearPoint(39.9, 116.4, 5000, "")
```

**参数说明：**
| 参数 | 类型 | 说明 |
|------|------|------|
| lat, lng | float64 | 基准点纬度、经度（WGS84） |
| maxDistanceM | float64 | 最大直线距离（米） |
| typeFilter | string | 地标类型过滤，如 "shopping"、"park"；空字符串表示不过滤 |

**返回：** `[]*LandmarkWithDistance`，按 `Distance` 升序。

---

## 4. HTTP API 接口

### 4.1 接口概述

地标管理模块的 HTTP 接口由 `gateway/handler/landmark_handler.go` 提供，支持以下接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/landmarks` | 获取全部地标（支持 category、district 筛选） |
| GET | `/api/landmarks/name/{name}` | 按名称精确查询 |
| GET | `/api/landmarks/search?q={keyword}` | 关键词模糊搜索 |
| GET | `/api/landmarks/{id}` | 根据ID获取详情 |
| GET | `/api/landmarks/stats` | 获取统计信息 |

**与房屋模块的联合接口（某小区周边地标）**：  
按「小区名 → 房源坐标 → 周边地标」的查询由房屋模块暴露，内部会调用 `LandmarkManager.FindLandmarksNearPoint`。详见《租房信息查询系统设计方案》4.4 节：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/houses/nearby_landmarks?community=xxx&type=shopping\|park&max_distance_m=3000` | 某小区周边商超/公园，返回带距离的地标列表 |

**架构说明**：
- 数据模型和核心逻辑位于 `fake_app/` 包
- 地标 HTTP 接口位于 `gateway/handler/landmark_handler.go`
- 某小区周边地标 HTTP 接口位于 `gateway/handler/house_handler.go`（GetNearbyLandmarks），依赖 HouseManager + LandmarkManager
- 主 Handler 在 `gateway/handler/handler.go` 中统一管理所有路由

### 4.2 响应格式

统一响应结构：
```json
{
  "code": 0,
  "message": "success",
  "data": { ... }
}
```

错误响应：
```json
{
  "code": 404,
  "message": "未找到地标: xxx"
}
```

### 4.3 接口详情

#### 4.3.1 获取全部地标

**请求：**
```
GET /api/landmarks
GET /api/landmarks?category=subway
GET /api/landmarks?district=海淀
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| category | string | 否 | 筛选类别：subway/company/landmark |
| district | string | 否 | 筛选行政区，如"海淀"、"朝阳" |

**实现说明：** 若同时传 `category` 与 `district`，以 `category` 为准（先按类别筛，否则按行政区，否则返回全部）。

**响应示例：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 50,
    "items": [
      {
        "id": "SS_001",
        "name": "西二旗站",
        "category": "subway",
        "district": "海淀",
        "longitude": 116.3289,
        "latitude": 40.0567,
        "details": {
          "station_id": "SS_001",
          "name": "西二旗站",
          "lines": ["13号线", "昌平线"],
          "type": "transfer"
        }
      }
    ]
  }
}
```

#### 4.3.2 按名称精确查询

**请求：**
```
GET /api/landmarks/name/西二旗站
GET /api/landmarks/name/百度
GET /api/landmarks/name/百度公司
```

**响应示例：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "SS_001",
    "name": "西二旗站",
    "category": "subway",
    "district": "海淀",
    "longitude": 116.3289,
    "latitude": 40.0567,
    "details": { ... }
  }
}
```

**错误响应：**
```json
{
  "code": 404,
  "message": "未找到地标: 不存在的地标"
}
```

#### 4.3.3 关键词搜索

**请求：**
```
GET /api/landmarks/search?q=百度
GET /api/landmarks/search?q=13号线
GET /api/landmarks/search?q=百度&category=company
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| q | string | 是 | 搜索关键词 |
| category | string | 否 | 限制类别：subway/company/landmark |

**响应示例：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 3,
    "items": [
      {
        "id": "F500_013",
        "name": "百度公司",
        "category": "company",
        ...
      }
    ]
  }
}
```

#### 4.3.4 根据ID获取详情

**请求：**
```
GET /api/landmarks/SS_001
GET /api/landmarks/F500_013
```

**响应示例：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "F500_013",
    "name": "百度公司",
    "category": "company",
    "district": "海淀",
    "longitude": 116.3189,
    "latitude": 40.0512,
    "details": {
      "company_id": "F500_013",
      "name": "百度公司",
      "short_name": "百度",
      "industry": "互联网",
      "rank_2024": 185,
      "nearby_subway": "西二旗站"
    }
  }
}
```

#### 4.3.5 获取统计信息

**请求：**
```
GET /api/landmarks/stats
```

**响应示例：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 105,
    "by_category": {
      "subway": 50,
      "company": 30,
      "landmark": 25
    },
    "by_district": {
      "海淀": 15,
      "朝阳": 28,
      "西城": 12,
      ...
    }
  }
}
```

---

## 5. 数据文件说明

### 5.1 文件位置
```
fake_app/data/
├── subway_stations.json      # 地铁站数据（50个）
├── fortune500_companies.json # 世界500强企业（30家）
├── landmarks.json            # 商圈地标（25个）
└── database.json             # 房源数据
```

### 5.2 数据加载规则

| 数据文件 | 加载方式 | ID字段 | 名称索引 |
|---------|----------|--------|----------|
| subway_stations.json | 全量加载 | station_id | name |
| fortune500_companies.json | 全量加载 | company_id | name, short_name, name_en |
| landmarks.json | 全量加载 | landmark_id | name |

### 5.3 数据更新

```go
// 运行时重新加载数据（支持热更新，仅 Go API，未提供 HTTP 接口）
err := manager.Reload()
```

---

## 6. 使用示例

### 6.1 基础查询示例

```go
package main

import (
    "fmt"
    "log"
    "github.com/yourproject/ocProxy/fake_app"
)

func main() {
    // 初始化管理器
    manager, err := fake_app.NewLandmarkManager("./fake_app/data")
    if err != nil {
        log.Fatal(err)
    }

    // 1. 精确查询地铁站
    station := manager.GetByName("西二旗站")
    if station != nil {
        fmt.Printf("地铁站: %s\n", station.Name)
        fmt.Printf("线路: %v\n", station.RawData["lines"])
    }

    // 2. 模糊搜索企业
    results := manager.SearchByKeyword("百度")
    for _, r := range results {
        fmt.Printf("企业: %s (排名: %d)\n",
            r.Name, r.RawData["rank_2024"])
    }

    // 3. 获取全部地铁站
    subways := manager.GetByCategory(fake_app.CategorySubway)
    fmt.Printf("共有 %d 个地铁站\n", len(subways))

    // 4. 获取海淀区所有地标
    haidian := manager.GetByDistrict("海淀")
    fmt.Printf("海淀区有 %d 个地标\n", len(haidian))
}
```

### 6.2 与房源数据结合使用

```go
// 查询距离西二旗站2km内的房源
func findHousesNearSubway(houses []*House, manager *fake_app.LandmarkManager, stationName string, radius float64) []*House {
    station := manager.GetByName(stationName)
    if station == nil {
        return nil
    }

    var results []*House
    for _, house := range houses {
        distance := calcDistance(
            house.Latitude, house.Longitude,
            station.Latitude, station.Longitude,
        )
        if distance <= radius {
            results = append(results, house)
        }
    }
    return results
}
```

### 6.3 HTTP 接口使用示例

```bash
# 1. 获取全部地标
curl http://localhost:8080/api/landmarks

# 2. 获取全部地铁站
curl http://localhost:8080/api/landmarks?category=subway

# 3. 按名称查询
curl http://localhost:8080/api/landmarks/name/西二旗站

# 4. 关键词搜索
curl "http://localhost:8080/api/landmarks/search?q=百度"

# 5. 搜索海淀区的地标
curl "http://localhost:8080/api/landmarks?district=海淀"

# 6. 获取统计信息
curl http://localhost:8080/api/landmarks/stats

# 7. 根据ID获取详情
curl http://localhost:8080/api/landmarks/SS_001
```

---

## 7. 性能指标

| 指标 | 数值 | 说明 |
|------|------|------|
| 初始化时间 | < 100ms | 加载全部105个地标 |
| 内存占用 | ~2MB | 包含原始数据 |
| 精确查询 | O(1) | 基于哈希索引 |
| 模糊搜索 | O(n) | n为地标总数 |
| 并发支持 | 读多写少 | 读写锁保护 |
| HTTP响应 | < 10ms | P99延迟 |

---

## 8. 错误处理

### 8.1 可能的错误

| 错误场景 | 错误信息 | 处理方式 |
|---------|----------|----------|
| 数据目录不存在 | "数据目录不存在" | 检查路径配置 |
| JSON解析失败 | "解析JSON失败" | 检查数据文件格式 |
| 数据格式错误 | "地铁站数据格式错误" | 检查数据结构 |
| 地标不存在 | 404 Not Found | 返回404状态码 |
| 参数错误 | 400 Bad Request | 返回400状态码 |

### 8.2 最佳实践

```go
// 始终检查错误
manager, err := fake_app.NewLandmarkManager(dataDir)
if err != nil {
    log.Printf("初始化失败: %v", err)
    // 使用默认配置或退出
    return
}

// 检查查询结果
landmark := manager.GetByName("不存在")
if landmark == nil {
    fmt.Println("未找到该地标")
    return
}
```

---

## 9. 扩展开发

### 9.1 添加新的地标类别

```go
const (
    CategoryHospital LandmarkCategory = "hospital" // 医院
    CategorySchool   LandmarkCategory = "school"   // 学校
)
```

### 9.2 添加新的查询接口

```go
// 按类型获取地标
func (lm *LandmarkManager) GetByType(typeName string) []*Landmark {
    lm.mu.RLock()
    defer lm.mu.RUnlock()

    var results []*Landmark
    for _, landmark := range lm.landmarks {
        if landmark.RawData["type"] == typeName {
            copy := *landmark
            results = append(results, &copy)
        }
    }
    return results
}
```

### 9.3 距离计算函数

```go
// Haversine距离计算（米）
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
```

---

## 10. API参考

### 10.1 LandmarkManager 方法列表

| 方法 | 参数 | 返回值 | 说明 |
|------|------|--------|------|
| NewLandmarkManager | dataDir string | (*LandmarkManager, error) | 创建管理器 |
| GetByName | name string | *Landmark | 精确查询 |
| SearchByKeyword | keyword string | []*Landmark | 模糊搜索 |
| GetAll | - | []*Landmark | 获取全部 |
| GetByCategory | category LandmarkCategory | []*Landmark | 按类别查询 |
| GetByDistrict | district string | []*Landmark | 按行政区查询 |
| GetByID | id string | *Landmark | 按ID查询 |
| GetStatistics | - | map[string]interface{} | 获取统计 |
| FindLandmarksNearPoint | lat, lng, maxDistanceM float64, typeFilter string | []*LandmarkWithDistance | 某点周边某类地标（如 shopping/park），按距离排序 |
| Reload | - | error | 重新加载数据 |

### 10.2 LandmarkHandler HTTP接口

**文件位置**: `gateway/handler/landmark_handler.go`

| 方法 | 处理器 | 路径 | 说明 |
|------|--------|------|------|
| GET | GetAllLandmarks | /api/landmarks | 获取全部（支持筛选） |
| GET | GetByName | /api/landmarks/name/{name} | 精确查询 |
| GET | SearchByKeyword | /api/landmarks/search | 模糊搜索 |
| GET | GetByID | /api/landmarks/{id} | 按ID查询 |
| GET | GetStatistics | /api/landmarks/stats | 统计信息 |

**与房屋模块联合**：某小区周边地标由 `house_handler.GetNearbyLandmarks` 提供，路径 `GET /api/houses/nearby_landmarks?community=&type=&max_distance_m=`，内部调用 `LandmarkManager.FindLandmarksNearPoint`。详见《租房信息查询系统设计方案》4.4。

**初始化方式**:
```go
// 在 gateway/handler/handler.go 的 NewHandler 中初始化
landmarkManager, err := fake_app.NewLandmarkManager("fake_app/data")
landmarkHandler := NewLandmarkHandler(landmarkManager)
```

**路由注册**:
```go
// 在 gateway/handler/handler.go 的 SetupRoutes 中注册
if h.landmarkHandler != nil {
    h.landmarkHandler.SetupLandmarkRoutes(r)
}
```

---

## 11. 变更日志

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0 | 2026-02-15 | 初始版本，支持地铁站/企业/地标三类数据管理 |
| v1.1 | 2026-02-15 | 新增HTTP RESTful API接口，支持远端查询 |
| v1.2 | 2026-02-15 | HTTP接口迁移至gateway模块统一管理 |
| v1.3 | 2026-02-17 | 新增 LandmarkWithDistance、FindLandmarksNearPoint；4.1 补充与房屋模块联合接口 GET /api/houses/nearby_landmarks 说明 |
| v1.4 | 2026-02-17 | 与实现对齐：2.2 RawData 在 HTTP 响应中为 details；4.3.1 GET /api/landmarks 同时传 category 与 district 时以 category 为准；5.3 Reload 仅 Go API、未提供 HTTP 接口 |

---

*文档版本：v1.4*
*模块路径：ocProxy/fake_app*
