# 租房信息查询系统设计方案

## 1. 系统概述

本系统提供完整的租房信息查询与管理功能，支持多维度房屋检索、地标距离计算、租赁状态管理等功能。

### 1.1 核心功能
- **房屋信息检索**：支持多维度筛选（价格、户型、区域、地铁等）
- **地标距离查询**：基于坐标计算与地标（地铁站、公司）的距离
- **HTTP API**：对外提供RESTful接口

### 1.2 数据规模
- 房源数量：以 data 目录下 database_2000.json（或 database.json）为准，如 100 套
- 覆盖区域：北京多行政区（房山、朝阳、海淀、大兴、西城、昌平等）
- 地标数据：subway_stations、fortune500_companies、landmarks（含 shopping/park 等类型）

### 1.3 按用户隔离的状态（多用户互不影响）
- **基础数据**：房源从 `database_*.json` 加载，只读不写。
- **按用户覆盖**：每个用户拥有独立的状态覆盖层（userID → houseID → status）。用户 A 将某房改为「已租」仅影响 A 的视角，用户 B 仍按基础数据（或 B 自己的覆盖）看到该房。
- **识别方式**：请求头 `X-User-ID` 标识当前用户，**所有房源相关接口均必填**。不带头时接口返回 400。带该头时，列表/详情/统计/按小区/附近房等接口按该用户视角返回「有效状态」；更新状态接口同样必带 `X-User-ID`。

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
    CommuteToXierqi  int      `json:"commute_to_xierqi"`  // 到西二旗通勤时间（分钟）
    AvailableFrom    string   `json:"available_from"`    // 可入住日期
    ListingPlatform  string   `json:"listing_platform"`  // 发布平台
    ListingURL       string   `json:"listing_url"`       // 房源链接
    Tags             []string `json:"tags"`              // 标签
    HiddenNoiseLevel string   `json:"hidden_noise_level"`// 噪音等级
    Status           string   `json:"status"`            // 展示状态：available/rented/offline，按用户视角合并基础数据与覆盖
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

    // 水电与通勤
    UtilitiesType       string // 水电类型，如 民水民电
    AvailableFromBefore string // 可入住日期上限，格式 2006-01-02
    CommuteToXierqiMax  int    // 到西二旗通勤时间上限（分钟）

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
| `/api/houses` | GET | 查询房屋列表（支持筛选、排序、分页；**必带 X-User-ID**，按用户视角返回状态） |
| `/api/houses/{id}` | GET | 根据ID获取房屋详情（**必带 X-User-ID**） |
| `/api/houses/by_community` | GET | 按小区名查询可租房源（指代、地铁信息、隐性属性、在租状态；**必带 X-User-ID**） |
| `/api/houses/nearby_landmarks` | GET | 某小区周边某类地标（商超/公园，按距离排序；**必带 X-User-ID**） |
| `/api/houses/nearby` | GET | 查询地标附近房屋（**必带 X-User-ID**） |
| `/api/houses/stats` | GET | 获取房屋统计信息（**必带 X-User-ID**，按用户视角统计 by_status） |
| `/api/houses/{id}/status` | PUT/PATCH | 更新当前用户视角下该房源状态（available/rented/offline），**必须带 X-User-ID**；响应 data 为修改后的房源完整对象 |
| `/api/houses/init` | POST | **初始化指定用户的房源数据**：清空该用户的状态覆盖，该用户视角恢复为初始状态。**必须带 X-User-ID** 指定要重置的用户。评测/比赛每启动新题目时调用。 |

---

## 4. HTTP API 详细设计

### 4.0 公共请求头（用户隔离）

| 请求头 | 必填 | 说明 |
|--------|------|------|
| X-User-ID | **是**（所有房源接口） | 当前用户标识。不带头时接口返回 400。带该头时，列表/详情/统计/按小区/附近房返回该用户视角下的 status；更新状态接口同样必填。 |

### 4.1 查询房屋列表

