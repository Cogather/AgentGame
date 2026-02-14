# 排行榜机制说明

本文档描述游戏系统中排行榜功能的设计与使用方法。

## 功能概述

排行榜模块负责维护游戏参与者的排名信息，支持按得分从高到低排序展示。排行榜数据来源于用户的任务完成情况，实时反映各参赛者的表现。

## 排行榜数据模型

### 排行项结构

| 字段名 | 类型 | 说明 |
|--------|------|------|
| rank | int | 排名（动态计算，第1名为最高分） |
| team_name | string | 队伍名 |
| user_id | string | 用户工号（唯一标识） |
| username | string | 用户姓名 |
| score | int | 总得分 |
| completed_tasks | int | 完成任务数 |
| update_time | datetime | 最后更新时间 |

### 排序规则

排行榜按以下优先级排序：

1. **得分**（降序）- 得分越高排名越靠前
2. **完成任务数**（降序）- 得分相同时，完成任务数多者排名靠前
3. **更新时间**（升序）- 前两项相同时，先达到该成绩的排名靠前

## 存储设计

### 数据文件

排行榜数据存储在独立文件中：

- 数据文件：`rankdata/rank.json`
- 备份文件：`rankdata/rank.json.backup`

### 数据结构示例

```json
{
  "version": 1,
  "update_at": 1707832800,
  "items": [
    {
      "team_name": "红队",
      "user_id": "EMP001",
      "username": "张三",
      "score": 1500,
      "completed_tasks": 15,
      "update_time": "2026-02-14T15:30:00Z"
    }
  ]
}
```

## HTTP 查询接口（对外）

### 1. 获取排行榜列表

**接口地址：** `GET /api/rank`

**请求参数：**

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| limit | int | 否 | 返回前N条记录，默认返回全部 |

**响应示例：**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "rank": 1,
      "team_name": "红队",
      "user_id": "EMP001",
      "username": "张三",
      "score": 1500,
      "completed_tasks": 15,
      "update_time": "2026-02-14 15:30:00"
    },
    {
      "rank": 2,
      "team_name": "蓝队",
      "user_id": "EMP002",
      "username": "李四",
      "score": 1200,
      "completed_tasks": 12,
      "update_time": "2026-02-14 15:25:00"
    }
  ]
}
```

**使用示例：**

```bash
# 获取完整排行榜
curl http://localhost:8080/api/rank

# 获取前10名
curl "http://localhost:8080/api/rank?limit=10"
```

### 2. 获取单个用户排行

**接口地址：** `GET /api/rank/{user_id}`

**响应示例（成功）：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rank": 3,
    "team_name": "红队",
    "user_id": "EMP003",
    "username": "王五",
    "score": 1000,
    "completed_tasks": 10,
    "update_time": "2026-02-14 15:20:00"
  }
}
```

**响应示例（用户不存在）：**

```json
{
  "code": 404,
  "message": "用户不在排行榜中: EMP999",
  "data": null
}
```

## 内部更新接口

### RankManager 方法

#### 1. UpdateOrCreate - 更新或创建排行数据

```go
func (rm *RankManager) UpdateOrCreate(userID string, req *RankUpdateRequest) error
```

**RankUpdateRequest 结构：**

| 字段名 | 类型 | 说明 |
|--------|------|------|
| team_name | string | 队伍名 |
| username | string | 用户姓名 |
| score | int | 设置总得分（直接赋值） |
| completed_tasks | int | 设置完成任务数（直接赋值） |
| add_score | int | 增加得分（增量） |
| add_tasks | int | 增加任务数（增量） |

**使用示例：**

```go
// 设置用户得分
rankManager.UpdateOrCreate("EMP001", &rank.RankUpdateRequest{
    TeamName:       "红队",
    Username:       "张三",
    Score:          1500,
    CompletedTasks: 15,
})

// 增量更新（完成任务后加分）
rankManager.UpdateOrCreate("EMP001", &rank.RankUpdateRequest{
    AddScore: 100,
    AddTasks: 1,
})
```

#### 2. RefreshUser - 刷新用户排行数据

```go
func (rm *RankManager) RefreshUser(userID string) error
```

用于触发用户排行的重新计算，更新其时间戳。

#### 3. DeleteUser - 从排行榜删除用户

```go
func (rm *RankManager) DeleteUser(userID string) error
```

#### 4. ReloadData - 重新加载数据

```go
func (rm *RankManager) ReloadData() error
```

从磁盘重新加载所有排行数据。

## 初始化与数据加载

### 服务启动时

1. 创建排行榜管理器，指定数据目录（`rankdata/`）
2. 自动从 `rank.json` 加载所有排行数据到内存
3. 按排序规则计算初始排名
4. 所有查询操作直接从内存读取，保证高性能

### 数据持久化

- 所有更新操作即时写入磁盘（原子写入）
- 使用临时文件 + 重命名机制，防止数据损坏
- 强制 `fsync` 确保数据落盘

## 使用示例

### 初始化排行榜管理器

```go
import "ocProxy/game/rank"

rankManager, err := rank.NewRankManager("rankdata")
if err != nil {
    log.Fatal(err)
}

// 设置HTTP路由
rankHandler := rank.NewHandler(rankManager)
rankHandler.SetupRoutes(router)
```

### 用户完成任务后更新排行

```go
// 任务完成后，给用户增加分数
func onTaskCompleted(userID string, taskScore int) {
    err := rankManager.UpdateOrCreate(userID, &rank.RankUpdateRequest{
        AddScore: taskScore,
        AddTasks: 1,
    })
    if err != nil {
        log.Printf("更新排行榜失败: %v", err)
    }
}
```

### 获取用户当前排名

```go
item, err := rankManager.GetUserRank("EMP001")
if err != nil {
    log.Printf("获取用户排名失败: %v", err)
    return
}
fmt.Printf("用户 %s 当前排名第 %d，得分 %d\n",
    item.Username, item.Rank, item.Score)
```

## 注意事项

1. **只读对外**：对外HTTP接口仅支持查询，不支持直接修改排行榜
2. **内部更新**：所有数据更新通过内部 `RankManager` 方法进行
3. **数据一致性**：用户信息和排行榜数据分离管理，但建议保持 `user_id` 一致
4. **并发安全**：所有操作均为线程安全，支持并发访问
5. **数据备份**：定期备份 `rankdata/rank.json` 文件，防止数据丢失

## 错误码说明

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 400 | 请求参数错误 |
| 404 | 用户不存在于排行榜 |
| 500 | 服务器内部错误 |
