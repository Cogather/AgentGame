# Fake App 租房信息查询系统设计方案

## 1. 系统概述

基于现有 `database.json` 中33套房源数据（含房山、朝阳、海淀、大兴、西城、昌平、顺义、丰台、通州、东城等10个行政区），设计一套支持多维度筛选、地标参照、距离计算的租房信息查询系统。

### 1.1 核心功能
- 多维度房源筛选（价格、户型、区域、地铁等）
- 地标参照查询（地铁站、公司、商圈）
- 距离计算与排序（直线距离/步行距离估算）
- 通勤时间估算

### 1.2 技术栈建议
- 坐标系统：WGS84（与现有数据一致）
- 距离算法：Haversine公式计算直线距离
- 数据格式：JSON

---

## 2. 现有数据结构分析

### 2.1 房源字段梳理
| 字段类别 | 字段名 | 说明 |
|---------|--------|------|
| 基础信息 | house_id, community, district, area, address | 唯一标识与位置描述 |
| 户型信息 | bedrooms, livingrooms, bathrooms, area_sqm | 房间配置 |
| 楼层信息 | floor, total_floors, elevator | 楼层与电梯 |
| 朝向装修 | orientation, decoration | 朝向与装修等级 |
| 价格信息 | price, price_unit, rental_type | 租金与租赁方式 |
| 物业信息 | property_type, utilities_type | 物业与水电类型 |
| 交通信息 | subway, subway_distance, subway_station | 地铁配套 |
| 坐标信息 | longitude, latitude, coordinate_system | WGS84坐标 |
| 时间信息 | available_from | 可入住时间 |
| 其他 | tags, hidden_noise_level, status | 标签与状态 |

### 2.2 数据分布统计
- **价格区间**：800-18000元/月
- **区域分布**：昌平(6)、朝阳(6)、通州(5)、房山(3)、大兴(3)、海淀(3)、西城(2)、丰台(2)、顺义(2)、东城(1)
- **户型分布**：1-4室，整租/合租
- **地铁覆盖**：27套有地铁配套，6套无地铁

---

## 3. 地标数据设计方案

### 3.1 地标数据结构设计

#### 3.1.1 地铁站数据 (subway_stations.json)
```json
{
  "stations": [
    {
      "station_id": "SS_001",
      "name": "西二旗站",
      "lines": ["13号线", "昌平线"],
      "district": "海淀",
      "longitude": 116.3289,
      "latitude": 40.0567,
      "coordinate_system": "WGS84",
      "type": "transfer",
      "category": "subway"
    }
  ]
}
```

#### 3.1.2 世界500强企业数据 (fortune500_companies.json)
```json
{
  "companies": [
    {
      "company_id": "F500_001",
      "name": "中国石油天然气集团",
      "name_en": "China National Petroleum",
      "industry": "石油",
      "rank_2024": 4,
      "address": "北京市东城区东直门北大街9号",
      "district": "东城",
      "longitude": 116.4312,
      "latitude": 39.9456,
      "coordinate_system": "WGS84",
      "category": "company"
    }
  ]
}
```

#### 3.1.3 商圈地标数据 (landmarks.json)
```json
{
  "landmarks": [
    {
      "landmark_id": "LM_001",
      "name": "三里屯太古里",
      "type": "shopping",
      "district": "朝阳",
      "address": "北京市朝阳区三里屯路19号",
      "longitude": 116.4567,
      "latitude": 39.9356,
      "coordinate_system": "WGS84",
      "category": "landmark"
    }
  ]
}
```

### 3.2 地标数据生成范围

#### 3.2.1 北京地铁站（约50个核心站点）
| 线路 | 重点站点 |
|------|----------|
| 1号线 | 大望路、国贸、西单、复兴门、木樨地 |
| 2号线 | 西直门、车公庄、复兴门、朝阳门、东直门 |
| 4号线/大兴线 | 中关村、西直门、北京南站、黄村西大街 |
| 5号线 | 立水桥、惠新西街南口、东单、天坛东门 |
| 6号线 | 十里堡、朝阳门、车公庄、白石桥南 |
| 7号线 | 百子湾、双合、磁器口、菜市口 |
| 8号线 | 奥林匹克公园、什刹海、大红门 |
| 10号线 | 国贸、三元桥、知春路、车道沟 |
| 13号线 | 西二旗、上地、大钟寺、光熙门 |
| 15号线 | 清华东路西口、奥林匹克公园、国展 |
| 昌平线 | 西二旗、生命科学园、昌平站 |
| 房山线 | 阎村、良乡大学城、郭公庄 |

#### 3.2.2 世界500强企业北京总部（约30家）
| 企业名称 | 行业 | 地址区域 |
|----------|------|----------|
| 中国石油天然气集团 | 石油 | 东城区 |
| 中国石化集团 | 石油 | 朝阳区 |
| 国家电网 | 电力 | 西城区 |
| 中国建筑集团 | 建筑 | 海淀区 |
| 中国工商银行 | 银行 | 西城区 |
| 中国建设银行 | 银行 | 西城区 |
| 中国农业银行 | 银行 | 东城区 |
| 中国银行 | 银行 | 西城区 |
| 中国人寿保险 | 保险 | 西城区 |
| 中国移动通信 | 电信 | 西城区 |
| 京东集团 | 电商 | 亦庄开发区 |
| 字节跳动 | 互联网 | 海淀区 |
| 百度公司 | 互联网 | 海淀区 |
| 小米集团 | 科技 | 海淀区 |

---

## 4. 距离计算方案

