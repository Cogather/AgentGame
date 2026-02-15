# 租房信息查询系统设计方案

## 1. 系统概述

本系统提供完整的租房信息查询与管理功能，支持多维度房屋检索、地标距离计算、租赁状态管理等功能。

### 1.1 核心功能
- **房屋信息检索**：支持多维度筛选（价格、户型、区域、地铁等）
- **地标距离查询**：基于坐标计算与地标（地铁站、公司）的距离
- **HTTP API**：对外提供RESTful接口

### 1.2 数据规模
- 房源数量：33套
- 覆盖区域：北京10个行政区
- 价格区间：800-18000元/月
- 地标数据：105个（50地铁站+30企业+25商圈）

---

## 2. 数据模型设计

### 2.1 房屋结构（House）
```go
type House struct {
    HouseID          string   `json:"house_id"`          // 房源唯一ID
    Community        string   `json:"community"`         // 小区名称
    District         string   `json:"district"`          // 行政区
    Area             string   `json:"area"`              // 商圈/区域
    Address          string   `json:"address"`           // 详细地址
    Bedrooms         int      `json:"bedrooms"`          // 卧室数
    Livingrooms      int      `json:"livingrooms"`       // 客厅数
    Bathrooms        int      `json:"bathrooms"`         // 卫生间数
    AreaSqm          float64  `json:"area_sqm"`          // 面积（平方米）
    Floor            string   `json:"floor"`             // 楼层
    TotalFloors      int      `json:"total_floors"`      // 总楼层
    Orientation      string   `json:"orientation"`       // 朝向
    Decoration       string   `json:"decoration"`        // 装修等级
    Price            int      `json:"price"`             // 租金（元/月）
    PriceUnit        string   `json:"price_unit"`        // 价格单位
    RentalType       string   `json:"rental_type"`       // 租赁类型：整租/合租
    PropertyType     string   `json:"property_type"`     // 物业类型
    UtilitiesType    string   `json:"utilities_type"`    // 水电类型
    Elevator         bool     `json:"elevator"`          // 是否有电梯
    Subway           string   `json:"subway"`            // 地铁线路
    SubwayDistance   int      `json:"subway_distance"`   // 距地铁距离（米）
    SubwayStation    string   `json:"subway_station"`    // 最近地铁站
    AvailableFrom    string   `json:"available_from"`    // 可入住日期
    ListingPlatform  string   `json:"listing_platform"`  // 发布平台
    ListingURL       string   `json:"listing_url"`       // 房源链接
    Tags             []string `json:"tags"`              // 标签
    HiddenNoiseLevel string   `json:"hidden_noise_level"`// 噪音等级
    Status           string   `json:"status"`            // 状态（只读）：available/rented/offline
    Longitude        float64  `json:"longitude"`         // 经度
    Latitude         float64  `json:"latitude"`          // 纬度
    CoordinateSystem string   `json:"coordinate_system"` // 坐标系
}
```

### 2.2 房屋状态枚举
```go
const (
    HouseStatusAvailable = "available" // 可租
    HouseStatusRented    = "rented"    // 已租
    HouseStatusOffline   = "offline"   // 下架
)
```

### 2.3 距离查询结果（HouseWithDistance）
```go
type HouseWithDistance struct {
    House
    DistanceToLandmark float64 `json:"distance_to_landmark"` // 直线距离（米）
    WalkingDistance    float64 `json:"walking_distance"`     // 估算步行距离（米）
    WalkingDuration    int     `json:"walking_duration"`     // 估算步行时间（分钟）
}
```

---

## 3. 查询功能设计

### 3.1 筛选条件（HouseQuery）
```go
type HouseQuery struct {
    // 基础筛选
    Districts   []string // 行政区列表
    Areas       []string // 商圈列表
    MinPrice    int      // 最低价格
    MaxPrice    int      // 最高价格
    Bedrooms    []int    // 卧室数列表（1,2,3表示1-3室）
    RentalType  string   // 租赁类型：整租/合租

    // 房屋属性
    Decoration    string // 装修：简装/精装/豪华
    Elevator      *bool  // 是否有电梯
    Orientation   string // 朝向
    MinArea       int    // 最小面积
    MaxArea       int    // 最大面积
    PropertyType  string // 物业类型

    // 地铁相关
    SubwayLine     string // 地铁线路
    MaxSubwayDist  int    // 最大地铁距离（米）
    SubwayStation  string // 指定地铁站

    // 地标距离（需配合地标ID使用）
    NearLandmarkID string  // 附近地标ID
    MaxDistance    float64 // 最大距离（米）

    // 排序
    SortBy    string // 排序字段：price/area/distance
    SortOrder string // 排序顺序：asc/desc

    // 分页
    Page     int // 页码（从1开始）
    PageSize int // 每页数量
}
```

### 3.2 查询接口列表

| 接口 | 方法 | 描述 |
|------|------|------|
| `/api/houses` | GET | 查询房屋列表（支持筛选） |
| `/api/houses/{id}` | GET | 根据ID获取房屋详情 |
| `/api/houses/nearby` | GET | 查询地标附近房屋 |
| `/api/houses/stats` | GET | 获取房屋统计信息 |