**请求：**
```
GET /api/houses?district=海淀&min_price=3000&max_price=8000&bedrooms=2,3&page=1&page_size=10
Header: X-User-ID: user_001   （必填）
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| district | string | 否 | 行政区，逗号分隔，如 "海淀,朝阳" |
| area | string | 否 | 商圈，逗号分隔，如 "西二旗,上地" |
| min_price | int | 否 | 最低价格（元/月） |
| max_price | int | 否 | 最高价格（元/月） |
| bedrooms | string | 否 | 卧室数，逗号分隔，如 "1,2" |
| rental_type | string | 否 | 整租/合租 |
| decoration | string | 否 | 精装/简装 |
| orientation | string | 否 | 朝向，如 朝南/朝东/南北 |
| elevator | string | 否 | 是否有电梯，true/false |
| min_area | int | 否 | 最小面积（平米） |
| max_area | int | 否 | 最大面积（平米） |
| property_type | string | 否 | 物业类型，如 住宅 |
| subway_line | string | 否 | 地铁线路，如 13号线 |
| max_subway_dist | int | 否 | 最大地铁距离（米），近地铁建议 800 |
| subway_station | string | 否 | 地铁站名，如 车公庄站 |
| utilities_type | string | 否 | 水电类型，如 民水民电 |
| available_from_before | string | 否 | 可入住日期上限，格式 YYYY-MM-DD，如 2026-03-10 |
| commute_to_xierqi_max | int | 否 | 到西二旗通勤时间上限（分钟） |
| sort_by | string | 否 | 排序字段：price/area/subway |
| sort_order | string | 否 | asc/desc |
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认20，最大100 |

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
    "subway": "13号线/昌平线",
    "subway_station": "西二旗站",
    "subway_distance": 800,
    "commute_to_xierqi": 14,
    "hidden_noise_level": "安静",
    "longitude": 116.3289,
    "latitude": 40.0567,
    ...
  }
}
```

### 4.3 按小区名查房源

用于指代消解（如「建清园那套还在吗」）、查询某小区地铁信息、隐性属性（如是否吵）等。返回该小区下所有可租房源（status=available），每条含完整字段（含 subway_*、commute_to_xierqi、hidden_noise_level）。

**请求：**
```
GET /api/houses/by_community?community=建清园(南区)
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| community | string | 是 | 小区名，与数据集中 community 字段精确匹配，如 建清园(南区)、保利锦上(二期) |

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 1,
    "page": 1,
    "page_size": 1,
    "items": [
      {
        "house_id": "HF_3",
        "community": "建清园(南区)",
        "district": "海淀",
        "area": "学院路",
        "subway": "昌平线",
        "subway_station": "学院桥站",
        "subway_distance": 1200,
        "commute_to_xierqi": 18,
        "hidden_noise_level": "安静",
        "status": "available",
        ...
      }
    ]
  }
}
```

**错误响应：** 缺少 community 时返回 400。

### 4.4 某小区周边地标（商超/公园）

以该小区内第一套房源的经纬度为基准点，查询周边某类地标（如 shopping、park），返回在指定距离内的地标列表并按距离排序。依赖 LandmarkManager.FindLandmarksNearPoint，数据来自 landmarks.json（type=shopping/park）。

**请求：**
```
GET /api/houses/nearby_landmarks?community=保利锦上(二期)&type=shopping&max_distance_m=3000
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| community | string | 是 | 小区名，用于定位基准点（取该小区首套房源坐标） |
| type | string | 否 | 地标类型：shopping（商超）/ park（公园），不传则不过滤类型 |
| max_distance_m | float | 否 | 最大距离（米），默认 3000 |

**响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "community": "保利锦上(二期)",
    "type": "shopping",
    "total": 2,
    "items": [
      {
        "landmark": {
          "id": "LM_002",
          "name": "国贸商城",
          "category": "landmark",
          "district": "朝阳",
          "longitude": 116.4623,
          "latitude": 39.9106,
          "details": { "type": "shopping", "type_name": "购物中心", ... }
        },
        "distance": 1250.5
      }
    ]
  }
}
```