### 4.1 Haversine距离计算
```
d = 2 * R * arcsin(sqrt(sin²(Δφ/2) + cos(φ1) * cos(φ2) * sin²(Δλ/2)))

其中：
- R: 地球半径（约6371km）
- φ1, φ2: 两点的纬度（弧度）
- λ1, λ2: 两点的经度（弧度）
- Δφ = φ2 - φ1
- Δλ = λ2 - λ1
```

### 4.2 步行距离估算模型
```
步行距离 ≈ 直线距离 * 系数

系数参考：
- 城市核心区：1.3-1.5
- 郊区/新建区：1.2-1.4
- 有直达道路：1.1-1.3
```

### 4.3 距离分级标准
| 等级 | 直线距离 | 标签 |
|------|----------|------|
| 极近 | 0-500m | 步行5分钟内 |
| 近 | 500m-1km | 步行10分钟内 |
| 较近 | 1km-2km | 步行15-20分钟 |
| 中等 | 2km-3km | 需交通工具 |
| 较远 | >3km | 通勤成本较高 |

---

## 5. 查询功能设计

### 5.1 基础筛选接口
```
GET /api/houses/search

参数：
- district: 行政区（可多选）
- min_price/max_price: 价格区间
- bedrooms: 卧室数量（1,2,3,4+）
- rental_type: 租赁类型（整租/合租）
- subway_line: 地铁线路
- min_area/max_area: 面积区间
- decoration: 装修等级（简装/精装/豪华）
- elevator: 是否有电梯（true/false）
- available_before: 最晚可入住时间
```

### 5.2 地标参照查询接口
```
GET /api/houses/nearby

参数：
- landmark_id: 地标ID（地铁站/公司/商圈）
- radius: 搜索半径（米），默认2000
- sort_by: 排序方式（distance/price/area）
```

### 5.3 通勤查询接口
```
GET /api/houses/commute

参数：
- destination_lat: 目的地纬度
- destination_lng: 目的地经度
- max_commute_distance: 最大通勤距离（米）
- commute_mode: 通勤方式（walk/subway/drive）
```

### 5.4 地标检索接口
```
GET /api/landmarks/search

参数：
- keyword: 搜索关键词
- category: 类别（subway/company/landmark）
- district: 行政区
```

---

## 6. 响应数据结构设计

### 6.1 房源列表响应
```json
{
  "total": 50,
  "page": 1,
  "page_size": 20,
  "houses": [
    {
      "house_id": "HF_2001",
      "community": "智学苑",
      "district": "海淀",
      "price": 8500,
      "bedrooms": 2,
      "area_sqm": 78,
      "subway_station": "西二旗站",
      "subway_distance": 800,
      "longitude": 116.3289,
      "latitude": 40.0567,
      "tags": ["近地铁", "南北通透"],
      "reference_distance": {
        "to_landmark": 850,
        "to_landmark_walking": 1100,
        "to_landmark_duration": 14
      }
    }
  ]
}
```

### 6.2 距离详情响应
```json
{
  "house_id": "HF_2001",
  "to_landmarks": [
    {
      "landmark_id": "SS_001",
      "name": "西二旗站",
      "category": "subway",
      "straight_distance": 850,
      "walking_distance": 1100,
      "walking_duration": 14,
      "distance_level": "近"
    },
    {
      "landmark_id": "F500_012",
      "name": "百度大厦",
      "category": "company",
      "straight_distance": 1200,
      "walking_distance": 1600,
      "walking_duration": 20,
      "distance_level": "较近"
    }
  ]
}
```

---

## 7. 文件结构规划

```
fake_app/
├── data/
│   ├── houses.json              # 房源数据（现有）
│   ├── subway_stations.json     # 地铁站数据（待生成）
│   ├── fortune500_companies.json # 世界500强企业数据（待生成）
│   └── landmarks.json           # 商圈地标数据（待生成）
├── search/
│   ├── indexer.py               # 索引构建
│   ├── filters.py               # 筛选逻辑
│   ├── distance.py              # 距离计算
│   └── query.py                 # 查询接口
└── README.md                    # 使用说明
```

---

## 8. 待生成JSON文件清单

### 8.1 subway_stations.json
- 包含北京地铁核心站点约50个
- 字段：station_id, name, lines[], district, longitude, latitude

### 8.2 fortune500_companies.json
- 包含北京世界500强企业约30家
- 字段：company_id, name, name_en, industry, rank_2024, address, district, longitude, latitude

### 8.3 landmarks.json
- 包含北京主要商圈地标约20个
- 字段：landmark_id, name, type, district, address, longitude, latitude

---

## 9. 使用场景示例

### 场景1：西二旗上班族找房
```
查询：距离西二旗地铁站2km内，整租，两居室，预算8000以内
结果：智学苑系列房源（HF_2001至HF_2005）
排序：按距离西二旗站距离升序
```

### 场景2：CBD工作者找房
```
查询：距离大望路站1km内，公寓类型，可短租
结果：阳光100公寓（HF_1001, HF_1005）
排序：按价格升序
```

### 场景3：多地标交叉查询
```
查询：距离百度大厦和13号线地铁站都在1.5km内的房源
结果：上地西里（HF_3001）、智学苑部分房源
计算：多地标距离加权排序
```

---

## 10. 扩展建议

1. **缓存机制**：对热门地标查询结果进行缓存
2. **实时交通**：接入实时地铁/公交数据优化通勤时间
3. **热力图**：基于查询数据生成租房价格热力图
4. **推荐算法**：基于用户历史查询行为进行个性化推荐

---

*文档版本：v1.0*
*生成日期：2026-02-15*