---

## 4. HTTP API 详细设计

### 4.1 查询房屋列表

**请求：**
```
GET /api/houses?district=海淀&min_price=3000&max_price=8000&bedrooms=2,3&page=1&page_size=10
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| district | string | 否 | 行政区，如"海淀" |
| area | string | 否 | 商圈，如"西二旗" |
| min_price | int | 否 | 最低价格 |
| max_price | int | 否 | 最高价格 |
| bedrooms | string | 否 | 卧室数，如"1,2,3" |
| rental_type | string | 否 | 整租/合租 |
| decoration | string | 否 | 简装/精装/豪华 |
| elevator | bool | 否 | 是否有电梯 |
| subway_line | string | 否 | 地铁线路 |
| max_subway_dist | int | 否 | 最大地铁距离 |
| near_landmark | string | 否 | 附近地标ID |
| max_distance | int | 否 | 最大距离（米） |
| sort_by | string | 否 | price/area/distance |
| sort_order | string | 否 | asc/desc |
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认20 |

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 15,
    "page": 1,
    "page_size": 10,
    "items": [
      {
        "house_id": "HF_2001",
        "community": "智学苑",
        "district": "海淀",
        "price": 8500,
        "bedrooms": 2,
        "area_sqm": 78,
        "status": "available",
        "subway_station": "西二旗站",
        "subway_distance": 800,
        "tags": ["近地铁", "南北通透"]
      }
    ]
  }
}
```

### 4.2 获取房屋详情

**请求：**
```
GET /api/houses/HF_2001
```

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "house_id": "HF_2001",
    "community": "智学苑",
    "district": "海淀",
    "area": "西二旗",
    "address": "西二旗大街",
    "bedrooms": 2,
    "livingrooms": 1,
    "bathrooms": 1,
    "area_sqm": 78,
    "price": 8500,
    "price_unit": "元/月",
    "status": "available",
    "longitude": 116.3289,
    "latitude": 40.0567,
    ...
  }
}
```

### 4.3 查询附近房屋

**请求：**
```
GET /api/houses/nearby?landmark_id=SS_001&max_distance=2000&sort_by=distance
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| landmark_id | string | 是 | 地标ID |
| max_distance | int | 否 | 最大距离（米），默认2000 |

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "landmark": {
      "id": "SS_001",
      "name": "西二旗站",
      "longitude": 116.3289,
      "latitude": 40.0567
    },
    "total": 5,
    "items": [
      {
        "house_id": "HF_2001",
        "community": "智学苑",
        "price": 8500,
        "distance_to_landmark": 850,
        "walking_distance": 1100,
        "walking_duration": 14
      }
    ]
  }
}
```

### 4.4 获取统计信息

**请求：**
```
GET /api/houses/stats
```

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 33,
    "by_status": {
      "available": 32,
      "rented": 1,
      "offline": 0
    },
    "by_district": {
      "海淀": 6,
      "朝阳": 6,
      "通州": 5,
      "昌平": 6,
      "大兴": 3,
      "房山": 3,
      "西城": 2,
      "丰台": 2,
      "顺义": 2,
      "东城": 1
    },
    "price_range": {
      "min": 800,
      "max": 18000,
      "avg": 5200
    },
    "by_bedrooms": {
      "1": 8,
      "2": 15,
      "3": 8,
      "4": 2
    }
  }
}
```

---

## 5. 核心算法设计

### 5.1 Haversine距离计算

```go
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

### 5.2 步行距离估算

```go
func estimateWalkingDistance(straightDist float64) float64 {
    // 城市道路系数：1.2-1.5
    coefficient := 1.3
    return straightDist * coefficient
}

func estimateWalkingDuration(walkingDist float64) int {
    // 步行速度：80米/分钟
    speed := 80.0
    return int(walkingDist / speed)
}
```

### 5.3 筛选匹配逻辑

```go
func matchHouse(house *House, query *HouseQuery) bool {
    // 价格范围
    if query.MinPrice > 0 && house.Price < query.MinPrice {
        return false
    }
    if query.MaxPrice > 0 && house.Price > query.MaxPrice {
        return false
    }

    // 行政区
    if len(query.Districts) > 0 && !contains(query.Districts, house.District) {
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

    // 电梯
    if query.Elevator != nil && house.Elevator != *query.Elevator {
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

    return true
}
```

---

## 6. 文件结构

```
fake_app/
├── house.go                 # 房屋数据模型和管理器
├── house_manager.go         # 房屋管理核心逻辑
├── house_query.go           # 查询条件和筛选逻辑
├── landmark.go              # 地标数据模型
└── data/
    ├── database.json        # 房源数据
    ├── subway_stations.json # 地铁站数据
    ├── fortune500_companies.json # 企业数据
    └── landmarks.json       # 商圈地标数据

gateway/handler/
├── handler.go               # 主处理器
├── house_handler.go         # 房屋HTTP接口处理器
└── landmark_handler.go      # 地标HTTP接口处理器
```

---

## 7. 变更日志

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0 | 2026-02-15 | 初始版本，完整租房查询方案设计 |

---

*文档版本：v1.0*
*模块路径：github.com/ocProxy/fake_app*