若地标服务不可用返回 503；若小区无房源则 items 为空数组、total=0。

### 4.5 查询附近房屋

以地标为圆心，查询在指定距离内的可租房源，返回带直线距离、步行距离、步行时间。

**请求：**
```
GET /api/houses/nearby?landmark_id=SS_001&max_distance=2000
```

**参数：**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| landmark_id | string | 是 | 地标ID或地标名称（支持按名称查找） |
| max_distance | float | 否 | 最大直线距离（米），默认 2000 |

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

### 4.6 获取统计信息

**请求：**
```
GET /api/houses/stats
Header: X-User-ID: user_001   （必填，按该用户视角统计 by_status）
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

### 4.7 更新当前用户视角下房源状态

用于「用户租赁/下架」等操作，仅影响该用户视角，其他用户不受影响。

**请求：**
```
PUT /api/houses/HF_2001/status
Header: X-User-ID: user_001   （必填）
Content-Type: application/json

{"status": "rented"}
```

**请求体：**
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| status | string | 是 | 取值：available / rented / offline |

**响应（成功）：** `data` 为**修改后的房源完整对象**（与 GET /api/houses/{id} 结构一致，含更新后的 status），便于评测集构造与前端展示。
```json
{
  "code": 0,
  "message": "success",
  "data": { "house_id": "HF_2001", "community": "...", "status": "rented", ... }
}
```

**错误：**
- 400：未提供 X-User-ID、缺少 status、或 status 非法
- 404：房屋不存在

### 4.8 初始化指定用户的房源数据

用于评测或比赛时，每启动一个新题目前，将**指定用户**的房源视角恢复为初始状态（清空该用户的租赁/退租等状态覆盖），不影响其他用户。

**请求：**
```
POST /api/houses/init
Header: X-User-ID: eval_user   （必填，指定要初始化的用户）
```

**响应（成功）：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "action": "reset_user",
    "user_id": "eval_user",
    "message": "该用户状态覆盖已清空，房源恢复为初始状态"
  }
}
```

**错误：**
- 400：未提供 X-User-ID

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

**实现对应：** 实际代码为 `matchQuery(house *House, query *HouseQuery, effectiveStatus string) bool`。先按「有效状态」筛：`effectiveStatus` 非空时用其与 `available` 比较，否则用 `house.Status`，**仅展示可租房源**（status=available）。再按下列条件匹配。

```go
// 伪代码：与 house.go matchQuery 对齐，effectiveStatus 由上层传入（用户视角下的状态）
func matchQuery(house *House, query *HouseQuery, effectiveStatus string) bool {
    statusCheck := house.Status
    if effectiveStatus != "" {
        statusCheck = effectiveStatus
    }
    if statusCheck != "available" {
        return false
    }
    // 价格、行政区、商圈、卧室数、租赁类型、装修、电梯、朝向、面积、物业类型
    if query.MinPrice > 0 && house.Price < query.MinPrice { return false }
    if query.MaxPrice > 0 && house.Price > query.MaxPrice { return false }
    if len(query.Districts) > 0 && !contains(query.Districts, house.District) { return false }
    if len(query.Areas) > 0 && !contains(query.Areas, house.Area) { return false }
    if len(query.Bedrooms) > 0 && !containsInt(query.Bedrooms, house.Bedrooms) { return false }
    if query.RentalType != "" && house.RentalType != query.RentalType { return false }
    if query.Decoration != "" && house.Decoration != query.Decoration { return false }
    if query.Elevator != nil && house.Elevator != *query.Elevator { return false }
    if query.Orientation != "" && house.Orientation != query.Orientation { return false }
    if query.MinArea > 0 && int(house.AreaSqm) < query.MinArea { return false }
    if query.MaxArea > 0 && int(house.AreaSqm) > query.MaxArea { return false }
    if query.PropertyType != "" && house.PropertyType != query.PropertyType { return false }
    if query.MaxSubwayDist > 0 && house.SubwayDistance > query.MaxSubwayDist { return false }
    if query.SubwayLine != "" && !strings.Contains(house.Subway, query.SubwayLine) { return false }
    if query.SubwayStation != "" && house.SubwayStation != query.SubwayStation { return false }
    if query.UtilitiesType != "" && house.UtilitiesType != query.UtilitiesType { return false }
    if query.AvailableFromBefore != "" && house.AvailableFrom > query.AvailableFromBefore { return false }
    if query.CommuteToXierqiMax > 0 && house.CommuteToXierqi > query.CommuteToXierqiMax { return false }
    return true
}
```

---

## 6. 文件结构

```
fake_app/
├── house.go                 # 房屋数据模型、HouseQuery、HouseManager（加载、Query、QueryWithPagination、GetByID、GetAll、GetStatistics、GetByCommunity、FindNearby、effectiveStatus、UpdateStatusForUser、按用户状态覆盖）
├── landmark.go              # 地标数据模型、LandmarkManager（含 FindLandmarksNearPoint、Reload）
└── data/
    ├── database_2000.json   # 房源数据（1～2000 条）
    ├── database_4000.json   # 房源数据（2001～4000 条，可选）
    ├── database_6000.json   # 等 database_数字.json，按数字升序合并加载
    ├── database.json        # 兼容旧版单文件（若有则参与合并，同 id 后加载覆盖）
    ├── subway_stations.json
    ├── fortune500_companies.json # 企业数据
    └── landmarks.json       # 商圈地标数据（含 type=shopping/park）

gateway/handler/
├── handler.go               # 主处理器
├── house_handler.go         # 房屋 HTTP 接口（含 by_community、nearby_landmarks）
└── landmark_handler.go      # 地标 HTTP 接口
```

### 6.1 房源数据加载规则

- **不固定文件名**：扫描 `data` 目录下所有 `database_数字.json`（如 database_2000.json、database_4000.json、database_6000.json）及可选 `database.json`。
- **按数字升序合并**：将 `database_数字.json` 按数字升序依次加载并合并到内存（同 `house_id` 时后加载的覆盖先加载的）。
- **database.json**：若存在则参与合并，视为数字序号最小（与 Python 生成脚本约定一致时，可仅使用 database_数字.json）。
- **单文件读错**：某个文件读取或解析失败时跳过并打日志，不中断整体加载；至少有一个文件成功且最终 `len(houses)>0` 即成功。

---

## 7. 变更日志

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0 | 2026-02-15 | 初始版本，完整租房查询方案设计 |
| v1.1 | 2026-02-17 | 新增 House.CommuteToXierqi；HouseQuery 增加 UtilitiesType、AvailableFromBefore、CommuteToXierqiMax；新增 GET /api/houses/by_community、GET /api/houses/nearby_landmarks；4.1 参数表补全 |
| v1.2 | 2026-02-17 | 房源数据加载改为不固定文件名：自动发现并合并所有 database_数字.json（按数字升序），可选 database.json；见 6.1 |
| v1.3 | 2026-02-17 | 按用户隔离状态：1.3 节、4.0 公共请求头 X-User-ID；新增 PUT/PATCH /api/houses/{id}/status；接口表与 4.7 更新 |
| v1.4 | 2026-02-17 | 与实现对齐：5.3 筛选逻辑改为 matchQuery(house, query, effectiveStatus)，含 status=available 及 Areas/Decoration/Orientation/UtilitiesType/AvailableFromBefore/CommuteToXierqiMax 等；6 文件结构补充 house.go/landmark.go 能力描述 |
| v1.5 | 2026-02-17 | 新增 POST /api/houses/init：按用户初始化房源数据（清空该用户状态覆盖），3.2 接口表与 4.8 节 |

---

*文档版本：v1.5*
*模块路径：ocProxy/fake_app*
